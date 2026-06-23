//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleInvoices dispatches GET /api/invoices (admin list).
func (s *Server) handleInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.listInvoicesAdmin(w, r)
}

// handleInvoiceByID dispatches /api/invoices/{id}, /api/invoices/{id}/download, /api/invoices/{id}/refund.
func (s *Server) handleInvoiceByID(w http.ResponseWriter, r *http.Request) {
	// Trim prefix and parse: could be "{id}", "{id}/download", or "{id}/refund"
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/invoices/")
	trimmed = strings.TrimSuffix(trimmed, "/")

	// Check for sub-resources
	if strings.HasSuffix(trimmed, "/download") {
		trimmed = strings.TrimSuffix(trimmed, "/download")
		id, err := parseID(trimmed)
		if err {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.downloadInvoice(w, r, id)
		return
	}

	if strings.HasSuffix(trimmed, "/refund") {
		trimmed = strings.TrimSuffix(trimmed, "/refund")
		id, err := parseID(trimmed)
		if err {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.refundInvoice(w, r, id)
		return
	}

	// Plain ID — get invoice detail
	id, err := parseID(trimmed)
	if err {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.getInvoiceAdmin(w, r, id)
}

// handlePortalInvoices dispatches GET /api/portal/invoices (customer list).
func (s *Server) handlePortalInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.listInvoicesCustomer(w, r)
}

// handlePortalInvoiceByID dispatches GET /api/portal/invoices/{id} (customer detail).
func (s *Server) handlePortalInvoiceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _, ok := pathID(r.URL.Path, "/api/portal/invoices/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	s.getInvoiceCustomer(w, r, id)
}

// parseID parses a numeric ID from a string segment. Returns (id, hadError).
func parseID(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	var id int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, true
		}
		id = id*10 + int64(ch-'0')
	}
	return id, false
}

// listInvoicesAdmin lists all invoices with optional filters.
// GET /api/invoices?date_from=&date_to=&status=&customer_id=
func (s *Server) listInvoicesAdmin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	status := q.Get("status")
	customerID := q.Get("customer_id")

	var conditions []string
	var args []any

	if dateFrom != "" {
		conditions = append(conditions, "i.created_at >= ?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		conditions = append(conditions, "i.created_at <= ?")
		args = append(args, dateTo)
	}
	if status != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, status)
	}
	if customerID != "" {
		conditions = append(conditions, "i.customer_id = ?")
		args = append(args, customerID)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	query := `SELECT i.id, i.invoice_number, i.customer_id, COALESCE(c.username, ''), COALESCE(i.plan_name, ''),
		i.amount, i.tax, i.total, i.currency, COALESCE(i.payment_method, ''), i.status, i.refunded_amount, i.created_at
		FROM invoices i
		LEFT JOIN customers c ON c.id = i.customer_id` + where + ` ORDER BY i.created_at DESC LIMIT 500`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[invoices] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	invoices := s.scanInvoiceRows(rows)
	if invoices == nil {
		invoices = []invoiceItem{}
	}

	writeJSON(w, map[string]any{"ok": true, "invoices": invoices})
}

