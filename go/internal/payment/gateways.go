package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

func (z *ZarinPalGateway) InitiatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
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

	authority := parseStringAny(data["authority"])
	if authority == "" {
		return nil, fmt.Errorf("zarinpal no authority returned")
	}

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: z.paymentURL() + authority,
		Authority:  authority,
	}, nil
}

// CreatePayment is kept for backward compatibility.
func (z *ZarinPalGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return z.InitiatePayment(ctx, amount, orderID, description, callbackURL)
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
		return &VerifyResult{Verified: false, Message: "invalid response", Raw: result}, nil
	}

	code := parseIntAny(data["code"])
	refID := parseStringAny(data["ref_id"])

	if code == 100 || code == 101 {
		return &VerifyResult{
			Verified: true,
			RefID:    refID,
			Raw:      data,
		}, nil
	}

	return &VerifyResult{
		Verified: false,
		Message:  fmt.Sprintf("verification failed with code: %d", code),
		Raw:      data,
	}, nil
}

func (z *ZarinPalGateway) ParseCallback(payload map[string]interface{}) (*CallbackData, error) {
	authority := parseStringAny(payload["Authority"])
	status := parseStringAny(payload["Status"])
	if authority == "" {
		return nil, fmt.Errorf("missing Authority")
	}
	return &CallbackData{
		Authority: authority,
		Status:    status,
		Raw:       payload,
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

func (n *NOWPaymentsGateway) InitiatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
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

	invoiceURL := parseStringAny(result["invoice_url"])
	invoiceID := parseStringAny(result["id"])

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: invoiceURL,
		InvoiceID:  invoiceID,
	}, nil
}

// CreatePayment is kept for backward compatibility.
func (n *NOWPaymentsGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return n.InitiatePayment(ctx, amount, orderID, description, callbackURL)
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

	status := parseStringAny(result["payment_status"])
	if status == "finished" || status == "confirmed" {
		return &VerifyResult{
			Verified: true,
			RefID:    parseStringAny(result["payment_id"]),
			Raw:      result,
		}, nil
	}

	return &VerifyResult{
		Verified: false,
		Message:  "payment status: " + status,
		Raw:      result,
	}, nil
}

func (n *NOWPaymentsGateway) ParseCallback(payload map[string]interface{}) (*CallbackData, error) {
	return &CallbackData{
		OrderID: parseStringAny(payload["invoice_id"]),
		Status:  parseStringAny(payload["payment_status"]),
		RefID:   parseStringAny(payload["payment_id"]),
		Raw:     payload,
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

func (c *CardToCardGateway) InitiatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return &PaymentResult{OrderID: orderID}, nil
}

// CreatePayment is kept for backward compatibility.
func (c *CardToCardGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return c.InitiatePayment(ctx, amount, orderID, description, callbackURL)
}

func (c *CardToCardGateway) VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error) {
	return &VerifyResult{
		Verified: false,
		Message:  "manual verification required",
	}, nil
}

func (c *CardToCardGateway) ParseCallback(payload map[string]interface{}) (*CallbackData, error) {
	return &CallbackData{Raw: payload}, nil
}

// TronadoGateway wraps Tronado verification and callback parsing.
type TronadoGateway struct {
	apiKey     string
	verifyURL  string
	paymentURL string
	client     *httpclient.Client
}

func NewTronadoGateway(apiKey string) *TronadoGateway {
	return &TronadoGateway{
		apiKey:     strings.TrimSpace(apiKey),
		verifyURL:  "https://bot.tronado.cloud/Order/GetStatus",
		paymentURL: "https://bot.tronado.cloud/Order/GetPaymentLink",
		client: httpclient.New().
			WithTimeout(30*time.Second).
			WithHeader("Content-Type", "application/json").
			WithHeader("x-api-key", strings.TrimSpace(apiKey)),
	}
}

func (t *TronadoGateway) Name() string {
	return "tronado"
}

