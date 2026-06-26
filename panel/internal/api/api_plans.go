package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) plans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPlans(w, r)
	case http.MethodPost:
		s.createPlan(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) planByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/plans/")
	if !ok || action != "" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getPlan(w, id)
	case http.MethodPatch:
		s.updatePlan(w, r, id)
	case http.MethodDelete:
		s.archivePlan(w, r, id)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	result, err := s.cachedQuery("plans:list", func() (any, error) {
		rows, err := s.DB.Query(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans ORDER BY is_active DESC, sort_order ASC, id DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		plans := []Plan{}
		for rows.Next() {
			plan, err := scanPlan(rows)
			if err != nil {
				return nil, err
			}
			plans = append(plans, plan)
		}
		return map[string]any{"ok": true, "plans": plans}, nil
	})
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	var in Plan
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.DataGB < 0 || in.SpeedMbps < 0 || in.DurationDays < 0 || in.Price < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	if in.BillingType == "" {
		in.BillingType = "quota"
	}
	if in.BillingType != "quota" && in.BillingType != "payg" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_billing_type"})
		return
	}
	if in.PricePerGB < 0 || in.PricePerDay < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	res, err := s.DB.Exec(`INSERT INTO plans(name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		in.Name, in.DataGB, in.SpeedMbps, in.DurationDays, in.Price, in.BillingType, in.PricePerGB, in.PricePerDay, in.DisconnectOnZero, in.AllowPasswordless, in.IsActive, in.SortOrder)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("plans:")
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "plan.created", "plan", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name, "billing_type": in.BillingType}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) getPlan(w http.ResponseWriter, id int64) {
	row := s.DB.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans WHERE id=$1 LIMIT 1`, id)
	plan, err := scanPlan(row)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "plan": plan})
}

func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request, id int64) {
	var in Plan
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.DataGB < 0 || in.SpeedMbps < 0 || in.DurationDays < 0 || in.Price < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	if in.BillingType == "" {
		in.BillingType = "quota"
	}
	if in.BillingType != "quota" && in.BillingType != "payg" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_billing_type"})
		return
	}
	if in.PricePerGB < 0 || in.PricePerDay < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE plans SET name=$1,data_gb=$2,speed_mbps=$3,duration_days=$4,price=$5,billing_type=$6,price_per_gb=$7,price_per_day=$8,disconnect_on_zero=$9,allow_passwordless=$10,is_active=$11,sort_order=$12 WHERE id=$13`,
		in.Name, in.DataGB, in.SpeedMbps, in.DurationDays, in.Price, in.BillingType, in.PricePerGB, in.PricePerDay, in.DisconnectOnZero, in.AllowPasswordless, in.IsActive, in.SortOrder, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("plans:")
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "plan.updated", "plan", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name, "billing_type": in.BillingType}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) archivePlan(w http.ResponseWriter, r *http.Request, id int64) {
	if _, err := s.DB.Exec(`UPDATE plans SET is_active=0 WHERE id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("plans:")
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "plan.deactivated", "plan", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) customerUsername(id int64) (string, error) {
	var username string
	err := s.DB.QueryRow(`SELECT username FROM customers WHERE id=$1 AND deleted_at IS NULL LIMIT 1`, id).Scan(&username)
	return username, err
}

func (s *Server) upsertRadCheck(username, attribute, value string) error {
	res, err := s.DB.Exec(`UPDATE radcheck SET value=$1 WHERE username=$2 AND attribute=$3`, value, username, attribute)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES($1,$2,':=',$3)`, username, attribute, value)
	return err
}

func (s *Server) upsertRadReply(username, attribute, value string) error {
	res, err := s.DB.Exec(`UPDATE radreply SET value=$1 WHERE username=$2 AND attribute=$3`, value, username, attribute)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = s.DB.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES($1,$2,':=',$3)`, username, attribute, value)
	return err
}

func speedLimitValue(mbps float64) string {
	if math.Abs(mbps-math.Round(mbps)) < 0.001 {
		v := strconv.FormatInt(int64(math.Round(mbps)), 10) + "M"
		return v + "/" + v
	}
	v := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", mbps), "0"), ".") + "M"
	return v + "/" + v
}

func validCustomerStatus(status string) bool {
	switch status {
	case "active", "disabled", "expired", "limited":
		return true
	default:
		return false
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type planScanner interface {
	Scan(dest ...any) error
}

func scanPlan(row planScanner) (Plan, error) {
	var p Plan
	var active bool
	var disconnectOnZero bool
	var allowPasswordless bool
	var created sql.NullTime
	var billingType sql.NullString
	if err := row.Scan(&p.ID, &p.Name, &p.DataGB, &p.SpeedMbps, &p.DurationDays, &p.Price, &billingType, &p.PricePerGB, &p.PricePerDay, &disconnectOnZero, &allowPasswordless, &active, &p.SortOrder, &created); err != nil {
		return p, err
	}
	p.IsActive = active
	p.DisconnectOnZero = disconnectOnZero
	p.AllowPasswordless = allowPasswordless
	if billingType.Valid {
		p.BillingType = billingType.String
	} else {
		p.BillingType = "quota"
	}
	if created.Valid {
		p.CreatedAt = created.Time.Format(time.RFC3339)
	}
	return p, nil
}

func pathID(urlPath, prefix string) (int64, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(urlPath, prefix), "/")
	if rest == "" || strings.HasPrefix(rest, "../") {
		return 0, "", false
	}
	parts := strings.Split(rest, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	return id, action, true
}
