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

// alertCooldown defines the minimum interval between repeated notifications
// for the same node/metric combination. Once a threshold breach is notified,
// subsequent breaches for the same metric are suppressed until this duration
// elapses, preventing Telegram spam.
const alertCooldown = 15 * time.Minute

// alertState tracks the last notification time for each node+metric pair.
// Key format: "nodeID:metric" (e.g., "3:cpu", "7:conn").
var (
	alertState   = make(map[string]time.Time)
	alertStateMu sync.Mutex
)

// alertKey builds the deduplication key for a specific node and metric.
func alertKey(nodeID int64, metric string) string {
	return fmt.Sprintf("%d:%s", nodeID, metric)
}

// CheckNodeAlerts queries all online nodes, compares their latest metrics
// against configured thresholds, and sends a Telegram notification when a
// threshold is breached (subject to cooldown). Clears the alert state when
// the metric recovers below the threshold.
// This function is designed to be called from the background worker.
func CheckNodeAlerts(db *sql.DB, notify func(string)) {
	rows, err := db.Query(`
		SELECT n.id, n.name,
		       n.alert_cpu_threshold, n.alert_ram_threshold, n.alert_disk_threshold,
		       COALESCE(n.alert_conn_threshold, 0),
		       COALESCE(ns.cpu_percent, 0), COALESCE(ns.ram_percent, 0), COALESCE(ns.disk_percent, 0)
		FROM nodes n
		JOIN node_status ns ON ns.node_id = n.id
		WHERE n.status = 'online'
		  AND (n.alert_cpu_threshold > 0 OR n.alert_ram_threshold > 0 OR n.alert_disk_threshold > 0 OR COALESCE(n.alert_conn_threshold, 0) > 0)
	`)
	if err != nil {
		log.Printf("[alerts] query error: %v", err)
		return
	}
	defer rows.Close()

	alertStateMu.Lock()
	defer alertStateMu.Unlock()

	now := time.Now()

	for rows.Next() {
		var nodeID int64
		var name string
		var cpuThresh, ramThresh, diskThresh, connThresh int
		var cpu, ram, disk float64

		if err := rows.Scan(&nodeID, &name, &cpuThresh, &ramThresh, &diskThresh, &connThresh, &cpu, &ram, &disk); err != nil {
			log.Printf("[alerts] scan error: %v", err)
			continue
		}

		// CPU check
		checkMetric(nodeID, name, "cpu", cpu, cpuThresh, now, notify)
		// RAM check
		checkMetric(nodeID, name, "ram", ram, ramThresh, now, notify)
		// Disk check
		checkMetric(nodeID, name, "disk", disk, diskThresh, now, notify)

		// Connection count check (if threshold configured)
		if connThresh > 0 {
			connCount := getNodeConnectionCount(db, nodeID)
			checkMetricCount(nodeID, name, "conn", connCount, connThresh, now, notify)
		}
	}
}

