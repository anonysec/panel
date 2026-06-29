//go:build !lite

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

	"KorisPanel/panel/internal/support"
)

// ──────────────────────────────────────────────────────────────────────────────
// Admin Ticket Endpoints (new support system via TicketService)
// ──────────────────────────────────────────────────────────────────────────────

// adminTickets handles GET /api/admin/tickets (list) and POST /api/admin/tickets (create on behalf of customer).
func (s *Server) adminTickets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.adminListTickets(w, r)
	case http.MethodPost:
		s.adminCreateTicket(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// adminTicketByID handles GET/PUT /api/admin/tickets/:id and POST /api/admin/tickets/:id/reply, POST /api/admin/tickets/:id/attach
func (s *Server) adminTicketByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/admin/tickets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		s.adminGetTicket(w, r, id)
	case action == "" && r.Method == http.MethodPut:
		s.adminUpdateTicket(w, r, id)
	case action == "reply" && r.Method == http.MethodPost:
		s.adminReplyTicket(w, r, id)
	case action == "attach" && r.Method == http.MethodPost:
		s.adminAttachFile(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// adminListTickets lists all tickets with filtering and pagination.
func (s *Server) adminListTickets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page := 1
	if p := q.Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	limit := 20
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	offset := (page - 1) * limit

	filter := support.ListFilter{
		Status:     strings.TrimSpace(q.Get("status")),
		Category:   strings.TrimSpace(q.Get("category")),
		Priority:   strings.TrimSpace(q.Get("priority")),
		AssignedTo: strings.TrimSpace(q.Get("assigned_to")),
		Limit:      limit,
		Offset:     offset,
	}

	tickets, total, err := s.Support.List(r.Context(), filter)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "list_failed"})
		return
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"tickets": tickets,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// adminCreateTicket creates a ticket on behalf of a customer (admin action).
func (s *Server) adminCreateTicket(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		CustomerID int64  `json:"customer_id"`
		Subject    string `json:"subject"`
		Category   string `json:"category"`
		Priority   string `json:"priority"`
		Body       string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(in.Body)

	if in.CustomerID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_id_required"})
		return
	}
	if in.Subject == "" || in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "subject_and_body_required"})
		return
	}

	// Verify customer exists
	var username string
	err := s.DB.QueryRowContext(r.Context(), `SELECT username FROM customers WHERE id = $1 AND deleted_at IS NULL LIMIT 1`, in.CustomerID).Scan(&username)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	ticket, err := s.Support.Create(r.Context(), in.CustomerID, in.Subject, in.Category, in.Priority, in.Body)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Set SLA deadline based on priority
	s.setSLADeadline(r.Context(), ticket.ID, ticket.Priority)

	// Telegram notification for urgent priority
	if ticket.Priority == "urgent" {
		s.Notify.SendEvent("support", fmt.Sprintf("🚨 Urgent Ticket #%d", ticket.ID),
			fmt.Sprintf("Subject: %s\nCustomer ID: %d\nCreated by admin", in.Subject, in.CustomerID))
	}

	admin, _, _ := s.currentAdmin(r)

	// Auto-assign the ticket
	assignedTo, _ := s.Support.AutoAssign(r.Context(), ticket.ID, in.Category)

	s.logAudit(admin, "support_ticket.created_for_customer", "ticket", strconv.FormatInt(ticket.ID, 10), nil,
		map[string]any{"customer_id": in.CustomerID, "subject": in.Subject, "category": in.Category, "priority": in.Priority}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "ticket": ticket, "assigned_to": assignedTo})
}

// adminGetTicket returns a single ticket with its messages.
func (s *Server) adminGetTicket(w http.ResponseWriter, r *http.Request, id int64) {
	ticket, err := s.Support.Get(r.Context(), id)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	messages, err := s.Support.GetMessages(r.Context(), id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "messages_failed"})
		return
	}

	writeJSON(w, map[string]any{
		"ok":       true,
		"ticket":   ticket,
		"messages": messages,
	})
}

