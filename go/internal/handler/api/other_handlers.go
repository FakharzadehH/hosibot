package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
	"hosibot/internal/pkg/utils"
)

// PanelHandler handles all panel API actions.
// Matches PHP api/panels.php behavior.
type PanelHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewPanelHandler(repos *Repos, logger *zap.Logger) *PanelHandler {
	return &PanelHandler{repos: repos, logger: logger}
}

// Handle routes panel API requests.
// POST /api/panels
func (h *PanelHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "panels":
		return h.listPanels(c, body)
	case "panel":
		return h.getPanel(c, body)
	case "panel_add":
		return h.addPanel(c, body)
	case "panel_edit":
		return h.editPanel(c, body)
	case "panel_delete":
		return h.deletePanel(c, body)
	case "set_inbounds":
		return h.setInbounds(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *PanelHandler) listPanels(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	panels, total, err := h.repos.Panel.FindAll(limit, page, q)
	if err != nil {
		h.logger.Error("Failed to list panels", zap.Error(err))
		return errorResponse(c, "Failed to retrieve panels")
	}

	return successResponse(c, "Successful", paginatedNamedResponse("panels", panels, total, page, limit))
}

func (h *PanelHandler) getPanel(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	panel, err := h.repos.Panel.FindByID(id)
	if err != nil {
		return errorResponse(c, "Panel not found")
	}

	categories, _, _ := h.repos.Setting.FindAllCategories(0, 1, "")
	return successResponse(c, "Successful", map[string]interface{}{
		"panel":    panel,
		"category": categories,
	})
}

func (h *PanelHandler) addPanel(c echo.Context, body map[string]interface{}) error {
	name := getStringField(body, "name")
	if name == "" {
		return errorResponse(c, "name is required")
	}
	if h.repos.ServicePanel == nil {
		return errorResponse(c, "Service panel repository unavailable")
	}

	exists, err := h.repos.ServicePanel.CountByName(name)
	if err == nil && exists > 0 {
		return errorResponse(c, "panel name exits")
	}

	location := getStringField(body, "location")
	if location == "" {
		return errorResponse(c, "location is required")
	}
	locationName := location
	if location != "/all" {
		basePanel, err := h.repos.Panel.FindByCode(location)
		if err != nil || basePanel == nil {
			return errorResponse(c, "location not found")
		}
		locationName = basePanel.NamePanel
	}

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
	hidePanelJSON := "{}"
	if v, ok := body["hide_panel"]; ok && v != nil {
		b, _ := json.Marshal(v)
		hidePanelJSON = string(b)
	}

	agent := getStringField(body, "agent")
	if strings.TrimSpace(agent) == "" {
		agent = "f"
	}
	note := getStringField(body, "note")
	dataLimitReset := getStringField(body, "data_limit_reset")
	if strings.TrimSpace(dataLimitReset) == "" {
		dataLimitReset = "no_reset"
	}

	panel := &models.ServicePanel{
		CodePanel:        utils.RandomCode(6),
		NamePanel:        name,
		PricePanel:       fmt.Sprintf("%d", getIntField(body, "price", 0)),
		VolumeConstraint: fmt.Sprintf("%d", getIntField(body, "data_limit", 0)),
		ServiceTime:      fmt.Sprintf("%d", getIntField(body, "time", 0)),
		Location:         locationName,
		Agent:            agent,
		Note:             note,
		DataLimitReset:   dataLimitReset,
		Inbounds:         inboundsJSON,
		Proxies:          proxiesJSON,
		Category:         getStringField(body, "category"),
		OneBuyStatus:     fmt.Sprintf("%d", getIntField(body, "one_buy_status", 0)),
		HidePanel:        hidePanelJSON,
		Status:           "active",
	}

	if err := h.repos.ServicePanel.Create(panel); err != nil {
		h.logger.Error("Failed to create service panel", zap.Error(err))
		return errorResponse(c, "An error occurred while editing panel")
	}

	return successResponse(c, "Successful", nil)
}

func (h *PanelHandler) editPanel(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}
	if h.repos.ServicePanel == nil {
		return errorResponse(c, "Service panel repository unavailable")
	}

	current, err := h.repos.ServicePanel.FindByID(id)
	if err != nil || current == nil {
		return errorResponse(c, "panel not found")
	}

	updates := map[string]interface{}{}

	fieldMap := map[string]string{
		"name":             "name_panel",
		"sublink":          "sublink",
		"config":           "config",
		"status":           "status",
		"agent":            "agent",
		"note":             "note",
		"data_limit_reset": "data_limit_reset",
		"category":         "category",
	}

	for jsonKey, dbCol := range fieldMap {
		if v := getStringField(body, jsonKey); v != "" {
			updates[dbCol] = v
		}
	}
	if v, ok := body["one_buy_status"]; ok {
		updates["one_buy_status"] = fmt.Sprintf("%v", v)
	}

	if v := getStringField(body, "location"); v != "" {
		locationName := v
		if v != "/all" {
			if basePanel, err := h.repos.Panel.FindByCode(v); err == nil && basePanel != nil {
				locationName = basePanel.NamePanel
			}
		}
		updates["Location"] = locationName
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

	if newName, ok := updates["name_panel"].(string); ok && strings.TrimSpace(newName) != "" && newName != current.NamePanel {
		exists, err := h.repos.ServicePanel.CountByName(newName)
		if err == nil && exists > 0 {
			return errorResponse(c, "panel name exits")
		}
		_ = h.repos.Setting.DB().Model(&models.Invoice{}).
			Where("Service_location = ?", current.NamePanel).
			Update("Service_location", newName).Error
	}

	if err := h.repos.ServicePanel.Update(id, updates); err != nil {
		return errorResponse(c, "Failed to update panel")
	}
	return successResponse(c, "panel updated successfully", nil)
}

func (h *PanelHandler) deletePanel(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}
	if h.repos.ServicePanel == nil {
		return errorResponse(c, "Service panel repository unavailable")
	}
	if _, err := h.repos.ServicePanel.FindByID(id); err != nil {
		return errorResponse(c, "panel not found")
	}
	if err := h.repos.ServicePanel.Delete(id); err != nil {
		return errorResponse(c, "Failed to delete panel")
	}
	return successResponse(c, "panel delete successfully", nil)
}

