package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

	return successResponse(c, "Successful", paginatedNamedResponse("invoices", invoices, total, page, limit))
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

	return successResponse(c, "Successful", paginatedNamedResponse("services", services, total, page, limit))
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
	panel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
	if panel == nil {
		panel, _ = h.repos.Panel.FindByCode(invoice.ServiceLocation)
	}

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
		// Remove from panel and mark as removed by admin.
		panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
		if panelModel != nil {
			if pc, err := panel.PanelFactory(panelModel); err == nil {
				ctx := context.Background()
				if err := pc.Authenticate(ctx); err == nil {
					_ = pc.DeleteUser(ctx, invoice.Username)
				}
			}
		}
		if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "removebyadmin"}); err != nil {
			return errorResponse(c, "Failed to remove service")
		}
	case "tow":
		// Refund + remove from panel + mark as removed by admin.
		if _, ok := body["amount"]; !ok {
			return errorResponse(c, "amount is required")
		}
		amount := getIntField(body, "amount", 0)
		if amount > 0 {
			_ = h.repos.User.UpdateBalance(invoice.IDUser, amount)
		}

		panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
		if panelModel != nil {
			if pc, err := panel.PanelFactory(panelModel); err == nil {
				ctx := context.Background()
				if err := pc.Authenticate(ctx); err == nil {
					_ = pc.DeleteUser(ctx, invoice.Username)
				}
			}
		}
		if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "removebyadmin"}); err != nil {
			return errorResponse(c, "Failed to remove service")
		}
	case "three":
		// Hard delete invoice row.
		if err := h.repos.Invoice.Delete(invoiceID); err != nil {
			return errorResponse(c, "Failed to remove service")
		}
	default:
		return errorResponse(c, "Invalid remove type")
	}

	return successResponse(c, "Service removed successfully", invoice)
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

	panelModel, err := h.repos.Panel.FindByCode(locationCode)
	if err != nil || panelModel == nil {
		return errorResponse(c, "panel code not found")
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
		ServiceLocation: panelModel.NamePanel,
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

	// Toggle status using PHP-compatible values.
	newStatus := "disablebyadmin"
	if invoice.Status == "disablebyadmin" {
		newStatus = "active"
	}

	if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": newStatus}); err != nil {
		return errorResponse(c, "Failed to change status")
	}

	// Enable/disable user on VPN panel
	panelModel, _ := h.repos.Panel.FindByName(invoice.ServiceLocation)
	if panelModel == nil {
		panelModel, _ = h.repos.Panel.FindByCode(invoice.ServiceLocation)
	}
	if panelModel != nil {
		if pc, err := panel.PanelFactory(panelModel); err == nil {
			ctx := context.Background()
			if err := pc.Authenticate(ctx); err == nil {
				if newStatus == "disablebyadmin" {
					_ = pc.DisableUser(ctx, invoice.Username)
				} else {
					_ = pc.EnableUser(ctx, invoice.Username)
				}
			}
		}
	}

	return successResponse(c, "Successful", nil)
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

	// Match PHP behavior: log extend action and force invoice status active.
	serviceOther := &models.ServiceOther{
		IDUser:   invoice.IDUser,
		Username: invoice.Username,
		Value:    fmt.Sprintf("%d_%d", volumeService, timeService),
		Type:     "extend_user_by_admin",
		Time:     time.Now().Format("2006/01/02 15:04:05"),
		Price:    "0",
		Status:   "active",
	}
	output, _ := json.Marshal(map[string]interface{}{
		"time_service":   timeService,
		"volume_service": volumeService,
	})
	serviceOther.Output = string(output)
	_ = h.repos.Setting.DB().Create(serviceOther).Error

	if err := h.repos.Invoice.Update(invoiceID, map[string]interface{}{"Status": "active"}); err != nil {
		return errorResponse(c, "Failed to extend service")
	}

	return successResponse(c, "Successful", nil)
}
