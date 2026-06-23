//go:build !lite

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunAutoRenewalCheck_NoCandidate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	// No customers expiring within 24h
	mock.ExpectQuery("SELECT DISTINCT c.id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err = engine.RunAutoRenewalCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunAutoRenewalCheck_ProcessesMultipleCustomers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	// Two customers need auto-renewal
	mock.ExpectQuery("SELECT DISTINCT c.id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10).AddRow(20))

	// ProcessAutoRenewal for customer 10
	mock.ExpectQuery("SELECT plan_id, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "wallet_balance"}).AddRow(1, 100.00))
	mock.ExpectQuery("SELECT name, price, COALESCE\\(currency, 'IRR'\\)").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "price", "currency"}).AddRow("Basic", 50.00, "IRR"))
	mock.ExpectExec("INSERT INTO invoices").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE customers SET wallet_balance").
		WithArgs(50.00, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO wallet_transactions").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// ProcessAutoRenewal for customer 20 — insufficient balance
	mock.ExpectQuery("SELECT plan_id, COALESCE\\(wallet_balance, 0\\)").
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "wallet_balance"}).AddRow(2, 10.00))
	mock.ExpectQuery("SELECT name, price, COALESCE\\(currency, 'IRR'\\)").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "price", "currency"}).AddRow("Premium", 200.00, "IRR"))

	err = engine.RunAutoRenewalCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Customer 10 should have been renewed, customer 20 should have failed gracefully
	if len(notified) < 2 {
		t.Fatalf("expected at least 2 notifications, got %d", len(notified))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunAutoRenewalCheck_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery("SELECT DISTINCT c.id").
		WillReturnError(fmt.Errorf("connection lost"))

	err = engine.RunAutoRenewalCheck(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSendExpiryWarnings_48hAnd24h(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	expiresIn36h := time.Now().Add(36 * time.Hour)

	// 48h query: one customer expiring in 36h
	mock.ExpectQuery("SELECT DISTINCT c.id, c.username, s.expires_at").
		WithArgs(48).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "expires_at"}).
			AddRow(10, "user1", expiresIn36h))

	// Check if already warned today — not warned yet
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM events").
		WithArgs("user1", "%48h%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Record the event
	mock.ExpectExec("INSERT INTO events").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 24h query: no customers yet within 24h window
	mock.ExpectQuery("SELECT DISTINCT c.id, c.username, s.expires_at").
		WithArgs(24).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "expires_at"}))

	err = engine.SendExpiryWarnings(context.Background())
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

func TestSendExpiryWarnings_SkipsDuplicates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	expiresIn20h := time.Now().Add(20 * time.Hour)

	// 48h query: customer already warned
	mock.ExpectQuery("SELECT DISTINCT c.id, c.username, s.expires_at").
		WithArgs(48).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "expires_at"}).
			AddRow(10, "user1", expiresIn20h))

	// Already sent 48h warning
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM events").
		WithArgs("user1", "%48h%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// 24h query: same customer, also already warned
	mock.ExpectQuery("SELECT DISTINCT c.id, c.username, s.expires_at").
		WithArgs(24).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "expires_at"}).
			AddRow(10, "user1", expiresIn20h))

	// Already sent 24h warning
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM events").
		WithArgs("user1", "%24h%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err = engine.SendExpiryWarnings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notified) != 0 {
		t.Fatalf("expected 0 notifications (all duplicates), got %d", len(notified))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