func (t *TronadoGateway) InitiatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	walletAddress := strings.TrimSpace(os.Getenv("TRONADO_WALLET_ADDRESS"))
	desc := strings.TrimSpace(description)
	if strings.HasPrefix(strings.ToLower(desc), "wallet:") {
		walletAddress = strings.TrimSpace(strings.TrimPrefix(desc, "wallet:"))
	}
	if walletAddress == "" {
		return nil, fmt.Errorf("wallet address is required for tronado payment")
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"PaymentID":     orderID,
		"WalletAddress": walletAddress,
		"TronAmount":    amount,
		"CallbackUrl":   callbackURL,
	})
	rawResp, err := t.client.Post(t.paymentURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("tronado create payment failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		return nil, fmt.Errorf("tronado parse create response failed: %w", err)
	}

	success := parseBoolAny(payload["IsSuccessful"])
	if !success {
		msg := parseStringAny(payload["Message"])
		if msg == "" {
			msg = parseStringAny(payload["message"])
		}
		if msg == "" {
			msg = "tronado create payment failed"
		}
		return nil, fmt.Errorf(msg)
	}

	token := ""
	if dataMap, ok := payload["Data"].(map[string]interface{}); ok {
		token = parseStringAny(dataMap["Token"])
	}
	if token == "" {
		return nil, fmt.Errorf("tronado did not return payment token")
	}

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: "https://t.me/tronado_robot/customerpayment?startapp=" + token,
		Authority:  token,
	}, nil
}

func (t *TronadoGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return t.InitiatePayment(ctx, amount, orderID, description, callbackURL)
}

func (t *TronadoGateway) VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error) {
	verifyBody, _ := json.Marshal(map[string]string{"id": authority})
	resp, err := t.client.Post(t.verifyURL, verifyBody)
	if err != nil {
		return nil, fmt.Errorf("tronado verify failed: %w", err)
	}

	var verifyResult map[string]interface{}
	if err := json.Unmarshal(resp, &verifyResult); err != nil {
		return nil, fmt.Errorf("tronado verify parse failed: %w", err)
	}

	isPaid := parseBoolAny(verifyResult["IsPaid"])
	return &VerifyResult{
		Verified: isPaid,
		RefID:    authority,
		Message:  parseStringAny(verifyResult["Message"]),
		Raw:      verifyResult,
	}, nil
}

func (t *TronadoGateway) ParseCallback(payload map[string]interface{}) (*CallbackData, error) {
	hash := parseStringAny(payload["Hash"])
	orderID := parseStringAny(payload["PaymentID"])
	if orderID == "" {
		return nil, fmt.Errorf("missing PaymentID")
	}

	orderRef := orderID
	if parts := strings.SplitN(hash, "TrndOrderID_", 2); len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		orderRef = strings.TrimSpace(parts[1])
	}

	status := "unpaid"
	if parseBoolAny(payload["IsPaid"]) {
		status = "paid"
	}

	return &CallbackData{
		OrderID:   orderID,
		Authority: orderRef,
		Status:    status,
		RefID:     hash,
		Raw:       payload,
	}, nil
}

// IranPayGateway wraps IranPay verification and callback parsing.
type IranPayGateway struct {
	apiKey    string
	verifyURL string
	client    *httpclient.Client
}

func NewIranPayGateway(apiKey string) *IranPayGateway {
	return &IranPayGateway{
		apiKey:    strings.TrimSpace(apiKey),
		verifyURL: "https://tetra98.ir/api/verify",
		client: httpclient.New().
			WithTimeout(30*time.Second).
			WithHeader("Content-Type", "application/json").
			WithHeader("Accept", "application/json"),
	}
}

func (i *IranPayGateway) Name() string {
	return "iranpay"
}

func (i *IranPayGateway) InitiatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"ApiKey":      i.apiKey,
		"Hash_id":     orderID,
		"Amount":      fmt.Sprintf("%d", amount*10),
		"CallbackURL": callbackURL,
	})
	rawResp, err := i.client.Post("https://tetra98.ir/api/create_order", reqBody)
	if err != nil {
		return nil, fmt.Errorf("iranpay create payment failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		return nil, fmt.Errorf("iranpay parse create response failed: %w", err)
	}

	status := parseIntAny(payload["status"])
	if status != 100 {
		msg := parseStringAny(payload["message"])
		if msg == "" {
			msg = "iranpay create payment failed"
		}
		return nil, fmt.Errorf(msg)
	}

	paymentURL := parseStringAny(payload["payment_url_bot"])
	if paymentURL == "" {
		paymentURL = parseStringAny(payload["payment_url"])
	}
	if paymentURL == "" {
		paymentURL = parseStringAny(payload["url"])
	}
	if paymentURL == "" {
		return nil, fmt.Errorf("iranpay did not return payment url")
	}

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: paymentURL,
		Authority:  parseStringAny(payload["Authority"]),
	}, nil
}

func (i *IranPayGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return i.InitiatePayment(ctx, amount, orderID, description, callbackURL)
}

