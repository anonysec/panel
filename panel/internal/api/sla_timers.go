//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Default SLA response targets in minutes per priority level.
const (
	defaultSLALowMinutes    = 480 // 8 hours
	defaultSLAMediumMinutes = 120 // 2 hours
	defaultSLAHighMinutes   = 30  // 30 minutes
)

// SLATargets holds the configured SLA response time targets per priority.
type SLATargets struct {
	LowMinutes    int `json:"low_minutes"`
	MediumMinutes int `json:"medium_minutes"`
	HighMinutes   int `json:"high_minutes"`
}

// loadSLATargets reads SLA response targets from the panel_settings table.
// Returns defaults if not configured.
func loadSLATargets(db *sql.DB) SLATargets {
	targets := SLATargets{
		LowMinutes:    defaultSLALowMinutes,
		MediumMinutes: defaultSLAMediumMinutes,
		HighMinutes:   defaultSLAHighMinutes,
	}

	rows, err := db.Query(`SELECT setting_key, setting_value FROM panel_settings WHERE setting_key IN ('sla_response_minutes_low', 'sla_response_minutes_medium', 'sla_response_minutes_high')`)
	if err != nil {
		return targets
	}
	defer rows.Close()

	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || v <= 0 {
			continue
		}
		switch key {
		case "sla_response_minutes_low":
			targets.LowMinutes = v
		case "sla_response_minutes_medium":
			targets.MediumMinutes = v
		case "sla_response_minutes_high":
			targets.HighMinutes = v
		}
	}
	return targets
}

// slaTargetForPriority returns the SLA target duration for a given priority.
func slaTargetForPriority(targets SLATargets, priority string) time.Duration {
	switch priority {
	case "high":
		return time.Duration(targets.HighMinutes) * time.Minute
	case "medium":
		return time.Duration(targets.MediumMinutes) * time.Minute
	default:
		return time.Duration(targets.LowMinutes) * time.Minute
	}
}

// CheckOverdueTickets finds open/in_progress tickets that have exceeded their
// SLA response target and sends a Telegram notification for each.
// A ticket is only alerted once (tracked via the sla_alerted_at column).
// Designed to be called from the background worker on every tick.
func CheckOverdueTickets(db *sql.DB, notify func(string)) {
	targets := loadSLATargets(db)

	// Query tickets that are open or in_progress, have no SLA alert yet,
	// and have exceeded their SLA response target based on priority.
	// We use the latest customer message time (or ticket creation time) as
	// the reference point for the SLA clock.
	rows, err := db.Query(`
		SELECT t.id, t.subject, t.priority, t.created_at,
		       COALESCE(c.username, CONCAT('customer_', t.customer_id)) AS customer_name,
		       COALESCE(
		           (SELECT MAX(tm.created_at) FROM ticket_messages tm
		            WHERE tm.ticket_id = t.id AND tm.sender_type = 'customer'),
		           t.created_at
		       ) AS last_customer_msg
		FROM tickets t
		LEFT JOIN customers c ON c.id = t.customer_id
		WHERE t.status IN ('open', 'in_progress')
		  AND t.sla_alerted_at IS NULL
	`)
	if err != nil {
		log.Printf("[sla] query error: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now()

	for rows.Next() {
		var ticketID int64
		var subject, priority, customerName string
		var createdAt, lastCustomerMsg time.Time

		if err := rows.Scan(&ticketID, &subject, &priority, &createdAt, &customerName, &lastCustomerMsg); err != nil {
			log.Printf("[sla] scan error: %v", err)
			continue
		}

		target := slaTargetForPriority(targets, priority)
		elapsed := now.Sub(lastCustomerMsg)

		if elapsed <= target {
			continue // not overdue yet
		}

		// Mark as alerted to prevent repeated notifications
		_, err := db.Exec(`UPDATE tickets SET sla_alerted_at = NOW() WHERE id = ?`, ticketID)
		if err != nil {
			log.Printf("[sla] failed to mark ticket #%d as alerted: %v", ticketID, err)
			continue
		}

		overdueDuration := elapsed - target
		overdueStr := formatDuration(overdueDuration)

		msg := fmt.Sprintf("🚨 *SLA Breach*\nTicket: #%d\nSubject: %s\nCustomer: %s\nPriority: %s\nOverdue by: %s",
			ticketID, subject, customerName, strings.ToUpper(priority), overdueStr)
		notify(msg)
		log.Printf("[sla] ticket #%d overdue by %s (priority=%s, target=%v)", ticketID, overdueStr, priority, target)
	}
}

// formatDuration formats a duration as a human-readable string like "2h 15m" or "45m".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "< 1m"
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// ──────────────────────────────────────────────────────────────────────────────
// SLA Settings API — GET/POST /api/admin/settings/sla
// ──────────────────────────────────────────────────────────────────────────────

// adminSLASettings handles GET (retrieve targets) and POST (update targets)
// for configuring SLA response time thresholds.
func (s *Server) adminSLASettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getSLASettings(w)
	case http.MethodPost:
		s.setSLASettings(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// getSLASettings returns the current SLA response targets.
func (s *Server) getSLASettings(w http.ResponseWriter) {
	targets := loadSLATargets(s.DB)
	writeJSON(w, map[string]any{
		"ok":      true,
		"targets": targets,
	})
}

// setSLASettings updates SLA response targets in panel_settings.
func (s *Server) setSLASettings(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		LowMinutes    *int `json:"low_minutes"`
		MediumMinutes *int `json:"medium_minutes"`
		HighMinutes   *int `json:"high_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate: all values must be positive integers if provided
	if in.LowMinutes != nil && *in.LowMinutes <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_low_minutes"})
		return
	}
	if in.MediumMinutes != nil && *in.MediumMinutes <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_medium_minutes"})
		return
	}
	if in.HighMinutes != nil && *in.HighMinutes <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_high_minutes"})
		return
	}

	// Update each provided value
	updates := map[string]int{}
	if in.LowMinutes != nil {
		updates["sla_response_minutes_low"] = *in.LowMinutes
	}
	if in.MediumMinutes != nil {
		updates["sla_response_minutes_medium"] = *in.MediumMinutes
	}
	if in.HighMinutes != nil {
		updates["sla_response_minutes_high"] = *in.HighMinutes
	}

	if len(updates) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_fields"})
		return
	}

	for key, val := range updates {
		_, err := s.DB.Exec(
			`INSERT INTO panel_settings(setting_key, setting_value) VALUES(?, ?) ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
			key, strconv.Itoa(val),
		)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
	}

	admin, _, _ := s.currentAdmin(r)
	s.logAudit(admin, "settings.sla_updated", "panel_settings", "", nil, map[string]any{"updates": updates}, clientIP(r))

	// Return the updated targets
	targets := loadSLATargets(s.DB)
	writeJSON(w, map[string]any{
		"ok":      true,
		"targets": targets,
	})
}
