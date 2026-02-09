package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
	"hosibot/internal/pkg/utils"
)

// ProductHandler handles all product API actions.
// Matches PHP api/product.php behavior.
type ProductHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewProductHandler(repos *Repos, logger *zap.Logger) *ProductHandler {
	return &ProductHandler{repos: repos, logger: logger}
}

// Handle routes product API requests.
// POST /api/products
func (h *ProductHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "products":
		return h.listProducts(c, body)
	case "product":
		return h.getProduct(c, body)
	case "product_add":
		return h.addProduct(c, body)
	case "product_edit":
		return h.editProduct(c, body)
	case "product_delete":
		return h.deleteProduct(c, body)
	case "set_inbounds":
		return h.setInbounds(c, body)
	case "remove_inbounds":
		return h.removeInbounds(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *ProductHandler) listProducts(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	products, total, err := h.repos.Product.FindAll(limit, page, q)
	if err != nil {
		h.logger.Error("Failed to list products", zap.Error(err))
		return errorResponse(c, "Failed to retrieve products")
	}

	resp := paginatedNamedResponse("products", products, total, page, limit)
	if panels, err := h.repos.Panel.FindActive(); err == nil {
		resp["panels"] = panels
	}
	if categories, _, err := h.repos.Setting.FindAllCategories(0, 1, ""); err == nil {
		resp["category"] = categories
	}
	return successResponse(c, "Successful", resp)
}

func (h *ProductHandler) getProduct(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	product, err := h.repos.Product.FindByID(id)
	if err != nil {
		return errorResponse(c, "Product not found")
	}

	// Get related info (panels list, categories, invoice stats)
	panels, _ := h.repos.Panel.FindActive()
	categories, _, _ := h.repos.Setting.FindAllCategories(0, 1, "")
	invoiceCount, sumInvoice, _ := h.repos.Invoice.ProductStatsByName(product.NameProduct)

	result := map[string]interface{}{
		"product":       product,
		"panels":        panels,
		"category":      categories,
		"count_invoice": invoiceCount,
		"sum_invoice":   sumInvoice,
	}

	return successResponse(c, "Successful", result)
}

func (h *ProductHandler) addProduct(c echo.Context, body map[string]interface{}) error {
	name := getStringField(body, "name")
	price := getIntField(body, "price", 0)
	dataLimit := getIntField(body, "data_limit", 0)
	serviceTime := getIntField(body, "time", 0)
	location := getStringField(body, "location")

	if name == "" || location == "" {
		return errorResponse(c, "name and location are required")
	}

	codeProduct := utils.RandomCode(8)

	// Handle inbounds, proxies, hide_panel as JSON
	inboundsJSON := ""
	if v, ok := body["inbounds"]; ok && v != nil {
		b, _ := json.Marshal(v)
		inboundsJSON = string(b)
	}
	proxiesJSON := ""
	if v, ok := body["proxies"]; ok && v != nil {
		b, _ := json.Marshal(v)
		proxiesJSON = string(b)
	}
	hidePanelJSON := ""
	if v, ok := body["hide_panel"]; ok && v != nil {
		b, _ := json.Marshal(v)
		hidePanelJSON = string(b)
	}

	product := &models.Product{
		CodeProduct:      codeProduct,
		NameProduct:      name,
		PriceProduct:     fmt.Sprintf("%d", price),
		VolumeConstraint: fmt.Sprintf("%d", dataLimit),
		Location:         location,
		ServiceTime:      fmt.Sprintf("%d", serviceTime),
		Agent:            getStringField(body, "agent"),
		Note:             getStringField(body, "note"),
		DataLimitReset:   getStringField(body, "data_limit_reset"),
		Inbounds:         inboundsJSON,
		Proxies:          proxiesJSON,
		Category:         getStringField(body, "category"),
		OneBuyStatus:     strconv.Itoa(getIntField(body, "one_buy_status", 0)),
		HidePanel:        hidePanelJSON,
	}

	if err := h.repos.Product.Create(product); err != nil {
		h.logger.Error("Failed to create product", zap.Error(err))
		return errorResponse(c, "Failed to create product")
	}

	return successResponse(c, "Product created successfully", product)
}

func (h *ProductHandler) editProduct(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	updates := make(map[string]interface{})

	if v := getStringField(body, "name"); v != "" {
		updates["name_product"] = v
	}
	if v, ok := body["price"]; ok {
		updates["price_product"] = fmt.Sprintf("%v", v)
	}
	if v, ok := body["volume"]; ok {
		updates["Volume_constraint"] = fmt.Sprintf("%v", v)
	}
	if v, ok := body["time"]; ok {
		updates["Service_time"] = fmt.Sprintf("%v", v)
	}
	if v := getStringField(body, "location"); v != "" {
		updates["Location"] = v
	}
	if v := getStringField(body, "agent"); v != "" {
		updates["agent"] = v
	}
	if v := getStringField(body, "note"); v != "" {
		updates["note"] = v
	}
	if v := getStringField(body, "data_limit_reset"); v != "" {
		updates["data_limit_reset"] = v
	}
	if v := getStringField(body, "category"); v != "" {
		updates["category"] = v
	}
	if v, ok := body["one_buy_status"]; ok {
		updates["one_buy_status"] = fmt.Sprintf("%v", v)
	}
	if v, ok := body["inbounds"]; ok && v != nil {
		b, _ := json.Marshal(v)
		updates["inbounds"] = string(b)
	}
	if v, ok := body["proxies"]; ok && v != nil {
		b, _ := json.Marshal(v)
		updates["proxies"] = string(b)
	}
	if v, ok := body["hide_panel"]; ok && v != nil {
		b, _ := json.Marshal(v)
		updates["hide_panel"] = string(b)
	}

	if len(updates) == 0 {
		return errorResponse(c, "No fields to update")
	}

	if err := h.repos.Product.Update(id, updates); err != nil {
		return errorResponse(c, "Failed to update product")
	}
	return successResponse(c, "Product updated successfully", nil)
}

func (h *ProductHandler) deleteProduct(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	if err := h.repos.Product.Delete(id); err != nil {
		return errorResponse(c, "Failed to delete product")
	}
	return successResponse(c, "Product deleted successfully", nil)
}

func (h *ProductHandler) setInbounds(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	input := getStringField(body, "input")

	if id == 0 {
		return errorResponse(c, "id is required")
	}
	if strings.TrimSpace(input) == "" {
		return errorResponse(c, "input is required")
	}

	product, err := h.repos.Product.FindByID(id)
	if err != nil {
		return errorResponse(c, "product not found")
	}

	panelModel, err := h.repos.Panel.FindByName(product.Location)
	if err != nil || panelModel == nil {
		return errorResponse(c, "panel not found")
	}

	updates := map[string]interface{}{}
	switch strings.ToLower(strings.TrimSpace(panelModel.Type)) {
	case "ibsng", "mikrotik":
		updates["inbounds"] = input
	case "marzban", "pasarguard", "marzneshin", "hiddify", "alireza_single", "x-ui_single", "xui", "wgdashboard", "s_ui":
		inboundsJSON, proxiesJSON, err := extractPanelTemplate(panelModel, input)
		if err != nil {
			return errorResponse(c, err.Error())
		}
		updates["inbounds"] = inboundsJSON
		updates["proxies"] = proxiesJSON
	default:
		return errorResponse(c, "panel_not_support_options")
	}

	if err := h.repos.Product.Update(id, updates); err != nil {
		return errorResponse(c, "Failed to set inbounds")
	}
	return successResponse(c, "Inbounds set successfully", nil)
}

func (h *ProductHandler) removeInbounds(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	if err := h.repos.Product.Update(id, map[string]interface{}{"inbounds": nil, "proxies": nil}); err != nil {
		return errorResponse(c, "Failed to remove inbounds")
	}
	return successResponse(c, "Inbounds removed successfully", nil)
}