// getInvoiceAdmin returns a single invoice detail for admin.
// GET /api/invoices/{id}
func (s *Server) getInvoiceAdmin(w http.ResponseWriter, r *http.Request, id int64) {
	inv, err := s.fetchInvoiceByID(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		log.Printf("[invoices] get invoice %d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "invoice": inv})
}

// downloadInvoice renders the invoice as HTML.
// GET /api/invoices/{id}/download
func (s *Server) downloadInvoice(w http.ResponseWriter, r *http.Request, id int64) {
	inv, err := s.fetchInvoiceByID(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		log.Printf("[invoices] download invoice %d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	html := renderInvoiceHTML(inv)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.html"`, inv.InvoiceNumber))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// refundInvoice issues a full or partial refund on an invoice.
// POST /api/invoices/{id}/refund
func (s *Server) refundInvoice(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Amount *float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Fetch current invoice state
	var invoiceNumber string
	var customerID, transactionID sql.NullInt64
	var total, refundedAmount float64
	var status, currency, gatewayName string
	var referenceID sql.NullString

	err := s.DB.QueryRow(`
		SELECT i.invoice_number, i.customer_id, i.transaction_id, i.total, i.refunded_amount, i.status, i.currency,
			COALESCE(pt.gateway_name, ''), COALESCE(pt.reference_id, '')
		FROM invoices i
		LEFT JOIN payment_transactions pt ON pt.id = i.transaction_id
		WHERE i.id = ?`, id,
	).Scan(&invoiceNumber, &customerID, &transactionID, &total, &refundedAmount, &status, &currency, &gatewayName, &referenceID)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		log.Printf("[invoices] refund fetch invoice %d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if status == "refunded" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "already_refunded"})
		return
	}

	// Determine refund amount
	refundAmount := total - refundedAmount // full refund by default
	if in.Amount != nil {
		refundAmount = *in.Amount
	}

	if refundAmount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
		return
	}

	maxRefundable := total - refundedAmount
	if refundAmount > maxRefundable {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "exceeds_refundable"})
		return
	}

	// Call gateway RefundPayment if available
	if gatewayName != "" && referenceID.Valid && referenceID.String != "" && s.PaymentRegistry != nil {
		gw, found := s.PaymentRegistry.Get(gatewayName)
		if found {
			if err := gw.RefundPayment(referenceID.String, refundAmount); err != nil {
				log.Printf("[invoices] gateway refund failed for invoice %s: %v", invoiceNumber, err)
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "gateway_refund_failed"})
				return
			}
		}
	}

	// Determine new status
	newRefundedAmount := refundedAmount + refundAmount
	newStatus := "partially_refunded"
	if newRefundedAmount >= total {
		newStatus = "refunded"
	}

	// Update invoice
	_, err = s.DB.Exec(
		`UPDATE invoices SET status = ?, refunded_amount = ? WHERE id = ?`,
		newStatus, newRefundedAmount, id,
	)
	if err != nil {
		log.Printf("[invoices] update invoice status failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Update payment transaction status if linked
	if transactionID.Valid {
		txStatus := "partially_refunded"
		if newStatus == "refunded" {
			txStatus = "refunded"
		}
		_, _ = s.DB.Exec(
			`UPDATE payment_transactions SET status = ?, updated_at = NOW() WHERE id = ?`,
			txStatus, transactionID.Int64,
		)
	}

	// Debit customer wallet
	if customerID.Valid {
		var username string
		_ = s.DB.QueryRow(`SELECT username FROM customers WHERE id = ? LIMIT 1`, customerID.Int64).Scan(&username)
		if username != "" {
			_ = s.applyWalletChange(username, -refundAmount, "debit",
				fmt.Sprintf("Refund for invoice %s", invoiceNumber), "admin")
		}
	}

	log.Printf("[invoices] refund issued: invoice=%s amount=%.2f new_status=%s", invoiceNumber, refundAmount, newStatus)

	writeJSON(w, map[string]any{
		"ok":              true,
		"refunded_amount": refundAmount,
		"new_status":      newStatus,
		"total_refunded":  newRefundedAmount,
	})
}

// listInvoicesCustomer lists invoices for the current customer.
// GET /api/portal/invoices?date_from=&date_to=&status=
func (s *Server) listInvoicesCustomer(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Get customer ID
	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username = ? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	q := r.URL.Query()
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	status := q.Get("status")

	conditions := []string{"i.customer_id = ?"}
	args := []any{customerID}

	if dateFrom != "" {
		conditions = append(conditions, "i.created_at >= ?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		conditions = append(conditions, "i.created_at <= ?")
		args = append(args, dateTo)
	}
	if status != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, status)
	}

	where := " WHERE " + strings.Join(conditions, " AND ")

	query := `SELECT i.id, i.invoice_number, i.customer_id, COALESCE(c.username, ''), COALESCE(i.plan_name, ''),
		i.amount, i.tax, i.total, i.currency, COALESCE(i.payment_method, ''), i.status, i.refunded_amount, i.created_at
		FROM invoices i
		LEFT JOIN customers c ON c.id = i.customer_id` + where + ` ORDER BY i.created_at DESC LIMIT 200`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[invoices] customer list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	invoices := s.scanInvoiceRows(rows)
	if invoices == nil {
		invoices = []invoiceItem{}
	}

	writeJSON(w, map[string]any{"ok": true, "invoices": invoices})
}

// getInvoiceCustomer returns a single invoice detail for the current customer (verifies ownership).
// GET /api/portal/invoices/{id}
func (s *Server) getInvoiceCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Get customer ID
	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username = ? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	inv, err := s.fetchInvoiceByID(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		log.Printf("[invoices] customer get invoice %d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Verify ownership
	if inv.CustomerID != customerID {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "invoice": inv})
}

// --- Helpers ---

type invoiceItem struct {
	ID             int64   `json:"id"`
	InvoiceNumber  string  `json:"invoice_number"`
	CustomerID     int64   `json:"customer_id"`
	Username       string  `json:"username"`
	PlanName       string  `json:"plan_name"`
	Amount         float64 `json:"amount"`
	Tax            float64 `json:"tax"`
	Total          float64 `json:"total"`
	Currency       string  `json:"currency"`
	PaymentMethod  string  `json:"payment_method"`
	Status         string  `json:"status"`
	RefundedAmount float64 `json:"refunded_amount"`
	CreatedAt      string  `json:"created_at"`
}

// scanInvoiceRows scans multiple invoice rows into a slice.
func (s *Server) scanInvoiceRows(rows *sql.Rows) []invoiceItem {
	var items []invoiceItem
	for rows.Next() {
		var item invoiceItem
		if err := rows.Scan(
			&item.ID, &item.InvoiceNumber, &item.CustomerID, &item.Username, &item.PlanName,
			&item.Amount, &item.Tax, &item.Total, &item.Currency, &item.PaymentMethod,
			&item.Status, &item.RefundedAmount, &item.CreatedAt,
		); err != nil {
			log.Printf("[invoices] scan row error: %v", err)
			return items
		}
		items = append(items, item)
	}
	return items
}

// fetchInvoiceByID fetches a single invoice with customer username.
func (s *Server) fetchInvoiceByID(id int64) (invoiceItem, error) {
	var inv invoiceItem
	err := s.DB.QueryRow(`
		SELECT i.id, i.invoice_number, i.customer_id, COALESCE(c.username, ''), COALESCE(i.plan_name, ''),
			i.amount, i.tax, i.total, i.currency, COALESCE(i.payment_method, ''), i.status, i.refunded_amount, i.created_at
		FROM invoices i
		LEFT JOIN customers c ON c.id = i.customer_id
		WHERE i.id = ?`, id,
	).Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.CustomerID, &inv.Username, &inv.PlanName,
		&inv.Amount, &inv.Tax, &inv.Total, &inv.Currency, &inv.PaymentMethod,
		&inv.Status, &inv.RefundedAmount, &inv.CreatedAt,
	)
	return inv, err
}

// renderInvoiceHTML generates a simple HTML invoice document.
func renderInvoiceHTML(inv invoiceItem) string {
	createdAt := inv.CreatedAt
	if t, err := time.Parse("2006-01-02 15:04:05", inv.CreatedAt); err == nil {
		createdAt = t.Format(time.RFC3339)
	}

	var refundedRow string
	if inv.RefundedAmount > 0 {
		refundedRow = fmt.Sprintf(`<tr><td>Refunded</td><td>%.2f %s</td></tr>`, inv.RefundedAmount, inv.Currency)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Invoice %s</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 40px; color: #333; }
  .invoice-header { border-bottom: 2px solid #333; padding-bottom: 20px; margin-bottom: 20px; }
  .invoice-header h1 { margin: 0; font-size: 28px; }
  .invoice-header .number { color: #666; font-size: 14px; }
  .details-table { width: 100%%; border-collapse: collapse; margin-top: 20px; }
  .details-table td { padding: 8px 12px; border-bottom: 1px solid #eee; }
  .details-table td:first-child { font-weight: 600; width: 200px; color: #555; }
  .status { display: inline-block; padding: 4px 12px; border-radius: 4px; font-size: 12px; font-weight: 600; text-transform: uppercase; }
  .status-paid { background: #d4edda; color: #155724; }
  .status-refunded { background: #f8d7da; color: #721c24; }
  .status-partially_refunded { background: #fff3cd; color: #856404; }
  .total-row td { font-size: 18px; font-weight: 700; border-top: 2px solid #333; }
</style>
</head>
<body>
<div class="invoice-header">
  <h1>Invoice</h1>
  <span class="number">%s</span>
</div>
<table class="details-table">
  <tr><td>Date</td><td>%s</td></tr>
  <tr><td>Customer</td><td>%s</td></tr>
  <tr><td>Plan</td><td>%s</td></tr>
  <tr><td>Amount</td><td>%.2f %s</td></tr>
  <tr><td>Tax</td><td>%.2f %s</td></tr>
  <tr class="total-row"><td>Total</td><td>%.2f %s</td></tr>
  %s
  <tr><td>Payment Method</td><td>%s</td></tr>
  <tr><td>Status</td><td><span class="status status-%s">%s</span></td></tr>
</table>
</body>
</html>`,
		inv.InvoiceNumber,
		inv.InvoiceNumber,
		createdAt,
		inv.Username,
		inv.PlanName,
		inv.Amount, inv.Currency,
		inv.Tax, inv.Currency,
		inv.Total, inv.Currency,
		refundedRow,
		inv.PaymentMethod,
		inv.Status, inv.Status,
	)
}
