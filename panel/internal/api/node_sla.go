//go:build !lite

package api

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"
)

// nodeSLA handles GET /api/admin/nodes/:id/sla
// Returns monthly availability percentage and downtime entries for a node.
// Query params:
//   - month: specific month in "2006-01" format (defaults to current month)
//   - months: number of months of history to include (default 6, max 12)
func (s *Server) nodeSLA(w http.ResponseWriter, r *http.Request, nodeID int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse optional ?months=N for multi-month history (default 6)
	monthsParam := r.URL.Query().Get("months")
	numMonths := 6
	if monthsParam != "" {
		if n, err := strconv.Atoi(monthsParam); err == nil && n > 0 && n <= 12 {
			numMonths = n
		}
	}

	// Parse optional ?month=2024-06 query param (defaults to current month)
	monthStr := r.URL.Query().Get("month")
	var currentMonthStart time.Time

	if monthStr != "" {
		parsed, err := time.Parse("2006-01", monthStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_month_format"})
			return
		}
		currentMonthStart = parsed
	} else {
		now := time.Now().UTC()
		currentMonthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		monthStr = currentMonthStart.Format("2006-01")
	}

	// Calculate current month SLA
	currentMonthEnd := currentMonthStart.AddDate(0, 1, 0)
	currentResult := s.calculateMonthSLA(nodeID, currentMonthStart, currentMonthEnd)

	// Calculate history for last N months
	type MonthSLA struct {
		Month               string  `json:"month"`
		AvailabilityPercent float64 `json:"availability_percent"`
		DowntimeMinutes     float64 `json:"downtime_minutes"`
		IncidentCount       int     `json:"incident_count"`
	}

	history := make([]MonthSLA, 0, numMonths)
	for i := 0; i < numMonths; i++ {
		mStart := currentMonthStart.AddDate(0, -i, 0)
		mEnd := mStart.AddDate(0, 1, 0)
		result := s.calculateMonthSLA(nodeID, mStart, mEnd)
		history = append(history, MonthSLA{
			Month:               mStart.Format("2006-01"),
			AvailabilityPercent: result.availabilityPercent,
			DowntimeMinutes:     result.downtimeMinutes,
			IncidentCount:       result.incidentCount,
		})
	}

	writeJSON(w, map[string]any{
		"ok":                   true,
		"node_id":              nodeID,
		"month":                monthStr,
		"total_hours":          currentResult.totalHours,
		"downtime_hours":       currentResult.downtimeHours,
		"downtime_minutes":     currentResult.downtimeMinutes,
		"availability_percent": currentResult.availabilityPercent,
		"downtimes":            currentResult.downtimes,
		"history":              history,
	})
}

// monthSLAResult holds calculated SLA data for a single month.
type monthSLAResult struct {
	totalHours          float64
	downtimeHours       float64
	downtimeMinutes     float64
	availabilityPercent float64
	incidentCount       int
	downtimes           []downtimeEntry
}

type downtimeEntry struct {
	StartedAt       string  `json:"started_at"`
	EndedAt         *string `json:"ended_at"`
	DurationSeconds int64   `json:"duration_seconds"`
	Reason          string  `json:"reason"`
}

// calculateMonthSLA queries downtimes and computes availability for one month window.
func (s *Server) calculateMonthSLA(nodeID int64, monthStart, monthEnd time.Time) monthSLAResult {
	totalSeconds := monthEnd.Sub(monthStart).Seconds()

	rows, err := s.DB.Query(`
		SELECT id, started_at, ended_at, duration_seconds, COALESCE(reason, '')
		FROM node_downtimes
		WHERE node_id = $1
		  AND started_at < $1
		  AND (ended_at IS NULL OR ended_at > $1)
		ORDER BY started_at ASC`,
		nodeID, monthEnd, monthStart,
	)
	if err != nil {
		log.Printf("[sla] query error node=%d: %v", nodeID, err)
		return monthSLAResult{totalHours: totalSeconds / 3600, availabilityPercent: 100, downtimes: []downtimeEntry{}}
	}
	defer rows.Close()

	var downtimes []downtimeEntry
	var totalDowntimeSeconds float64

	now := time.Now().UTC()

	for rows.Next() {
		var id int64
		var startedAt time.Time
		var endedAt sql.NullTime
		var durationSec int
		var reason string

		if err := rows.Scan(&id, &startedAt, &endedAt, &durationSec, &reason); err != nil {
			log.Printf("[sla] scan error node=%d: %v", nodeID, err)
			continue
		}

		entry := downtimeEntry{
			StartedAt: startedAt.UTC().Format(time.RFC3339),
			Reason:    reason,
		}

		// Calculate effective downtime within the month window
		effectiveStart := startedAt
		if effectiveStart.Before(monthStart) {
			effectiveStart = monthStart
		}

		var effectiveEnd time.Time
		if endedAt.Valid {
			effectiveEnd = endedAt.Time
			if effectiveEnd.After(monthEnd) {
				effectiveEnd = monthEnd
			}
			endStr := endedAt.Time.UTC().Format(time.RFC3339)
			entry.EndedAt = &endStr
			entry.DurationSeconds = int64(effectiveEnd.Sub(effectiveStart).Seconds())
		} else {
			// Ongoing downtime — calculate up to now (or month end, whichever is earlier)
			effectiveEnd = now
			if effectiveEnd.After(monthEnd) {
				effectiveEnd = monthEnd
			}
			entry.DurationSeconds = int64(effectiveEnd.Sub(effectiveStart).Seconds())
		}

		totalDowntimeSeconds += float64(entry.DurationSeconds)
		downtimes = append(downtimes, entry)
	}

	if downtimes == nil {
		downtimes = []downtimeEntry{}
	}

	// Calculate availability percentage
	availabilityPercent := 0.0
	if totalSeconds > 0 {
		availabilityPercent = (totalSeconds - totalDowntimeSeconds) / totalSeconds * 100
		if availabilityPercent < 0 {
			availabilityPercent = 0
		}
	}

	// Round to 2 decimal places
	availabilityPercent = float64(int(availabilityPercent*100)) / 100

	totalHours := totalSeconds / 3600
	downtimeHours := totalDowntimeSeconds / 3600
	downtimeHours = float64(int(downtimeHours*100)) / 100
	downtimeMinutes := float64(int(totalDowntimeSeconds/60*100)) / 100

	return monthSLAResult{
		totalHours:          totalHours,
		downtimeHours:       downtimeHours,
		downtimeMinutes:     downtimeMinutes,
		availabilityPercent: availabilityPercent,
		incidentCount:       len(downtimes),
		downtimes:           downtimes,
	}
}