// getNodeConnectionCount returns the latest online_users value for a node
// from the most recent usage snapshot.
func getNodeConnectionCount(db *sql.DB, nodeID int64) int {
	var count int
	err := db.QueryRow(`SELECT COALESCE(online_users, 0) FROM node_usage_snapshots WHERE node_id=$1 ORDER BY id DESC LIMIT 1`, nodeID).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

// checkMetric compares a percentage metric against its threshold and sends a
// notification on breach (subject to cooldown). Clears alert state on recovery.
func checkMetric(nodeID int64, nodeName, metric string, value float64, threshold int, now time.Time, notify func(string)) {
	if threshold <= 0 {
		return
	}

	key := alertKey(nodeID, metric)
	breached := value > float64(threshold)

	if breached {
		lastNotified, exists := alertState[key]
		if !exists || now.Sub(lastNotified) >= alertCooldown {
			// First breach or cooldown elapsed — send notification
			alertState[key] = now
			msg := fmt.Sprintf("⚠️ *Node Alert*\nNode: `%s`\n%s usage: %.1f%% (threshold: %d%%)",
				nodeName, strings.ToUpper(metric), value, threshold)
			notify(msg)
			log.Printf("[alerts] %s on node %s: %.1f%% > %d%%", metric, nodeName, value, threshold)
		}
	} else {
		if _, exists := alertState[key]; exists {
			// Recovered — clear state so next breach triggers immediately
			delete(alertState, key)
			log.Printf("[alerts] %s recovered on node %s: %.1f%% <= %d%%", metric, nodeName, value, threshold)
		}
	}
}

// checkMetricCount compares an absolute count metric against its threshold.
func checkMetricCount(nodeID int64, nodeName, metric string, value int, threshold int, now time.Time, notify func(string)) {
	if threshold <= 0 {
		return
	}

	key := alertKey(nodeID, metric)
	breached := value > threshold

	if breached {
		lastNotified, exists := alertState[key]
		if !exists || now.Sub(lastNotified) >= alertCooldown {
			alertState[key] = now
			msg := fmt.Sprintf("⚠️ *Node Alert*\nNode: `%s`\nConnections: %d (threshold: %d)",
				nodeName, value, threshold)
			notify(msg)
			log.Printf("[alerts] %s on node %s: %d > %d", metric, nodeName, value, threshold)
		}
	} else {
		if _, exists := alertState[key]; exists {
			delete(alertState, key)
			log.Printf("[alerts] %s recovered on node %s: %d <= %d", metric, nodeName, value, threshold)
		}
	}
}

// nodeAlerts handles GET/POST /api/admin/nodes/:id/alerts to configure thresholds.
// GET returns current thresholds; POST updates them.
// POST expects JSON: {"cpu_threshold": 80, "ram_threshold": 90, "disk_threshold": 85, "conn_threshold": 500}
func (s *Server) nodeAlerts(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/admin/nodes/{id}/alerts
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[1] != "alerts" {
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
		s.getNodeAlerts(w, nodeID)
	case http.MethodPost:
		s.setNodeAlerts(w, r, nodeID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// getNodeAlerts returns the current alert thresholds for a node.
func (s *Server) getNodeAlerts(w http.ResponseWriter, nodeID int64) {
	var cpu, ram, disk, conn int
	err := s.DB.QueryRow(`SELECT alert_cpu_threshold, alert_ram_threshold, alert_disk_threshold, COALESCE(alert_conn_threshold, 0) FROM nodes WHERE id=$1`, nodeID).Scan(&cpu, &ram, &disk, &conn)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{
		"ok":             true,
		"cpu_threshold":  cpu,
		"ram_threshold":  ram,
		"disk_threshold": disk,
		"conn_threshold": conn,
	})
}

// setNodeAlerts updates the alert thresholds for a node.
func (s *Server) setNodeAlerts(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		CPUThreshold  *int `json:"cpu_threshold"`
		RAMThreshold  *int `json:"ram_threshold"`
		DiskThreshold *int `json:"disk_threshold"`
		ConnThreshold *int `json:"conn_threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate thresholds (0 = disabled, 1-100 valid for percentages)
	if in.CPUThreshold != nil && (*in.CPUThreshold < 0 || *in.CPUThreshold > 100) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_cpu_threshold"})
		return
	}
	if in.RAMThreshold != nil && (*in.RAMThreshold < 0 || *in.RAMThreshold > 100) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_ram_threshold"})
		return
	}
	if in.DiskThreshold != nil && (*in.DiskThreshold < 0 || *in.DiskThreshold > 100) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_disk_threshold"})
		return
	}
	// Connection threshold: 0 = disabled, any positive integer is valid
	if in.ConnThreshold != nil && *in.ConnThreshold < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_conn_threshold"})
		return
	}

	// Build partial update
	sets := []string{}
	args := []any{}
	if in.CPUThreshold != nil {
		sets = append(sets, "alert_cpu_threshold = $1")
		args = append(args, *in.CPUThreshold)
	}
	if in.RAMThreshold != nil {
		sets = append(sets, "alert_ram_threshold = $1")
		args = append(args, *in.RAMThreshold)
	}
	if in.DiskThreshold != nil {
		sets = append(sets, "alert_disk_threshold = $1")
		args = append(args, *in.DiskThreshold)
	}
	if in.ConnThreshold != nil {
		sets = append(sets, "alert_conn_threshold = $1")
		args = append(args, *in.ConnThreshold)
	}

	if len(sets) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_fields"})
		return
	}

	args = append(args, nodeID)
	query := fmt.Sprintf("UPDATE nodes SET %s WHERE id = ?", strings.Join(sets, ", "))

	if _, err := s.DB.Exec(query, args...); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	writeJSON(w, map[string]any{"ok": true})
}
