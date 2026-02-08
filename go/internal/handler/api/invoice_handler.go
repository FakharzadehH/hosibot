package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
	"hosibot/internal/panel"
	"hosibot/internal/pkg/utils"
)

// InvoiceHandler handles all invoice/service API actions.
// Matches PHP api/invoice.php behavior.
type InvoiceHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewInvoiceHandler(repos *Repos, logger *zap.Logger) *InvoiceHandler {
	return &InvoiceHandler{repos: repos, logger: logger}
}

// Handle routes invoice API requests.
// POST /api/invoices
func (h *InvoiceHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "invoices":
		return h.listInvoices(c, body)
	case "services":
		return h.listServices(c, body)
	case "invoice":
		return h.getInvoice(c, body)
	case "remove_service":
		return h.removeService(c, body)
	case "invoice_add":
		return h.addInvoice(c, body)
	case "change_status_config":
		return h.changeStatusConfig(c, body)
	case "extend_service_admin":
		return h.extendServiceAdmin(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *InvoiceHandler) listInvoices(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	invoices, total, err := h.repos.Invoice.FindAll(limit, page, q)
	if err != nil {
		h.logger.Error("Failed to list invoices", zap.Error(err))
		return errorResponse(c, "Failed to retrieve invoices")
	}

	return successResponse(c, "Successful", paginatedResponse(invoices, total, page, limit))
}

func (h *InvoiceHandler) listServices(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	services, total, err := h.repos.Invoice.FindServices(limit, page, q)
	if err != nil {
		h.logger.Error("Failed to list services", zap.Error(err))
		return errorResponse(c, "Failed to retrieve services")
	}

	return successResponse(c, "Successful", paginatedResponse(services, total, page, limit))
}

func (h *InvoiceHandler) getInvoice(c echo.Context, body map[string]interface{}) error {
	invoiceID := getStringField(body, "id_invoice")
	if invoiceID == "" {
		return errorResponse(c, "id_invoice is required")
	}

	invoice, err := h.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return errorResponse(c, "Invoice not found")
	}

	// Get related panel info
	panel, _ := h.repos.Panel.FindByCode(invoice.ServiceLocation)

	result := map[string]interface{}{
		"invoice": invoice,
		"panel":   panel,
	}

	return successResponse(c, "Successful", result)
}

func (h *InvoiceHandler) removeService(c echo.Context, body map[string]interface{}) error {
	invoiceID := getStringField(body, "id_invoice")
	removeType := getStringField(body, "type")

	if invoiceID == "" {
		return errorResponse(c, "id_invoice is required")
	}

	invoice, err := h.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return errorResponse(c, "Invoice not found")
	}

	switch removeType {
	case "one":
		// Just mark as removed (deactivate service)
		if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "removed"}); err != nil {
			return errorResponse(c, "Failed to remove service")
		}
	case "tow":
		// Remove from panel and mark as removed
		panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
		if panelModel != nil {
			if pc, err := panel.PanelFactory(panelModel); err == nil {
				ctx := context.Background()
				if err := pc.Authenticate(ctx); err == nil {
					_ = pc.DeleteUser(ctx, invoice.Username)
				}
			}
		}
		if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "removed"}); err != nil {
			return errorResponse(c, "Failed to remove service")
		}
	case "three":
		// Remove and refund
		amount := getIntField(body, "amount", 0)
		if amount > 0 {
			_ = h.repos.User.UpdateBalance(invoice.IDUser, amount)
		}
		if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "removed"}); err != nil {
			return errorResponse(c, "Failed to remove service")
		}
	default:
		return errorResponse(c, "Invalid remove type")
	}

	return successResponse(c, "Service removed successfully", nil)
}

