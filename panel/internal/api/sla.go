//go:build !lite

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// SLA Configuration & Breach Detection
// Requirements: 16.1, 16.2, 16.3, 16.4, 16.5
// ──────────────────────────────────────────────────────────────────────────────

// handleSLAConfig handles GET and PATCH for /api/sla/config.
func (s *Server) handleSLAConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getSLAConfig(w)
	case http.MethodPatch:
		s.updateSLAConfig(w, r)
	default:
		writeJSONCode(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
	}
}

// getSLAConfig returns current SLA response time targets from sla_config table.
func (s *Server) getSLAConfig(w http.ResponseWriter) {
	rows, err := s.DB.Query(`SELECT priority, response_minutes FROM sla_config ORDER BY response_minutes ASC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type slaEntry struct {
		Priority        string `json:"priority"`
		ResponseMinutes int    `json:"response_minutes"`
	}

	configs := []slaEntry{}
	for rows.Next() {
		var entry slaEntry
		if err := rows.Scan(&entry.Priority, &entry.ResponseMinutes); err != nil {
			continue
		}
		configs = append(configs, entry)
	}

	writeJSON(w, map[string]any{"ok": true, "config": configs})
}

// updateSLAConfig updates SLA targets in the sla_config table.
// Accepts a JSON array of {priority, response_minutes} objects.
func (s *Server) updateSLAConfig(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in []struct {
		Priority        string `json:"priority"`
		ResponseMinutes int    `json:"response_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if len(in) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty_config"})
		return
	}

	validPriorities := map[string]bool{"urgent": true, "high": true, "normal": true, "low": true}

	for _, entry := range in {
		if !validPriorities[entry.Priority] {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_priority"})
			return
		}
		if entry.ResponseMinutes <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_response_minutes"})
			return
		}
	}

	for _, entry := range in {
		_, err := s.DB.Exec(
			`UPDATE sla_config SET response_minutes = ? WHERE priority = ?`,
			entry.ResponseMinutes, entry.Priority,
		)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
	}

	admin, _, _ := s.currentAdmin(r)
	s.logAudit(admin, "sla.config_updated", "sla_config", "", nil, map[string]any{"updates": in}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true})
}

