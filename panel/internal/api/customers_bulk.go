package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

// CustomerBulkRequest represents a request to perform bulk operations on customers.
type CustomerBulkRequest struct {
	Action      string         `json:"action"`
	CustomerIDs []int64        `json:"customer_ids"`
	Params      map[string]any `json:"params"`
}

// CustomerBulkError represents a single per-customer error in a bulk operation.
type CustomerBulkError struct {
	ID    int64  `json:"id"`
	Error string `json:"error"`
}

// validCustomerBulkActions defines the set of supported bulk actions.
var validCustomerBulkActions = map[string]bool{
	"extend":      true,
	"change_plan": true,
	"add_data":    true,
	"disable":     true,
	"enable":      true,
	"delete":      true,
	"assign_tag":  true,
}

// adminCustomersBulk handles POST /api/admin/customers/bulk
func (s *Server) adminCustomersBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var req CustomerBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate action
	if !validCustomerBulkActions[req.Action] {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_action"})
		return
	}

	// Validate customer_ids
	if len(req.CustomerIDs) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_ids_required"})
		return
	}
	if len(req.CustomerIDs) > 100 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "too_many_customers"})
		return
	}

	// Validate params for actions that require them
	if err := s.validateCustomerBulkParams(req.Action, req.Params); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	var affected int
	errors := make([]CustomerBulkError, 0)

	for _, customerID := range req.CustomerIDs {
		err := s.executeCustomerBulkAction(customerID, req.Action, req.Params, actor, ip)
		if err != nil {
			errors = append(errors, CustomerBulkError{
				ID:    customerID,
				Error: err.Error(),
			})
		} else {
			affected++
		}
	}

	// Log audit trail
	s.logAudit(actor, "customers.bulk_"+req.Action, "customer", "", nil, map[string]any{
		"action":       req.Action,
		"customer_ids": req.CustomerIDs,
		"params":       req.Params,
		"count":        len(req.CustomerIDs),
		"succeeded":    affected,
		"failed":       len(errors),
	}, ip)

	log.Printf("[customers] bulk action=%s customers=%d affected=%d errors=%d by=%s",
		req.Action, len(req.CustomerIDs), affected, len(errors), actor)

	writeJSON(w, map[string]any{
		"ok":        true,
		"total":     len(req.CustomerIDs),
		"affected":  affected,
		"succeeded": affected,
		"failed":    len(errors),
		"failures":  errors,
	})
}

// validateCustomerBulkParams validates params for actions that require them.
func (s *Server) validateCustomerBulkParams(action string, params map[string]any) error {
	switch action {
	case "extend":
		days, ok := params["days"]
		if !ok {
			return fmt.Errorf("params_days_required")
		}
		d, valid := toPositiveInt(days)
		if !valid || d <= 0 {
			return fmt.Errorf("params_days_invalid")
		}
	case "change_plan":
		planID, ok := params["plan_id"]
		if !ok {
			return fmt.Errorf("params_plan_id_required")
		}
		p, valid := toPositiveInt(planID)
		if !valid || p <= 0 {
			return fmt.Errorf("params_plan_id_invalid")
		}
	case "add_data":
		dataGB, ok := params["data_gb"]
		if !ok {
			return fmt.Errorf("params_data_gb_required")
		}
		d, valid := toPositiveInt(dataGB)
		if !valid || d <= 0 {
			return fmt.Errorf("params_data_gb_invalid")
		}
	case "assign_tag":
		tagID, ok := params["tag_id"]
		if !ok {
			return fmt.Errorf("params_tag_id_required")
		}
		t, valid := toPositiveInt(tagID)
		if !valid || t <= 0 {
			return fmt.Errorf("params_tag_id_invalid")
		}
	}
	return nil
}

// executeCustomerBulkAction performs a single bulk action on one customer.
func (s *Server) executeCustomerBulkAction(customerID int64, action string, params map[string]any, actor, ip string) error {
	switch action {
	case "extend":
		return s.customerBulkExtend(customerID, params, actor, ip)
	case "change_plan":
		return s.customerBulkChangePlan(customerID, params, actor, ip)
	case "add_data":
		return s.customerBulkAddData(customerID, params, actor, ip)
	case "disable":
		return s.customerBulkDisable(customerID, actor, ip)
	case "enable":
		return s.customerBulkEnable(customerID, actor, ip)
	case "delete":
		return s.customerBulkDelete(customerID, actor, ip)
	case "assign_tag":
		return s.customerBulkAssignTag(customerID, params, actor, ip)
	default:
		return fmt.Errorf("unsupported action")
	}
}

// customerBulkExtend extends a customer's subscription by N days.
func (s *Server) customerBulkExtend(customerID int64, params map[string]any, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	days, _ := toPositiveInt(params["days"])

	// Extend the active subscription's expires_at by N days
	res, err := s.DB.Exec(
		`UPDATE subscriptions SET expires_at = expires_at + INTERVAL '1 day' * $1 WHERE customer_id=$2 AND status='active' ORDER BY id DESC LIMIT 1`,
		days, customerID,
	)
	if err != nil {
		return fmt.Errorf("failed to extend subscription: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no active subscription found")
	}

	s.logAudit(actor, "customer.subscription_extended", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "days": days, "bulk": true,
	}, ip)
	return nil
}

