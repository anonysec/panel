//go:build !lite

package api

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// CheckSLABreachesStandalone queries all open tickets where sla_breached=0 AND
// sla_deadline_at IS NOT NULL AND sla_deadline_at < NOW(), marks them as
// breached, and notifies the admin.
// Designed to be called from the background worker on each tick.
func CheckSLABreachesStandalone(db *sql.DB, notify func(string)) {
	rows, err := db.Query(`
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

		_, err := db.Exec(`UPDATE tickets SET sla_breached = 1 WHERE id = ?`, ticketID)
		if err != nil {
			log.Printf("[sla] failed to mark ticket %d as breached: %v", ticketID, err)
			continue
		}

		log.Printf("[sla] ticket %d breached SLA deadline", ticketID)

		if notify != nil {
			msg := fmt.Sprintf("🚨 *SLA Breach*\nTicket #%d breached SLA\nSubject: %s\nPriority: %s",
				ticketID, subject, priority)
			notify(msg)
		}
	}
}

// AutoCloseStaleTicketsStandalone closes tickets that have had no customer reply
// for longer than the configured auto_close_days (default 7).
// Designed to be called from the background worker on each tick.
func AutoCloseStaleTicketsStandalone(db *sql.DB, notify func(string)) {
	rows, err := db.Query(`
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

	for _, st := range staleTickets {
		// Close the ticket
		_, err := db.Exec(`UPDATE tickets SET status = 'closed', closed_at = NOW() WHERE id = ?`, st.ID)
		if err != nil {
			log.Printf("[sla] failed to auto-close ticket %d: %v", st.ID, err)
			continue
		}

		// Insert system message
		_, err = db.Exec(
			`INSERT INTO ticket_messages (ticket_id, sender_type, sender_name, message) VALUES (?, 'system', 'System', ?)`,
			st.ID, "Ticket auto-closed due to inactivity",
		)
		if err != nil {
			log.Printf("[sla] failed to insert auto-close message for ticket %d: %v", st.ID, err)
		}

		log.Printf("[sla] ticket %d auto-closed after %d days of inactivity", st.ID, st.AutoCloseDays)

		// Notify
		if notify != nil {
			notify(fmt.Sprintf("📋 Ticket #%d auto-closed after %d days of inactivity.", st.ID, st.AutoCloseDays))
		}
	}
}
