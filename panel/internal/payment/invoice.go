//go:build !lite

package payment

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FormatInvoiceNumber formats year, month, and sequence into INV-YYYYMM-NNNNN.
func FormatInvoiceNumber(year int, month int, seq int) string {
	return fmt.Sprintf("INV-%04d%02d-%05d", year, month, seq)
}

// GenerateInvoiceNumber generates the next sequential invoice number for the current month.
// Format: INV-YYYYMM-NNNNN (e.g., INV-202501-00001).
func GenerateInvoiceNumber(db *sql.DB) (string, error) {
	now := time.Now().UTC()
	year := now.Year()
	month := int(now.Month())

	pattern := fmt.Sprintf("INV-%04d%02d-%%", year, month)

	var lastNumber string
	err := db.QueryRow(
		"SELECT invoice_number FROM invoices WHERE invoice_number LIKE ? ORDER BY invoice_number DESC LIMIT 1",
		pattern,
	).Scan(&lastNumber)

	if err == sql.ErrNoRows {
		return FormatInvoiceNumber(year, month, 1), nil
	}
	if err != nil {
		return "", fmt.Errorf("query latest invoice number: %w", err)
	}

	// Extract sequence from INV-YYYYMM-NNNNN
	parts := strings.Split(lastNumber, "-")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid invoice number format: %s", lastNumber)
	}

	seq, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("parse invoice sequence: %w", err)
	}

	return FormatInvoiceNumber(year, month, seq+1), nil
}
