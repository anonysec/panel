//go:build !lite

package payment

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestFormatInvoiceNumber(t *testing.T) {
	tests := []struct {
		name     string
		year     int
		month    int
		seq      int
		expected string
	}{
		{"first of january", 2025, 1, 1, "INV-202501-00001"},
		{"large sequence", 2025, 1, 42, "INV-202501-00042"},
		{"december", 2024, 12, 99999, "INV-202412-99999"},
		{"single digit month", 2025, 3, 5, "INV-202503-00005"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatInvoiceNumber(tt.year, tt.month, tt.seq)
			if got != tt.expected {
				t.Errorf("FormatInvoiceNumber(%d, %d, %d) = %q, want %q", tt.year, tt.month, tt.seq, got, tt.expected)
			}
		})
	}
}

func TestGenerateInvoiceNumber_FirstOfMonth(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	// No existing invoices this month
	mock.ExpectQuery("SELECT invoice_number FROM invoices").
		WillReturnRows(sqlmock.NewRows([]string{"invoice_number"}))

	got, err := GenerateInvoiceNumber(db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should start at 00001 — we just check the suffix
	if got == "" {
		t.Fatal("expected non-empty invoice number")
	}
	if !hasSequence(got, "00001") {
		t.Errorf("expected sequence 00001, got %q", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestGenerateInvoiceNumber_Increments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"invoice_number"}).
		AddRow("INV-202501-00007")

	mock.ExpectQuery("SELECT invoice_number FROM invoices").
		WillReturnRows(rows)

	got, err := GenerateInvoiceNumber(db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasSequence(got, "00008") {
		t.Errorf("expected sequence 00008, got %q", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func hasSequence(invoiceNum, seq string) bool {
	parts := splitInvoice(invoiceNum)
	if len(parts) != 3 {
		return false
	}
	return parts[2] == seq
}

func splitInvoice(s string) []string {
	// Split INV-YYYYMM-NNNNN into ["INV", "YYYYMM", "NNNNN"]
	parts := make([]string, 0, 3)
	idx := 0
	for i, c := range s {
		if c == '-' {
			parts = append(parts, s[idx:i])
			idx = i + 1
		}
	}
	parts = append(parts, s[idx:])
	return parts
}
