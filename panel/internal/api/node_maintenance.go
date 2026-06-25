package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// nodeMaintenance handles POST /api/admin/nodes/:id/maintenance
// Enables or disables maintenance mode for a single node with notifications and connection draining.
// Request body: {"enabled": true/false, "reason": "...", "estimated_duration": "2h", "notify_users": true}
func (s *Server) nodeMaintenance(w http.ResponseWriter, r *http.Request, nodeID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		Enabled           bool   `json:"enabled"`
		Reason            string `json:"reason"`
		EstimatedDuration string `json:"estimated_duration"` // e.g. "2h", "30m", "1h30m"
		NotifyUsers       bool   `json:"notify_users"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		in.Reason = "Scheduled maintenance"
	}
	if len(in.Reason) > 255 {
		in.Reason = in.Reason[:255]
	}

	in.EstimatedDuration = strings.TrimSpace(in.EstimatedDuration)
	if len(in.EstimatedDuration) > 50 {
		in.EstimatedDuration = in.EstimatedDuration[:50]
	}

	// Get node name for notifications
	var nodeName string
	err := s.DB.QueryRow(`SELECT name FROM nodes WHERE id=? LIMIT 1`, nodeID).Scan(&nodeName)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "node_not_found"})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	if in.Enabled {
		err = s.enableMaintenance(nodeID, nodeName, in.Reason, in.EstimatedDuration, in.NotifyUsers, actor)
	} else {
		err = s.disableMaintenance(nodeID, nodeName, in.NotifyUsers, actor)
	}

	if err != nil {
		log.Printf("[maintenance] error nodeID=%d enabled=%v: %v", nodeID, in.Enabled, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "maintenance_failed"})
		return
	}

	// Audit log
	s.logAudit(actor, "node.maintenance", "node", strconv.FormatInt(nodeID, 10), nil, map[string]any{
		"enabled":            in.Enabled,
		"reason":             in.Reason,
		"estimated_duration": in.EstimatedDuration,
	}, ip)

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	log.Printf("[maintenance] node=%s(%d) enabled=%v reason=%q estimated=%s by=%s", nodeName, nodeID, in.Enabled, in.Reason, in.EstimatedDuration, actor)

	writeJSON(w, map[string]any{
		"ok":                 true,
		"enabled":            in.Enabled,
		"node_id":            nodeID,
		"reason":             in.Reason,
		"estimated_duration": in.EstimatedDuration,
	})
}

// enableMaintenance sets a node into maintenance mode:
// - Updates nodes table (maintenance_mode=TRUE, status='maintenance')
// - Disables all running cores on the node via gRPC to drain connections
// - Records a downtime entry for SLA tracking
// - Optionally sends Telegram notification
func (s *Server) enableMaintenance(nodeID int64, nodeName, reason, estimatedDuration string, notifyUsers bool, actor string) error {
	// Update node status
	_, err := s.DB.Exec(
		`UPDATE nodes SET maintenance_mode=TRUE, status='maintenance' WHERE id=?`,
		nodeID,
	)
	if err != nil {
		return fmt.Errorf("update node status: %w", err)
	}

	// Drain connections by disabling cores via gRPC
	if s.CoreMgr != nil {
		ctx := context.Background()
		statuses, err := s.CoreMgr.AllCoreStatuses(ctx, nodeID)
		if err == nil {
			for _, cs := range statuses {
				if cs.State == "running" {
					if disableErr := s.CoreMgr.DisableCore(ctx, nodeID, cs.Type); disableErr != nil {
						log.Printf("[knode] maintenance drain: DisableCore %q on node %d failed: %v", cs.Type, nodeID, disableErr)
					}
				}
			}
		} else {
			log.Printf("[knode] maintenance: AllCoreStatuses failed for node %d: %v", nodeID, err)
		}
	}

	// Record downtime entry for SLA tracking (task 3.8)
	_, err = s.DB.Exec(
		`INSERT INTO node_downtimes(node_id, started_at, reason) VALUES(?, NOW(), ?)`,
		nodeID, reason,
	)
	if err != nil {
		return fmt.Errorf("record downtime: %w", err)
	}

	// Send Telegram notification
	if notifyUsers && s.Notify != nil {
		durationLine := ""
		if estimatedDuration != "" {
			durationLine = fmt.Sprintf("\nEstimated Duration: %s", escapeMarkdownNotify(estimatedDuration))
		}
		msg := fmt.Sprintf("🔧 *Maintenance Started*\nNode: `%s`\nReason: %s%s\nTime: %s",
			escapeMarkdownNotify(nodeName),
			escapeMarkdownNotify(reason),
			durationLine,
			time.Now().UTC().Format(time.RFC3339),
		)
		s.Notify.Send(msg)
	}

	return nil
}

// disableMaintenance takes a node out of maintenance mode:
// - Updates nodes table (maintenance_mode=FALSE, status='online')
// - Re-enables cores on the node via gRPC to resume accepting connections
// - Closes the open downtime entry (sets ended_at, calculates duration for SLA tracking)
// - Optionally sends Telegram notification
func (s *Server) disableMaintenance(nodeID int64, nodeName string, notifyUsers bool, actor string) error {
	// Update node status
	_, err := s.DB.Exec(
		`UPDATE nodes SET maintenance_mode=FALSE, status='online' WHERE id=?`,
		nodeID,
	)
	if err != nil {
		return fmt.Errorf("update node status: %w", err)
	}

	// Re-enable cores via gRPC (resume accepting connections)
	if s.CoreMgr != nil {
		ctx := context.Background()
		rows, qErr := s.DB.Query(`SELECT core_name, COALESCE(port, 0) FROM node_cores WHERE node_id = ? AND status = 'installed'`, nodeID)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var coreName string
				var port int
				if scanErr := rows.Scan(&coreName, &port); scanErr != nil {
					continue
				}
				if enableErr := s.CoreMgr.EnableCore(ctx, nodeID, coreName, port, nil); enableErr != nil {
					log.Printf("[knode] maintenance resume: EnableCore %q on node %d failed: %v", coreName, nodeID, enableErr)
				}
			}
		}
	}

	// Close the most recent open downtime entry for this node (for SLA tracking)
	_, err = s.DB.Exec(
		`UPDATE node_downtimes SET ended_at=NOW(), duration_seconds=TIMESTAMPDIFF(SECOND, started_at, NOW()) WHERE node_id=? AND ended_at IS NULL ORDER BY started_at DESC LIMIT 1`,
		nodeID,
	)
	if err != nil {
		return fmt.Errorf("close downtime entry: %w", err)
	}

	// Send Telegram notification
	if notifyUsers && s.Notify != nil {
		msg := fmt.Sprintf("✅ *Maintenance Ended*\nNode: `%s`\nStatus: Online\nTime: %s",
			escapeMarkdownNotify(nodeName),
			time.Now().UTC().Format(time.RFC3339),
		)
		s.Notify.Send(msg)
	}

	return nil
}

// escapeMarkdownNotify escapes Telegram Markdown special characters for notification messages.
func escapeMarkdownNotify(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"`", "\\`",
	)
	return replacer.Replace(s)
}
