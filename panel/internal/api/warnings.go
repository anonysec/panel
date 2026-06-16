package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// checkDataWarnings checks the current data usage for the given username against
// configured warning thresholds and the plan data cap. If any threshold is crossed
// and hasn't been warned before, it creates an event and dispatches a notification.
// If usage >= 100% of the cap, it sets the customer status to 'limited'.
//
// This function is called after accounting updates.
func (s *Server) checkDataWarnings(username string) {
	// 1. Query total usage (SUM of acctinputoctets + acctoutputoctets) from radacct
	//    for the customer (active sessions or within current subscription period).
	var totalUsage int64
	err := s.DB.QueryRow(
		`SELECT COALESCE(SUM(acctinputoctets + acctoutputoctets), 0)
		 FROM radacct WHERE username = ?`,
		username,
	).Scan(&totalUsage)
	if err != nil {
		log.Printf("[data-warnings] failed to query usage for %s: %v", username, err)
		return
	}

	if totalUsage == 0 {
		return
	}

	// 2. Query the customer's plan data cap (plans.data_gb * 1024^3) via
	//    customers → subscriptions → plans join.
	//    We look for the active subscription's plan, falling back to the customer's direct plan_id.
	var dataCapBytes int64
	var customerID int64
	err = s.DB.QueryRow(
		`SELECT c.id, COALESCE(p.data_gb, 0) * 1073741824
		 FROM customers c
		 LEFT JOIN subscriptions sub ON sub.username = c.username AND sub.status = 'active'
		 LEFT JOIN plans p ON p.id = COALESCE(sub.plan_id, c.plan_id)
		 WHERE c.username = ?
		 LIMIT 1`,
		username,
	).Scan(&customerID, &dataCapBytes)
	if err != nil {
		log.Printf("[data-warnings] failed to query plan cap for %s: %v", username, err)
		return
	}

	// No data cap configured (unlimited plan) — nothing to warn about
	if dataCapBytes <= 0 {
		return
	}

	// 3. Load thresholds from panel_settings key 'data_warning_thresholds' (JSON array like [80, 95])
	thresholds := []int{80, 95} // default
	var thresholdJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'data_warning_thresholds'`,
	).Scan(&thresholdJSON); err == nil && thresholdJSON != "" {
		var parsed []int
		if json.Unmarshal([]byte(thresholdJSON), &parsed) == nil && len(parsed) > 0 {
			thresholds = parsed
		}
	}

	// Calculate usage percentage
	usagePercent := float64(totalUsage) / float64(dataCapBytes) * 100.0

	// 4. For each threshold crossed that hasn't already triggered a warning:
	//    INSERT event (type='data_warning', severity='warning'), dispatch notification
	for _, threshold := range thresholds {
		if usagePercent < float64(threshold) {
			continue
		}

		// Check if we already created a warning for this threshold for this user
		var existingCount int
		err := s.DB.QueryRow(
			`SELECT COUNT(*) FROM events
			 WHERE type = 'data_warning' AND related = ? AND title LIKE ?`,
			username, fmt.Sprintf("%%%d%%", threshold),
		).Scan(&existingCount)
		if err != nil {
			log.Printf("[data-warnings] failed to check existing warnings for %s: %v", username, err)
			continue
		}
		if existingCount > 0 {
			continue
		}

		// Create warning event and dispatch notification
		title := fmt.Sprintf("Data usage at %d%% for %s", threshold, username)
		message := fmt.Sprintf(
			"Customer %s has used %d%% of their data cap (%d bytes of %d bytes allocated).",
			username, threshold, totalUsage, dataCapBytes,
		)
		s.createEvent("data_warning", "warning", title, message, "system", username)
	}

	// 5. If usage >= 100% cap: UPDATE customers SET status='limited', INSERT event
	if totalUsage >= dataCapBytes {
		// Check if we already recorded a data_cap_reached event for this user
		var capReachedCount int
		_ = s.DB.QueryRow(
			`SELECT COUNT(*) FROM events
			 WHERE type = 'data_cap_reached' AND related = ?`,
			username,
		).Scan(&capReachedCount)

		if capReachedCount == 0 {
			// Update customer status to limited
			_, err := s.DB.Exec(
				`UPDATE customers SET status = 'limited' WHERE username = ? AND status != 'limited'`,
				username,
			)
			if err != nil {
				log.Printf("[data-warnings] failed to set customer %s to limited: %v", username, err)
			}

			// Create data cap reached event
			title := fmt.Sprintf("Data cap reached for %s", username)
			message := fmt.Sprintf(
				"Customer %s has exceeded their data cap (used %d bytes, cap %d bytes). Status set to limited.",
				username, totalUsage, dataCapBytes,
			)
			s.createEvent("data_cap_reached", "error", title, message, "system", username)
		}
	}
}



// portalWarnings returns active data warning events for the authenticated customer.
// GET /api/portal/warnings
func (s *Server) portalWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	rows, err := s.DB.Query(
		`SELECT id, type, severity, title, message, created_at
		 FROM events
		 WHERE related = ? AND type IN ('data_warning', 'data_cap_reached')
		 ORDER BY created_at DESC LIMIT 20`,
		username,
	)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type Warning struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		CreatedAt string `json:"created_at"`
	}

	warnings := []Warning{}
	for rows.Next() {
		var w Warning
		var createdAt sql.NullTime
		if err := rows.Scan(&w.ID, &w.Type, &w.Severity, &w.Title, &w.Message, &createdAt); err != nil {
			continue
		}
		if createdAt.Valid {
			w.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		warnings = append(warnings, w)
	}

	writeJSON(w, map[string]any{"ok": true, "warnings": warnings})
}

// dataWarningThresholds handles PUT /api/settings/data-warning-thresholds
// to allow admins to configure the data usage warning threshold percentages.
func (s *Server) dataWarningThresholds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	var in struct {
		Thresholds []int `json:"thresholds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Validate the array is non-empty
	if len(in.Thresholds) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "thresholds array must not be empty")
		return
	}

	// Validate the array contains no more than 10 thresholds
	if len(in.Thresholds) > 10 {
		writeError(w, http.StatusBadRequest, "bad_request", "thresholds array must contain no more than 10 items")
		return
	}

	// Validate each threshold is between 0 and 100 inclusive
	for _, t := range in.Thresholds {
		if t < 0 || t > 100 {
			writeError(w, http.StatusBadRequest, "bad_request", "each threshold must be between 0 and 100")
			return
		}
	}

	// Serialize thresholds to JSON
	thresholdJSON, err := json.Marshal(in.Thresholds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to serialize thresholds")
		return
	}

	// Update panel_settings
	_, err = s.DB.Exec(
		`UPDATE panel_settings SET setting_value = ? WHERE setting_key = 'data_warning_thresholds'`,
		string(thresholdJSON),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update thresholds")
		return
	}

	writeJSON(w, map[string]any{"ok": true, "thresholds": in.Thresholds})
}
