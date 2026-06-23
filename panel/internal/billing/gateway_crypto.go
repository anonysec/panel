//go:build !lite

package billing

import (
	"context"
	"fmt"
	"log"
	"time"
)

// CryptoGateway handles cryptocurrency payments.
// This is a placeholder/stub implementation — actual crypto payment processing
// requires integration with a payment processor (e.g., NOWPayments, CoinGate).
type CryptoGateway struct {
	walletAddress    string
	network          string // btc, eth, usdt
	minConfirmations int
}

// NewCryptoGateway creates a CryptoGateway from a config map.
// Expected keys: wallet_address, network (btc/eth/usdt), min_confirmations.
func NewCryptoGateway(config map[string]string) *CryptoGateway {
	minConf := 3 // default
	if v, ok := config["min_confirmations"]; ok {
		n := 0
		for _, c := range v {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		if n > 0 {
			minConf = n
		}
	}

	return &CryptoGateway{
		walletAddress:    config["wallet_address"],
		network:          config["network"],
		minConfirmations: minConf,
	}
}

// Name returns the gateway identifier.
func (g *CryptoGateway) Name() string {
	return "crypto"
}

// CreatePayment generates a unique payment reference and returns wallet details.
// In production, this would call a payment processor to generate a unique deposit address.
func (g *CryptoGateway) CreatePayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error) {
	ref := fmt.Sprintf("CRYPTO-%d-%d", req.CustomerID, time.Now().UnixNano())

	paymentInfo := fmt.Sprintf(
		"Send %.8f %s to address: %s\nNetwork: %s\nMin confirmations: %d\nReference: %s",
		req.Amount, g.networkSymbol(), g.walletAddress, g.network, g.minConfirmations, ref,
	)

	log.Printf("[billing] crypto payment created: ref=%s, network=%s, amount=%.8f, customer=%d",
		ref, g.network, req.Amount, req.CustomerID)

	return &PaymentResponse{
		PaymentURL: paymentInfo,
		Reference:  ref,
	}, nil
}

// VerifyPayment checks if the crypto payment was received.
// In production, this would query a blockchain explorer or payment processor webhook status.
// This stub returns unverified — real verification happens via webhook callback.
func (g *CryptoGateway) VerifyPayment(ctx context.Context, ref string) (*PaymentVerification, error) {
	// Stub: In a real implementation, this would:
	// 1. Query the payment processor API for transaction status
	// 2. Check blockchain confirmations against minConfirmations
	// 3. Return verified=true only when confirmations >= minConfirmations

	log.Printf("[billing] crypto payment verification requested: ref=%s, network=%s (stub: pending webhook confirmation)",
		ref, g.network)

	return &PaymentVerification{
		Verified:  false,
		Reference: ref,
		Amount:    0,
	}, nil
}

// RefundPayment logs the refund request with the customer's wallet address.
// Crypto refunds require manual processing or integration with a payment processor.
func (g *CryptoGateway) RefundPayment(ctx context.Context, ref string, amount float64) error {
	log.Printf("[billing] crypto refund requested: ref=%s, amount=%.8f %s, network=%s (requires manual processing)",
		ref, amount, g.networkSymbol(), g.network)
	return nil
}

// networkSymbol returns the display symbol for the configured network.
func (g *CryptoGateway) networkSymbol() string {
	switch g.network {
	case "btc":
		return "BTC"
	case "eth":
		return "ETH"
	case "usdt":
		return "USDT"
	default:
		return g.network
	}
}
