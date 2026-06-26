//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// handleResellerPayouts handles GET /api/reseller/payouts (list own payout history)
// and POST /api/reseller/payouts (request a payout).
func (s *Server) handleResellerPayouts(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listResellerPayouts(w, actor)
	case http.MethodPost:
		s.createResellerPayout(w, r, actor)
	default:
		writeJSONCode(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
	}
}

// listResellerPayouts returns payout history, current balance, and min payout for a reseller.
func (s *Server) listResellerPayouts(w http.ResponseWriter, username string) {
	// Get current balance and min payout amount
	var balance float64
	var minPayout float64
	err := s.DB.QueryRow(`SELECT COALESCE(payout_balance, 0), COALESCE(min_payout_amount, 0) FROM admins WHERE username=$1`, username).Scan(&balance, &minPayout)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	rows, err := s.DB.Query(`SELECT id, amount, status, COALESCE(payment_details,''), COALESCE(admin_note,''), requested_at, processed_at, COALESCE(processed_by,'') FROM reseller_payouts WHERE reseller_username=$1 ORDER BY requested_at DESC`, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type Payout struct {
		ID             int64   `json:"id"`
		Amount         float64 `json:"amount"`
		Status         string  `json:"status"`
		PaymentDetails string  `json:"payment_details"`
		AdminNote      string  `json:"admin_note,omitempty"`
		RequestedAt    string  `json:"requested_at"`
		ProcessedAt    *string `json:"processed_at,omitempty"`
		ProcessedBy    string  `json:"processed_by,omitempty"`
	}

	payouts := []Payout{}
	for rows.Next() {
		var p Payout
		var requestedAt time.Time
		var processedAt sql.NullTime
		var processedBy string
		if err := rows.Scan(&p.ID, &p.Amount, &p.Status, &p.PaymentDetails, &p.AdminNote, &requestedAt, &processedAt, &processedBy); err != nil {
			continue
		}
		p.RequestedAt = requestedAt.Format(time.RFC3339)
		if processedAt.Valid {
			t := processedAt.Time.Format(time.RFC3339)
			p.ProcessedAt = &t
		}
		p.ProcessedBy = processedBy
		payouts = append(payouts, p)
	}

	writeJSON(w, map[string]any{
		"ok":         true,
		"payouts":    payouts,
		"balance":    balance,
		"min_payout": minPayout,
	})
}

// createResellerPayout handles a reseller requesting a payout.
func (s *Server) createResellerPayout(w http.ResponseWriter, r *http.Request, username string) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Amount         float64 `json:"amount"`
		PaymentDetails string  `json:"payment_details"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.Amount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
		return
	}

	// Get current balance and min payout amount
	var balance float64
	var minPayout float64
	err := s.DB.QueryRow(`SELECT COALESCE(payout_balance, 0), COALESCE(min_payout_amount, 0) FROM admins WHERE username=$1`, username).Scan(&balance, &minPayout)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if in.Amount > balance {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_balance"})
		return
	}

	if minPayout > 0 && in.Amount < minPayout {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "below_minimum"})
		return
	}

	res, err := s.DB.Exec(`INSERT INTO reseller_payouts (reseller_username, amount, status, payment_details) VALUES ($1, $2, 'pending', $3)`, username, in.Amount, in.PaymentDetails)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	id, _ := res.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// handleAdminPayouts handles GET /api/admin/payouts (list all payout requests).
func (s *Server) handleAdminPayouts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONCode(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
		return
	}

	statusFilter := r.URL.Query().Get("status")

	query := `SELECT id, reseller_username, amount, status, COALESCE(payment_details,''), COALESCE(admin_note,''), requested_at, processed_at, COALESCE(processed_by,'') FROM reseller_payouts`
	args := []any{}

	if statusFilter != "" {
		query += ` WHERE status=$1`
		args = append(args, statusFilter)
	}
	query += ` ORDER BY requested_at DESC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type Payout struct {
		ID               int64   `json:"id"`
		ResellerUsername string  `json:"reseller_username"`
		Amount           float64 `json:"amount"`
		Status           string  `json:"status"`
		PaymentDetails   string  `json:"payment_details"`
		AdminNote        string  `json:"admin_note,omitempty"`
		RequestedAt      string  `json:"requested_at"`
		ProcessedAt      *string `json:"processed_at,omitempty"`
		ProcessedBy      string  `json:"processed_by,omitempty"`
	}

	payouts := []Payout{}
	for rows.Next() {
		var p Payout
		var requestedAt time.Time
		var processedAt sql.NullTime
		var processedBy string
		if err := rows.Scan(&p.ID, &p.ResellerUsername, &p.Amount, &p.Status, &p.PaymentDetails, &p.AdminNote, &requestedAt, &processedAt, &processedBy); err != nil {
			continue
		}
		p.RequestedAt = requestedAt.Format(time.RFC3339)
		if processedAt.Valid {
			t := processedAt.Time.Format(time.RFC3339)
			p.ProcessedAt = &t
		}
		p.ProcessedBy = processedBy
		payouts = append(payouts, p)
	}

	writeJSON(w, map[string]any{"ok": true, "payouts": payouts})
}

// handleAdminPayoutByID handles PATCH /api/admin/payouts/{id} (approve/reject a payout).
func (s *Server) handleAdminPayoutByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSONCode(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
		return
	}

	id, _, ok := pathID(r.URL.Path, "/api/admin/payouts/")
	if !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	admin, _, _ := s.currentAdmin(r)

	limitBody(w, r, maxJSONBody)

	var in struct {
		Action    string `json:"action"`
		AdminNote string `json:"admin_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.Action != "approve" && in.Action != "reject" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_action"})
		return
	}

	// Fetch the payout record
	var resellerUsername string
	var amount float64
	var status string
	err := s.DB.QueryRow(`SELECT reseller_username, amount, status FROM reseller_payouts WHERE id=$1`, id).Scan(&resellerUsername, &amount, &status)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if status != "pending" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "already_processed"})
		return
	}

	if in.Action == "approve" {
		// Verify reseller has sufficient balance
		var currentBalance float64
		err := s.DB.QueryRow(`SELECT COALESCE(payout_balance, 0) FROM admins WHERE username=$1`, resellerUsername).Scan(&currentBalance)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		if amount > currentBalance {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_balance"})
			return
		}

		// Begin transaction: update payout status and deduct balance
		tx, err := s.DB.Begin()
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec(`UPDATE reseller_payouts SET status='approved', processed_at=NOW(), processed_by=$1, admin_note=$2 WHERE id=$3`, admin, in.AdminNote, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}

		_, err = tx.Exec(`UPDATE admins SET payout_balance = payout_balance - $1 WHERE username=$2`, amount, resellerUsername)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}

		if err := tx.Commit(); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
	} else {
		// Reject
		_, err := s.DB.Exec(`UPDATE reseller_payouts SET status='rejected', processed_at=NOW(), processed_by=$1, admin_note=$2 WHERE id=$3`, admin, in.AdminNote, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}
