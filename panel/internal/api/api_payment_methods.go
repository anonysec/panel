package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) paymentMethods(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPaymentMethods(w, false)
	case http.MethodPost:
		s.createPaymentMethod(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) paymentMethodByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/payment-methods/")
	if !ok || action != "" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.updatePaymentMethod(w, r, id)
	case http.MethodDelete:
		s.deactivatePaymentMethod(w, r, id)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalPaymentMethods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.listPaymentMethods(w, true)
}

func (s *Server) listPaymentMethods(w http.ResponseWriter, activeOnly bool) {
	var rows *sql.Rows
	var err error
	if activeOnly {
		rows, err = s.DB.Query(`SELECT id,name,type,COALESCE(config_json->>'instructions',''),is_active,sort_order,created_at FROM payment_methods WHERE is_active=TRUE ORDER BY sort_order ASC, id DESC`)
	} else {
		rows, err = s.DB.Query(`SELECT id,name,type,COALESCE(config_json->>'instructions',''),is_active,sort_order,created_at FROM payment_methods ORDER BY is_active DESC, sort_order ASC, id DESC`)
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	methods := []PaymentMethod{}
	for rows.Next() {
		method, err := scanPaymentMethod(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		methods = append(methods, method)
	}
	writeJSON(w, map[string]any{"ok": true, "methods": methods})
}

func (s *Server) createPaymentMethod(w http.ResponseWriter, r *http.Request) {
	var in PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	if in.Type == "" {
		in.Type = "manual"
	}
	res, err := s.DB.Exec(`INSERT INTO payment_methods(name,type,config_json,is_active,sort_order) VALUES($1,$2,JSON_OBJECT('instructions', $3),$4,$5)`, in.Name, in.Type, in.Instructions, boolInt(in.IsActive), in.SortOrder)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment_method.created", "payment_method", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) updatePaymentMethod(w http.ResponseWriter, r *http.Request, id int64) {
	var in PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	if in.Type == "" {
		in.Type = "manual"
	}
	if _, err := s.DB.Exec(`UPDATE payment_methods SET name=$1,type=$2,config_json=JSON_OBJECT('instructions', $3),is_active=$4,sort_order=$5 WHERE id=$6`, in.Name, in.Type, in.Instructions, boolInt(in.IsActive), in.SortOrder, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment_method.updated", "payment_method", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deactivatePaymentMethod(w http.ResponseWriter, r *http.Request, id int64) {
	if _, err := s.DB.Exec(`UPDATE payment_methods SET is_active=0 WHERE id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment_method.deactivated", "payment_method", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

type paymentMethodScanner interface{ Scan(dest ...any) error }

func scanPaymentMethod(row paymentMethodScanner) (PaymentMethod, error) {
	var method PaymentMethod
	var active bool
	var created sql.NullTime
	if err := row.Scan(&method.ID, &method.Name, &method.Type, &method.Instructions, &active, &method.SortOrder, &created); err != nil {
		return method, err
	}
	method.IsActive = active
	if created.Valid {
		method.CreatedAt = created.Time.Format(time.RFC3339)
	}
	return method, nil
}
