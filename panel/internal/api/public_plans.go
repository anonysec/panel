//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// publicPlans returns active plans with pricing for the landing page.
// No authentication required.
func (s *Server) publicPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.DB.Query(`SELECT id, name, price, data_gb, speed_mbps, duration_days, features FROM plans WHERE is_active = 1 ORDER BY price ASC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type PublicPlan struct {
		ID           int64    `json:"id"`
		Name         string   `json:"name"`
		Price        float64  `json:"price"`
		DataGB       float64  `json:"data_gb"`
		SpeedMbps    float64  `json:"speed_mbps"`
		DurationDays int      `json:"duration_days"`
		Features     []string `json:"features"`
	}

	plans := []PublicPlan{}
	for rows.Next() {
		var p PublicPlan
		var featuresRaw sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.DataGB, &p.SpeedMbps, &p.DurationDays, &featuresRaw); err != nil {
			continue
		}
		p.Features = []string{}
		if featuresRaw.Valid && featuresRaw.String != "" {
			var parsed []string
			if err := json.Unmarshal([]byte(featuresRaw.String), &parsed); err == nil {
				p.Features = parsed
			}
		}
		plans = append(plans, p)
	}

	writeJSON(w, map[string]any{"ok": true, "plans": plans})
}
