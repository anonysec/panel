package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// BulkActionRequest represents a request to perform bulk operations on customers.
type BulkActionRequest struct {
	CustomerIDs []int64        `json:"customer_ids"`
	Action      string         `json:"action"` // "enable", "disable", "delete", "traffic_reset", "extend", "change_plan", "assign_tag"
	Params      map[string]any `json:"params"`
}

// BulkFailure represents a single failure within a bulk operation.
type BulkFailure struct {
	CustomerID int64  `json:"customer_id"`
	Error      string `json:"error"`
}

// customersBulk handles POST /api/customers/bulk
func (s *Server) customersBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var req BulkActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if len(req.CustomerIDs) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_ids_required"})
		return
	}

	if len(req.CustomerIDs) > 200 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bulk_limit_exceeded"})
		return
	}

	validActions := map[string]bool{
		"enable":        true,
		"disable":       true,
		"delete":        true,
		"traffic_reset": true,
		"extend":        true,
		"change_plan":   true,
		"assign_tag":    true,
	}
	if !validActions[req.Action] {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_action"})
		return
	}

	// Validate params for actions that require them
	if err := s.validateBulkParams(req.Action, req.Params); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	succeeded := 0
	failures := []BulkFailure{}

	for _, customerID := range req.CustomerIDs {
		var err error
		switch req.Action {
		case "enable":
			err = s.bulkSetStatus(customerID, "active", actor, ip)
		case "disable":
			err = s.bulkSetStatus(customerID, "disabled", actor, ip)
		case "delete":
			err = s.bulkDelete(customerID, actor, ip)
		case "traffic_reset":
			err = s.bulkTrafficReset(customerID, actor, ip)
		case "extend":
			err = s.bulkExtend(customerID, req.Params, actor, ip)
		case "change_plan":
			err = s.bulkChangePlan(customerID, req.Params, actor, ip)
		case "assign_tag":
			err = s.bulkAssignTag(customerID, req.Params, actor, ip)
		}

		if err != nil {
			failures = append(failures, BulkFailure{
				CustomerID: customerID,
				Error:      err.Error(),
			})
		} else {
			succeeded++
		}
	}

	// Log audit trail
	s.logAudit(actor, "customers.bulk_"+req.Action, "customer", "", nil, map[string]any{
		"count": len(req.CustomerIDs),
	}, ip)

	writeJSON(w, map[string]any{
		"ok":        true,
		"total":     len(req.CustomerIDs),
		"succeeded": succeeded,
		"failed":    len(failures),
		"failures":  failures,
	})
}

// bulkSetStatus sets a customer's status within a bulk operation.
func (s *Server) bulkSetStatus(customerID int64, status, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	if _, err := s.DB.Exec(`UPDATE customers SET status=$1 WHERE id=$2 AND deleted_at IS NULL`, status, customerID); err != nil {
		return err
	}

	if status != "active" {
		s.disconnectCustomerSessions(username)
	}

	s.logAudit(actor, "customer.status_changed", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"username": username, "status": status, "bulk": true}, ip)
	return nil
}

// bulkDelete soft-deletes a customer within a bulk operation.
func (s *Server) bulkDelete(customerID int64, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	res, err := s.DB.Exec(`UPDATE customers SET deleted_at=NOW(), status='deleted' WHERE id=$1 AND deleted_at IS NULL`, customerID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("customer already deleted")
	}

	s.disconnectCustomerSessions(username)
	s.logAudit(actor, "customer.deleted", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"username": username, "bulk": true}, ip)
	return nil
}

// bulkTrafficReset resets traffic counters for a customer within a bulk operation.
// Steps:
// 1. Look up customer username
// 2. Zero radacct counters for active sessions
// 3. Insert wallet_transaction of type "adjustment"
// 4. If customer status == 'limited', update to 'active'
// 5. Insert audit_log entry
func (s *Server) bulkTrafficReset(customerID int64, actor, ip string) error {
	// 1. Look up customer username
	var username string
	var status string
	err := s.DB.QueryRow(`SELECT username, status FROM customers WHERE id=$1 AND deleted_at IS NULL LIMIT 1`, customerID).Scan(&username, &status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	// 2. Zero radacct counters for active sessions
	_, err = s.DB.Exec(`UPDATE radacct SET acctinputoctets=0, acctoutputoctets=0 WHERE username=$1 AND acctstoptime IS NULL`, username)
	if err != nil {
		return fmt.Errorf("failed to reset radacct counters: %v", err)
	}

	// 3. Insert wallet_transaction of type "adjustment" with description "Traffic reset (bulk)"
	_, err = s.DB.Exec(`INSERT INTO wallet_transactions(customer_id, username, amount, type, description, actor) VALUES($1, $2, 0, 'adjustment', 'Traffic reset (bulk)', $3)`,
		customerID, username, actor)
	if err != nil {
		return fmt.Errorf("failed to insert wallet transaction: %v", err)
	}

	// 4. If customer status == 'limited', update to 'active'
	if status == "limited" {
		_, err = s.DB.Exec(`UPDATE customers SET status='active' WHERE id=$1 AND deleted_at IS NULL`, customerID)
		if err != nil {
			return fmt.Errorf("failed to update customer status: %v", err)
		}
	}

	// 5. Insert audit_log entry
	s.logAudit(actor, "customer.traffic_reset", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"username": username, "bulk": true}, ip)

	return nil
}

// validateBulkParams validates params for actions that require them in the basic bulk endpoint.
func (s *Server) validateBulkParams(action string, params map[string]any) error {
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

// bulkExtend extends a customer's subscription by N days.
func (s *Server) bulkExtend(customerID int64, params map[string]any, actor, ip string) error {
	username, err := s.customerUsername(customerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	days, _ := toPositiveInt(params["days"])

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

// bulkChangePlan changes a customer's plan.
func (s *Server) bulkChangePlan(customerID int64, params map[string]any, actor, ip string) error {
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

// bulkAssignTag assigns a tag to a customer (INSERT IGNORE for idempotency).
func (s *Server) bulkAssignTag(customerID int64, params map[string]any, actor, ip string) error {
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