// nodesSLASummary handles GET /api/admin/nodes/sla-summary
// Returns fleet-wide SLA overview for all nodes in the current month.
func (s *Server) nodesSLASummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse optional ?month=2024-06
	monthStr := r.URL.Query().Get("month")
	var monthStart time.Time

	if monthStr != "" {
		parsed, err := time.Parse("2006-01", monthStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_month_format"})
			return
		}
		monthStart = parsed
	} else {
		now := time.Now().UTC()
		monthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		monthStr = monthStart.Format("2006-01")
	}
	monthEnd := monthStart.AddDate(0, 1, 0)

	// Get all active nodes
	nodeRows, err := s.DB.Query(`SELECT id, name, status FROM nodes WHERE deleted_at IS NULL ORDER BY name ASC`)
	if err != nil {
		log.Printf("[sla] fleet query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer nodeRows.Close()

	type NodeSLASummary struct {
		NodeID              int64   `json:"node_id"`
		Name                string  `json:"name"`
		Status              string  `json:"status"`
		AvailabilityPercent float64 `json:"availability_percent"`
		DowntimeMinutes     float64 `json:"downtime_minutes"`
		IncidentCount       int     `json:"incident_count"`
	}

	var nodes []NodeSLASummary
	var totalAvailability float64
	var totalNodes int

	for nodeRows.Next() {
		var nodeID int64
		var name, status string
		if err := nodeRows.Scan(&nodeID, &name, &status); err != nil {
			log.Printf("[sla] fleet scan error: %v", err)
			continue
		}

		result := s.calculateMonthSLA(nodeID, monthStart, monthEnd)
		nodes = append(nodes, NodeSLASummary{
			NodeID:              nodeID,
			Name:                name,
			Status:              status,
			AvailabilityPercent: result.availabilityPercent,
			DowntimeMinutes:     result.downtimeMinutes,
			IncidentCount:       result.incidentCount,
		})
		totalAvailability += result.availabilityPercent
		totalNodes++
	}

	if nodes == nil {
		nodes = []NodeSLASummary{}
	}

	// Calculate fleet-wide average availability
	fleetAvailability := 0.0
	if totalNodes > 0 {
		fleetAvailability = float64(int(totalAvailability/float64(totalNodes)*100)) / 100
	}

	writeJSON(w, map[string]any{
		"ok":                 true,
		"month":              monthStr,
		"fleet_availability": fleetAvailability,
		"total_nodes":        totalNodes,
		"nodes":              nodes,
	})
}

// RecordNodeDowntime creates a new downtime entry when a node goes offline.
// Called from the background worker when a node transitions from online/stale → offline.
func RecordNodeDowntime(db *sql.DB, nodeID int64, reason string) {
	// Only create a new downtime if there isn't already an open one
	var openCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM node_downtimes WHERE node_id = $1 AND ended_at IS NULL`, nodeID).Scan(&openCount)
	if err != nil {
		log.Printf("[sla] error checking open downtime node=%d: %v", nodeID, err)
		return
	}
	if openCount > 0 {
		return // Already has an open downtime entry
	}

	_, err = db.Exec(
		`INSERT INTO node_downtimes(node_id, started_at, reason) VALUES($1, NOW(), $2)`,
		nodeID, reason,
	)
	if err != nil {
		log.Printf("[sla] error recording downtime node=%d: %v", nodeID, err)
	}
}

// CloseNodeDowntime closes any open downtime entry for a node when it comes back online.
// Called from the gRPC metrics stream handler when a node reconnects.
func CloseNodeDowntime(db *sql.DB, nodeID int64) {
	_, err := db.Exec(
		`UPDATE node_downtimes SET ended_at = NOW(), duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INT WHERE node_id = $1 AND ended_at IS NULL`,
		nodeID,
	)
	if err != nil {
		log.Printf("[sla] error closing downtime node=%d: %v", nodeID, err)
	}
}
