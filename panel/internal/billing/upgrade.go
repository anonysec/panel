//go:build !lite

package billing

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"
)

// UpgradePlan performs a pro-rated plan upgrade for a customer. It calculates
// the remaining credit on the current plan, applies it toward the new plan cost,
// creates an invoice, deducts the net cost from the wallet, updates the customer's
// plan_id, records a wallet transaction, and logs the change in plan_changes and events.
//
// The entire operation runs inside a database transaction.
func (b *BillingEngine) UpgradePlan(ctx context.Context, customerID int64, newPlanID int64) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upgrade tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch customer's current plan, username, and wallet balance
	var currentPlanID sql.NullInt64
	var username string
	var balance float64
	err = tx.QueryRowContext(ctx, `
		SELECT plan_id, username, COALESCE(wallet_balance, 0)
		FROM customers WHERE id = ? AND deleted_at IS NULL`, customerID,
	).Scan(&currentPlanID, &username, &balance)
	if err != nil {
		return fmt.Errorf("fetch customer %d: %w", customerID, err)
	}
	if !currentPlanID.Valid {
		return fmt.Errorf("customer %d has no active plan", customerID)
	}
	if currentPlanID.Int64 == newPlanID {
		return fmt.Errorf("customer %d is already on plan %d", customerID, newPlanID)
	}

	// 2. Fetch current plan info
	var currentPlan PlanInfo
	var currentDurationDays int
	var currentCurrency string
	err = tx.QueryRowContext(ctx, `
		SELECT id, name, price, duration_days, COALESCE(currency, 'IRR')
		FROM plans WHERE id = ?`, currentPlanID.Int64,
	).Scan(&currentPlan.ID, &currentPlan.Name, &currentPlan.Price, &currentDurationDays, &currentCurrency)
	if err != nil {
		return fmt.Errorf("fetch current plan %d: %w", currentPlanID.Int64, err)
	}

	// 3. Fetch new plan info
	var newPlan PlanInfo
	var newDurationDays int
	var newCurrency string
	var newPlanActive bool
	err = tx.QueryRowContext(ctx, `
		SELECT id, name, price, duration_days, COALESCE(currency, 'IRR'), is_active
		FROM plans WHERE id = ?`, newPlanID,
	).Scan(&newPlan.ID, &newPlan.Name, &newPlan.Price, &newDurationDays, &newCurrency, &newPlanActive)
	if err != nil {
		return fmt.Errorf("fetch new plan %d: %w", newPlanID, err)
	}
	if !newPlanActive {
		return fmt.Errorf("plan %d is not active", newPlanID)
	}

	// 4. Calculate days remaining on current subscription
	var expiresAt sql.NullTime
	var startedAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT started_at, expires_at
		FROM subscriptions
		WHERE customer_id = ? AND status = 'active'
		ORDER BY id DESC LIMIT 1`, customerID,
	).Scan(&startedAt, &expiresAt)
	if err != nil {
		return fmt.Errorf("fetch active subscription for customer %d: %w", customerID, err)
	}

	now := time.Now().UTC()
	var daysRemaining int
	var totalDays int

	if expiresAt.Valid {
		totalDays = currentDurationDays
		if totalDays <= 0 {
			// fallback: calculate from actual subscription duration
			totalDays = int(math.Ceil(expiresAt.Time.Sub(startedAt).Hours() / 24))
		}
		remaining := expiresAt.Time.Sub(now)
		daysRemaining = int(math.Ceil(remaining.Hours() / 24))
		if daysRemaining < 0 {
			daysRemaining = 0
		}
		if daysRemaining > totalDays {
			daysRemaining = totalDays
		}
	} else {
		// No expiry — treat as full period remaining
		totalDays = currentDurationDays
		daysRemaining = totalDays
	}

	// 5. Calculate pro-rated credit and upgrade cost
	// Credit = currentPlan.Price * (daysRemaining / totalDays)
	var creditRemaining float64
	if totalDays > 0 {
		creditRemaining = currentPlan.Price * (float64(daysRemaining) / float64(totalDays))
	}
	// Round to 2 decimal places
	creditRemaining = math.Round(creditRemaining*100) / 100

	// Net cost = new plan price - credit from unused current plan
	cost := newPlan.Price - creditRemaining
	cost = math.Round(cost*100) / 100

	// 6. Check wallet balance (only if cost > 0)
	if cost > 0 && balance < cost {
		return fmt.Errorf("insufficient balance for upgrade: need %.2f, have %.2f", cost, balance)
	}

	// If cost <= 0 (downgrade or credit exceeds new plan), set to 0 — no charge
	if cost < 0 {
		cost = 0
	}

	// 7. Create invoice for the upgrade
	inv := &Invoice{
		CustomerID:    customerID,
		InvoiceNumber: fmt.Sprintf("UP-%d-%d", customerID, now.Unix()),
		Amount:        cost,
		Currency:      newCurrency,
		Status:        "paid",
		Type:          "subscription",
		Description:   fmt.Sprintf("Plan upgrade: %s → %s (credit: %.2f)", currentPlan.Name, newPlan.Name, creditRemaining),
		PlanID:        &newPlanID,
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO invoices (customer_id, invoice_number, amount, currency, status, type, description, plan_id, gateway_id, payment_ref, pdf_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.CustomerID, inv.InvoiceNumber, inv.Amount, inv.Currency,
		inv.Status, inv.Type, inv.Description,
		inv.PlanID, inv.GatewayID, inv.PaymentRef, inv.PDFPath,
	)
	if err != nil {
		return fmt.Errorf("create upgrade invoice: %w", err)
	}
	invoiceID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get invoice id: %w", err)
	}
	inv.ID = invoiceID

	// 8. Deduct from wallet (only if cost > 0)
	if cost > 0 {
		_, err = tx.ExecContext(ctx, `
			UPDATE customers SET wallet_balance = wallet_balance - ? WHERE id = ?`,
			cost, customerID,
		)
		if err != nil {
			return fmt.Errorf("deduct wallet for upgrade: %w", err)
		}
	}

	// 9. Update customer's plan_id
	_, err = tx.ExecContext(ctx, `
		UPDATE customers SET plan_id = ? WHERE id = ?`,
		newPlanID, customerID,
	)
	if err != nil {
		return fmt.Errorf("update customer plan: %w", err)
	}

	// 10. Cancel old subscription, create new one
	_, err = tx.ExecContext(ctx, `
		UPDATE subscriptions SET status = 'cancelled'
		WHERE customer_id = ? AND status = 'active'`,
		customerID,
	)
	if err != nil {
		return fmt.Errorf("cancel old subscription: %w", err)
	}

	newExpires := now.AddDate(0, 0, newDurationDays)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO subscriptions (customer_id, username, plan_id, status, started_at, expires_at, paid_amount)
		VALUES (?, ?, ?, 'active', ?, ?, ?)`,
		customerID, username, newPlanID, now, newExpires, cost,
	)
	if err != nil {
		return fmt.Errorf("create new subscription: %w", err)
	}

	// 11. Record wallet transaction with adjustment details
	txDesc := fmt.Sprintf("Plan upgrade: %s → %s (credit %.2f applied)", currentPlan.Name, newPlan.Name, creditRemaining)
	if cost > 0 {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO wallet_transactions (customer_id, username, amount, type, description, actor, reference_type, reference_id, invoice_id)
			VALUES (?, ?, ?, 'purchase', ?, 'system', 'invoice', ?, ?)`,
			customerID, username, -cost, txDesc, invoiceID, invoiceID,
		)
		if err != nil {
			return fmt.Errorf("record wallet transaction: %w", err)
		}
	}

	// 12. Record in plan_changes table for audit
	changeType := "upgrade"
	if newPlan.Price < currentPlan.Price {
		changeType = "downgrade"
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO plan_changes (customer_id, username, old_plan_id, new_plan_id, change_type, prorated_credit, actor, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'system', NOW())`,
		customerID, username, currentPlanID.Int64, newPlanID, changeType, creditRemaining,
	)
	if err != nil {
		return fmt.Errorf("record plan change: %w", err)
	}

	// 13. Log in events table for audit trail
	_, err = tx.ExecContext(ctx, `
		INSERT INTO events (type, severity, title, message, actor, related)
		VALUES ('plan_upgrade', 'info', ?, ?, 'system', ?)`,
		fmt.Sprintf("Plan %s: %s → %s", changeType, currentPlan.Name, newPlan.Name),
		fmt.Sprintf("Customer %s upgraded from %s (%.2f) to %s (%.2f). Credit: %.2f, Charged: %.2f",
			username, currentPlan.Name, currentPlan.Price, newPlan.Name, newPlan.Price, creditRemaining, cost),
		username,
	)
	if err != nil {
		return fmt.Errorf("record upgrade event: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upgrade tx: %w", err)
	}

	b.notify(fmt.Sprintf("plan %s: customer %s (%s → %s), credit=%.2f, charged=%.2f",
		changeType, username, currentPlan.Name, newPlan.Name, creditRemaining, cost))
	log.Printf("[billing] plan %s completed for customer %d (%s), %s → %s, credit=%.2f, cost=%.2f",
		changeType, customerID, username, currentPlan.Name, newPlan.Name, creditRemaining, cost)

	return nil
}
