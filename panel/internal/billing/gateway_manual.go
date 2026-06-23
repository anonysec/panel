//go:build !lite

package billing

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ManualGateway handles manual bank transfer payments.
// An admin approves payments manually after the customer sends the transfer.
type ManualGateway struct {
	bankName      string
	accountNumber string
	holderName    string
}

// NewManualGateway creates a ManualGateway from a config map.
// Expected keys: bank_name, account_number, holder_name.
func NewManualGateway(config map[string]string) *ManualGateway {
	return &ManualGateway{
		bankName:      config["bank_name"],
		accountNumber: config["account_number"],
		holderName:    config["holder_name"],
	}
}

// Name returns the gateway identifier.
func (g *ManualGateway) Name() string {
	return "manual"
}

// CreatePayment returns a reference ID and bank transfer instructions.
// The customer must transfer the amount to the bank details provided.
func (g *ManualGateway) CreatePayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error) {
	ref := fmt.Sprintf("MAN-%d-%d", req.CustomerID, time.Now().UnixNano())

	instructions := fmt.Sprintf(
		"Please transfer %.2f %s to:\nBank: %s\nAccount: %s\nHolder: %s\nReference: %s",
		req.Amount, req.Currency,
		g.bankName, g.accountNumber, g.holderName, ref,
	)

	log.Printf("[billing] manual payment created: ref=%s, amount=%.2f %s, customer=%d",
		ref, req.Amount, req.Currency, req.CustomerID)

	return &PaymentResponse{
		PaymentURL: instructions,
		Reference:  ref,
	}, nil
}

// VerifyPayment always returns verified for manual gateway.
// Admin marks the payment as verified through the admin panel.
func (g *ManualGateway) VerifyPayment(ctx context.Context, ref string) (*PaymentVerification, error) {
	log.Printf("[billing] manual payment verified (admin-approved): ref=%s", ref)
	return &PaymentVerification{
		Verified:  true,
		Reference: ref,
	}, nil
}

// RefundPayment logs the refund request for manual processing by admin.
func (g *ManualGateway) RefundPayment(ctx context.Context, ref string, amount float64) error {
	log.Printf("[billing] manual refund requested: ref=%s, amount=%.2f (requires admin action)", ref, amount)
	return nil
}
