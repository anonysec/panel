package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type PaymentRequest struct {
	Amount      int64  // in smallest unit (Rials for ZarinPal)
	Description string
	CallbackURL string
	Username    string
}

type PaymentResult struct {
	Success    bool
	Authority  string
	PaymentURL string
	RefID      string
	Error      string
}

type Gateway interface {
	Name() string
	CreatePayment(req PaymentRequest) (*PaymentResult, error)
	VerifyPayment(authority string, amount int64) (*PaymentResult, error)
}

// ZarinPal implementation
type ZarinPal struct {
	merchantID string
	sandbox    bool
}

func NewZarinPal() *ZarinPal {
	return &ZarinPal{
		merchantID: os.Getenv("PANEL_ZARINPAL_MERCHANT"),
		sandbox:    os.Getenv("PANEL_ZARINPAL_SANDBOX") == "true",
	}
}

func (z *ZarinPal) Name() string { return "zarinpal" }

func (z *ZarinPal) baseURL() string {
	if z.sandbox {
		return "https://sandbox.zarinpal.com"
	}
	return "https://api.zarinpal.com"
}

func (z *ZarinPal) CreatePayment(req PaymentRequest) (*PaymentResult, error) {
	body := map[string]any{
		"merchant_id":  z.merchantID,
		"amount":       req.Amount,
		"description":  req.Description,
		"callback_url": req.CallbackURL,
	}
	data, _ := json.Marshal(body)
	resp, err := http.Post(z.baseURL()+"/pg/v4/payment/request.json", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			Authority string `json:"authority"`
			Code      int    `json:"code"`
		} `json:"data"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.Data.Code != 100 {
		return &PaymentResult{Success: false, Error: fmt.Sprintf("zarinpal code: %d", result.Data.Code)}, nil
	}
	payURL := fmt.Sprintf("%s/pg/StartPay/%s", z.baseURL(), result.Data.Authority)
	return &PaymentResult{Success: true, Authority: result.Data.Authority, PaymentURL: payURL}, nil
}

func (z *ZarinPal) VerifyPayment(authority string, amount int64) (*PaymentResult, error) {
	body := map[string]any{
		"merchant_id": z.merchantID,
		"authority":   authority,
		"amount":      amount,
	}
	data, _ := json.Marshal(body)
	resp, err := http.Post(z.baseURL()+"/pg/v4/payment/verify.json", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			Code  int    `json:"code"`
			RefID string `json:"ref_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.Data.Code == 100 || result.Data.Code == 101 {
		return &PaymentResult{Success: true, RefID: fmt.Sprint(result.Data.RefID)}, nil
	}
	return &PaymentResult{Success: false, Error: fmt.Sprintf("verify code: %d", result.Data.Code)}, nil
}