// customerBulkChangePlan changes a customer's plan.
func (s *Server) customerBulkChangePlan(customerID int64, params map[string]any, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	planID, _ := toPositiveInt(params["plan_id"])

	// Verify plan exists
	var planExists int
	err = s.DB.QueryRow(`SELECT 1 FROM plans WHERE id=$1 AND is_active=TRUE LIMIT 1`, planID).Scan(&planExists)
	if err != nil {
		return fmt.Errorf("plan not found or inactive")
	}

	// Update customer's plan_id
	_, err = s.DB.Exec(`UPDATE customers SET plan_id=$1 WHERE id=$2 AND deleted_at IS NULL`, planID, customerID)
	if err != nil {
		return fmt.Errorf("failed to change plan: %v", err)
	}

	// Update active subscription's plan reference
	_, _ = s.DB.Exec(`UPDATE subscriptions SET plan_id=$1 WHERE customer_id=$2 AND status='active' ORDER BY id DESC LIMIT 1`, planID, customerID)

	s.logAudit(actor, "customer.plan_changed", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "plan_id": planID, "bulk": true,
	}, ip)
	return nil
}

// customerBulkAddData adds data allowance (in GB) to a customer's subscription.
func (s *Server) customerBulkAddData(customerID int64, params map[string]any, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	dataGB, _ := toPositiveInt(params["data_gb"])

	// Add data to the customer's radreply Max-Octets or update subscription data limit
	// Use radreply to increase the data allowance by data_gb (converted to bytes)
	dataBytes := int64(dataGB) * 1024 * 1024 * 1024

	// Increase the customer's data allowance in subscriptions table
	res, err := s.DB.Exec(
		`UPDATE subscriptions SET data_limit_bytes = data_limit_bytes + $1 WHERE customer_id=$2 AND status='active' ORDER BY id DESC LIMIT 1`,
		dataBytes, customerID,
	)
	if err != nil {
		return fmt.Errorf("failed to add data: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no active subscription found")
	}

	s.logAudit(actor, "customer.data_added", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "data_gb": dataGB, "bulk": true,
	}, ip)
	return nil
}

// customerBulkDisable sets a customer's status to 'disabled'.
func (s *Server) customerBulkDisable(customerID int64, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(`UPDATE customers SET status='disabled' WHERE id=$1 AND deleted_at IS NULL`, customerID)
	if err != nil {
		return fmt.Errorf("failed to disable customer: %v", err)
	}

	s.disconnectCustomerSessions(username)

	s.logAudit(actor, "customer.status_changed", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "status": "disabled", "bulk": true,
	}, ip)
	return nil
}

// customerBulkEnable sets a customer's status to 'active'.
func (s *Server) customerBulkEnable(customerID int64, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(`UPDATE customers SET status='active' WHERE id=$1 AND deleted_at IS NULL`, customerID)
	if err != nil {
		return fmt.Errorf("failed to enable customer: %v", err)
	}

	s.logAudit(actor, "customer.status_changed", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "status": "active", "bulk": true,
	}, ip)
	return nil
}

// customerBulkDelete soft-deletes a customer (sets deleted_at).
func (s *Server) customerBulkDelete(customerID int64, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	res, err := s.DB.Exec(`UPDATE customers SET deleted_at=NOW(), status='deleted' WHERE id=$1 AND deleted_at IS NULL`, customerID)
	if err != nil {
		return fmt.Errorf("failed to delete customer: %v", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("customer already deleted")
	}

	s.disconnectCustomerSessions(username)

	s.logAudit(actor, "customer.deleted", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "bulk": true,
	}, ip)
	return nil
}

// toPositiveInt converts a JSON number (float64) or string to a positive int.
// Returns the value and true if valid, or 0 and false if invalid.
func toPositiveInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n <= 0 || n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil || i <= 0 {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(n)
		if err != nil || i <= 0 {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// customerBulkAssignTag assigns a tag to a customer (INSERT IGNORE for idempotency).
func (s *Server) customerBulkAssignTag(customerID int64, params map[string]any, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	tagID, _ := toPositiveInt(params["tag_id"])

	// Verify tag exists
	var tagExists int
	err = s.DB.QueryRow(`SELECT 1 FROM user_tags WHERE id=$1 LIMIT 1`, tagID).Scan(&tagExists)
	if err != nil {
		return fmt.Errorf("tag not found")
	}

	// INSERT IGNORE for idempotency — no error if already assigned
	_, err = s.DB.Exec(`INSERT INTO customer_tags (customer_id, tag_id) VALUES ($1, $2) ON CONFLICT (customer_id, tag_id) DO NOTHING`, customerID, tagID)
	if err != nil {
		return fmt.Errorf("failed to assign tag: %v", err)
	}

	s.logAudit(actor, "customer.tag_assigned", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username": username, "tag_id": tagID, "bulk": true,
	}, ip)
	return nil
}
