package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bwAlertLevel tracks which alert level has been sent for each node.
// 0 = no alert, 1 = soft limit (threshold%), 2 = hard limit (100%).
var (
	bwAlertLevel   = make(map[int64]int) // key: node_id → highest alert level sent
	bwAlertLevelMu sync.Mutex
)

// UpdateBandwidthQuota adds the push delta (rx+tx bytes) to the node's
// current_usage_gb in node_bandwidth_quotas. Called from the metrics stream handler.
// This is a best-effort approximation — if no quota row exists, it's a no-op.
func UpdateBandwidthQuota(db *sql.DB, nodeID int64, rxBytes, txBytes int64) {
	totalBytes := rxBytes + txBytes
	if totalBytes <= 0 {
		return
	}
	// Convert bytes to GB (approximate)
	deltaGB := float64(totalBytes) / (1024 * 1024 * 1024)

	_, err := db.Exec(`UPDATE node_bandwidth_quotas SET current_usage_gb = current_usage_gb + $1 WHERE node_id = $2`, deltaGB, nodeID)
	if err != nil {
		log.Printf("[quota] failed to update bandwidth usage for node %d: %v", nodeID, err)
	}
}

// CheckBandwidthQuotas queries all nodes with bandwidth quotas and sends
// alerts when usage exceeds configured thresholds. Two alert levels:
//   - Soft limit: usage >= alert_threshold_pct (default 80%) — warning
//   - Hard limit: usage >= 100% — critical alert
//
// Each level is sent only once per reset cycle. Designed to be called from
// the background worker.
func CheckBandwidthQuotas(db *sql.DB, notify func(string)) {
	rows, err := db.Query(`
		SELECT q.node_id, n.name, q.monthly_limit_gb, q.current_usage_gb, q.alert_threshold_pct
		FROM node_bandwidth_quotas q
		JOIN nodes n ON n.id = q.node_id
		WHERE q.monthly_limit_gb > 0
	`)
	if err != nil {
		log.Printf("[quota] check query error: %v", err)
		return
	}
	defer rows.Close()

	bwAlertLevelMu.Lock()
	defer bwAlertLevelMu.Unlock()

	for rows.Next() {
		var nodeID int64
		var nodeName string
		var limitGB int
		var usageGB float64
		var thresholdPct int

		if err := rows.Scan(&nodeID, &nodeName, &limitGB, &usageGB, &thresholdPct); err != nil {
			log.Printf("[quota] scan error: %v", err)
			continue
		}

		if thresholdPct <= 0 {
			thresholdPct = 80
		}

		usagePct := usageGB / float64(limitGB) * 100
		currentLevel := bwAlertLevel[nodeID]

		// Hard limit check (100%)
		if usagePct >= 100 && currentLevel < 2 {
			bwAlertLevel[nodeID] = 2
			msg := fmt.Sprintf("🚨 *Bandwidth Quota Exceeded*\nNode: `%s`\nUsage: %.1f GB / %d GB (%.0f%%)\nMonthly limit reached!",
				nodeName, usageGB, limitGB, usagePct)
			notify(msg)
			log.Printf("[quota] hard limit reached for node %s: %.1f/%d GB (%.0f%%)", nodeName, usageGB, limitGB, usagePct)
		} else if usagePct >= float64(thresholdPct) && currentLevel < 1 {
			// Soft limit check (threshold%)
			bwAlertLevel[nodeID] = 1
			msg := fmt.Sprintf("⚠️ *Bandwidth Quota Warning*\nNode: `%s`\nUsage: %.1f GB / %d GB (%.0f%%)\nThreshold: %d%%",
				nodeName, usageGB, limitGB, usagePct, thresholdPct)
			notify(msg)
			log.Printf("[quota] soft limit alert for node %s: %.1f/%d GB (%.0f%% >= %d%%)", nodeName, usageGB, limitGB, usagePct, thresholdPct)
		} else if usagePct < float64(thresholdPct) && currentLevel > 0 {
			// Recovered below threshold — clear state
			delete(bwAlertLevel, nodeID)
			log.Printf("[quota] bandwidth recovered for node %s: %.1f/%d GB (%.0f%% < %d%%)", nodeName, usageGB, limitGB, usagePct, thresholdPct)
		}
	}
}

