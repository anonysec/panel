//go:build !lite

// Package billing provides the billing engine for KorisPanel.
// It handles invoice management, payment gateway integration,
// pro-rated plan upgrades, auto-renewal, and data pack purchases.
package billing

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// PaymentGateway defines the interface for payment providers.
// Implementations include manual, Zarinpal, crypto, and Stripe gateways.
type PaymentGateway interface {
	// Name returns the unique identifier for this gateway.
	Name() string
	// CreatePayment initiates a new payment request.
	CreatePayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)
	// VerifyPayment checks whether a payment reference has been completed.
	VerifyPayment(ctx context.Context, ref string) (*PaymentVerification, error)
	// RefundPayment issues a refund for the given reference and amount.
	RefundPayment(ctx context.Context, ref string, amount float64) error
}

// Invoice represents a billing invoice linked to a customer.
type Invoice struct {
	ID            int64      `json:"id"`
	CustomerID    int64      `json:"customer_id"`
	InvoiceNumber string     `json:"invoice_number"`
	Amount        float64    `json:"amount"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"` // draft, paid, cancelled, refunded
	Type          string     `json:"type"`   // subscription, topup, data_pack, refund
	Description   string     `json:"description"`
	PlanID        *int64     `json:"plan_id,omitempty"`
	GatewayID     *int64     `json:"gateway_id,omitempty"`
	PaymentRef    string     `json:"payment_ref,omitempty"`
	PDFPath       string     `json:"pdf_path,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
}

// PaymentRequest holds the details needed to initiate a payment.
type PaymentRequest struct {
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Description string  `json:"description"`
	CallbackURL string  `json:"callback_url"`
	CustomerID  int64   `json:"customer_id"`
}

// PaymentResponse is returned after successfully creating a payment.
type PaymentResponse struct {
	PaymentURL string `json:"payment_url"` // redirect URL for online payment
	Reference  string `json:"reference"`   // internal reference
}

// PaymentVerification contains the result of verifying a payment.
type PaymentVerification struct {
	Verified  bool    `json:"verified"`
	Reference string  `json:"reference"`
	Amount    float64 `json:"amount"`
}

// DataPack represents a purchasable data add-on.
type DataPack struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	DataGB   int     `json:"data_gb"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	IsActive bool    `json:"is_active"`
}

// PlanInfo holds basic plan details used for pro-rating calculations.
type PlanInfo struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

// BillingEngine orchestrates billing operations including invoices,
// payment gateways, auto-renewal, and data pack purchases.
type BillingEngine struct {
	db       *sql.DB
	gateways map[string]PaymentGateway
	notify   func(msg string)
}

// New creates a new BillingEngine with the given database connection.
func New(db *sql.DB) *BillingEngine {
	return &BillingEngine{
		db:       db,
		gateways: make(map[string]PaymentGateway),
		notify:   func(msg string) { log.Printf("[billing] %s", msg) },
	}
}

// SetNotify sets a custom notification function for billing events.
func (b *BillingEngine) SetNotify(fn func(msg string)) {
	if fn != nil {
		b.notify = fn
	}
}

// RegisterGateway registers a payment gateway for use in billing operations.
func (b *BillingEngine) RegisterGateway(gw PaymentGateway) {
	if gw == nil {
		return
	}
	b.gateways[gw.Name()] = gw
	log.Printf("[billing] registered payment gateway: %s", gw.Name())
}

