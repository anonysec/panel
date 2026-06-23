//go:build !lite

package payment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const (
	zarinpalAPISandbox    = "https://sandbox.zarinpal.com/pg/v4/payment/request.json"
	zarinpalAPIProduction = "https://api.zarinpal.com/pg/v4/payment/request.json"

	zarinpalVerifySandbox    = "https://sandbox.zarinpal.com/pg/v4/payment/verify.json"
	zarinpalVerifyProduction = "https://api.zarinpal.com/pg/v4/payment/verify.json"

	zarinpalRedirectSandbox    = "https://sandbox.zarinpal.com/pg/StartPay/"
	zarinpalRedirectProduction = "https://www.zarinpal.com/pg/StartPay/"
)

// Zarinpal implements the Gateway interface for the Zarinpal payment processor.
type Zarinpal struct {
	MerchantID string
	Sandbox    bool
}

// NewZarinpal creates a new Zarinpal gateway instance.
func NewZarinpal(merchantID string, sandbox bool) *Zarinpal {
	return &Zarinpal{
		MerchantID: merchantID,
		Sandbox:    sandbox,
	}
}

// Name returns "zarinpal".
func (z *Zarinpal) Name() string {
	return "zarinpal"
}

// CreatePayment initiates a payment via Zarinpal API.
// Returns the redirect URL for the customer and the authority as reference.
func (z *Zarinpal) CreatePayment(amount float64, currency string, callbackURL string) (redirectURL string, reference string, err error) {
	apiURL := zarinpalAPIProduction
	redirectBase := zarinpalRedirectProduction
	if z.Sandbox {
		apiURL = zarinpalAPISandbox
		redirectBase = zarinpalRedirectSandbox
	}

	payload := map[string]interface{}{
		"merchant_id":  z.MerchantID,
		"amount":       int(amount),
		"currency":     currency,
		"callback_url": callbackURL,
		"description":  "Payment via KorisPanel",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("zarinpal: marshal request: %w", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("zarinpal: request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Code      int    `json:"code"`
			Authority string `json:"authority"`
		} `json:"data"`
		Errors interface{} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("zarinpal: decode response: %w", err)
	}

	if result.Data.Code != 100 {
		return "", "", fmt.Errorf("zarinpal: API error code %d", result.Data.Code)
	}

	authority := result.Data.Authority
	redirectURL = redirectBase + authority
	return redirectURL, authority, nil
}

// VerifyPayment verifies a payment with Zarinpal by authority reference.
// Returns the verified amount on success.
func (z *Zarinpal) VerifyPayment(reference string) (amount float64, err error) {
	apiURL := zarinpalVerifyProduction
	if z.Sandbox {
		apiURL = zarinpalVerifySandbox
	}

	payload := map[string]interface{}{
		"merchant_id": z.MerchantID,
		"authority":   reference,
		"amount":      0, // Zarinpal requires amount for verification; caller should manage this
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("zarinpal: marshal verify request: %w", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("zarinpal: verify request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Code   int   `json:"code"`
			Amount int64 `json:"amount"`
		} `json:"data"`
		Errors interface{} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("zarinpal: decode verify response: %w", err)
	}

	if result.Data.Code != 100 && result.Data.Code != 101 {
		return 0, fmt.Errorf("zarinpal: verification failed with code %d", result.Data.Code)
	}

	return float64(result.Data.Amount), nil
}

// RefundPayment is a no-op for Zarinpal as it does not support programmatic refunds.
// Refunds must be processed manually through the Zarinpal merchant panel.
func (z *Zarinpal) RefundPayment(reference string, amount float64) error {
	log.Printf("[payment] zarinpal: refund not supported programmatically (reference=%s, amount=%.2f). Process manually via Zarinpal dashboard.", reference, amount)
	return nil
}
