package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// adminCustomerNotes handles GET (list) and POST (create) for per-customer admin notes.
// Route: /api/admin/customers/{id}/notes
func (s *Server) adminCustomerNotes(w http.ResponseWriter, r *http.Request, customerID int64) {
	switch r.Method {
	case http.MethodGet:
		s.listCustomerNotes(w, r, customerID)
	case http.MethodPost:
		s.createCustomerNote(w, r, customerID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCustomerNotes(w http.ResponseWriter, _ *http.Request, customerID int64) {
	// Verify customer exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM customers WHERE id=$1 AND deleted_at IS NULL`, customerID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	rows, err := s.DB.Query(`SELECT id, admin_username, body, created_at FROM user_notes WHERE customer_id=$1 ORDER BY created_at DESC`, customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type Note struct {
		ID            int64  `json:"id"`
		AdminUsername string `json:"admin_username"`
		Body          string `json:"body"`
		CreatedAt     string `json:"created_at"`
	}

	notes := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.AdminUsername, &n.Body, &n.CreatedAt); err != nil {
			continue
		}
		notes = append(notes, n)
	}
	writeJSON(w, map[string]any{"ok": true, "notes": notes})
}

func (s *Server) createCustomerNote(w http.ResponseWriter, r *http.Request, customerID int64) {
	limitBody(w, r, maxJSONBody)
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

	// Verify customer exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM customers WHERE id=$1 AND deleted_at IS NULL`, customerID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	actor, _, _ := s.currentAdmin(r)

	res, err := s.DB.Exec(`INSERT INTO user_notes(customer_id, admin_username, body) VALUES($1,$2,$3)`,
		customerID, actor, in.Body)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}