func (h *PanelHandler) setInbounds(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	input := getStringField(body, "input")

	if id == 0 {
		return errorResponse(c, "id is required")
	}
	if strings.TrimSpace(input) == "" {
		return errorResponse(c, "input is required")
	}
	if h.repos.ServicePanel == nil {
		return errorResponse(c, "Service panel repository unavailable")
	}

	panelRow, err := h.repos.ServicePanel.FindByID(id)
	if err != nil || panelRow == nil {
		return errorResponse(c, "panel not found")
	}
	panelModel, err := h.repos.Panel.FindByName(panelRow.Location)
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

	if err := h.repos.ServicePanel.Update(id, updates); err != nil {
		return errorResponse(c, "Failed to set inbounds")
	}
	return successResponse(c, "successfully", nil)
}

// DiscountHandler handles discount API actions.
// Matches PHP api/discount.php behavior.
type DiscountHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewDiscountHandler(repos *Repos, logger *zap.Logger) *DiscountHandler {
	return &DiscountHandler{repos: repos, logger: logger}
}

// Handle routes discount API requests.
// POST /api/discounts
func (h *DiscountHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "discounts":
		return h.listDiscounts(c, body)
	case "discount":
		return h.getDiscount(c, body)
	case "discount_add":
		return h.addDiscount(c, body)
	case "discount_delete":
		return h.deleteDiscount(c, body)
	case "discount_sell_lists":
		return h.listDiscountSells(c, body)
	case "discount_sell":
		return h.getDiscountSell(c, body)
	case "discount_sell_add":
		return h.addDiscountSell(c, body)
	case "discount_sell_delete":
		return h.deleteDiscountSell(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *DiscountHandler) listDiscounts(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	discounts, total, err := h.repos.Setting.FindAllDiscounts(limit, page, q)
	if err != nil {
		return errorResponse(c, "Failed to retrieve discounts")
	}

	return successResponse(c, "Successful", paginatedNamedResponse("discount", discounts, total, page, limit))
}

func (h *DiscountHandler) getDiscount(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	discount, err := h.repos.Setting.FindDiscountByID(id)
	if err != nil {
		return errorResponse(c, "Discount not found")
	}

	return successResponse(c, "Successful", discount)
}

func (h *DiscountHandler) addDiscount(c echo.Context, body map[string]interface{}) error {
	code := getStringField(body, "code")
	price := getIntField(body, "price", 0)
	limitUse := getIntField(body, "limit_use", 0)

	if code == "" {
		return errorResponse(c, "code is required")
	}

	discount := &models.Discount{
		Code:      code,
		Price:     fmt.Sprintf("%d", price),
		LimitUse:  fmt.Sprintf("%d", limitUse),
		LimitUsed: "0",
	}

	if err := h.repos.Setting.CreateDiscount(discount); err != nil {
		return errorResponse(c, "Failed to create discount")
	}

	return successResponse(c, "Discount created successfully", discount)
}

func (h *DiscountHandler) deleteDiscount(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	if err := h.repos.Setting.DeleteDiscount(id); err != nil {
		return errorResponse(c, "Failed to delete discount")
	}
	return successResponse(c, "Discount deleted successfully", nil)
}

func (h *DiscountHandler) listDiscountSells(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	sells, total, err := h.repos.Setting.FindAllDiscountSells(limit, page, q)
	if err != nil {
		return errorResponse(c, "Failed to retrieve discount sells")
	}

	return successResponse(c, "Successful", paginatedNamedResponse("discountsell", sells, total, page, limit))
}

func (h *DiscountHandler) getDiscountSell(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	sell, err := h.repos.Setting.FindDiscountSellByID(id)
	if err != nil {
		return errorResponse(c, "Discount sell not found")
	}

	return successResponse(c, "Successful", sell)
}

func (h *DiscountHandler) addDiscountSell(c echo.Context, body map[string]interface{}) error {
	code := getStringField(body, "code")
	if code == "" {
		return errorResponse(c, "code is required")
	}

	sell := &models.DiscountSell{
		CodeDiscount:  code,
		Price:         fmt.Sprintf("%d", getIntField(body, "percent", 0)),
		LimitDiscount: fmt.Sprintf("%d", getIntField(body, "limit_use", 0)),
		Agent:         getStringField(body, "agent"),
		UseFirst:      getStringField(body, "usefirst"),
		UseUser:       getStringField(body, "useuser"),
		CodeProduct:   getStringField(body, "code_product"),
		CodePanel:     getStringField(body, "code_panel"),
		Time:          getStringField(body, "time"),
		Type:          getStringField(body, "type"),
		UsedDiscount:  "0",
	}

	if err := h.repos.Setting.CreateDiscountSell(sell); err != nil {
		return errorResponse(c, "Failed to create discount sell")
	}

	return successResponse(c, "Discount sell created successfully", sell)
}

func (h *DiscountHandler) deleteDiscountSell(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	if err := h.repos.Setting.DeleteDiscountSell(id); err != nil {
		return errorResponse(c, "Failed to delete discount sell")
	}
	return successResponse(c, "Discount sell deleted successfully", nil)
}

// CategoryHandler handles category API actions.
// Matches PHP api/category.php behavior.
type CategoryHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewCategoryHandler(repos *Repos, logger *zap.Logger) *CategoryHandler {
	return &CategoryHandler{repos: repos, logger: logger}
}

// Handle routes category API requests.
// POST /api/categories
func (h *CategoryHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "categorys", "categories":
		return h.listCategories(c, body)
	case "category":
		return h.getCategory(c, body)
	case "category_add":
		return h.addCategory(c, body)
	case "category_edit":
		return h.editCategory(c, body)
	case "category_delete":
		return h.deleteCategory(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *CategoryHandler) listCategories(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)
	page := getIntField(body, "page", 1)
	q := getStringField(body, "q")

	categories, total, err := h.repos.Setting.FindAllCategories(limit, page, q)
	if err != nil {
		return errorResponse(c, "Failed to retrieve categories")
	}

	return successResponse(c, "Successful", paginatedNamedResponse("category", categories, total, page, limit))
}

func (h *CategoryHandler) getCategory(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	category, err := h.repos.Setting.FindCategoryByID(id)
	if err != nil {
		return errorResponse(c, "Category not found")
	}

	return successResponse(c, "Successful", category)
}

func (h *CategoryHandler) addCategory(c echo.Context, body map[string]interface{}) error {
	remark := getStringField(body, "remark")
	if remark == "" {
		return errorResponse(c, "remark is required")
	}

	category := &models.Category{Remark: remark}
	if err := h.repos.Setting.CreateCategory(category); err != nil {
		return errorResponse(c, "Failed to create category")
	}

	return successResponse(c, "Category created successfully", category)
}

func (h *CategoryHandler) editCategory(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	updates := make(map[string]interface{})
	if v := getStringField(body, "remark"); v != "" {
		updates["remark"] = v
	}

	if len(updates) == 0 {
		return errorResponse(c, "No fields to update")
	}

	if err := h.repos.Setting.UpdateCategory(id, updates); err != nil {
		return errorResponse(c, "Failed to update category")
	}
	return successResponse(c, "Category updated successfully", nil)
}

func (h *CategoryHandler) deleteCategory(c echo.Context, body map[string]interface{}) error {
	id := getIntField(body, "id", 0)
	if id == 0 {
		return errorResponse(c, "id is required")
	}

	if err := h.repos.Setting.DeleteCategory(id); err != nil {
		return errorResponse(c, "Failed to delete category")
	}
	return successResponse(c, "Category deleted successfully", nil)
}

// SettingsHandler handles settings API actions.
// Matches PHP api/settings.php behavior.
type SettingsHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewSettingsHandler(repos *Repos, logger *zap.Logger) *SettingsHandler {
	return &SettingsHandler{repos: repos, logger: logger}
}

// Handle routes settings API requests.
// POST /api/settings
func (h *SettingsHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "keyboard_set":
		return h.keyboardSet(c, body)
	case "setting_info":
		return h.settingInfo(c, body)
	case "save_setting_shop":
		return h.saveSettingShop(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *SettingsHandler) keyboardSet(c echo.Context, body map[string]interface{}) error {
	if reset, ok := body["keyboard_reset"]; ok && reset == true {
		defaultKeyboard := `{"keyboard":[[{"text":"text_sell"},{"text":"text_extend"}],[{"text":"text_usertest"},{"text":"text_wheel_luck"}],[{"text":"text_Purchased_services"},{"text":"accountwallet"}],[{"text":"text_affiliates"},{"text":"text_Tariff_list"}],[{"text":"text_support"},{"text":"text_help"}]]}`
		if err := h.repos.Setting.UpdateSetting("keyboardmain", defaultKeyboard); err != nil {
			return errorResponse(c, "Failed to reset keyboard")
		}
		return successResponse(c, "Successful", nil)
	}

	if keyboard, ok := body["keyboard"]; ok {
		keyboardJSON, _ := json.Marshal(map[string]interface{}{
			"keyboard": keyboard,
		})
		if err := h.repos.Setting.UpdateSetting("keyboardmain", string(keyboardJSON)); err != nil {
			return errorResponse(c, "Failed to set keyboard")
		}
		return successResponse(c, "Successful", nil)
	}

	return errorResponse(c, "keyboard or keyboard_reset is required")
}

func (h *SettingsHandler) settingInfo(c echo.Context, _ map[string]interface{}) error {
	setting, err := h.repos.Setting.GetSettings()
	if err != nil {
		return errorResponse(c, "Failed to get settings")
	}

	shopSettings, _ := h.repos.Setting.GetAllShopSettings()
	paySettings, _ := h.repos.Setting.GetAllPaySettings()

	shopKV := map[string]string{}
	for _, item := range shopSettings {
		shopKV[item.NameValue] = item.Value
	}
	result := map[string]interface{}{
		"setting_shop":    shopKV,
		"setting_General": setting,
		// Backward-compatible aliases
		"setting":      setting,
		"shopSettings": shopSettings,
		"paySettings":  paySettings,
	}

	return successResponse(c, "Successful", result)
}

func (h *SettingsHandler) saveSettingShop(c echo.Context, body map[string]interface{}) error {
	dataRaw, ok := body["data"]
	if !ok {
		return errorResponse(c, "data is required")
	}

	dataArr, ok := dataRaw.([]interface{})
	if !ok {
		return errorResponse(c, "data must be an array")
	}

	for _, item := range dataArr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		nameValue := getStringFieldFromMap(itemMap, "name_value")
		value := itemMap["value"]
		settingType := getStringFieldFromMap(itemMap, "type")
		isJSON, _ := itemMap["json"].(bool)

		var valueStr string
		if isJSON {
			b, _ := json.Marshal(value)
			valueStr = string(b)
		} else {
			valueStr = fmt.Sprintf("%v", value)
		}

		switch settingType {
		case "shop":
			_ = h.repos.Setting.SetShopSetting(nameValue, valueStr)
		case "general":
			_ = h.repos.Setting.UpdateSetting(nameValue, valueStr)
		}
	}

	return successResponse(c, "Settings saved successfully", nil)
}

// getStringFieldFromMap is a helper for nested maps.
func getStringFieldFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ServiceHandler handles service_other API actions.
type ServiceHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewServiceHandler(repos *Repos, logger *zap.Logger) *ServiceHandler {
	return &ServiceHandler{repos: repos, logger: logger}
}

// Handle routes service API requests.
// POST /api/services
func (h *ServiceHandler) Handle(c echo.Context) error {
	action, body, err := parseBodyAction(c)
	if err != nil {
		return errorResponse(c, "Invalid request body")
	}

	switch action {
	case "services":
		return h.listServices(c, body)
	default:
		return errorResponse(c, "Unknown action: "+action)
	}
}

func (h *ServiceHandler) listServices(c echo.Context, body map[string]interface{}) error {
	limit := getIntField(body, "limit", 50)

	services, err := h.repos.Setting.FindAllServiceOther(limit)
	if err != nil {
		return errorResponse(c, "Failed to retrieve services")
	}

	return successResponse(c, "Successful", services)
}
