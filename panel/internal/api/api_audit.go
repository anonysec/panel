package api

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) logAudit(actor, action, entityType, entityID string, before, after map[string]any, ip string) {
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(after)
	_, _ = s.DB.Exec(`INSERT INTO audit_logs(actor,action,entity_type,entity_id,before_json,after_json,ip) VALUES($1,$2,$3,$4,$5,$6,$7)`, actor, action, entityType, entityID, string(bj), string(aj), ip)
}

func (s *Server) auditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	offset := 0
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	if v, _ := strconv.Atoi(r.URL.Query().Get("offset")); v > 0 {
		offset = v
	}
	rows, err := s.DB.Query(`SELECT id,actor,action,entity_type,entity_id,COALESCE(before_json,''),COALESCE(after_json,''),ip,created_at FROM audit_logs ORDER BY id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type AuditLog struct {
		ID         int64  `json:"id"`
		Actor      string `json:"actor"`
		Action     string `json:"action"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		BeforeJSON string `json:"before_json"`
		AfterJSON  string `json:"after_json"`
		IP         string `json:"ip"`
		CreatedAt  string `json:"created_at"`
	}
	out := []AuditLog{}
	for rows.Next() {
		var a AuditLog
		var before, after []byte
		var created sql.NullTime
		if err := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.EntityType, &a.EntityID, &before, &after, &a.IP, &created); err != nil {
			continue
		}
		a.BeforeJSON = string(before)
		a.AfterJSON = string(after)
		if created.Valid {
			a.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, a)
	}
	writeJSON(w, map[string]any{"ok": true, "logs": out, "limit": limit, "offset": offset})
}

func (s *Server) createEvent(eventType, severity, title, message, actor, related string) {
	_, _ = s.DB.Exec(`INSERT INTO events(type,severity,title,message,actor,related) VALUES($1,$2,$3,$4,$5,$6)`, eventType, severity, title, message, actor, related)
	// Send Telegram notification for warning/error events and key info events
	if severity == "warning" || severity == "error" {
		s.Notify.SendEvent(eventType, title, message)
	} else if eventType == "customer" || eventType == "payment" || eventType == "node" {
		s.Notify.SendEvent(eventType, title, message)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	offset := 0
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	if v, _ := strconv.Atoi(r.URL.Query().Get("offset")); v > 0 {
		offset = v
	}
	where := "1=1"
	args := []any{}
	if seen := r.URL.Query().Get("seen"); seen != "" {
		where += " AND seen=$1"
		args = append(args, seen)
	}
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		where += " AND type=$1"
		args = append(args, eventType)
	}
	query := fmt.Sprintf(`SELECT id,type,severity,title,COALESCE(message,''),COALESCE(actor,''),COALESCE(related,''),seen,notified,created_at FROM events WHERE %s ORDER BY id DESC LIMIT $1 OFFSET $2`, where)
	args = append(args, limit, offset)
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type Event struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		Actor     string `json:"actor"`
		Related   string `json:"related"`
		Seen      bool   `json:"seen"`
		Notified  bool   `json:"notified"`
		CreatedAt string `json:"created_at"`
	}
	out := []Event{}
	for rows.Next() {
		var e Event
		var created sql.NullTime
		var seen, notified int
		if err := rows.Scan(&e.ID, &e.Type, &e.Severity, &e.Title, &e.Message, &e.Actor, &e.Related, &seen, &notified, &created); err != nil {
			continue
		}
		e.Seen = seen == 1
		e.Notified = notified == 1
		if created.Valid {
			e.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, e)
	}
	var unseenCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE seen=0`).Scan(&unseenCount)
	writeJSON(w, map[string]any{"ok": true, "events": out, "unseen_count": unseenCount, "limit": limit, "offset": offset})
}

func (s *Server) eventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	id, action, ok := pathID(r.URL.Path, "/api/events/")
	if !ok || action != "seen" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE events SET seen=1,notified=1 WHERE id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) portalEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	limit := 100
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	rows, err := s.DB.Query(`SELECT id,type,severity,title,COALESCE(message,''),COALESCE(actor,''),COALESCE(related,''),seen,notified,created_at FROM events WHERE related=$1 ORDER BY id DESC LIMIT $2`, username, limit)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type Event struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		Actor     string `json:"actor"`
		Related   string `json:"related"`
		Seen      bool   `json:"seen"`
		Notified  bool   `json:"notified"`
		CreatedAt string `json:"created_at"`
	}
	out := []Event{}
	for rows.Next() {
		var e Event
		var created sql.NullTime
		var seen, notified int
		if err := rows.Scan(&e.ID, &e.Type, &e.Severity, &e.Title, &e.Message, &e.Actor, &e.Related, &seen, &notified, &created); err != nil {
			continue
		}
		e.Seen = seen == 1
		e.Notified = notified == 1
		if created.Valid {
			e.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, e)
	}
	var unseenCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE related=$1 AND seen=0`, username).Scan(&unseenCount)
	writeJSON(w, map[string]any{"ok": true, "events": out, "unseen_count": unseenCount})
}

