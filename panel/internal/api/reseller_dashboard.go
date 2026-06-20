package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── Reseller Dashboard Stats ───────────────────────────────────────────────

func (s *Server) resellerDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	var credit float64
	_ = s.DB.QueryRow(`SELECT COALESCE(credit, 0) FROM admins WHERE username=?`, actor).Scan(&credit)

	totalUsers := s.count(`SELECT COUNT(*) FROM customers WHERE created_by=? AND deleted_at IS NULL`, actor)
	activeUsers := s.count(`SELECT COUNT(*) FROM customers WHERE created_by=? AND deleted_at IS NULL AND status='active'`, actor)

	var totalUsageBytes int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(ra.acctinputoctets + ra.acctoutputoctets), 0) FROM radacct ra INNER JOIN customers c ON c.username = ra.username WHERE c.created_by = ? AND c.deleted_at IS NULL`, actor).Scan(&totalUsageBytes)

	writeJSON(w, map[string]any{
		"ok":                true,
		"credit":            credit,
		"total_users":       totalUsers,
		"active_users":      activeUsers,
		"total_usage_bytes": totalUsageBytes,
	})
}

// ─── Reseller Plan Prices ───────────────────────────────────────────────────

func (s *Server) resellerPlanPrices(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listResellerPlanPrices(w, actor)
	case http.MethodPost:
		s.setResellerPlanPrice(w, r, actor)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listResellerPlanPrices(w http.ResponseWriter, actor string) {
	var resellerID int64
	if err := s.DB.QueryRow(`SELECT id FROM admins WHERE username=?`, actor).Scan(&resellerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	rows, err := s.DB.Query(`
		SELECT p.id, p.name, p.data_gb, p.speed_mbps, p.duration_days, p.price,
			COALESCE(rp.sell_price, 0)
		FROM plans p
		INNER JOIN reseller_allowed_plans rap ON rap.plan_id = p.id AND rap.reseller_id = ?
		LEFT JOIN reseller_plan_prices rp ON rp.plan_id = p.id AND rp.reseller_id = ?
		WHERE p.is_active = 1 AND COALESCE(p.billing_type, 'quota') != 'payg'
		ORDER BY p.sort_order ASC, p.id DESC`, resellerID, resellerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type PlanPrice struct {
		ID             int64   `json:"id"`
		Name           string  `json:"name"`
		DataGB         float64 `json:"data_gb"`
		SpeedMbps      float64 `json:"speed_mbps"`
		DurationDays   int     `json:"duration_days"`
		WholesalePrice float64 `json:"wholesale_price"`
		SellPrice      float64 `json:"sell_price"`
	}

	plans := []PlanPrice{}
	for rows.Next() {
		var p PlanPrice
		if err := rows.Scan(&p.ID, &p.Name, &p.DataGB, &p.SpeedMbps, &p.DurationDays, &p.WholesalePrice, &p.SellPrice); err == nil {
			plans = append(plans, p)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "plans": plans})
}

func (s *Server) setResellerPlanPrice(w http.ResponseWriter, r *http.Request, actor string) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		PlanID    int64   `json:"plan_id"`
		SellPrice float64 `json:"sell_price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.PlanID <= 0 || in.SellPrice < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_input"})
		return
	}

	var resellerID int64
	if err := s.DB.QueryRow(`SELECT id FROM admins WHERE username=?`, actor).Scan(&resellerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	_, err := s.DB.Exec(`
		INSERT INTO reseller_plan_prices (reseller_id, plan_id, sell_price)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE sell_price = VALUES(sell_price), updated_at = NOW()`,
		resellerID, in.PlanID, in.SellPrice)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ─── Reseller Tickets ───────────────────────────────────────────────────────

func (s *Server) resellerTickets(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	// Route: /api/reseller/tickets/:id or /api/reseller/tickets/:id/reply
	path := r.URL.Path
	trimmed := strings.TrimPrefix(path, "/api/reseller/tickets")
	trimmed = strings.TrimPrefix(trimmed, "/")

	if trimmed == "" {
		// /api/reseller/tickets
		switch r.Method {
		case http.MethodGet:
			s.listResellerTickets(w, actor)
		case http.MethodPost:
			s.createResellerTicket(w, r, actor)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
		return
	}

	// /api/reseller/tickets/:id or /api/reseller/tickets/:id/reply
	parts := strings.SplitN(trimmed, "/", 2)
	ticketID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	if len(parts) == 2 && parts[1] == "reply" {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.replyResellerTicket(w, r, actor, ticketID)
		return
	}

	if len(parts) == 2 && parts[1] == "close" {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.closeResellerTicket(w, actor, ticketID)
		return
	}

	// GET /api/reseller/tickets/:id
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.getResellerTicket(w, actor, ticketID)
}

func (s *Server) listResellerTickets(w http.ResponseWriter, actor string) {
	rows, err := s.DB.Query(`
		SELECT id, subject, status, created_at, updated_at
		FROM reseller_tickets
		WHERE reseller_username = ?
		ORDER BY id DESC LIMIT 200`, actor)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type TicketItem struct {
		ID        int64  `json:"id"`
		Subject   string `json:"subject"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	tickets := []TicketItem{}
	for rows.Next() {
		var t TicketItem
		var created, updated time.Time
		if err := rows.Scan(&t.ID, &t.Subject, &t.Status, &created, &updated); err == nil {
			t.CreatedAt = created.Format(time.RFC3339)
			t.UpdatedAt = updated.Format(time.RFC3339)
			tickets = append(tickets, t)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "tickets": tickets})
}

func (s *Server) createResellerTicket(w http.ResponseWriter, r *http.Request, actor string) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Subject string `json:"subject"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Subject = strings.TrimSpace(in.Subject)
	in.Message = strings.TrimSpace(in.Message)
	if in.Subject == "" || in.Message == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "subject_message_required"})
		return
	}

	res, err := s.DB.Exec(`INSERT INTO reseller_tickets (reseller_username, subject) VALUES (?, ?)`, actor, in.Subject)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ticketID, _ := res.LastInsertId()

	_, err = s.DB.Exec(`INSERT INTO reseller_ticket_messages (ticket_id, sender, message) VALUES (?, ?, ?)`, ticketID, actor, in.Message)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s.createEvent("reseller", "info", fmt.Sprintf("Reseller ticket: %s", in.Subject), fmt.Sprintf("Reseller %s created ticket #%d", actor, ticketID), actor, actor)
	writeJSON(w, map[string]any{"ok": true, "id": ticketID})
}

