//go:build !lite

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"KorisPanel/panel/internal/payment"
)

// handlePaymentInitiate handles POST /api/portal/pay.
// Customer initiates a payment through a registered gateway.
func (s *Server) handlePaymentInitiate(w http.ResponseWriter, r *http.Request) {
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
		GatewayName string  `json:"gateway_name"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.GatewayName == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "gateway_name_required"})
		return
	}
	if in.Amount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
		return
	}
	if in.Currency == "" {
		in.Currency = "IRR"
	}

	// Check gateway is registered in the plugin registry
	if s.PaymentRegistry == nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "payment_not_configured"})
		return
	}

	gateway, found := s.PaymentRegistry.Get(in.GatewayName)
	if !found {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "gateway_not_found"})
		return
	}

	// Check gateway is active in the database
	var isActive bool
	err := s.DB.QueryRow(`SELECT is_active FROM payment_gateways WHERE name = $1 LIMIT 1`, in.GatewayName).Scan(&isActive)
	if err != nil || !isActive {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "gateway_not_active"})
		return
	}

	// Get customer ID
	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	// Build callback URL
	callbackURL := s.getPanelURL(r) + "/api/gateway/callback/" + in.GatewayName

	// Create payment via gateway
	redirectURL, reference, err := gateway.CreatePayment(in.Amount, in.Currency, callbackURL)
	if err != nil {
		log.Printf("[payment] gateway %s CreatePayment failed: %v", in.GatewayName, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "gateway_error"})
		return
	}

	// Insert pending transaction
	_, err = s.DB.Exec(
		`INSERT INTO payment_transactions (customer_id, gateway_name, reference_id, amount, currency, status) VALUES ($1, $2, $3, $4, $5, 'pending')`,
		customerID, in.GatewayName, reference, in.Amount, in.Currency,
	)
	if err != nil {
		log.Printf("[payment] failed to insert transaction: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{
		"ok":           true,
		"redirect_url": redirectURL,
		"reference":    reference,
	})
}

// handleGatewayCallback handles POST/GET /api/gateway/callback/{name}.
// This is a PUBLIC endpoint called by payment gateways after payment processing.
// For Zarinpal specifically, the callback comes as a GET redirect with Authority and Status query params.
func (s *Server) handleGatewayCallback(w http.ResponseWriter, r *http.Request) {
	// Extract gateway name from URL path: /api/gateway/callback/{name}
	gatewayName := strings.TrimPrefix(r.URL.Path, "/api/gateway/callback/")
	gatewayName = strings.TrimSuffix(gatewayName, "/")
	if gatewayName == "" {
		http.Error(w, "gateway name required", http.StatusBadRequest)
		return
	}

	if s.PaymentRegistry == nil {
		http.Error(w, "payment not configured", http.StatusInternalServerError)
		return
	}

	gateway, found := s.PaymentRegistry.Get(gatewayName)
	if !found {
		http.Error(w, "gateway not found", http.StatusNotFound)
		return
	}

	// Read callback data (store raw for audit)
	var callbackData string
	var reference string

	// Zarinpal sends a GET redirect with Authority and Status query params
	if r.Method == http.MethodGet && gatewayName == "zarinpal" {
		reference = r.URL.Query().Get("Authority")
		status := r.URL.Query().Get("Status")
		callbackData = fmt.Sprintf("Authority=%s&Status=%s", reference, status)

		if status != "OK" {
			// Payment was cancelled or failed at gateway level
			s.markTransactionFailed(gatewayName, reference, callbackData)
			// Redirect customer back to portal with failure indicator
			portalURL := s.getPanelURL(r) + "/portal/#/payment?status=failed"
			http.Redirect(w, r, portalURL, http.StatusFound)
			return
		}
	} else {
		// Generic POST callback — read body
		bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBody))
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		callbackData = string(bodyBytes)

		// Try to extract reference from JSON body
		var cbData map[string]any
		if json.Unmarshal(bodyBytes, &cbData) == nil {
			// Check common reference field names
			for _, key := range []string{"reference", "authority", "Authority", "ref_id", "reference_id", "token"} {
				if v, ok := cbData[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						reference = s
						break
					}
				}
			}
		}

		// Fallback: check query params for reference
		if reference == "" {
			reference = r.URL.Query().Get("Authority")
			if reference == "" {
				reference = r.URL.Query().Get("reference")
			}
			if reference == "" {
				reference = r.URL.Query().Get("ref_id")
			}
		}
	}

	if reference == "" {
		log.Printf("[payment] callback for %s: no reference found in callback data", gatewayName)
		http.Error(w, "no reference", http.StatusBadRequest)
		return
	}

	// Store raw callback data
	_, _ = s.DB.Exec(
		`UPDATE payment_transactions SET callback_data = $1 WHERE gateway_name = $2 AND reference_id = $3`,
		callbackData, gatewayName, reference,
	)

	// Verify payment with the gateway
	verifiedAmount, err := gateway.VerifyPayment(reference)
	if err != nil {
		log.Printf("[payment] verification failed for %s ref=%s: %v", gatewayName, reference, err)
		s.markTransactionFailed(gatewayName, reference, callbackData)
		s.respondToCallback(w, r, gatewayName, false)
		return
	}

	// Payment verified — complete the transaction
	s.completePaymentTransaction(w, r, gatewayName, reference, verifiedAmount)
}

// completePaymentTransaction handles a successful payment verification.
// It updates the transaction status, credits the wallet, and generates an invoice.
func (s *Server) completePaymentTransaction(w http.ResponseWriter, r *http.Request, gatewayName, reference string, amount float64) {
	// Get transaction details
	var txID, customerID int64
	var txAmount float64
	var currency string
	err := s.DB.QueryRow(
		`SELECT id, customer_id, amount, currency FROM payment_transactions WHERE gateway_name = $1 AND reference_id = $2 AND status = 'pending' LIMIT 1`,
		gatewayName, reference,
	).Scan(&txID, &customerID, &txAmount, &currency)
	if err != nil {
		log.Printf("[payment] transaction not found for %s ref=%s: %v", gatewayName, reference, err)
		s.respondToCallback(w, r, gatewayName, false)
		return
	}

	// Use the amount from our transaction record (trust our record over gateway for consistency)
	if amount > 0 {
		txAmount = amount
	}

	// Update transaction status to completed
	_, err = s.DB.Exec(
		`UPDATE payment_transactions SET status = 'completed', updated_at = NOW() WHERE id = $1`,
		txID,
	)
	if err != nil {
		log.Printf("[payment] failed to mark transaction completed: %v", err)
		s.respondToCallback(w, r, gatewayName, false)
		return
	}

	// Credit the customer's wallet
	_, err = s.DB.Exec(
		`UPDATE customers SET wallet_balance = COALESCE(wallet_balance, 0) + $1 WHERE id = $2`,
		txAmount, customerID,
	)
	if err != nil {
		log.Printf("[payment] failed to credit wallet for customer %d: %v", customerID, err)
	}

	// Also update the wallets table if it exists (legacy support)
	var username string
	_ = s.DB.QueryRow(`SELECT username FROM customers WHERE id = $1 LIMIT 1`, customerID).Scan(&username)
	if username != "" {
		_, _ = s.DB.Exec(
			`INSERT INTO wallets(customer_id, username, credit) VALUES($1, $2, $3) ON CONFLICT (username) DO UPDATE SET credit = wallets.credit + EXCLUDED.credit, customer_id = COALESCE(EXCLUDED.customer_id, wallets.customer_id)`,
			customerID, username, txAmount,
		)

		// Record wallet transaction
		_, _ = s.DB.Exec(
			`INSERT INTO wallet_transactions(customer_id, username, amount, type, description, actor, reference_type, reference_id) VALUES($1, $2, $3, 'credit', $4, 'gateway', 'payment_transaction', $5)`,
			customerID, username, txAmount, fmt.Sprintf("Payment via %s", gatewayName), txID,
		)
	}

	// Generate invoice
	invoiceNumber, err := payment.GenerateInvoiceNumber(s.DB)
	if err != nil {
		log.Printf("[payment] failed to generate invoice number: %v", err)
		invoiceNumber = fmt.Sprintf("INV-ERR-%d", txID)
	}

	_, err = s.DB.Exec(
		`INSERT INTO invoices (invoice_number, customer_id, transaction_id, amount, tax, total, currency, payment_method, status) VALUES ($1, $2, $3, $4, 0, $5, $6, $7, 'paid')`,
		invoiceNumber, customerID, txID, txAmount, txAmount, currency, gatewayName,
	)
	if err != nil {
		log.Printf("[payment] failed to insert invoice: %v", err)
	}

	log.Printf("[payment] payment completed: gateway=%s ref=%s customer=%d amount=%.2f %s invoice=%s",
		gatewayName, reference, customerID, txAmount, currency, invoiceNumber)

	s.respondToCallback(w, r, gatewayName, true)
}

// markTransactionFailed updates a payment transaction to failed status.
func (s *Server) markTransactionFailed(gatewayName, reference, callbackData string) {
	_, err := s.DB.Exec(
		`UPDATE payment_transactions SET status = 'failed', callback_data = COALESCE($1, callback_data), updated_at = NOW() WHERE gateway_name = $2 AND reference_id = $3 AND status = 'pending'`,
		callbackData, gatewayName, reference,
	)
	if err != nil {
		log.Printf("[payment] failed to mark transaction as failed: gateway=%s ref=%s err=%v", gatewayName, reference, err)
	}
}

// respondToCallback sends the appropriate response for a gateway callback.
// For Zarinpal GET callbacks, it redirects to the portal.
// For POST callbacks, it returns HTTP 200 with a status indicator.
func (s *Server) respondToCallback(w http.ResponseWriter, r *http.Request, gatewayName string, success bool) {
	// Zarinpal GET callback — redirect customer to portal
	if r.Method == http.MethodGet && gatewayName == "zarinpal" {
		status := "success"
		if !success {
			status = "failed"
		}
		portalURL := s.getPanelURL(r) + "/portal/#/payment?status=" + status
		http.Redirect(w, r, portalURL, http.StatusFound)
		return
	}

	// POST callback — return HTTP 200 (some gateways expect this)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if success {
		_, _ = w.Write([]byte(`{"status":"OK"}`))
	} else {
		_, _ = w.Write([]byte(`{"status":"FAILED"}`))
	}
}