// CreateInvoice inserts a new invoice into the database.
// The invoice ID and CreatedAt fields are populated on success.
func (b *BillingEngine) CreateInvoice(ctx context.Context, inv *Invoice) error {
	if inv == nil {
		return fmt.Errorf("invoice is nil")
	}

	result, err := b.db.ExecContext(ctx, `
		INSERT INTO invoices (customer_id, invoice_number, amount, currency, status, type, description, plan_id, gateway_id, payment_ref, pdf_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.CustomerID, inv.InvoiceNumber, inv.Amount, inv.Currency,
		inv.Status, inv.Type, inv.Description,
		inv.PlanID, inv.GatewayID, inv.PaymentRef, inv.PDFPath,
	)
	if err != nil {
		return fmt.Errorf("insert invoice: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get invoice id: %w", err)
	}
	inv.ID = id
	inv.CreatedAt = time.Now().UTC()

	log.Printf("[billing] created invoice %s for customer %d, amount=%.2f %s",
		inv.InvoiceNumber, inv.CustomerID, inv.Amount, inv.Currency)
	return nil
}

// MarkInvoicePaid updates an invoice status to "paid" and records the payment reference.
func (b *BillingEngine) MarkInvoicePaid(ctx context.Context, invoiceID int64, paymentRef string) error {
	now := time.Now().UTC()
	_, err := b.db.ExecContext(ctx, `
		UPDATE invoices SET status = 'paid', payment_ref = ?, paid_at = ? WHERE id = ?`,
		paymentRef, now, invoiceID,
	)
	if err != nil {
		return fmt.Errorf("mark invoice paid: %w", err)
	}

	log.Printf("[billing] invoice %d marked paid, ref=%s", invoiceID, paymentRef)
	return nil
}

// GetInvoice retrieves a single invoice by ID.
func (b *BillingEngine) GetInvoice(ctx context.Context, id int64) (*Invoice, error) {
	inv := &Invoice{}
	var planID, gatewayID sql.NullInt64
	var paidAt sql.NullTime
	var description, paymentRef, pdfPath sql.NullString

	err := b.db.QueryRowContext(ctx, `
		SELECT id, customer_id, invoice_number, amount, currency, status, type,
		       description, plan_id, gateway_id, payment_ref, pdf_path, created_at, paid_at
		FROM invoices WHERE id = ?`, id,
	).Scan(
		&inv.ID, &inv.CustomerID, &inv.InvoiceNumber, &inv.Amount, &inv.Currency,
		&inv.Status, &inv.Type, &description, &planID, &gatewayID,
		&paymentRef, &pdfPath, &inv.CreatedAt, &paidAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get invoice %d: %w", id, err)
	}

	if planID.Valid {
		inv.PlanID = &planID.Int64
	}
	if gatewayID.Valid {
		inv.GatewayID = &gatewayID.Int64
	}
	if paidAt.Valid {
		inv.PaidAt = &paidAt.Time
	}
	if description.Valid {
		inv.Description = description.String
	}
	if paymentRef.Valid {
		inv.PaymentRef = paymentRef.String
	}
	if pdfPath.Valid {
		inv.PDFPath = pdfPath.String
	}

	return inv, nil
}

// ListInvoices retrieves recent invoices for a customer, ordered by creation date descending.
func (b *BillingEngine) ListInvoices(ctx context.Context, customerID int64, limit int) ([]Invoice, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := b.db.QueryContext(ctx, `
		SELECT id, customer_id, invoice_number, amount, currency, status, type,
		       description, plan_id, gateway_id, payment_ref, pdf_path, created_at, paid_at
		FROM invoices WHERE customer_id = ?
		ORDER BY created_at DESC LIMIT ?`, customerID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()

	var invoices []Invoice
	for rows.Next() {
		var inv Invoice
		var planID, gatewayID sql.NullInt64
		var paidAt sql.NullTime
		var description, paymentRef, pdfPath sql.NullString

		if err := rows.Scan(
			&inv.ID, &inv.CustomerID, &inv.InvoiceNumber, &inv.Amount, &inv.Currency,
			&inv.Status, &inv.Type, &description, &planID, &gatewayID,
			&paymentRef, &pdfPath, &inv.CreatedAt, &paidAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoice row: %w", err)
		}

		if planID.Valid {
			inv.PlanID = &planID.Int64
		}
		if gatewayID.Valid {
			inv.GatewayID = &gatewayID.Int64
		}
		if paidAt.Valid {
			inv.PaidAt = &paidAt.Time
		}
		if description.Valid {
			inv.Description = description.String
		}
		if paymentRef.Valid {
			inv.PaymentRef = paymentRef.String
		}
		if pdfPath.Valid {
			inv.PDFPath = pdfPath.String
		}

		invoices = append(invoices, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoices: %w", err)
	}

	return invoices, nil
}

// ProrateUpgrade calculates the credit amount when upgrading from one plan to another.
// It returns the price difference adjusted for the remaining billing period.
// Formula: (newPlan.Price - currentPlan.Price) * (daysRemaining / totalDays)
func (b *BillingEngine) ProrateUpgrade(currentPlan, newPlan PlanInfo, daysRemaining, totalDays int) float64 {
	if totalDays <= 0 {
		return 0
	}
	if daysRemaining <= 0 {
		return newPlan.Price - currentPlan.Price
	}
	if daysRemaining > totalDays {
		daysRemaining = totalDays
	}

	priceDiff := newPlan.Price - currentPlan.Price
	ratio := float64(daysRemaining) / float64(totalDays)
	return priceDiff * ratio
}

// ProcessAutoRenewal attempts to renew a customer's subscription by deducting
// from their wallet balance. It creates an invoice and updates the subscription.
func (b *BillingEngine) ProcessAutoRenewal(ctx context.Context, customerID int64) error {
	// Fetch customer's current plan and wallet balance
	var planID sql.NullInt64
	var balance float64
	err := b.db.QueryRowContext(ctx, `
		SELECT plan_id, COALESCE(wallet_balance, 0)
		FROM customers WHERE id = ? AND deleted_at IS NULL`, customerID,
	).Scan(&planID, &balance)
	if err != nil {
		return fmt.Errorf("fetch customer %d: %w", customerID, err)
	}
	if !planID.Valid {
		return fmt.Errorf("customer %d has no active plan", customerID)
	}

	// Fetch plan price
	var planPrice float64
	var planName string
	var currency string
	err = b.db.QueryRowContext(ctx, `
		SELECT name, price, COALESCE(currency, 'IRR') FROM plans WHERE id = ?`, planID.Int64,
	).Scan(&planName, &planPrice, &currency)
	if err != nil {
		return fmt.Errorf("fetch plan %d: %w", planID.Int64, err)
	}

	// Check sufficient balance
	if balance < planPrice {
		// Log debt event if balance is negative (customer in debt)
		if balance < 0 {
			b.notify(fmt.Sprintf("customer %d is in debt (balance=%.2f), renewal blocked",
				customerID, balance))
			_, _ = b.db.ExecContext(ctx, `
				INSERT INTO events (type, severity, title, message, actor, related)
				VALUES ('billing_debt', 'warning', ?, ?, 'system', ?)`,
				fmt.Sprintf("Renewal blocked: customer %d in debt", customerID),
				fmt.Sprintf("Customer has outstanding debt of %.2f %s. Renewal for plan %s blocked.",
					-balance, currency, planName),
				fmt.Sprintf("%d", customerID))
		}
		b.notify(fmt.Sprintf("auto-renewal failed for customer %d: insufficient balance (%.2f < %.2f)",
			customerID, balance, planPrice))
		return fmt.Errorf("insufficient balance for customer %d: %.2f < %.2f",
			customerID, balance, planPrice)
	}

	// Create renewal invoice
	inv := &Invoice{
		CustomerID:    customerID,
		InvoiceNumber: fmt.Sprintf("AR-%d-%d", customerID, time.Now().Unix()),
		Amount:        planPrice,
		Currency:      currency,
		Status:        "paid",
		Type:          "subscription",
		Description:   fmt.Sprintf("Auto-renewal: %s", planName),
		PlanID:        &planID.Int64,
	}
	if err := b.CreateInvoice(ctx, inv); err != nil {
		return fmt.Errorf("create renewal invoice: %w", err)
	}

	// Deduct from wallet
	_, err = b.db.ExecContext(ctx, `
		UPDATE customers SET wallet_balance = wallet_balance - ? WHERE id = ?`,
		planPrice, customerID,
	)
	if err != nil {
		return fmt.Errorf("deduct wallet: %w", err)
	}

	// Record wallet transaction
	_, err = b.db.ExecContext(ctx, `
		INSERT INTO wallet_transactions (customer_id, invoice_id, amount, type, description, created_at)
		VALUES (?, ?, ?, 'debit', ?, NOW())`,
		customerID, inv.ID, -planPrice, fmt.Sprintf("Auto-renewal: %s", planName),
	)
	if err != nil {
		return fmt.Errorf("record wallet transaction: %w", err)
	}

	b.notify(fmt.Sprintf("auto-renewal successful for customer %d, plan=%s, amount=%.2f %s",
		customerID, planName, planPrice, currency))
	log.Printf("[billing] auto-renewal completed for customer %d, invoice=%s", customerID, inv.InvoiceNumber)
	return nil
}

// PurchaseDataPack processes a data pack purchase for a customer.
// It deducts the cost from the wallet and adds the data to the customer's allowance.
func (b *BillingEngine) PurchaseDataPack(ctx context.Context, customerID int64, packID int64) error {
	// Fetch data pack details
	var pack DataPack
	err := b.db.QueryRowContext(ctx, `
		SELECT id, name, data_gb, price, currency, is_active
		FROM data_packs WHERE id = ?`, packID,
	).Scan(&pack.ID, &pack.Name, &pack.DataGB, &pack.Price, &pack.Currency, &pack.IsActive)
	if err != nil {
		return fmt.Errorf("fetch data pack %d: %w", packID, err)
	}
	if !pack.IsActive {
		return fmt.Errorf("data pack %d is not active", packID)
	}

	// Check wallet balance
	var balance float64
	err = b.db.QueryRowContext(ctx, `
		SELECT COALESCE(wallet_balance, 0) FROM customers WHERE id = ? AND deleted_at IS NULL`,
		customerID,
	).Scan(&balance)
	if err != nil {
		return fmt.Errorf("fetch customer balance: %w", err)
	}
	if balance < pack.Price {
		return fmt.Errorf("insufficient balance for data pack: %.2f < %.2f", balance, pack.Price)
	}

	// Create invoice
	inv := &Invoice{
		CustomerID:    customerID,
		InvoiceNumber: fmt.Sprintf("DP-%d-%d", customerID, time.Now().Unix()),
		Amount:        pack.Price,
		Currency:      pack.Currency,
		Status:        "paid",
		Type:          "data_pack",
		Description:   fmt.Sprintf("Data Pack: %s (%d GB)", pack.Name, pack.DataGB),
	}
	if err := b.CreateInvoice(ctx, inv); err != nil {
		return fmt.Errorf("create data pack invoice: %w", err)
	}

	// Deduct from wallet
	_, err = b.db.ExecContext(ctx, `
		UPDATE customers SET wallet_balance = wallet_balance - ? WHERE id = ?`,
		pack.Price, customerID,
	)
	if err != nil {
		return fmt.Errorf("deduct wallet for data pack: %w", err)
	}

	// Add data to customer's allowance
	_, err = b.db.ExecContext(ctx, `
		UPDATE customers SET data_limit_gb = COALESCE(data_limit_gb, 0) + ? WHERE id = ?`,
		pack.DataGB, customerID,
	)
	if err != nil {
		return fmt.Errorf("add data allowance: %w", err)
	}

	// Record wallet transaction
	_, err = b.db.ExecContext(ctx, `
		INSERT INTO wallet_transactions (customer_id, invoice_id, amount, type, description, created_at)
		VALUES (?, ?, ?, 'debit', ?, NOW())`,
		customerID, inv.ID, -pack.Price, fmt.Sprintf("Data Pack: %s", pack.Name),
	)
	if err != nil {
		return fmt.Errorf("record data pack transaction: %w", err)
	}

	b.notify(fmt.Sprintf("data pack purchased: customer %d, pack=%s (%d GB), cost=%.2f %s",
		customerID, pack.Name, pack.DataGB, pack.Price, pack.Currency))
	log.Printf("[billing] data pack purchase completed for customer %d, pack=%s", customerID, pack.Name)
	return nil
}