// adminUpdateTicket handles status transitions, priority, category, and assignment updates.
func (s *Server) adminUpdateTicket(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Status     string `json:"status"`
		Priority   string `json:"priority"`
		Category   string `json:"category"`
		AssignedTo string `json:"assigned_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	admin, _, _ := s.currentAdmin(r)

	// Update status if provided
	if in.Status != "" {
		if err := s.Support.UpdateStatus(r.Context(), id, in.Status); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		s.logAudit(admin, "support_ticket.status_changed", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"status": in.Status}, clientIP(r))
	}

	// Update priority if provided
	if in.Priority != "" {
		switch in.Priority {
		case "low", "normal", "medium", "high", "urgent":
			// Update priority and recalculate SLA deadline
			_, err := s.DB.ExecContext(r.Context(), `UPDATE tickets SET priority = $1, updated_at = NOW() WHERE id = $2`, in.Priority, id)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "priority_update_failed"})
				return
			}
			// Recalculate SLA deadline based on new priority
			s.setSLADeadline(r.Context(), id, in.Priority)

			// Telegram notification for urgent priority
			if in.Priority == "urgent" {
				s.Notify.SendEvent("support", fmt.Sprintf("🚨 Ticket #%d Urgent", id),
					fmt.Sprintf("Ticket #%d has been set to URGENT priority by %s", id, admin))
			}
			s.logAudit(admin, "support_ticket.priority_changed", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"priority": in.Priority}, clientIP(r))
		default:
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_priority"})
			return
		}
	}

	// Update category if provided
	if in.Category != "" {
		switch in.Category {
		case "billing", "technical", "general":
			_, err := s.DB.ExecContext(r.Context(), `UPDATE tickets SET category = $1, updated_at = NOW() WHERE id = $2`, in.Category, id)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "category_update_failed"})
				return
			}
			s.logAudit(admin, "support_ticket.category_changed", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"category": in.Category}, clientIP(r))
		default:
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_category"})
			return
		}
	}

	// Update assignment if provided
	if in.AssignedTo != "" {
		_, err := s.DB.ExecContext(r.Context(), `UPDATE tickets SET assigned_to = $1, updated_at = NOW() WHERE id = $2`, in.AssignedTo, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "assign_failed"})
			return
		}
		s.logAudit(admin, "support_ticket.assigned", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"assigned_to": in.AssignedTo}, clientIP(r))
	}

	writeJSON(w, map[string]any{"ok": true})
}

// adminReplyTicket adds an admin reply to a ticket.
func (s *Server) adminReplyTicket(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Body       string `json:"body"`
		IsInternal bool   `json:"is_internal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "body_required"})
		return
	}

	admin, _, _ := s.currentAdmin(r)

	msg, err := s.Support.Reply(r.Context(), id, "admin", admin, in.Body, in.IsInternal)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s.logAudit(admin, "support_ticket.replied", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"internal": in.IsInternal}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

// ──────────────────────────────────────────────────────────────────────────────
// Customer Ticket Endpoints (new support system via TicketService)
// ──────────────────────────────────────────────────────────────────────────────

// customerTickets handles GET (list) and POST (create) for /api/customer/tickets.
func (s *Server) customerTickets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.customerListTickets(w, r)
	case http.MethodPost:
		s.customerCreateTicket(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// customerTicketByID handles POST /api/customer/tickets/:id/reply, POST /api/customer/tickets/:id/rate, and POST /api/customer/tickets/:id/attach.
func (s *Server) customerTicketByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/customer/tickets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	switch action {
	case "reply":
		s.customerReplyTicket(w, r, id)
	case "rate":
		s.customerRateTicket(w, r, id)
	case "attach":
		s.customerAttachFile(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// customerListTickets lists tickets belonging to the current customer.
func (s *Server) customerListTickets(w http.ResponseWriter, r *http.Request) {
	// Check if the support service is initialized
	if s.Support == nil {
		log.Printf("[tickets] Support service is nil — service not initialized")
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "service_unavailable"})
		return
	}

	username, _ := s.currentCustomer(r)

	// Get customer ID from username
	var customerID int64
	err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM customers WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if err != nil {
		log.Printf("[tickets] customer username %q could not be resolved to a customer_id: %v", username, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	filter := support.ListFilter{
		CustomerID: customerID,
		Limit:      50,
		Offset:     0,
	}

	tickets, total, err := s.Support.List(r.Context(), filter)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "list_failed"})
		return
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"tickets": tickets,
		"total":   total,
	})
}

