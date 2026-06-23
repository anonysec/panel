//go:build !lite

package billing

import (
	"context"
	"fmt"
	"math"
	"time"
)

// DebtInfo represents a customer's debt status based on their wallet balance.
type DebtInfo struct {
	HasDebt           bool       `json:"has_debt"`
	OutstandingAmount float64    `json:"outstanding_amount"`
	BlockedSince      *time.Time `json:"blocked_since,omitempty"`
}

// GetDebtInfo checks whether a customer has a negative wallet balance (debt).
// A negative balance means the customer owes money and renewal is blocked.
func (b *BillingEngine) GetDebtInfo(ctx context.Context, customerID int64) (*DebtInfo, error) {
	var balance float64
	err := b.db.QueryRowContext(ctx, `
		SELECT COALESCE(wallet_balance, 0) FROM customers WHERE id = ? AND deleted_at IS NULL`,
		customerID,
	).Scan(&balance)
	if err != nil {
		return nil, fmt.Errorf("fetch customer %d balance: %w", customerID, err)
	}

	info := &DebtInfo{
		HasDebt:           balance < 0,
		OutstandingAmount: 0,
	}

	if balance < 0 {
		info.OutstandingAmount = math.Abs(balance)

		// Try to find when the balance first went negative by looking at the
		// earliest wallet transaction that brought the balance below zero.
		var blockedSince *time.Time
		var ts time.Time
		err := b.db.QueryRowContext(ctx, `
			SELECT MIN(created_at) FROM wallet_transactions
			WHERE customer_id = ? AND amount < 0
			ORDER BY created_at ASC LIMIT 1`,
			customerID,
		).Scan(&ts)
		if err == nil && !ts.IsZero() {
			blockedSince = &ts
			info.BlockedSince = blockedSince
		}
	}

	return info, nil
}
