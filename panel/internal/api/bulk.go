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
	CustomerIDs []int64 `json:"customer_ids"`
	Action      string  `json:"action"` // "enable", "disable", "delete", "traffic_reset"
}

// BulkActionResponse represents the result of a bulk operation.
type BulkActionResponse struct {
	OK        bool          `json:"ok"`
	Succeeded []int64       `json:"succeeded"`
	Failed    []BulkFailure `json:"failed"`
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

	var req BulkActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if len(req.CustomerIDs) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "customer_ids is required")
		return
	}

	if len(req.CustomerIDs) > 200 {
		writeError(w, http.StatusBadRequest, "bulk_limit_exceeded", "bulk actions are limited to 200 customers at a time")
		return
	}

	validActions := map[string]bool{
		"enable":        true,
		"disable":       true,
		"delete":        true,
		"traffic_reset": true,
	}
	if !validActions[req.Action] {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid action: must be enable, disable, delete, or traffic_reset")
		return
	}

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	succeeded := []int64{}
	failed := []BulkFailure{}

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
		}

		if err != nil {
			failed = append(failed, BulkFailure{
				CustomerID: customerID,
				Error:      err.Error(),
			})
		} else {
			succeeded = append(succeeded, customerID)
		}
	}

	writeJSON(w, BulkActionResponse{
		OK:        len(failed) == 0,
		Succeeded: succeeded,
		Failed:    failed,
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

	if _, err := s.DB.Exec(`UPDATE customers SET status=? WHERE id=? AND deleted_at IS NULL`, status, customerID); err != nil {
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

	res, err := s.DB.Exec(`UPDATE customers SET deleted_at=NOW(), status='deleted' WHERE id=? AND deleted_at IS NULL`, customerID)
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
	err := s.DB.QueryRow(`SELECT username, status FROM customers WHERE id=? AND deleted_at IS NULL LIMIT 1`, customerID).Scan(&username, &status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("customer not found")
	}
	if err != nil {
		return err
	}

	// 2. Zero radacct counters for active sessions
	_, err = s.DB.Exec(`UPDATE radacct SET acctinputoctets=0, acctoutputoctets=0 WHERE username=? AND acctstoptime IS NULL`, username)
	if err != nil {
		return fmt.Errorf("failed to reset radacct counters: %v", err)
	}

	// 3. Insert wallet_transaction of type "adjustment" with description "Traffic reset (bulk)"
	_, err = s.DB.Exec(`INSERT INTO wallet_transactions(customer_id, username, amount, type, description, actor) VALUES(?, ?, 0, 'adjustment', 'Traffic reset (bulk)', ?)`,
		customerID, username, actor)
	if err != nil {
		return fmt.Errorf("failed to insert wallet transaction: %v", err)
	}

	// 4. If customer status == 'limited', update to 'active'
	if status == "limited" {
		_, err = s.DB.Exec(`UPDATE customers SET status='active' WHERE id=? AND deleted_at IS NULL`, customerID)
		if err != nil {
			return fmt.Errorf("failed to update customer status: %v", err)
		}
	}

	// 5. Insert audit_log entry
	s.logAudit(actor, "customer.traffic_reset", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"username": username, "bulk": true}, ip)

	return nil
}