func (h *InvoiceHandler) addInvoice(c echo.Context, body map[string]interface{}) error {
	chatID := getStringField(body, "chat_id")
	username := getStringField(body, "username")
	codeProduct := getStringField(body, "code_product")
	locationCode := getStringField(body, "location_code")
	timeService := getIntField(body, "time_service", 0)
	volumeService := getIntField(body, "volume_service", 0)

	if chatID == "" || codeProduct == "" || locationCode == "" {
		return errorResponse(c, "chat_id, code_product, and location_code are required")
	}

	// Get product details
	product, err := h.repos.Product.FindByCode(codeProduct)
	if err != nil {
		return errorResponse(c, "Product not found")
	}

	// Generate username if not provided
	if username == "" {
		username = utils.GenerateUsername("")
	}

	// Use product defaults if not overridden
	if timeService == 0 {
		timeService = utils.ParseInt(product.ServiceTime, 30)
	}
	if volumeService == 0 {
		volumeService = utils.ParseInt(product.VolumeConstraint, 0)
	}

	invoiceID := utils.GenerateUUID()

	invoice := &models.Invoice{
		IDInvoice:       invoiceID,
		IDUser:          chatID,
		Username:        username,
		ServiceLocation: locationCode,
		TimeSell:        utils.NowUnix(),
		NameProduct:     product.NameProduct,
		PriceProduct:    product.PriceProduct,
		Volume:          fmt.Sprintf("%d", volumeService),
		ServiceTime:     fmt.Sprintf("%d", timeService),
		Status:          "active",
		Note:            product.Note,
	}

	if err := h.repos.Invoice.Create(invoice); err != nil {
		h.logger.Error("Failed to create invoice", zap.Error(err))
		return errorResponse(c, "Failed to create invoice")
	}

	// Create user on the VPN panel
	panelModel, _ := h.repos.Panel.FindByCode(locationCode)
	if panelModel != nil {
		if pc, err := panel.PanelFactory(panelModel); err == nil {
			ctx := context.Background()
			if err := pc.Authenticate(ctx); err == nil {
				var inbounds map[string][]string
				if panelModel.Inbounds != "" {
					_ = json.Unmarshal([]byte(panelModel.Inbounds), &inbounds)
				}
				var proxies map[string]string
				if panelModel.Proxies != "" {
					_ = json.Unmarshal([]byte(panelModel.Proxies), &proxies)
				}

				req := panel.CreateUserRequest{
					Username:   username,
					DataLimit:  int64(volumeService) * 1024 * 1024 * 1024,
					ExpireDays: timeService,
					Inbounds:   inbounds,
					Proxies:    proxies,
				}
				panelUser, err := pc.CreateUser(ctx, req)
				if err != nil {
					h.logger.Error("Failed to create user on panel", zap.Error(err))
				} else if panelUser != nil && panelUser.SubLink != "" {
					_ = h.repos.Invoice.Update(invoiceID, map[string]interface{}{
						"uuid": panelUser.SubLink,
					})
				}
			}
		}
	}

	return successResponse(c, "Invoice created successfully", invoice)
}

func (h *InvoiceHandler) changeStatusConfig(c echo.Context, body map[string]interface{}) error {
	invoiceID := getStringField(body, "id_invoice")
	if invoiceID == "" {
		return errorResponse(c, "id_invoice is required")
	}

	invoice, err := h.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return errorResponse(c, "Invoice not found")
	}

	// Toggle status
	newStatus := "active"
	if invoice.Status == "active" {
		newStatus = "disabled"
	}

	if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": newStatus}); err != nil {
		return errorResponse(c, "Failed to change status")
	}

	// Enable/disable user on VPN panel
	panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
	if panelModel != nil {
		if pc, err := panel.PanelFactory(panelModel); err == nil {
			ctx := context.Background()
			if err := pc.Authenticate(ctx); err == nil {
				if newStatus == "disabled" {
					_ = pc.DisableUser(ctx, invoice.Username)
				} else {
					_ = pc.EnableUser(ctx, invoice.Username)
				}
			}
		}
	}

	return successResponse(c, "Status changed successfully", map[string]string{"new_status": newStatus})
}

func (h *InvoiceHandler) extendServiceAdmin(c echo.Context, body map[string]interface{}) error {
	invoiceID := getStringField(body, "id_invoice")
	timeService := getIntField(body, "time_service", 0)
	volumeService := getIntField(body, "volume_service", 0)

	if invoiceID == "" {
		return errorResponse(c, "id_invoice is required")
	}

	_, err := h.repos.Invoice.FindByID(invoiceID)
	if err != nil {
		return errorResponse(c, "Invoice not found")
	}

	// Extend user on panel
	invoice, _ := h.repos.Invoice.FindByID(invoiceID)
	if invoice != nil {
		panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
		if panelModel != nil {
			if pc, err := panel.PanelFactory(panelModel); err == nil {
				ctx := context.Background()
				if err := pc.Authenticate(ctx); err == nil {
					panelUser, err := pc.GetUser(ctx, invoice.Username)
					if err == nil && panelUser != nil {
						modReq := panel.ModifyUserRequest{
							Status: "active",
						}
						if volumeService > 0 {
							modReq.DataLimit = panelUser.DataLimit + int64(volumeService)*1024*1024*1024
						}
						if timeService > 0 {
							newExpire := panelUser.ExpireTime + int64(timeService)*86400
							modReq.ExpireTime = newExpire
						}
						_, _ = pc.ModifyUser(ctx, invoice.Username, modReq)
					}
				}
			}
		}
	}

	updates := map[string]interface{}{}
	if timeService > 0 {
		updates["Service_time"] = fmt.Sprintf("%d", timeService)
	}
	if volumeService > 0 {
		updates["Volume"] = fmt.Sprintf("%d", volumeService)
	}

	if len(updates) > 0 {
		if err := h.repos.Invoice.Update(invoiceID, updates); err != nil {
			return errorResponse(c, "Failed to extend service")
		}
	}

	return successResponse(c, "Service extended successfully", nil)
}