func (s *Server) getResellerTicket(w http.ResponseWriter, actor string, ticketID int64) {
	type Msg struct {
		ID        int64  `json:"id"`
		Sender    string `json:"sender"`
		Message   string `json:"message"`
		CreatedAt string `json:"created_at"`
	}

	var subject, status string
	var created, updated time.Time
	err := s.DB.QueryRow(`SELECT subject, status, created_at, updated_at FROM reseller_tickets WHERE id=? AND reseller_username=?`, ticketID, actor).Scan(&subject, &status, &created, &updated)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	rows, err := s.DB.Query(`SELECT id, sender, message, created_at FROM reseller_ticket_messages WHERE ticket_id=? ORDER BY id ASC`, ticketID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	messages := []Msg{}
	for rows.Next() {
		var m Msg
		var t time.Time
		if err := rows.Scan(&m.ID, &m.Sender, &m.Message, &t); err == nil {
			m.CreatedAt = t.Format(time.RFC3339)
			messages = append(messages, m)
		}
	}

	writeJSON(w, map[string]any{
		"ok":         true,
		"id":         ticketID,
		"subject":    subject,
		"status":     status,
		"created_at": created.Format(time.RFC3339),
		"updated_at": updated.Format(time.RFC3339),
		"messages":   messages,
	})
}

func (s *Server) replyResellerTicket(w http.ResponseWriter, r *http.Request, actor string, ticketID int64) {
	limitBody(w, r, maxJSONBody)
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

	// Verify ticket belongs to this reseller
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM reseller_tickets WHERE id=? AND reseller_username=?`, ticketID, actor).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	_, err := s.DB.Exec(`INSERT INTO reseller_ticket_messages (ticket_id, sender, message) VALUES (?, ?, ?)`, ticketID, actor, in.Message)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Reopen ticket if it was closed
	_, _ = s.DB.Exec(`UPDATE reseller_tickets SET status='open', updated_at=NOW() WHERE id=?`, ticketID)

	writeJSON(w, map[string]any{"ok": true})
}

// ─── Reseller Close Ticket ──────────────────────────────────────────────────

func (s *Server) closeResellerTicket(w http.ResponseWriter, actor string, ticketID int64) {
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM reseller_tickets WHERE id=? AND reseller_username=?`, ticketID, actor).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	_, _ = s.DB.Exec(`UPDATE reseller_tickets SET status='closed', updated_at=NOW() WHERE id=?`, ticketID)
	writeJSON(w, map[string]any{"ok": true})
}
