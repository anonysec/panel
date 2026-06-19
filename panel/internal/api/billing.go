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

// ========== Promo Codes ==========

type PromoCode struct {
	ID              int64   `json:"id"`
	Code            string  `json:"code"`
	Type            string  `json:"type"` // percent or fixed
	Value           float64 `json:"value"`
	MaxUses         int     `json:"max_uses"`
	UsedCount       int     `json:"used_count"`
	MinAmount       float64 `json:"min_amount"`
	ApplicablePlans string  `json:"applicable_plans,omitempty"`
	StartsAt        string  `json:"starts_at,omitempty"`
	ExpiresAt       string  `json:"expires_at,omitempty"`
	IsActive        bool    `json:"is_active"`
	CreatedBy       string  `json:"created_by"`
	CreatedAt       string  `json:"created_at"`
}

func (s *Server) promoCodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPromoCodes(w)
	case http.MethodPost:
		s.createPromoCode(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listPromoCodes(w http.ResponseWriter) {
	rows, err := s.DB.Query(`SELECT id, code, type, value, max_uses, used_count, min_amount, COALESCE(applicable_plans,''), starts_at, expires_at, is_active, created_by, created_at FROM promo_codes ORDER BY id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	codes := []PromoCode{}
	for rows.Next() {
		var p PromoCode
		var active int
		var starts, expires, created sql.NullTime
		if err := rows.Scan(&p.ID, &p.Code, &p.Type, &p.Value, &p.MaxUses, &p.UsedCount, &p.MinAmount, &p.ApplicablePlans, &starts, &expires, &active, &p.CreatedBy, &created); err != nil {
			continue
		}
		p.IsActive = active == 1
		if starts.Valid {
			p.StartsAt = starts.Time.Format(time.RFC3339)
		}
		if expires.Valid {
			p.ExpiresAt = expires.Time.Format(time.RFC3339)
		}
		if created.Valid {
			p.CreatedAt = created.Time.Format(time.RFC3339)
		}
		codes = append(codes, p)
	}
	writeJSON(w, map[string]any{"ok": true, "promo_codes": codes})
}

func (s *Server) createPromoCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code            string  `json:"code"`
		Type            string  `json:"type"`
		Value           float64 `json:"value"`
		MaxUses         int     `json:"max_uses"`
		MinAmount       float64 `json:"min_amount"`
		ApplicablePlans string  `json:"applicable_plans"`
		StartsAt        string  `json:"starts_at"`
		ExpiresAt       string  `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.Code == "" || len(in.Code) < 3 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "code_required"})
		return
	}
	if in.Type != "percent" && in.Type != "fixed" {
		in.Type = "percent"
	}
	if in.Value <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "value_required"})
		return
	}
	if in.Type == "percent" && in.Value > 100 {
		in.Value = 100
	}

	var startsAt, expiresAt interface{}
	if in.StartsAt != "" {
		startsAt = in.StartsAt
	}
	if in.ExpiresAt != "" {
		expiresAt = in.ExpiresAt
	}

	actor, _, _ := s.currentAdmin(r)
	res, err := s.DB.Exec(`INSERT INTO promo_codes(code, type, value, max_uses, min_amount, applicable_plans, starts_at, expires_at, created_by) VALUES(?,?,?,?,?,?,?,?,?)`,
		in.Code, in.Type, in.Value, in.MaxUses, in.MinAmount, nullString(in.ApplicablePlans), startsAt, expiresAt, actor)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "code_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	s.logAudit(actor, "promo.created", "promo", strconv.FormatInt(id, 10), nil, map[string]any{"code": in.Code}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) promoCodeByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/promo-codes/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if _, err := s.DB.Exec(`DELETE FROM promo_codes WHERE id=?`, id); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case http.MethodPatch:
		var in struct {
			IsActive *bool `json:"is_active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		if in.IsActive != nil {
			active := 0
			if *in.IsActive {
				active = 1
			}
			s.DB.Exec(`UPDATE promo_codes SET is_active=? WHERE id=?`, active, id)
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// portalApplyPromo validates and applies a promo code for a customer.
// Returns the discount amount/percentage that can be applied to their next purchase.
func (s *Server) portalApplyPromo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var in struct {
		Code   string  `json:"code"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.Code == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "code_required"})
		return
	}

	// Look up promo code
	var promo PromoCode
	var active int
	var starts, expires sql.NullTime
	err := s.DB.QueryRow(`SELECT id, code, type, value, max_uses, used_count, min_amount, is_active, starts_at, expires_at FROM promo_codes WHERE code=?`, in.Code).Scan(
		&promo.ID, &promo.Code, &promo.Type, &promo.Value, &promo.MaxUses, &promo.UsedCount, &promo.MinAmount, &active, &starts, &expires)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "invalid_code"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	promo.IsActive = active == 1

	// Validate
	if !promo.IsActive {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "code_inactive"})
		return
	}
	if promo.MaxUses > 0 && promo.UsedCount >= promo.MaxUses {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "code_exhausted"})
		return
	}
	now := time.Now()
	if starts.Valid && now.Before(starts.Time) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "code_not_started"})
		return
	}
	if expires.Valid && now.After(expires.Time) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "code_expired"})
		return
	}
	if in.Amount > 0 && in.Amount < promo.MinAmount {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": fmt.Sprintf("min_amount_%.2f", promo.MinAmount)})
		return
	}

	// Check if user already used this code
	var alreadyUsed int
	s.DB.QueryRow(`SELECT COUNT(*) FROM promo_usage WHERE promo_id=? AND username=?`, promo.ID, username).Scan(&alreadyUsed)
	if alreadyUsed > 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "already_used"})
		return
	}

	// Calculate discount
	var discount float64
	if promo.Type == "percent" {
		discount = in.Amount * (promo.Value / 100)
	} else {
		discount = promo.Value
	}
	if discount > in.Amount {
		discount = in.Amount
	}

	writeJSON(w, map[string]any{
		"ok":              true,
		"code":            promo.Code,
		"type":            promo.Type,
		"value":           promo.Value,
		"discount_amount": discount,
		"final_amount":    in.Amount - discount,
	})
}