// customerCreateTicket creates a new ticket for the logged-in customer.
func (s *Server) customerCreateTicket(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	username, _ := s.currentCustomer(r)

	var in struct {
		Subject  string `json:"subject"`
		Category string `json:"category"`
		Priority string `json:"priority"`
		Body     string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(in.Body)

	if in.Subject == "" || in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "subject_and_body_required"})
		return
	}

	// Get customer ID
	var customerID int64
	err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM customers WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	ticket, err := s.Support.Create(r.Context(), customerID, in.Subject, in.Category, in.Priority, in.Body)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Set SLA deadline based on priority
	s.setSLADeadline(r.Context(), ticket.ID, ticket.Priority)

	// Auto-assign the ticket
	assignedTo, _ := s.Support.AutoAssign(r.Context(), ticket.ID, in.Category)

	// Notify admin via WebSocket
	s.broadcastNotification(map[string]any{
		"id":        fmt.Sprintf("support-ticket-%d", ticket.ID),
		"type":      "new_support_ticket",
		"message":   fmt.Sprintf("New support ticket from %s: %s", username, in.Subject),
		"timestamp": ticket.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"read":      false,
	})

	// Telegram notification — urgent gets special notification
	if ticket.Priority == "urgent" {
		s.Notify.SendEvent("support", fmt.Sprintf("🚨 Urgent Ticket #%d", ticket.ID),
			fmt.Sprintf("From: %s\nSubject: %s\nCategory: %s\nPriority: URGENT\nAssigned: %s",
				username, in.Subject, in.Category, assignedTo))
	} else {
		s.Notify.SendEvent("support", fmt.Sprintf("🎫 New Support Ticket #%d", ticket.ID),
			fmt.Sprintf("From: %s\nSubject: %s\nCategory: %s\nPriority: %s\nAssigned: %s",
				username, in.Subject, in.Category, in.Priority, assignedTo))
	}

	s.logAudit(username, "support_ticket.created", "ticket", strconv.FormatInt(ticket.ID, 10), nil,
		map[string]any{"subject": in.Subject, "category": in.Category, "priority": in.Priority}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "ticket": ticket, "assigned_to": assignedTo})
}

// customerReplyTicket adds a customer reply to their own ticket.
func (s *Server) customerReplyTicket(w http.ResponseWriter, r *http.Request, ticketID int64) {
	limitBody(w, r, maxJSONBody)

	username, _ := s.currentCustomer(r)

	// Verify ticket belongs to this customer
	if !s.supportTicketBelongsToCustomer(r, ticketID, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "body_required"})
		return
	}

	msg, err := s.Support.Reply(r.Context(), ticketID, "customer", username, in.Body, false)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Notify admin
	s.Notify.SendEvent("support", fmt.Sprintf("💬 Ticket #%d Reply", ticketID),
		fmt.Sprintf("From: %s\nMessage: %s", username, truncate(in.Body, 100)))

	s.logAudit(username, "support_ticket.replied", "ticket", strconv.FormatInt(ticketID, 10), nil, nil, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "message": msg})
}

// customerRateTicket allows rating a resolved/closed ticket.
func (s *Server) customerRateTicket(w http.ResponseWriter, r *http.Request, ticketID int64) {
	limitBody(w, r, maxJSONBody)

	username, _ := s.currentCustomer(r)

	// Verify ticket belongs to this customer
	if !s.supportTicketBelongsToCustomer(r, ticketID, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	var in struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if err := s.Support.Rate(r.Context(), ticketID, in.Rating); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s.logAudit(username, "support_ticket.rated", "ticket", strconv.FormatInt(ticketID, 10), nil, map[string]any{"rating": in.Rating}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true})
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// supportTicketBelongsToCustomer checks that a ticket (in the new support schema)
// belongs to the customer identified by username.
func (s *Server) supportTicketBelongsToCustomer(r *http.Request, ticketID int64, username string) bool {
	var customerID int64
	err := s.DB.QueryRowContext(r.Context(), `SELECT id FROM customers WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if err != nil {
		return false
	}
	ticket, err := s.Support.Get(r.Context(), ticketID)
	if err != nil {
		return false
	}
	return ticket.CustomerID == customerID
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// setSLADeadline calculates and sets the sla_deadline_at for a ticket based on its priority.
// It looks up the response_minutes from sla_config for the given priority.
func (s *Server) setSLADeadline(ctx context.Context, ticketID int64, priority string) {
	var minutes int
	err := s.DB.QueryRowContext(ctx,
		`SELECT response_minutes FROM sla_config WHERE priority = $1`, priority).Scan(&minutes)
	if err != nil {
		// SLA config not found for this priority — skip silently
		return
	}

	deadline := time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
	_, _ = s.DB.ExecContext(ctx,
		`UPDATE tickets SET sla_deadline_at = $1 WHERE id = $2`, deadline, ticketID)
}
