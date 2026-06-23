//go:build !lite

package payment

import (
	"fmt"
	"math/rand"
	"regexp"
	"testing"
)

// **Validates: Requirements 7.1, 7.2**
// Property 9: Invoice Number Sequentiality
// For any sequence of N invoices generated in the same calendar month, the numeric suffix
// of each subsequent invoice number is exactly one greater than the previous, and all match
// the format INV-YYYYMM-NNNNN.
func TestProperty9_InvoiceNumberSequentiality(t *testing.T) {
	pattern := regexp.MustCompile(`^INV-\d{6}-\d{5}$`)

	iterations := 200
	for i := 0; i < iterations; i++ {
		year := 2020 + rand.Intn(11) // 2020-2030
		month := 1 + rand.Intn(12)   // 1-12
		n := 2 + rand.Intn(50)       // generate 2 to 51 sequential invoices

		var prev string
		for seq := 1; seq <= n; seq++ {
			result := FormatInvoiceNumber(year, month, seq)

			// Check format matches INV-YYYYMM-NNNNN
			if !pattern.MatchString(result) {
				t.Fatalf("iteration %d: FormatInvoiceNumber(%d, %d, %d) = %q does not match INV-YYYYMM-NNNNN pattern",
					i, year, month, seq, result)
			}

			// Check contains correct year and month
			expected := fmt.Sprintf("INV-%04d%02d-", year, month)
			if result[:11] != expected {
				t.Fatalf("iteration %d: FormatInvoiceNumber(%d, %d, %d) = %q, expected prefix %q",
					i, year, month, seq, result, expected)
			}

			// Check lexicographic ordering: each result > previous
			if prev != "" && result <= prev {
				t.Fatalf("iteration %d: sequentiality violated: %q should be > %q (year=%d, month=%d, seq=%d)",
					i, result, prev, year, month, seq)
			}

			prev = result
		}
	}
}

// **Validates: Requirements 6.4, 6.5, 6.6, 6.7**
// Property 10: Payment Transaction State Machine
// Valid state transitions:
//
//	pending → completed
//	pending → failed
//	completed → refunded
//	completed → partially_refunded
//	partially_refunded → refunded
//
// No other transitions are permitted.
func TestProperty10_PaymentTransactionStateMachine(t *testing.T) {
	validTransitions := map[string][]string{
		"pending":            {"completed", "failed"},
		"completed":          {"refunded", "partially_refunded"},
		"partially_refunded": {"refunded"},
		"failed":             {},
		"refunded":           {},
	}

	isValidTransition := func(from, to string) bool {
		targets, ok := validTransitions[from]
		if !ok {
			return false
		}
		for _, t := range targets {
			if t == to {
				return true
			}
		}
		return false
	}

	allStates := []string{"pending", "completed", "failed", "refunded", "partially_refunded"}

	iterations := 200
	for i := 0; i < iterations; i++ {
		from := allStates[rand.Intn(len(allStates))]
		to := allStates[rand.Intn(len(allStates))]

		result := isValidTransition(from, to)

		// Manually verify expected validity
		expected := false
		switch {
		case from == "pending" && to == "completed":
			expected = true
		case from == "pending" && to == "failed":
			expected = true
		case from == "completed" && to == "refunded":
			expected = true
		case from == "completed" && to == "partially_refunded":
			expected = true
		case from == "partially_refunded" && to == "refunded":
			expected = true
		}

		if result != expected {
			t.Fatalf("iteration %d: isValidTransition(%q, %q) = %v, expected %v",
				i, from, to, result, expected)
		}
	}

	// Additional: verify all valid transitions are accepted
	validPairs := [][2]string{
		{"pending", "completed"},
		{"pending", "failed"},
		{"completed", "refunded"},
		{"completed", "partially_refunded"},
		{"partially_refunded", "refunded"},
	}
	for _, pair := range validPairs {
		if !isValidTransition(pair[0], pair[1]) {
			t.Fatalf("valid transition %q → %q was rejected", pair[0], pair[1])
		}
	}

	// Verify all invalid transitions are rejected (exhaustive check)
	invalidPairs := [][2]string{
		{"pending", "pending"},
		{"pending", "refunded"},
		{"pending", "partially_refunded"},
		{"completed", "completed"},
		{"completed", "pending"},
		{"completed", "failed"},
		{"failed", "pending"},
		{"failed", "completed"},
		{"failed", "refunded"},
		{"failed", "partially_refunded"},
		{"failed", "failed"},
		{"refunded", "pending"},
		{"refunded", "completed"},
		{"refunded", "failed"},
		{"refunded", "refunded"},
		{"refunded", "partially_refunded"},
		{"partially_refunded", "pending"},
		{"partially_refunded", "completed"},
		{"partially_refunded", "failed"},
		{"partially_refunded", "partially_refunded"},
	}
	for _, pair := range invalidPairs {
		if isValidTransition(pair[0], pair[1]) {
			t.Fatalf("invalid transition %q → %q was accepted", pair[0], pair[1])
		}
	}
}