// handleSLAStats returns SLA compliance statistics per priority.
func (s *Server) handleSLAStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONCode(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
		return
	}

	// Count tickets per priority: met (sla_breached=0) vs breached (sla_breached=1)
	rows, err := s.DB.Query(`
		SELECT priority, sla_breached, COUNT(*) AS cnt
		FROM tickets
		WHERE priority IS NOT NULL AND priority != ''
		GROUP BY priority, sla_breached
	`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type priorityStat struct {
		Total      int     `json:"total"`
		Met        int     `json:"met"`
		Breached   int     `json:"breached"`
		PercentMet float64 `json:"percent_met"`
	}

	stats := map[string]*priorityStat{}
	for rows.Next() {
		var priority string
		var breached int
		var count int
		if err := rows.Scan(&priority, &breached, &count); err != nil {
			continue
		}
		if _, ok := stats[priority]; !ok {
			stats[priority] = &priorityStat{}
		}
		if breached == 0 {
			stats[priority].Met += count
		} else {
			stats[priority].Breached += count
		}
		stats[priority].Total += count
	}

	// Calculate percentage met
	for _, stat := range stats {
		if stat.Total > 0 {
			stat.PercentMet = float64(stat.Met) / float64(stat.Total) * 100
		}
	}

	// Calculate average first-response time (time between ticket created_at and first admin reply)
	var avgResponseMinutes float64
	err = s.DB.QueryRow(`
		SELECT COALESCE(AVG(TIMESTAMPDIFF(MINUTE, t.created_at, first_reply.reply_at)), 0)
		FROM tickets t
		INNER JOIN (
			SELECT ticket_id, MIN(created_at) AS reply_at
			FROM ticket_messages
			WHERE sender_type = 'admin'
			GROUP BY ticket_id
		) first_reply ON first_reply.ticket_id = t.id
	`).Scan(&avgResponseMinutes)
	if err != nil {
		avgResponseMinutes = 0
	}

	writeJSON(w, map[string]any{
		"ok":                   true,
		"stats":                stats,
		"avg_response_minutes": avgResponseMinutes,
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Background Workers
// ──────────────────────────────────────────────────────────────────────────────

// checkSLABreaches queries all open tickets where sla_breached=0 AND
// sla_deadline_at IS NOT NULL AND sla_deadline_at < NOW(), marks them as
// breached, and notifies the admin.
func (s *Server) checkSLABreaches() {
	rows, err := s.DB.Query(`
		SELECT id, subject, priority
		FROM tickets
		WHERE status IN ('open', 'in_progress')
		  AND sla_breached = 0
		  AND sla_deadline_at IS NOT NULL
		  AND sla_deadline_at < NOW()
	`)
	if err != nil {
		log.Printf("[sla] breach check query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ticketID int64
		var subject, priority string
		if err := rows.Scan(&ticketID, &subject, &priority); err != nil {
			log.Printf("[sla] breach scan error: %v", err)
			continue
		}

		_, err := s.DB.Exec(`UPDATE tickets SET sla_breached = 1 WHERE id = ?`, ticketID)
		if err != nil {
			log.Printf("[sla] failed to mark ticket %d as breached: %v", ticketID, err)
			continue
		}

		log.Printf("[sla] ticket %d breached SLA deadline", ticketID)

		// Notify admin
		if s.Notify != nil {
			msg := fmt.Sprintf("🚨 *SLA Breach*\nTicket #%d breached SLA\nSubject: %s\nPriority: %s",
				ticketID, subject, priority)
			s.Notify.SendEvent("sla", fmt.Sprintf("Ticket #%d breached SLA", ticketID), msg)
		}
	}
}

// autoCloseStaleTickets closes tickets that have had no customer reply
// for longer than the configured auto_close_days (default 7).
func (s *Server) autoCloseStaleTickets() {
	// Find open tickets where the last customer reply (or created_at if no replies)
	// was more than auto_close_days ago.
	rows, err := s.DB.Query(`
		SELECT t.id, t.username, t.auto_close_days,
		       COALESCE(
		           (SELECT MAX(tm.created_at) FROM ticket_messages tm
		            WHERE tm.ticket_id = t.id AND tm.sender_type = 'customer'),
		           t.created_at
		       ) AS last_activity
		FROM tickets t
		WHERE t.status IN ('open', 'in_progress')
		  AND t.auto_close_days > 0
	`)
	if err != nil {
		log.Printf("[sla] auto-close query error: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now()

	type staleTicket struct {
		ID            int64
		Username      string
		AutoCloseDays int
	}

	var staleTickets []staleTicket
	for rows.Next() {
		var ticketID int64
		var username string
		var autoCloseDays int
		var lastActivity time.Time
		if err := rows.Scan(&ticketID, &username, &autoCloseDays, &lastActivity); err != nil {
			log.Printf("[sla] auto-close scan error: %v", err)
			continue
		}

		deadline := lastActivity.Add(time.Duration(autoCloseDays) * 24 * time.Hour)
		if now.After(deadline) {
			staleTickets = append(staleTickets, staleTicket{
				ID:            ticketID,
				Username:      username,
				AutoCloseDays: autoCloseDays,
			})
		}
	}
	rows.Close()

	for _, st := range staleTickets {
		// Close the ticket
		_, err := s.DB.Exec(`UPDATE tickets SET status = 'closed', closed_at = NOW() WHERE id = ?`, st.ID)
		if err != nil {
			log.Printf("[sla] failed to auto-close ticket %d: %v", st.ID, err)
			continue
		}

		// Insert system message
		_, err = s.DB.Exec(
			`INSERT INTO ticket_messages (ticket_id, sender_type, sender_name, message) VALUES (?, 'system', 'System', ?)`,
			st.ID, "Ticket auto-closed due to inactivity",
		)
		if err != nil {
			log.Printf("[sla] failed to insert auto-close message for ticket %d: %v", st.ID, err)
		}

		log.Printf("[sla] ticket %d auto-closed after %d days of inactivity", st.ID, st.AutoCloseDays)

		// Notify customer if notification channel configured
		if s.Notify != nil && st.Username != "" {
			s.Notify.SendEvent("ticket", fmt.Sprintf("Ticket #%d Auto-Closed", st.ID),
				fmt.Sprintf("Ticket #%d has been auto-closed due to %d days of inactivity.", st.ID, st.AutoCloseDays))
		}
	}
}

// CheckSLABreaches is the exported version for the background worker to call.
func CheckSLABreaches(s *Server) {
	s.checkSLABreaches()
}

// AutoCloseStaleTickets is the exported version for the background worker to call.
func AutoCloseStaleTickets(s *Server) {
	s.autoCloseStaleTickets()
}
