//go:build !lite

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpgradePlan_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	now := time.Now().UTC()
	startedAt := now.AddDate(0, 0, -15)
	expiresAt := now.AddDate(0, 0, 15) // 15 days remaining of 30

	mock.ExpectBegin()

	// Fetch customer
	mock.ExpectQuery("SELECT plan_id, username, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "username", "wallet_balance"}).
			AddRow(10, "user1", 50.00))

	// Fetch current plan
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\)").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency"}).
			AddRow(10, "Basic", 10.00, 30, "IRR"))

	// Fetch new plan
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\), is_active").
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency", "is_active"}).
			AddRow(20, "Premium", 20.00, 30, "IRR", true))

	// Fetch active subscription
	mock.ExpectQuery("SELECT started_at, expires_at").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"started_at", "expires_at"}).
			AddRow(startedAt, expiresAt))

	// Create invoice
	mock.ExpectExec("INSERT INTO invoices").
		WillReturnResult(sqlmock.NewResult(100, 1))

	// Deduct from wallet (cost = 20 - 10*(15/30) = 20 - 5 = 15)
	mock.ExpectExec("UPDATE customers SET wallet_balance").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Update customer plan_id
	mock.ExpectExec("UPDATE customers SET plan_id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Cancel old subscription
	mock.ExpectExec("UPDATE subscriptions SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Create new subscription
	mock.ExpectExec("INSERT INTO subscriptions").
		WillReturnResult(sqlmock.NewResult(200, 1))

	// Record wallet transaction
	mock.ExpectExec("INSERT INTO wallet_transactions").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Record plan change
	mock.ExpectExec("INSERT INTO plan_changes").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Record event
	mock.ExpectExec("INSERT INTO events").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err = engine.UpgradePlan(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notified) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notified))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpgradePlan_InsufficientBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	now := time.Now().UTC()
	startedAt := now.AddDate(0, 0, -15)
	expiresAt := now.AddDate(0, 0, 15)

	mock.ExpectBegin()

	// Fetch customer — low balance
	mock.ExpectQuery("SELECT plan_id, username, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "username", "wallet_balance"}).
			AddRow(10, "user1", 5.00))

	// Fetch current plan
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\)").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency"}).
			AddRow(10, "Basic", 10.00, 30, "IRR"))

	// Fetch new plan — expensive
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\), is_active").
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency", "is_active"}).
			AddRow(20, "Premium", 100.00, 30, "IRR", true))

	// Fetch subscription
	mock.ExpectQuery("SELECT started_at, expires_at").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"started_at", "expires_at"}).
			AddRow(startedAt, expiresAt))

	mock.ExpectRollback()

	err = engine.UpgradePlan(context.Background(), 1, 20)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != fmt.Sprintf("insufficient balance for upgrade: need %.2f, have %.2f", 95.00, 5.00) {
		t.Fatalf("unexpected error message: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpgradePlan_NoPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectBegin()

	// Customer has no plan
	mock.ExpectQuery("SELECT plan_id, username, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "username", "wallet_balance"}).
			AddRow(nil, "user1", 50.00))

	mock.ExpectRollback()

	err = engine.UpgradePlan(context.Background(), 1, 20)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "customer 1 has no active plan" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradePlan_SamePlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectBegin()

	// Customer is already on plan 10
	mock.ExpectQuery("SELECT plan_id, username, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "username", "wallet_balance"}).
			AddRow(10, "user1", 50.00))

	mock.ExpectRollback()

	err = engine.UpgradePlan(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "customer 1 is already on plan 10" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradePlan_InactivePlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectBegin()

	// Fetch customer
	mock.ExpectQuery("SELECT plan_id, username, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "username", "wallet_balance"}).
			AddRow(10, "user1", 50.00))

	// Current plan
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\)").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency"}).
			AddRow(10, "Basic", 10.00, 30, "IRR"))

	// New plan is inactive
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\), is_active").
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency", "is_active"}).
			AddRow(20, "Legacy", 5.00, 30, "IRR", false))

	mock.ExpectRollback()

	err = engine.UpgradePlan(context.Background(), 1, 20)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "plan 20 is not active" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradePlan_ZeroCostDowngrade(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	now := time.Now().UTC()
	startedAt := now.AddDate(0, 0, -5)
	expiresAt := now.AddDate(0, 0, 25) // 25 of 30 days remaining

	mock.ExpectBegin()

	// Fetch customer
	mock.ExpectQuery("SELECT plan_id, username, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "username", "wallet_balance"}).
			AddRow(10, "user1", 5.00))

	// Fetch current plan — expensive plan
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\)").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency"}).
			AddRow(10, "Premium", 30.00, 30, "IRR"))

	// Fetch new plan — cheaper plan
	mock.ExpectQuery("SELECT id, name, price, duration_days, COALESCE\\(currency, 'IRR'\\), is_active").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "price", "duration_days", "currency", "is_active"}).
			AddRow(5, "Basic", 10.00, 30, "IRR", true))

	// Fetch subscription
	mock.ExpectQuery("SELECT started_at, expires_at").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"started_at", "expires_at"}).
			AddRow(startedAt, expiresAt))

	// Cost = 10 - 30*(25/30) = 10 - 25 = -15 → clamped to 0
	// Create invoice with amount 0
	mock.ExpectExec("INSERT INTO invoices").
		WillReturnResult(sqlmock.NewResult(100, 1))

	// No wallet deduction (cost=0)

	// Update customer plan_id
	mock.ExpectExec("UPDATE customers SET plan_id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Cancel old subscription
	mock.ExpectExec("UPDATE subscriptions SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Create new subscription
	mock.ExpectExec("INSERT INTO subscriptions").
		WillReturnResult(sqlmock.NewResult(200, 1))

	// No wallet transaction (cost=0)

	// Record plan change
	mock.ExpectExec("INSERT INTO plan_changes").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Record event
	mock.ExpectExec("INSERT INTO events").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err = engine.UpgradePlan(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notified) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notified))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
