//go:build !lite

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"KorisPanel/panel/internal/auth"
	"KorisPanel/panel/internal/payment"

	"github.com/DATA-DOG/go-sqlmock"
)

// mockPaymentGateway implements the payment.Gateway interface for testing.
type mockPaymentGateway struct {
	name          string
	redirectURL   string
	reference     string
	createErr     error
	verifyAmount  float64
	verifyErr     error
	refundErr     error
	createCalled  bool
	verifyCalled  bool
	refundCalled  bool
	lastAmount    float64
	lastCurrency  string
	lastCallback  string
	lastReference string
}

func (m *mockPaymentGateway) Name() string { return m.name }

func (m *mockPaymentGateway) CreatePayment(amount float64, currency string, callbackURL string) (string, string, error) {
	m.createCalled = true
	m.lastAmount = amount
	m.lastCurrency = currency
	m.lastCallback = callbackURL
	return m.redirectURL, m.reference, m.createErr
}

func (m *mockPaymentGateway) VerifyPayment(reference string) (float64, error) {
	m.verifyCalled = true
	m.lastReference = reference
	return m.verifyAmount, m.verifyErr
}

func (m *mockPaymentGateway) RefundPayment(reference string, amount float64) error {
	m.refundCalled = true
	m.lastReference = reference
	m.lastAmount = amount
	return m.refundErr
}

// testSessionSecret is the shared secret for generating valid session tokens in tests.
const testSessionSecret = "test-secret-key-for-integration-tests"

// makeCustomerCookie creates a valid customer session cookie for the given username.
func makeCustomerCookie(username string) *http.Cookie {
	token := auth.MakeSession(username, testSessionSecret, 1*time.Hour)
	return &http.Cookie{
		Name:  auth.CustomerCookieName,
		Value: token,
	}
}

func TestPaymentIntegration(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"initiate payment", testInitiatePayment},
		{"gateway callback success", testGatewayCallbackSuccess},
		{"gateway callback failure", testGatewayCallbackFailure},
		{"gateway callback unknown reference", testGatewayCallbackUnknownReference},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

