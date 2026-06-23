//go:build !lite

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAdminBillingRevenue_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/billing/revenue", nil)
	rec := httptest.NewRecorder()

	s.adminBillingRevenue(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestAdminBillingRevenue_InvalidPeriod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/billing/revenue?period=hourly", nil)
	rec := httptest.NewRecorder()

	s.adminBillingRevenue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "invalid_period" {
		t.Errorf("error = %v, want invalid_period", resp["error"])
	}
}

func TestAdminBillingRevenue_InvalidFromDate(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/billing/revenue?from=not-a-date", nil)
	rec := httptest.NewRecorder()

	s.adminBillingRevenue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "invalid_from_date" {
		t.Errorf("error = %v, want invalid_from_date", resp["error"])
	}
}

func TestAdminBillingRevenue_InvalidToDate(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/billing/revenue?to=bad", nil)
	rec := httptest.NewRecorder()

	s.adminBillingRevenue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "invalid_to_date" {
		t.Errorf("error = %v, want invalid_to_date", resp["error"])
	}
}

func TestAdminBillingRevenue_DailySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Breakdown query
	breakdownRows := sqlmock.NewRows([]string{"period_date", "total_amount", "tx_count"}).
		AddRow("2024-01-15", 500.00, 12).
		AddRow("2024-01-16", 750.00, 18)
	mock.ExpectQuery("SELECT .+ FROM wallet_transactions").WillReturnRows(breakdownRows)

	// By type query
	typeRows := sqlmock.NewRows([]string{"type", "total"}).
		AddRow("purchase", -1200.00).
		AddRow("topup", 800.00).
		AddRow("refund", 200.00)
	mock.ExpectQuery("SELECT type, .+ FROM wallet_transactions").WillReturnRows(typeRows)

	// MRR query
	mrrRows := sqlmock.NewRows([]string{"mrr"}).AddRow(5000.00)
	mock.ExpectQuery("SELECT .+ FROM subscriptions").WillReturnRows(mrrRows)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/billing/revenue?period=daily&from=2024-01-01&to=2024-01-31", nil)
	rec := httptest.NewRecorder()

	s.adminBillingRevenue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	revenue, ok := resp["revenue"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'revenue' field")
	}

	if revenue["period"] != "daily" {
		t.Errorf("period = %v, want daily", revenue["period"])
	}
	if revenue["mrr"].(float64) != 5000.00 {
		t.Errorf("mrr = %v, want 5000", revenue["mrr"])
	}

	breakdown, ok := revenue["breakdown"].([]any)
	if !ok {
		t.Fatal("missing or invalid 'breakdown' field")
	}
	if len(breakdown) != 2 {
		t.Errorf("breakdown length = %d, want 2", len(breakdown))
	}

	byType, ok := revenue["by_type"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'by_type' field")
	}
	if byType["purchase"].(float64) != -1200.00 {
		t.Errorf("by_type.purchase = %v, want -1200", byType["purchase"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestAdminBillingRevenue_DefaultPeriod(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Empty results
	mock.ExpectQuery("SELECT .+ FROM wallet_transactions").
		WillReturnRows(sqlmock.NewRows([]string{"period_date", "total_amount", "tx_count"}))
	mock.ExpectQuery("SELECT type, .+ FROM wallet_transactions").
		WillReturnRows(sqlmock.NewRows([]string{"type", "total"}))
	mock.ExpectQuery("SELECT .+ FROM subscriptions").
		WillReturnRows(sqlmock.NewRows([]string{"mrr"}).AddRow(0))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/billing/revenue", nil)
	rec := httptest.NewRecorder()

	s.adminBillingRevenue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	revenue := resp["revenue"].(map[string]any)

	if revenue["period"] != "daily" {
		t.Errorf("default period = %v, want daily", revenue["period"])
	}
	if revenue["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", revenue["total"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
