package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) tickets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTickets(w, r, "")
	case http.MethodPost:
		s.createTicket(w, r, "admin", "")
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ticketByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/tickets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.getTicket(w, r, id, "")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "reply":
		s.replyTicket(w, r, id, "admin", "")
	case "close":
		s.setTicketStatus(w, r, id, "closed")
	case "open":
		s.setTicketStatus(w, r, id, "open")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) portalTickets(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listTickets(w, r, username)
	case http.MethodPost:
		s.createTicket(w, r, "customer", username)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalTicketByID(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	id, action, ok := pathID(r.URL.Path, "/api/portal/tickets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.getTicket(w, r, id, username)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "reply":
		s.replyTicket(w, r, id, "customer", username)
	case "close":
		if !s.ticketBelongsTo(id, username) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		s.setTicketStatus(w, r, id, "closed")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request, username string) {
	where := "t.deleted_at IS NULL"
	args := []any{}
	if username != "" {
		where += " AND t.username=$1"
		args = append(args, username)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where += " AND t.status=$1"
		args = append(args, status)
	}
	rows, err := s.DB.Query(`SELECT t.id,t.customer_id,t.username,t.subject,t.status,t.priority,t.created_at,t.updated_at,t.closed_at FROM tickets t WHERE `+where+` ORDER BY t.updated_at DESC,t.id DESC LIMIT 500`, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, t)
	}
	writeJSON(w, map[string]any{"ok": true, "tickets": out})
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request, senderType, forcedUsername string) {
	actor := forcedUsername
	if senderType == "admin" {
		actor, _, _ = s.currentAdmin(r)
	}
	var in struct {
		Username string `json:"username"`
		Subject  string `json:"subject"`
		Priority string `json:"priority"`
		Message  string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if forcedUsername != "" {
		in.Username = forcedUsername
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Priority = strings.TrimSpace(in.Priority)
	in.Message = strings.TrimSpace(in.Message)
	if in.Priority == "" {
		in.Priority = "normal"
	}
	if !validTicketPriority(in.Priority) || in.Username == "" || in.Subject == "" || in.Message == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_ticket"})
		return
	}
	var customerID sql.NullInt64
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, in.Username).Scan(&customerID)
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO tickets(customer_id,username,subject,priority,status) VALUES($1,$2,$3,$4, 'open')`, nullableInt(customerID), in.Username, in.Subject, in.Priority)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO ticket_messages(ticket_id,sender_type,sender_name,message) VALUES($1,$2,$3,$4)`, id, senderType, actor, in.Message); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.logAudit(actor, "ticket.created", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"username": in.Username, "subject": in.Subject}, clientIP(r))
	severity := "info"
	if in.Priority == "high" {
		severity = "warning"
	}
	s.createEvent("ticket", severity, fmt.Sprintf("New ticket #%d: %s", id, in.Subject), fmt.Sprintf("Ticket #%d created by %s for %s", id, actor, in.Username), actor, in.Username)
	if senderType == "customer" {
		s.broadcastNotification(map[string]any{
			"id":        fmt.Sprintf("ticket-%d-%d", id, time.Now().UnixMilli()),
			"type":      "new_ticket",
			"message":   fmt.Sprintf("New support ticket from %s: %s", in.Username, in.Subject),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"read":      false,
		})
		// Telegram notification to admin
		s.Notify.SendEvent("ticket", fmt.Sprintf("🎫 New Ticket #%d", id), fmt.Sprintf("From: %s\nSubject: %s\nPriority: %s", in.Username, in.Subject, in.Priority))
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request, id int64, username string) {
	if username != "" && !s.ticketBelongsTo(id, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	ticket, err := scanTicket(s.DB.QueryRow(`SELECT id,customer_id,username,subject,status,priority,created_at,updated_at,closed_at FROM tickets WHERE id=$1 AND deleted_at IS NULL LIMIT 1`, id))
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	messages, err := s.ticketMessages(id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "ticket": TicketDetail{Ticket: ticket, Messages: messages}})
}

func (s *Server) replyTicket(w http.ResponseWriter, r *http.Request, id int64, senderType, username string) {
	if username != "" && !s.ticketBelongsTo(id, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	sender := username
	if senderType == "admin" {
		sender, _, _ = s.currentAdmin(r)
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "message_required"})
		return
	}
	if _, err := s.DB.Exec(`INSERT INTO ticket_messages(ticket_id,sender_type,sender_name,message) VALUES($1,$2,$3,$4)`, id, senderType, sender, in.Message); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = s.DB.Exec(`UPDATE tickets SET status='open',updated_at=NOW() WHERE id=$1`, id)
	s.logAudit(sender, "ticket.replied", "ticket", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	ticketUser := username
	if ticketUser == "" {
		var tu string
		_ = s.DB.QueryRow(`SELECT username FROM tickets WHERE id=$1 LIMIT 1`, id).Scan(&tu)
		ticketUser = tu
	}
	s.createEvent("ticket", "info", fmt.Sprintf("Ticket #%d replied", id), fmt.Sprintf("%s replied to ticket #%d", sender, id), sender, ticketUser)
	// Telegram notification when customer replies to admin
	if senderType == "customer" {
		s.Notify.SendEvent("ticket", fmt.Sprintf("💬 Ticket #%d Reply", id), fmt.Sprintf("From: %s\nMessage: %s", sender, in.Message[:min(len(in.Message), 100)]))
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) setTicketStatus(w http.ResponseWriter, r *http.Request, id int64, status string) {
	closedExpr := "NULL"
	if status == "closed" {
		closedExpr = "NOW()"
	}
	if _, err := s.DB.Exec(`UPDATE tickets SET status=$1,closed_at=`+closedExpr+`,updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, status, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	if actor == "" {
		actor, _ = s.currentCustomer(r)
	}
	s.logAudit(actor, "ticket.status_changed", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"status": status}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) ticketBelongsTo(id int64, username string) bool {
	var count int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM tickets WHERE id=$1 AND username=$2 AND deleted_at IS NULL`, id, username).Scan(&count)
	return count > 0
}

func (s *Server) ticketMessages(id int64) ([]TicketMessage, error) {
	rows, err := s.DB.Query(`SELECT id,ticket_id,sender_type,sender_name,message,created_at FROM ticket_messages WHERE ticket_id=$1 ORDER BY id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TicketMessage{}
	for rows.Next() {
		var m TicketMessage
		var created sql.NullTime
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderType, &m.SenderName, &m.Message, &created); err != nil {
			return out, err
		}
		if created.Valid {
			m.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type ticketScanner interface{ Scan(dest ...any) error }

func scanTicket(row ticketScanner) (Ticket, error) {
	var t Ticket
	var customerID sql.NullInt64
	var created, updated, closed sql.NullTime
	if err := row.Scan(&t.ID, &customerID, &t.Username, &t.Subject, &t.Status, &t.Priority, &created, &updated, &closed); err != nil {
		return t, err
	}
	if customerID.Valid {
		t.CustomerID = &customerID.Int64
	}
	if created.Valid {
		t.CreatedAt = created.Time.Format(time.RFC3339)
	}
	if updated.Valid {
		t.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	if closed.Valid {
		t.ClosedAt = closed.Time.Format(time.RFC3339)
	}
	return t, nil
}

func validTicketPriority(priority string) bool {
	switch priority {
	case "low", "normal", "high":
		return true
	default:
		return false
	}
}
