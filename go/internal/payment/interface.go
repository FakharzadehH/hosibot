package payment

import "context"

// PaymentResult contains the result of a payment creation.
type PaymentResult struct {
	OrderID    string `json:"order_id"`
	PaymentURL string `json:"payment_url"`
	Authority  string `json:"authority,omitempty"`
	InvoiceID  string `json:"invoice_id,omitempty"`
}

// VerifyResult contains the result of a payment verification.
type VerifyResult struct {
	Verified bool   `json:"verified"`
	RefID    string `json:"ref_id,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Gateway defines the interface for payment gateway implementations.
type Gateway interface {
	// Name returns the gateway identifier.
	Name() string

	// CreatePayment initiates a new payment.
	CreatePayment(ctx context.Context, amount int, orderID, description, callbackURL string) (*PaymentResult, error)

	// VerifyPayment verifies a payment after callback.
	VerifyPayment(ctx context.Context, authority string, amount int) (*VerifyResult, error)
}