func testInitiatePayment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	gw := &mockPaymentGateway{
		name:        "test_gateway",
		redirectURL: "https://payment.example.com/checkout/abc123",
		reference:   "REF-001",
	}

	registry := payment.NewRegistry()
	registry.Register(gw)

	srv := &Server{
		DB:              db,
		PaymentRegistry: registry,
	}
	srv.Config.SessionSecret = testSessionSecret

	// Mock: currentCustomer checks customer status
	mock.ExpectQuery("SELECT status FROM customers WHERE username=\\? AND deleted_at IS NULL LIMIT 1").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))

	// Mock: check gateway is active in DB
	mock.ExpectQuery("SELECT is_active FROM payment_gateways WHERE name = \\? LIMIT 1").
		WithArgs("test_gateway").
		WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(1))

	// Mock: get customer ID
	mock.ExpectQuery("SELECT id FROM customers WHERE username = \\? AND deleted_at IS NULL LIMIT 1").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// Mock: getPanelURL query (may return no rows — fallback to request Host)
	mock.ExpectQuery("SELECT setting_value FROM panel_settings WHERE setting_key='panel_domain'").
		WillReturnRows(sqlmock.NewRows([]string{"setting_value"}))

	// Mock: insert pending transaction
	mock.ExpectExec("INSERT INTO payment_transactions").
		WithArgs(int64(42), "test_gateway", "REF-001", 50000.0, "IRR").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Build request
	body := map[string]any{
		"gateway_name": "test_gateway",
		"amount":       50000,
		"currency":     "IRR",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/portal/pay", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(makeCustomerCookie("testuser"))

	rr := httptest.NewRecorder()
	// Call handler directly (bypasses middleware, session is validated inside handler)
	srv.handlePaymentInitiate(rr, req)

	// Verify response
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp["ok"])
	}
	if resp["redirect_url"] != "https://payment.example.com/checkout/abc123" {
		t.Fatalf("expected redirect_url, got %v", resp["redirect_url"])
	}
	if resp["reference"] != "REF-001" {
		t.Fatalf("expected reference REF-001, got %v", resp["reference"])
	}

	// Verify gateway was called correctly
	if !gw.createCalled {
		t.Fatal("expected CreatePayment to be called")
	}
	if gw.lastAmount != 50000 {
		t.Fatalf("expected amount 50000, got %v", gw.lastAmount)
	}
	if gw.lastCurrency != "IRR" {
		t.Fatalf("expected currency IRR, got %v", gw.lastCurrency)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func testGatewayCallbackSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	gw := &mockPaymentGateway{
		name:         "test_gateway",
		verifyAmount: 50000.0,
	}

	registry := payment.NewRegistry()
	registry.Register(gw)

	srv := &Server{
		DB:              db,
		PaymentRegistry: registry,
	}
	srv.Config.SessionSecret = testSessionSecret

	// Mock: store raw callback data
	mock.ExpectExec("UPDATE payment_transactions SET callback_data = \\? WHERE gateway_name = \\? AND reference_id = \\?").
		WithArgs(sqlmock.AnyArg(), "test_gateway", "REF-001").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Mock: completePaymentTransaction - get transaction details
	mock.ExpectQuery("SELECT id, customer_id, amount, currency FROM payment_transactions WHERE gateway_name = \\? AND reference_id = \\? AND status = 'pending' LIMIT 1").
		WithArgs("test_gateway", "REF-001").
		WillReturnRows(sqlmock.NewRows([]string{"id", "customer_id", "amount", "currency"}).
			AddRow(10, 42, 50000.0, "IRR"))

	// Mock: update transaction status to completed
	mock.ExpectExec("UPDATE payment_transactions SET status = 'completed'").
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Mock: credit wallet
	mock.ExpectExec("UPDATE customers SET wallet_balance = COALESCE\\(wallet_balance, 0\\) \\+ \\? WHERE id = \\?").
		WithArgs(50000.0, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Mock: get username for wallets table
	mock.ExpectQuery("SELECT username FROM customers WHERE id = \\? LIMIT 1").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("testuser"))

	// Mock: insert into wallets (legacy support)
	mock.ExpectExec("INSERT INTO wallets").
		WithArgs(int64(42), "testuser", 50000.0).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Mock: insert wallet_transaction
	mock.ExpectExec("INSERT INTO wallet_transactions").
		WithArgs(int64(42), "testuser", 50000.0, sqlmock.AnyArg(), int64(10)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Mock: GenerateInvoiceNumber query
	mock.ExpectQuery("SELECT invoice_number FROM invoices WHERE invoice_number LIKE \\?").
		WillReturnRows(sqlmock.NewRows([]string{"invoice_number"})) // No previous invoices → first one

	// Mock: insert invoice
	mock.ExpectExec("INSERT INTO invoices").
		WithArgs(sqlmock.AnyArg(), int64(42), int64(10), 50000.0, 50000.0, "IRR", "test_gateway").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Build POST callback request with JSON body containing reference
	callbackBody := map[string]any{
		"reference": "REF-001",
		"status":    "success",
	}
	callbackBytes, _ := json.Marshal(callbackBody)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/callback/test_gateway", bytes.NewReader(callbackBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleGatewayCallback(rr, req)

	// For POST callbacks, we expect HTTP 200 with success status
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "OK" {
		t.Fatalf("expected status OK, got %v", resp["status"])
	}

	// Verify gateway VerifyPayment was called
	if !gw.verifyCalled {
		t.Fatal("expected VerifyPayment to be called")
	}
	if gw.lastReference != "REF-001" {
		t.Fatalf("expected reference REF-001, got %v", gw.lastReference)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func testGatewayCallbackFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	gw := &mockPaymentGateway{
		name:      "test_gateway",
		verifyErr: fmt.Errorf("payment verification failed: insufficient funds"),
	}

	registry := payment.NewRegistry()
	registry.Register(gw)

	srv := &Server{
		DB:              db,
		PaymentRegistry: registry,
	}
	srv.Config.SessionSecret = testSessionSecret

	// Mock: store raw callback data
	mock.ExpectExec("UPDATE payment_transactions SET callback_data = \\? WHERE gateway_name = \\? AND reference_id = \\?").
		WithArgs(sqlmock.AnyArg(), "test_gateway", "REF-FAIL").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Mock: markTransactionFailed
	mock.ExpectExec("UPDATE payment_transactions SET status = 'failed'").
		WithArgs(sqlmock.AnyArg(), "test_gateway", "REF-FAIL").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Build POST callback request
	callbackBody := map[string]any{
		"reference": "REF-FAIL",
		"status":    "failed",
	}
	callbackBytes, _ := json.Marshal(callbackBody)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/callback/test_gateway", bytes.NewReader(callbackBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleGatewayCallback(rr, req)

	// For POST callbacks with failure, we still get HTTP 200 with FAILED status
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "FAILED" {
		t.Fatalf("expected status FAILED, got %v", resp["status"])
	}

	// Verify gateway VerifyPayment was called (it returns error)
	if !gw.verifyCalled {
		t.Fatal("expected VerifyPayment to be called")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func testGatewayCallbackUnknownReference(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	gw := &mockPaymentGateway{
		name:         "test_gateway",
		verifyAmount: 50000.0,
	}

	registry := payment.NewRegistry()
	registry.Register(gw)

	srv := &Server{
		DB:              db,
		PaymentRegistry: registry,
	}
	srv.Config.SessionSecret = testSessionSecret

	// Mock: store raw callback data
	mock.ExpectExec("UPDATE payment_transactions SET callback_data = \\? WHERE gateway_name = \\? AND reference_id = \\?").
		WithArgs(sqlmock.AnyArg(), "test_gateway", "REF-UNKNOWN").
		WillReturnResult(sqlmock.NewResult(0, 0)) // no rows affected — doesn't exist, but ok

	// Mock: completePaymentTransaction - transaction not found (no pending transaction with that reference)
	mock.ExpectQuery("SELECT id, customer_id, amount, currency FROM payment_transactions WHERE gateway_name = \\? AND reference_id = \\? AND status = 'pending' LIMIT 1").
		WithArgs("test_gateway", "REF-UNKNOWN").
		WillReturnRows(sqlmock.NewRows([]string{"id", "customer_id", "amount", "currency"})) // empty — no matching row

	// Build POST callback request with unknown reference
	callbackBody := map[string]any{
		"reference": "REF-UNKNOWN",
		"status":    "success",
	}
	callbackBytes, _ := json.Marshal(callbackBody)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/callback/test_gateway", bytes.NewReader(callbackBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleGatewayCallback(rr, req)

	// Graceful handling: returns FAILED since transaction can't be found
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "FAILED" {
		t.Fatalf("expected status FAILED for unknown reference, got %v", resp["status"])
	}

	// VerifyPayment was still called (gateway verifies reference first)
	if !gw.verifyCalled {
		t.Fatal("expected VerifyPayment to be called")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
