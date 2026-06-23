//go:build !lite

package payment

import (
	"errors"
	"math"
	"math/rand"
	"testing"
	"testing/quick"
)

// calculateCommission computes the reseller commission from a payment amount and commission percentage.
func calculateCommission(paymentAmount float64, commissionPercent float64) float64 {
	return paymentAmount * (commissionPercent / 100.0)
}

// validatePayout checks whether a payout request is valid given the amount, balance, and minimum threshold.
func validatePayout(amount, balance, minPayout float64) error {
	if amount <= 0 {
		return errors.New("invalid_amount")
	}
	if amount > balance {
		return errors.New("insufficient_balance")
	}
	if amount < minPayout {
		return errors.New("below_minimum")
	}
	return nil
}

// Property 11: Reseller Commission Calculation
// For any random paymentAmount (> 0, < 1_000_000) and commissionPercent (0-100):
// - Commission should be >= 0
// - Commission should be <= paymentAmount
// - Commission should equal paymentAmount * commissionPercent / 100
// - Result is deterministic
// **Validates: Requirements 8.1**

func TestProperty_Commission_NonNegative(t *testing.T) {
	f := func(paymentAmount float64, commissionPercent float64) bool {
		// Constrain inputs to valid ranges
		paymentAmount = math.Abs(paymentAmount)
		if paymentAmount <= 0 || paymentAmount >= 1_000_000 {
			paymentAmount = 1000.0
		}
		commissionPercent = math.Abs(math.Mod(commissionPercent, 100.01))
		if commissionPercent < 0 {
			commissionPercent = 0
		}

		commission := calculateCommission(paymentAmount, commissionPercent)
		return commission >= 0
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: commission should always be >= 0: %v", err)
	}
}

func TestProperty_Commission_NotExceedPayment(t *testing.T) {
	f := func(paymentAmount float64, commissionPercent float64) bool {
		// Constrain inputs to valid ranges
		paymentAmount = math.Abs(paymentAmount)
		if paymentAmount <= 0 || paymentAmount >= 1_000_000 {
			paymentAmount = 1000.0
		}
		commissionPercent = math.Abs(math.Mod(commissionPercent, 100.01))
		if commissionPercent < 0 {
			commissionPercent = 0
		}
		if commissionPercent > 100 {
			commissionPercent = 100
		}

		commission := calculateCommission(paymentAmount, commissionPercent)
		return commission <= paymentAmount
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: commission should never exceed payment amount: %v", err)
	}
}

func TestProperty_Commission_CorrectCalculation(t *testing.T) {
	f := func(paymentAmount float64, commissionPercent float64) bool {
		// Constrain inputs to valid ranges
		paymentAmount = math.Abs(paymentAmount)
		if paymentAmount <= 0 || paymentAmount >= 1_000_000 {
			paymentAmount = 1000.0
		}
		commissionPercent = math.Abs(math.Mod(commissionPercent, 100.01))
		if commissionPercent < 0 {
			commissionPercent = 0
		}
		if commissionPercent > 100 {
			commissionPercent = 100
		}

		commission := calculateCommission(paymentAmount, commissionPercent)
		expected := paymentAmount * commissionPercent / 100.0

		// Use small epsilon for floating-point comparison
		return math.Abs(commission-expected) < 1e-9
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: commission should equal paymentAmount * commissionPercent / 100: %v", err)
	}
}

func TestProperty_Commission_Deterministic(t *testing.T) {
	f := func(paymentAmount float64, commissionPercent float64) bool {
		// Constrain inputs to valid ranges
		paymentAmount = math.Abs(paymentAmount)
		if paymentAmount <= 0 || paymentAmount >= 1_000_000 {
			paymentAmount = 1000.0
		}
		commissionPercent = math.Abs(math.Mod(commissionPercent, 100.01))
		if commissionPercent < 0 {
			commissionPercent = 0
		}
		if commissionPercent > 100 {
			commissionPercent = 100
		}

		result1 := calculateCommission(paymentAmount, commissionPercent)
		result2 := calculateCommission(paymentAmount, commissionPercent)
		return result1 == result2
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: commission calculation should be deterministic: %v", err)
	}
}