func (s *Server) portalEventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	id, action, ok := pathID(r.URL.Path, "/api/portal/events/")
	if !ok || action != "seen" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE events SET seen=1,notified=1 WHERE id=$1 AND related=$2`, id, username); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func csvResponse(w http.ResponseWriter, filename string, headers []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(headers)
	for _, row := range rows {
		_ = cw.Write(row)
	}
	cw.Flush()
}

func (s *Server) exportCustomersCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at FROM customers c LEFT JOIN plans p ON p.id=c.plan_id LEFT JOIN wallets w ON w.username=c.username WHERE c.deleted_at IS NULL ORDER BY c.id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id int64
		var username, displayName, status, plan string
		var credit float64
		var created sql.NullTime
		if err := rows.Scan(&id, &username, &displayName, &status, &plan, &credit, &created); err != nil {
			continue
		}
		createdStr := ""
		if created.Valid {
			createdStr = created.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, displayName, status, plan, fmt.Sprintf("%.2f", credit), createdStr})
	}
	csvResponse(w, "customers.csv", []string{"id", "username", "display_name", "status", "plan", "credit", "created_at"}, out)
}

func (s *Server) exportPaymentsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT id,username,amount,method,status,COALESCE(intent_type,'wallet_topup'),intent_id,created_at FROM payments ORDER BY id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id int64
		var username, method, status, intentType string
		var amount float64
		var intentID sql.NullInt64
		var created sql.NullTime
		if err := rows.Scan(&id, &username, &amount, &method, &status, &intentType, &intentID, &created); err != nil {
			continue
		}
		intentIDStr := ""
		if intentID.Valid {
			intentIDStr = strconv.FormatInt(intentID.Int64, 10)
		}
		createdStr := ""
		if created.Valid {
			createdStr = created.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, fmt.Sprintf("%.2f", amount), method, status, intentType, intentIDStr, createdStr})
	}
	csvResponse(w, "payments.csv", []string{"id", "username", "amount", "method", "status", "intent_type", "intent_id", "created_at"}, out)
}

func (s *Server) exportRadacctCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT radacctid,username,acctstarttime,acctstoptime,COALESCE(acctsessiontime,0),COALESCE(acctinputoctets,0),COALESCE(acctoutputoctets,0),framedipaddress,acctterminatecause FROM radacct ORDER BY radacctid DESC LIMIT 10000`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id, sessionTime, inputBytes, outputBytes int64
		var username, framedIP, terminateCause string
		var start, stop sql.NullTime
		if err := rows.Scan(&id, &username, &start, &stop, &sessionTime, &inputBytes, &outputBytes, &framedIP, &terminateCause); err != nil {
			continue
		}
		startStr, stopStr := "", ""
		if start.Valid {
			startStr = start.Time.Format(time.RFC3339)
		}
		if stop.Valid {
			stopStr = stop.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, startStr, stopStr, strconv.FormatInt(sessionTime, 10), strconv.FormatInt(inputBytes, 10), strconv.FormatInt(outputBytes, 10), framedIP, terminateCause})
	}
	csvResponse(w, "radacct.csv", []string{"id", "username", "start_time", "stop_time", "session_seconds", "input_bytes", "output_bytes", "framed_ip", "terminate_cause"}, out)
}

func (s *Server) exportWalletTransactionsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT id,username,amount,type,description,actor,COALESCE(reference_type,''),reference_id,created_at FROM wallet_transactions ORDER BY id DESC LIMIT 10000`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id int64
		var amount float64
		var username, ttype, description, actor, refType string
		var refID sql.NullInt64
		var created sql.NullTime
		if err := rows.Scan(&id, &username, &amount, &ttype, &description, &actor, &refType, &refID, &created); err != nil {
			continue
		}
		refIDStr := ""
		if refID.Valid {
			refIDStr = strconv.FormatInt(refID.Int64, 10)
		}
		createdStr := ""
		if created.Valid {
			createdStr = created.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, fmt.Sprintf("%.2f", amount), ttype, description, actor, refType, refIDStr, createdStr})
	}
	csvResponse(w, "wallet-transactions.csv", []string{"id", "username", "amount", "type", "description", "actor", "reference_type", "reference_id", "created_at"}, out)
}

// ─── Database Backup Export/Import ───────────────────────────────────────────
