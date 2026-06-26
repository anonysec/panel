package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// bandwidthNotifyTracker tracks daily notification state for bandwidth quotas.
// Prevents duplicate notifications within the same day for each node.
var (
	bwNotifyTracker   = make(map[int64]bandwidthNotifyState)
	bwNotifyTrackerMu sync.Mutex
)

type bandwidthNotifyState struct {
	Level int       // 1 = warning (>=80%), 2 = critical (>=100%)
	Date  time.Time // date of last notification
}

// checkBandwidthQuotas checks all nodes with bandwidth_quota_gb configured
// and sends notifications when thresholds are exceeded.
// Called by the background worker on each tick.
func (s *Server) checkBandwidthQuotas() {
	rows, err := s.DB.Query(`
		SELECT id, name, bandwidth_quota_gb, bandwidth_used_bytes
		FROM nodes
		WHERE bandwidth_quota_gb IS NOT NULL AND bandwidth_quota_gb > 0
	`)
	if err != nil {
		log.Printf("[bandwidth] check query error: %v", err)
		return
	}
	defer rows.Close()

	bwNotifyTrackerMu.Lock()
	defer bwNotifyTrackerMu.Unlock()

	today := time.Now().UTC().Truncate(24 * time.Hour)

	for rows.Next() {
		var nodeID int64
		var nodeName string
		var quotaGB int
		var usedBytes int64

		if err := rows.Scan(&nodeID, &nodeName, &quotaGB, &usedBytes); err != nil {
			log.Printf("[bandwidth] scan error: %v", err)
			continue
		}

		// Calculate percentage: (bandwidth_used_bytes / (bandwidth_quota_gb * 1073741824)) * 100
		quotaBytes := int64(quotaGB) * 1073741824
		if quotaBytes <= 0 {
			continue
		}
		percentage := int(float64(usedBytes) / float64(quotaBytes) * 100)

		log.Printf("[bandwidth] node %s at %d%% of quota", nodeName, percentage)

		state := bwNotifyTracker[nodeID]

		// Check if we already notified today at this level or higher
		alreadyNotifiedToday := state.Date.Equal(today)

		if percentage >= 100 && (!alreadyNotifiedToday || state.Level < 2) {
			// Critical: send notification, optionally disable new user assignments
			msg := fmt.Sprintf("🚨 *Bandwidth Quota Exceeded*\nNode: `%s`\nUsage: %d%% (%d GB / %d GB)\nMonthly quota reached!",
				nodeName, percentage, usedBytes/1073741824, quotaGB)
			if s.Notify != nil {
				s.Notify.Send(msg)
			}
			bwNotifyTracker[nodeID] = bandwidthNotifyState{Level: 2, Date: today}
			log.Printf("[bandwidth] critical: node %s at %d%% of quota — notified admin", nodeName, percentage)
		} else if percentage >= 80 && percentage < 100 && (!alreadyNotifiedToday || state.Level < 1) {
			// Warning: send warning notification
			msg := fmt.Sprintf("⚠️ *Bandwidth Quota Warning*\nNode: `%s`\nUsage: %d%% (%d GB / %d GB)\nApproaching monthly quota.",
				nodeName, percentage, usedBytes/1073741824, quotaGB)
			if s.Notify != nil {
				s.Notify.Send(msg)
			}
			bwNotifyTracker[nodeID] = bandwidthNotifyState{Level: 1, Date: today}
			log.Printf("[bandwidth] warning: node %s at %d%% of quota — notified admin", nodeName, percentage)
		}
	}
}

// resetMonthlyBandwidth resets bandwidth counters for all nodes with quotas
// on the 1st of each month. Called by the background worker.
func (s *Server) resetMonthlyBandwidth() {
	now := time.Now().UTC()
	if now.Day() != 1 {
		return
	}

	result, err := s.DB.Exec(`
		UPDATE nodes
		SET bandwidth_used_bytes = 0, bandwidth_reset_at = NOW()
		WHERE bandwidth_quota_gb IS NOT NULL
	`)
	if err != nil {
		log.Printf("[bandwidth] monthly reset error: %v", err)
		return
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		log.Printf("[bandwidth] monthly reset: cleared bandwidth counters for %d nodes", affected)

		// Clear notification tracker so alerts can fire again next cycle
		bwNotifyTrackerMu.Lock()
		bwNotifyTracker = make(map[int64]bandwidthNotifyState)
		bwNotifyTrackerMu.Unlock()
	}
}

// nodeBandwidthQuota handles GET/POST for the node-level bandwidth quota
// using the nodes table columns (bandwidth_quota_gb, bandwidth_used_bytes, bandwidth_reset_at).
// GET /api/admin/nodes/{id}/bandwidth returns current quota and usage.
// POST /api/admin/nodes/{id}/bandwidth sets or updates the quota.
func (s *Server) nodeBandwidthQuota(w http.ResponseWriter, r *http.Request, nodeID int64) {
	switch r.Method {
	case http.MethodGet:
		s.getNodeBandwidth(w, nodeID)
	case http.MethodPost:
		s.setNodeBandwidth(w, r, nodeID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getNodeBandwidth returns bandwidth quota info for a node.
func (s *Server) getNodeBandwidth(w http.ResponseWriter, nodeID int64) {
	var quotaGB sql.NullInt64
	var usedBytes int64
	var resetAt sql.NullTime

	err := s.DB.QueryRow(`
		SELECT bandwidth_quota_gb, bandwidth_used_bytes, bandwidth_reset_at
		FROM nodes WHERE id = $1
	`, nodeID).Scan(&quotaGB, &usedBytes, &resetAt)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "node_not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	resp := map[string]any{
		"ok":                   true,
		"bandwidth_used_bytes": usedBytes,
	}

	if quotaGB.Valid {
		resp["bandwidth_quota_gb"] = quotaGB.Int64
		quotaBytes := quotaGB.Int64 * 1073741824
		if quotaBytes > 0 {
			resp["usage_percent"] = float64(usedBytes) / float64(quotaBytes) * 100
		} else {
			resp["usage_percent"] = 0.0
		}
	} else {
		resp["bandwidth_quota_gb"] = nil
		resp["usage_percent"] = 0.0
	}

	if resetAt.Valid {
		resp["bandwidth_reset_at"] = resetAt.Time.UTC().Format(time.RFC3339)
	} else {
		resp["bandwidth_reset_at"] = nil
	}

	writeJSON(w, resp)
}

// setNodeBandwidth sets or clears the bandwidth quota for a node.
func (s *Server) setNodeBandwidth(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		BandwidthQuotaGB *int64 `json:"bandwidth_quota_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	var quotaVal any
	if in.BandwidthQuotaGB != nil {
		if *in.BandwidthQuotaGB < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_quota"})
			return
		}
		if *in.BandwidthQuotaGB == 0 {
			quotaVal = nil // 0 means remove quota
		} else {
			quotaVal = *in.BandwidthQuotaGB
		}
	}

	res, err := s.DB.Exec(`UPDATE nodes SET bandwidth_quota_gb = $1 WHERE id = $2`, quotaVal, nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "node_not_found"})
		return
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	writeJSON(w, map[string]any{"ok": true})
}