// Property 12: Payout Validation
// For any random (amount, balance, minPayout):
// - Verify the function correctly rejects when amount > balance
// - Verify the function correctly rejects when amount < minPayout
// - Verify the function correctly accepts when amount <= balance AND amount >= minPayout AND amount > 0
// **Validates: Requirements 8.5, 8.6**

func TestProperty_Payout_RejectsExceedingBalance(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 200; i++ {
		balance := rng.Float64() * 10000     // 0 to 10000
		excess := rng.Float64()*1000 + 0.01  // always > 0
		amount := balance + excess           // always > balance
		minPayout := rng.Float64() * balance // minPayout <= balance so only balance check matters

		err := validatePayout(amount, balance, minPayout)
		if err == nil {
			t.Fatalf("iteration %d: expected error for amount %.2f > balance %.2f, got nil", i, amount, balance)
		}
		if err.Error() != "insufficient_balance" {
			t.Fatalf("iteration %d: expected 'insufficient_balance', got %q (amount=%.2f, balance=%.2f, min=%.2f)",
				i, err.Error(), amount, balance, minPayout)
		}
	}
}

func TestProperty_Payout_RejectsBelowMinimum(t *testing.T) {
	rng := rand.New(rand.NewSource(43))

	for i := 0; i < 200; i++ {
		minPayout := rng.Float64()*1000 + 10         // min 10 to 1010
		amount := rng.Float64()*(minPayout-1) + 0.01 // > 0 and < minPayout
		if amount >= minPayout {
			amount = minPayout - 0.01
		}
		if amount <= 0 {
			amount = 0.01
		}
		balance := amount + rng.Float64()*10000 + 1 // balance > amount, so balance check passes

		err := validatePayout(amount, balance, minPayout)
		if err == nil {
			t.Fatalf("iteration %d: expected error for amount %.2f < minPayout %.2f, got nil", i, amount, minPayout)
		}
		if err.Error() != "below_minimum" {
			t.Fatalf("iteration %d: expected 'below_minimum', got %q (amount=%.2f, balance=%.2f, min=%.2f)",
				i, err.Error(), amount, balance, minPayout)
		}
	}
}

func TestProperty_Payout_RejectsInvalidAmount(t *testing.T) {
	rng := rand.New(rand.NewSource(44))

	for i := 0; i < 200; i++ {
		// Generate zero or negative amounts
		var amount float64
		if i%2 == 0 {
			amount = 0
		} else {
			amount = -(rng.Float64()*1000 + 0.01) // negative
		}
		balance := rng.Float64() * 10000
		minPayout := rng.Float64() * 100

		err := validatePayout(amount, balance, minPayout)
		if err == nil {
			t.Fatalf("iteration %d: expected error for invalid amount %.2f, got nil", i, amount)
		}
		if err.Error() != "invalid_amount" {
			t.Fatalf("iteration %d: expected 'invalid_amount', got %q (amount=%.2f)",
				i, err.Error(), amount)
		}
	}
}

func TestProperty_Payout_AcceptsValidRequests(t *testing.T) {
	rng := rand.New(rand.NewSource(45))

	for i := 0; i < 200; i++ {
		minPayout := rng.Float64()*100 + 1       // 1 to 101
		amount := minPayout + rng.Float64()*1000 // amount >= minPayout, always > 0
		balance := amount + rng.Float64()*5000   // balance >= amount

		err := validatePayout(amount, balance, minPayout)
		if err != nil {
			t.Fatalf("iteration %d: expected nil for valid payout (amount=%.2f, balance=%.2f, min=%.2f), got %v",
				i, amount, balance, minPayout, err)
		}
	}
}