// ResetBandwidthQuotas resets current_usage_gb to 0 for all quotas whose
// reset_day matches today's day of the month. Should be called from the
// background worker (runs every minute, but the reset is idempotent for the day).
func ResetBandwidthQuotas(db *sql.DB) {
	today := time.Now().Day()

	result, err := db.Exec(`UPDATE node_bandwidth_quotas SET current_usage_gb = 0 WHERE reset_day = $1 AND current_usage_gb > 0`, today)
	if err != nil {
		log.Printf("[quota] reset error: %v", err)
		return
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		log.Printf("[quota] reset bandwidth quotas for %d nodes (reset_day=%d)", affected, today)

		// Clear alert state for reset nodes so alerts can fire again next cycle
		bwAlertLevelMu.Lock()
		rows, err := db.Query(`SELECT node_id FROM node_bandwidth_quotas WHERE reset_day = $1`, today)
		if err == nil {
			for rows.Next() {
				var nodeID int64
				if rows.Scan(&nodeID) == nil {
					delete(bwAlertLevel, nodeID)
				}
			}
			rows.Close()
		}
		bwAlertLevelMu.Unlock()
	}
}

// nodeQuota handles GET/POST /api/admin/nodes/:id/quota (and /api/admin/nodes/:id/bandwidth-quota)
func (s *Server) nodeQuota(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/admin/nodes/{id}/quota or /api/admin/nodes/{id}/bandwidth-quota
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || (parts[1] != "quota" && parts[1] != "bandwidth-quota") {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	nodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	// Verify node exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM nodes WHERE id=$1 LIMIT 1`, nodeID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "node_not_found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getNodeQuota(w, nodeID)
	case http.MethodPost:
		s.setNodeQuota(w, r, nodeID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// getNodeQuota returns the current quota config and usage for a node.
func (s *Server) getNodeQuota(w http.ResponseWriter, nodeID int64) {
	var limitGB int
	var usageGB float64
	var thresholdPct int
	var resetDay int

	err := s.DB.QueryRow(`SELECT monthly_limit_gb, current_usage_gb, alert_threshold_pct, reset_day FROM node_bandwidth_quotas WHERE node_id = $1`, nodeID).
		Scan(&limitGB, &usageGB, &thresholdPct, &resetDay)
	if err == sql.ErrNoRows {
		// No quota configured — return defaults
		writeJSON(w, map[string]any{
			"ok":                  true,
			"monthly_limit_gb":    0,
			"current_usage_gb":    0.0,
			"usage_percent":       0.0,
			"alert_threshold_pct": 80,
			"reset_day":           1,
			"configured":          false,
		})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	var usagePct float64
	if limitGB > 0 {
		usagePct = usageGB / float64(limitGB) * 100
	}

	writeJSON(w, map[string]any{
		"ok":                  true,
		"monthly_limit_gb":    limitGB,
		"current_usage_gb":    usageGB,
		"usage_percent":       usagePct,
		"alert_threshold_pct": thresholdPct,
		"reset_day":           resetDay,
		"configured":          true,
	})
}

// setNodeQuota creates or updates the bandwidth quota for a node.
func (s *Server) setNodeQuota(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		MonthlyLimitGB    *int `json:"monthly_limit_gb"`
		AlertThresholdPct *int `json:"alert_threshold_pct"`
		ResetDay          *int `json:"reset_day"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Defaults
	limitGB := 0
	thresholdPct := 80
	resetDay := 1

	if in.MonthlyLimitGB != nil {
		if *in.MonthlyLimitGB < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_monthly_limit"})
			return
		}
		limitGB = *in.MonthlyLimitGB
	}
	if in.AlertThresholdPct != nil {
		if *in.AlertThresholdPct < 0 || *in.AlertThresholdPct > 100 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_threshold"})
			return
		}
		thresholdPct = *in.AlertThresholdPct
	}
	if in.ResetDay != nil {
		if *in.ResetDay < 1 || *in.ResetDay > 28 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_reset_day"})
			return
		}
		resetDay = *in.ResetDay
	}

	_, err := s.DB.Exec(`
		INSERT INTO node_bandwidth_quotas (node_id, monthly_limit_gb, alert_threshold_pct, reset_day)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (node_id) DO UPDATE SET monthly_limit_gb = EXCLUDED.monthly_limit_gb, alert_threshold_pct = EXCLUDED.alert_threshold_pct, reset_day = EXCLUDED.reset_day
	`, nodeID, limitGB, thresholdPct, resetDay)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	writeJSON(w, map[string]any{"ok": true})
}
