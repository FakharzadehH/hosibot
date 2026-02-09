package api

import (
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// PaymentHandler handles all payment API actions.
// Matches PHP api/payment.php behavior.
type PaymentHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewPaymentHandler(repos *Repos, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{repos: repos, logger: logger}
}

// Handle routes payment API requests.
// POST /api/payments
func (h *PaymentHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "payments":
		return h.listPayments(c, body)
	case "payment":
		return h.getPayment(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *PaymentHandler) listPayments(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if page <= 0 {
		page = 1
	}

	payments, total, err := h.repos.Payment.FindAll(limit, page, q)
	if err != nil {
		h.logger.Error("Failed to list payments", zap.Error(err))
		return errorResponse(c, "Failed to retrieve payments")
	}

	items := make([]map[string]interface{}, 0, len(payments))
	for _, p := range payments {
		items = append(items, map[string]interface{}{
			"id":             p.IDOrder,
			"id_user":        p.IDUser,
			"time":           p.Time,
			"price":          p.Price,
			"payment_status": p.PaymentStatus,
			"Payment_Method": p.PaymentMethod,
		})
	}

	totalRecord := int(total) / limit
	if int(total)%limit != 0 {
		totalRecord++
	}

	return successResponse(c, "Successful", map[string]interface{}{
		"payments": items,
		"pagination": map[string]interface{}{
			"total_record": totalRecord,
			"total_pages":  limit, // keep legacy PHP behavior
			"current_page": page,
			"per_page":     limit,
		},
	})
}

func (h *PaymentHandler) getPayment(c echo.Context, body map[string]interface{}) error {
	orderID := getStringField(body, "id_order")
	if orderID == "" {
		return errorResponse(c, "id_order is required")
	}

	payment, err := h.repos.Payment.FindByOrderID(orderID)
	if err != nil {
		return errorResponse(c, "Payment not found")
	}

	return successResponse(c, "Successful", payment)
}
