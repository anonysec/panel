//go:build !lite

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetDebtInfo_NoDebt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery("SELECT COALESCE\\(wallet_balance, 0\\) FROM customers").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"wallet_balance"}).AddRow(100.00))

	info, err := engine.GetDebtInfo(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.HasDebt {
		t.Fatal("expected no debt")
	}
	if info.OutstandingAmount != 0 {
		t.Fatalf("expected outstanding_amount=0, got %f", info.OutstandingAmount)
	}
	if info.BlockedSince != nil {
		t.Fatal("expected blocked_since to be nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetDebtInfo_ZeroBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery("SELECT COALESCE\\(wallet_balance, 0\\) FROM customers").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"wallet_balance"}).AddRow(0.00))

	info, err := engine.GetDebtInfo(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.HasDebt {
		t.Fatal("expected no debt for zero balance")
	}
	if info.OutstandingAmount != 0 {
		t.Fatalf("expected outstanding_amount=0, got %f", info.OutstandingAmount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetDebtInfo_HasDebt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery("SELECT COALESCE\\(wallet_balance, 0\\) FROM customers").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"wallet_balance"}).AddRow(-25.50))

	blockedTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT MIN\\(created_at\\) FROM wallet_transactions").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"min_created_at"}).AddRow(blockedTime))

	info, err := engine.GetDebtInfo(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.HasDebt {
		t.Fatal("expected has_debt=true")
	}
	if info.OutstandingAmount != 25.50 {
		t.Fatalf("expected outstanding_amount=25.50, got %f", info.OutstandingAmount)
	}
	if info.BlockedSince == nil {
		t.Fatal("expected blocked_since to be set")
	}
	if !info.BlockedSince.Equal(blockedTime) {
		t.Fatalf("expected blocked_since=%v, got %v", blockedTime, *info.BlockedSince)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetDebtInfo_CustomerNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery("SELECT COALESCE\\(wallet_balance, 0\\) FROM customers").
		WithArgs(int64(999)).
		WillReturnError(fmt.Errorf("sql: no rows in result set"))

	_, err = engine.GetDebtInfo(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for non-existent customer")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
