//go:build !lite

package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// customerBillingDebt returns the customer's debt status (negative wallet balance).
// GET /api/customer/billing/debt
func (s *Server) customerBillingDebt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Look up customer ID from username
	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	if s.Billing == nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "billing_unavailable"})
		return
	}

	debt, err := s.Billing.GetDebtInfo(r.Context(), customerID)
	if err != nil {
		log.Printf("[billing] debt check failed: customer=%s err=%v", username, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "debt_check_failed"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "debt": debt})
}

// customerDataPacks lists all active data packs available for purchase.
func (s *Server) customerDataPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.DB.Query(`SELECT id, name, data_gb, price, currency FROM data_packs WHERE is_active = 1 ORDER BY price ASC`)
	if err != nil {
		log.Printf("[billing] list data packs error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type DataPackResponse struct {
		ID       int64   `json:"id"`
		Name     string  `json:"name"`
		DataGB   int     `json:"data_gb"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
	}

	packs := []DataPackResponse{}
	for rows.Next() {
		var p DataPackResponse
		if err := rows.Scan(&p.ID, &p.Name, &p.DataGB, &p.Price, &p.Currency); err != nil {
			continue
		}
		packs = append(packs, p)
	}

	writeJSON(w, map[string]any{"ok": true, "data_packs": packs})
}

// customerBuyDataPack processes a data pack purchase for the logged-in customer.
func (s *Server) customerBuyDataPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		PackID int64 `json:"pack_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.PackID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "pack_id_required"})
		return
	}

	// Look up customer ID from username
	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	if s.Billing == nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "billing_unavailable"})
		return
	}

	if err := s.Billing.PurchaseDataPack(r.Context(), customerID, in.PackID); err != nil {
		log.Printf("[billing] data pack purchase failed: customer=%s pack=%d err=%v", username, in.PackID, err)
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "message": "data_pack_purchased"})
}

// adminUpgradePlan allows an admin to trigger a pro-rated plan upgrade for a customer.
func (s *Server) adminUpgradePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		CustomerID int64 `json:"customer_id"`
		NewPlanID  int64 `json:"new_plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.CustomerID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_id_required"})
		return
	}
	if in.NewPlanID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "new_plan_id_required"})
		return
	}

	if s.Billing == nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "billing_unavailable"})
		return
	}

	if err := s.Billing.UpgradePlan(r.Context(), in.CustomerID, in.NewPlanID); err != nil {
		log.Printf("[billing] plan upgrade failed: customer=%d plan=%d err=%v", in.CustomerID, in.NewPlanID, err)
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "billing.upgrade", "customer", "", nil, map[string]any{
		"customer_id": in.CustomerID,
		"new_plan_id": in.NewPlanID,
	}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "message": "plan_upgraded"})
}