func (i *IranPayGateway) VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error) {
	parts := strings.SplitN(authority, "|", 2)
	if len(parts) != 2 {
		return &VerifyResult{Verified: false, Message: "invalid authority format"}, nil
	}
	hashID := strings.TrimSpace(parts[0])
	auth := strings.TrimSpace(parts[1])
	if hashID == "" || auth == "" {
		return &VerifyResult{Verified: false, Message: "invalid authority format"}, nil
	}

	verifyBody, _ := json.Marshal(map[string]interface{}{
		"ApiKey":    i.apiKey,
		"authority": auth,
		"hashid":    hashID,
	})
	resp, err := i.client.Post(i.verifyURL, verifyBody)
	if err != nil {
		return nil, fmt.Errorf("iranpay verify failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("iranpay verify parse failed: %w", err)
	}

	status := parseIntAny(result["status"])
	return &VerifyResult{
		Verified: status == 100,
		RefID:    auth,
		Message:  parseStringAny(result["message"]),
		Raw:      result,
	}, nil
}

func (i *IranPayGateway) ParseCallback(payload map[string]interface{}) (*CallbackData, error) {
	return &CallbackData{
		OrderID:   parseStringAny(payload["hashid"]),
		Authority: parseStringAny(payload["authority"]),
		Status:    parseStringAny(payload["status"]),
		Raw:       payload,
	}, nil
}

// AqayePardakhtGateway wraps AqayePardakht verification and callback parsing.
type AqayePardakhtGateway struct {
	pin       string
	verifyURL string
	client    *httpclient.Client
}

func NewAqayePardakhtGateway(pin string) *AqayePardakhtGateway {
	return &AqayePardakhtGateway{
		pin:       strings.TrimSpace(pin),
		verifyURL: "https://panel.aqayepardakht.ir/api/v2/verify",
		client: httpclient.New().
			WithTimeout(30*time.Second).
			WithHeader("Content-Type", "application/json"),
	}
}

func (a *AqayePardakhtGateway) Name() string {
	return "aqayepardakht"
}

func (a *AqayePardakhtGateway) InitiatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"pin":        a.pin,
		"amount":     amount,
		"callback":   callbackURL,
		"invoice_id": orderID,
	})
	rawResp, err := a.client.Post("https://panel.aqayepardakht.ir/api/v2/create", reqBody)
	if err != nil {
		return nil, fmt.Errorf("aqayepardakht create payment failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rawResp, &payload); err != nil {
		return nil, fmt.Errorf("aqayepardakht parse create response failed: %w", err)
	}

	status := strings.ToLower(strings.TrimSpace(parseStringAny(payload["status"])))
	if status != "success" {
		msg := parseStringAny(payload["message"])
		if msg == "" {
			msg = "aqayepardakht create payment failed"
		}
		return nil, fmt.Errorf(msg)
	}

	transID := parseStringAny(payload["transid"])
	if transID == "" {
		return nil, fmt.Errorf("aqayepardakht did not return transid")
	}

	return &PaymentResult{
		OrderID:    orderID,
		PaymentURL: "https://panel.aqayepardakht.ir/startpay/" + transID,
		Authority:  transID,
	}, nil
}

func (a *AqayePardakhtGateway) CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error) {
	return a.InitiatePayment(ctx, amount, orderID, description, callbackURL)
}

func (a *AqayePardakhtGateway) VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error) {
	verifyBody, _ := json.Marshal(map[string]interface{}{
		"pin":     a.pin,
		"amount":  amount,
		"transid": authority,
	})
	resp, err := a.client.Post(a.verifyURL, verifyBody)
	if err != nil {
		return nil, fmt.Errorf("aqayepardakht verify failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("aqayepardakht verify parse failed: %w", err)
	}

	code := parseStringAny(result["code"])
	switch code {
	case "1":
		return &VerifyResult{Verified: true, RefID: authority, Raw: result}, nil
	case "2":
		return &VerifyResult{Verified: false, RefID: authority, Message: "already paid", Raw: result}, nil
	default:
		return &VerifyResult{Verified: false, RefID: authority, Message: "verification failed", Raw: result}, nil
	}
}

func (a *AqayePardakhtGateway) ParseCallback(payload map[string]interface{}) (*CallbackData, error) {
	return &CallbackData{
		OrderID:   parseStringAny(payload["invoice_id"]),
		Authority: parseStringAny(payload["transid"]),
		Raw:       payload,
	}, nil
}

func parseStringAny(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	case int:
		return fmt.Sprintf("%d", t)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func parseIntAny(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(t), "%d", &n)
		return n
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	default:
		return 0
	}
}

func parseBoolAny(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "ok" || s == "paid"
	case float64:
		return int(t) == 1
	case int:
		return t == 1
	default:
		return false
	}
}
