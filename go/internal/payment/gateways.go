package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hosibot/internal/pkg/httpclient"
)

// ZarinPalGateway implements the Gateway interface for ZarinPal.
type ZarinPalGateway struct {
	merchantID string
	sandbox    bool
	client     *httpclient.Client
}

func NewZarinPalGateway(merchantID string, sandbox bool) *ZarinPalGateway {
	return &ZarinPalGateway{
		merchantID: merchantID,
		sandbox:    sandbox,
		client:     httpclient.New().WithTimeout(30 * time.Second),
	}
}

func (z *ZarinPalGateway) Name() string {
	return "zarinpal"
}

func (z *ZarinPalGateway) baseURL() string {
	if z.sandbox {
		return "https://sandbox.zarinpal.com"
	}
	return "https://api.zarinpal.com"
}

func (z *ZarinPalGateway) paymentURL() string {
	if z.sandbox {
		return "https://sandbox.zarinpal.com/pg/StartPay/"
	}
	return "https://www.zarinpal.com/pg/StartPay/"
}

func (z *ZarinPalGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	body := map[string]interface{}{
		"merchant_id":  z.merchantID,
		"amount":       amount,
		"description":  description,
		"callback_url": callbackURL,
	}

	bodyJSON, _ := json.Marshal(body)
	resp, err := z.client.Post(z.baseURL()+"/pg/v4/payment/request.json", bodyJSON)
	if err != nil {
		return nil, fmt.Errorf("zarinpal create payment failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("zarinpal parse error: %w", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("zarinpal unexpected response format")
	}

	authority, _ := data["authority"].(string)
	if authority == "" {
		return nil, fmt.Errorf("zarinpal no authority returned")
	}

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: z.paymentURL() + authority,
		Authority:  authority,
	}, nil
}

func (z *ZarinPalGateway) VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error) {
	body := map[string]interface{}{
		"merchant_id": z.merchantID,
		"amount":      amount,
		"authority":   authority,
	}

	bodyJSON, _ := json.Marshal(body)
	resp, err := z.client.Post(z.baseURL()+"/pg/v4/payment/verify.json", bodyJSON)
	if err != nil {
		return nil, fmt.Errorf("zarinpal verify failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("zarinpal verify parse error: %w", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &VerifyResult{Verified: false, Message: "invalid response"}, nil
	}

	code, _ := data["code"].(float64)
	refID := fmt.Sprintf("%.0f", data["ref_id"])

	if code == 100 || code == 101 {
		return &VerifyResult{
			Verified: true,
			RefID:    refID,
		}, nil
	}

	return &VerifyResult{
		Verified: false,
		Message:  fmt.Sprintf("verification failed with code: %.0f", code),
	}, nil
}

// NOWPaymentsGateway implements the Gateway interface for NOWPayments (crypto).
type NOWPaymentsGateway struct {
	apiKey string
	client *httpclient.Client
}

func NewNOWPaymentsGateway(apiKey string) *NOWPaymentsGateway {
	return &NOWPaymentsGateway{
		apiKey: apiKey,
		client: httpclient.New().
			WithTimeout(30*time.Second).
			WithHeader("x-api-key", apiKey).
			WithHeader("Content-Type", "application/json"),
	}
}

func (n *NOWPaymentsGateway) Name() string {
	return "nowpayments"
}

func (n *NOWPaymentsGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	body := map[string]interface{}{
		"price_amount":      amount,
		"price_currency":    "usd",
		"order_id":          orderID,
		"order_description": description,
		"ipn_callback_url":  callbackURL,
	}

	bodyJSON, _ := json.Marshal(body)
	resp, err := n.client.Post("https://api.nowpayments.io/v1/invoice", bodyJSON)
	if err != nil {
		return nil, fmt.Errorf("nowpayments create failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("nowpayments parse error: %w", err)
	}

	invoiceURL, _ := result["invoice_url"].(string)
	invoiceID := fmt.Sprintf("%.0f", result["id"])

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: invoiceURL,
		InvoiceID:  invoiceID,
	}, nil
}

func (n *NOWPaymentsGateway) VerifyPayment(ctx context.Context, paymentID string, amount int) (*VerifyResult, error) {
	resp, err := n.client.Get("https://api.nowpayments.io/v1/payment/" + paymentID)
	if err != nil {
		return nil, fmt.Errorf("nowpayments verify failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("nowpayments verify parse error: %w", err)
	}

	status, _ := result["payment_status"].(string)
	if status == "finished" || status == "confirmed" {
		return &VerifyResult{
			Verified: true,
			RefID:    fmt.Sprintf("%.0f", result["payment_id"]),
		}, nil
	}

	return &VerifyResult{
		Verified: false,
		Message:  "payment status: " + status,
	}, nil
}

// CardToCardGateway handles manual card-to-card payments.
type CardToCardGateway struct{}

func NewCardToCardGateway() *CardToCardGateway {
	return &CardToCardGateway{}
}

func (c *CardToCardGateway) Name() string {
	return "card"
}

func (c *CardToCardGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	// Card-to-card doesn't create an online payment, it just records the order
	return &PaymentResult{
		OrderID: orderID,
	}, nil
}

func (c *CardToCardGateway) VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error) {
	// Card-to-card is verified manually by admin
	return &VerifyResult{
		Verified: false,
		Message:  "manual verification required",
	}, nil
}
