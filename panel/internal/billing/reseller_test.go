//go:build !lite

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPurchaseResellerCredit_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	mock.ExpectQuery(`SELECT username FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("reseller1"))

	mock.ExpectExec(`UPDATE admins SET credit = credit \+ \$1 WHERE id = \$2`).
		WithArgs(500.00, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec("INSERT INTO reseller_transactions").
		WithArgs("reseller1", 500.00, "Credit purchase (ref: PAY-123)").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = engine.PurchaseResellerCredit(context.Background(), 1, 500.00, "PAY-123")
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

func TestPurchaseResellerCredit_NegativeAmount(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	err = engine.PurchaseResellerCredit(context.Background(), 1, -50.00, "REF")
	if err == nil {
		t.Fatal("expected error for negative amount, got nil")
	}
}

func TestPurchaseResellerCredit_ResellerNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery(`SELECT username FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}))

	err = engine.PurchaseResellerCredit(context.Background(), 99, 100.00, "REF")
	if err == nil {
		t.Fatal("expected error for missing reseller, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetResellerMargin_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT username FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("reseller1"))

	// Total purchased (allocations)
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(amount\), 0\) FROM reseller_transactions`).
		WithArgs("reseller1", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1000.00))

	// Total sold (deductions, stored as negative)
	mock.ExpectQuery(`SELECT COALESCE\(-SUM\(amount\), 0\) FROM reseller_transactions`).
		WithArgs("reseller1", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1500.00))

	// Active customer count
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM customers`).
		WithArgs("reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	info, err := engine.GetResellerMargin(context.Background(), 1, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.ResellerID != 1 {
		t.Errorf("expected reseller_id=1, got %d", info.ResellerID)
	}
	if info.TotalPurchased != 1000.00 {
		t.Errorf("expected total_purchased=1000, got %.2f", info.TotalPurchased)
	}
	if info.TotalSold != 1500.00 {
		t.Errorf("expected total_sold=1500, got %.2f", info.TotalSold)
	}
	if info.TotalMargin != 500.00 {
		t.Errorf("expected total_margin=500, got %.2f", info.TotalMargin)
	}
	// margin_percent = 500/1500 * 100 = 33.33...
	expectedPercent := (500.0 / 1500.0) * 100
	if info.MarginPercent < expectedPercent-0.01 || info.MarginPercent > expectedPercent+0.01 {
		t.Errorf("expected margin_percent=%.2f, got %.2f", expectedPercent, info.MarginPercent)
	}
	if info.CustomerCount != 10 {
		t.Errorf("expected customer_count=10, got %d", info.CustomerCount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetResellerMargin_ResellerNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery(`SELECT username FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}))

	_, err = engine.GetResellerMargin(context.Background(), 99, time.Now(), time.Now())
	if err == nil {
		t.Fatal("expected error for missing reseller, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetResellerMargin_ZeroSold(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT username FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("reseller1"))

	mock.ExpectQuery(`SELECT COALESCE\(SUM\(amount\), 0\) FROM reseller_transactions`).
		WithArgs("reseller1", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(500.00))

	mock.ExpectQuery(`SELECT COALESCE\(-SUM\(amount\), 0\) FROM reseller_transactions`).
		WithArgs("reseller1", from, to).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0.00))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM customers`).
		WithArgs("reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	info, err := engine.GetResellerMargin(context.Background(), 1, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.MarginPercent != 0 {
		t.Errorf("expected margin_percent=0 when no sales, got %.2f", info.MarginPercent)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestResellerCreateSubscription_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	// Fetch reseller
	mock.ExpectQuery(`SELECT username, COALESCE\(credit, 0\) FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "credit"}).AddRow("reseller1", 500.00))

	// Verify customer belongs to reseller
	mock.ExpectQuery(`SELECT username FROM customers WHERE id = \$1 AND created_by = \$2 AND deleted_at IS NULL`).
		WithArgs(int64(10), "reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("customer1"))

	// Check plan is allowed
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reseller_allowed_plans WHERE reseller_id = \$1 AND plan_id = \$2`).
		WithArgs(int64(1), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Fetch plan details
	mock.ExpectQuery(`SELECT name, price, data_gb, duration_days FROM plans WHERE id = \$1 AND is_active = TRUE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "price", "data_gb", "duration_days"}).AddRow("Basic VPN", 100.00, 50.0, 30))

	// Deduct credit
	mock.ExpectExec(`UPDATE admins SET credit = credit - \$`).
		WithArgs(100.00, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Record transaction
	mock.ExpectExec("INSERT INTO reseller_transactions").
		WithArgs("reseller1", -100.00, "Subscription for customer1: Basic VPN", "reseller1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Assign plan to customer
	mock.ExpectExec(`UPDATE customers SET plan_id = \$`).
		WithArgs(int64(5), 50.0, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = engine.ResellerCreateSubscription(context.Background(), 1, 10, 5)
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

func TestResellerCreateSubscription_InsufficientCredit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery(`SELECT username, COALESCE\(credit, 0\) FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "credit"}).AddRow("reseller1", 50.00))

	mock.ExpectQuery(`SELECT username FROM customers WHERE id = \$1 AND created_by = \$2 AND deleted_at IS NULL`).
		WithArgs(int64(10), "reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("customer1"))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reseller_allowed_plans WHERE reseller_id = \$1 AND plan_id = \$2`).
		WithArgs(int64(1), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT name, price, data_gb, duration_days FROM plans WHERE id = \$1 AND is_active = TRUE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "price", "data_gb", "duration_days"}).AddRow("Premium VPN", 200.00, 100.0, 30))

	err = engine.ResellerCreateSubscription(context.Background(), 1, 10, 5)
	if err == nil {
		t.Fatal("expected error for insufficient credit, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestResellerCreateSubscription_PlanNotAllowed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery(`SELECT username, COALESCE\(credit, 0\) FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "credit"}).AddRow("reseller1", 500.00))

	mock.ExpectQuery(`SELECT username FROM customers WHERE id = \$1 AND created_by = \$2 AND deleted_at IS NULL`).
		WithArgs(int64(10), "reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("customer1"))

	// Plan not allowed
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reseller_allowed_plans WHERE reseller_id = \$1 AND plan_id = \$2`).
		WithArgs(int64(1), int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	err = engine.ResellerCreateSubscription(context.Background(), 1, 10, 99)
	if err == nil {
		t.Fatal("expected error for plan not allowed, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestResellerCreateSubscription_CustomerNotOwned(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery(`SELECT username, COALESCE\(credit, 0\) FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "credit"}).AddRow("reseller1", 500.00))

	// Customer not found (not owned by this reseller)
	mock.ExpectQuery(`SELECT username FROM customers WHERE id = \$1 AND created_by = \$2 AND deleted_at IS NULL`).
		WithArgs(int64(99), "reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"username"}))

	err = engine.ResellerCreateSubscription(context.Background(), 1, 99, 5)
	if err == nil {
		t.Fatal("expected error for customer not owned, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestResellerCreateSubscription_FreePlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	var notified []string
	engine.SetNotify(func(msg string) { notified = append(notified, msg) })

	mock.ExpectQuery(`SELECT username, COALESCE\(credit, 0\) FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"username", "credit"}).AddRow("reseller1", 0.00))

	mock.ExpectQuery(`SELECT username FROM customers WHERE id = \$1 AND created_by = \$2 AND deleted_at IS NULL`).
		WithArgs(int64(10), "reseller1").
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("customer1"))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reseller_allowed_plans WHERE reseller_id = \$1 AND plan_id = \$2`).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Free plan (price=0)
	mock.ExpectQuery(`SELECT name, price, data_gb, duration_days FROM plans WHERE id = \$1 AND is_active = TRUE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "price", "data_gb", "duration_days"}).AddRow("Trial", 0.00, 5.0, 7))

	// No credit deduction for free plan — jump straight to plan assignment
	mock.ExpectExec(`UPDATE customers SET plan_id = \$`).
		WithArgs(int64(2), 5.0, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = engine.ResellerCreateSubscription(context.Background(), 1, 10, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPurchaseResellerCredit_ZeroAmount(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	err = engine.PurchaseResellerCredit(context.Background(), 1, 0, "REF")
	if err == nil {
		t.Fatal("expected error for zero amount, got nil")
	}
}

func TestGetResellerMargin_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)

	mock.ExpectQuery(`SELECT username FROM admins WHERE id = \$1 AND role = 'reseller'`).
		WithArgs(int64(1)).
		WillReturnError(fmt.Errorf("connection lost"))

	_, err = engine.GetResellerMargin(context.Background(), 1, time.Now(), time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
