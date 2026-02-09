package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
	"hosibot/internal/panel"
	"hosibot/internal/pkg/utils"
)

var miniappActiveStatuses = []string{
	"active",
	"end_of_time",
	"end_of_volume",
	"sendedwarn",
	"send_on_hold",
}

// MiniAppHandler handles legacy api/miniapp.php actions.
type MiniAppHandler struct {
	repos  *Repos
	logger *zap.Logger
}

func NewMiniAppHandler(repos *Repos, logger *zap.Logger) *MiniAppHandler {
	return &MiniAppHandler{repos: repos, logger: logger}
}

func (h *MiniAppHandler) Handle(c echo.Context) error {
	body := map[string]interface{}{}
	switch c.Request().Method {
	case http.MethodGet:
		q := c.QueryParams()
		for key := range q {
			body[key] = c.QueryParam(key)
		}
	case http.MethodPost:
		if err := c.Bind(&body); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"status": false,
				"msg":    "Data invalid",
				"obj":    []interface{}{},
			})
		}
	default:
		return c.JSON(http.StatusMethodNotAllowed, map[string]interface{}{
			"status": false,
			"msg":    "Method invalid",
			"obj":    []interface{}{},
		})
	}

	action := strings.TrimSpace(getStringField(body, "actions"))
	if action == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"status": false,
			"msg":    "Action Invalid",
			"obj":    []interface{}{},
		})
	}
	c.Set("api_actions", action)

	switch action {
	case "invoices":
		return h.invoices(c, body)
	case "service":
		return h.service(c, body)
	case "user_info":
		return h.userInfo(c)
	case "countries":
		return h.countries(c, body)
	case "categories":
		return h.categories(c, body)
	case "time_ranges":
		return h.timeRanges(c, body)
	case "services":
		return h.services(c, body)
	case "custom_price":
		return h.customPrice(c, body)
	case "purchase":
		return h.purchase(c, body)
	default:
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": false,
			"msg":    "Action Invalid",
			"obj":    []interface{}{},
		})
	}
}

func (h *MiniAppHandler) invoices(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}

	limit := getIntField(body, "limit", 10)
	if limit <= 0 {
		limit = 10
	}
	if limit > 10 {
		limit = 10
	}
	page := getIntField(body, "page", 1)
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	search := strings.TrimSpace(getStringField(body, "q"))

	type invoiceRow struct {
		Username        string `gorm:"column:username"`
		Note            string `gorm:"column:note"`
		ServiceLocation string `gorm:"column:Service_location"`
	}
	db := h.repos.Setting.DB().Model(&models.Invoice{}).
		Where("id_user = ? AND Status IN ?", user.ID, miniappActiveStatuses)
	if search != "" {
		db = db.Where("username LIKE ?", "%"+search+"%")
	}

	var totalItems int64
	_ = db.Count(&totalItems).Error

	var rows []invoiceRow
	if err := db.Select("username, note, Service_location").
		Order("time_sell DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		h.logger.Error("miniapp invoices query failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "Database error",
			"obj":    []interface{}{},
		})
	}

	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		status := "نامشخص"
		expire := "نامشخص"
		panelModel, _ := h.repos.Panel.FindByName(row.ServiceLocation)
		if panelModel != nil {
			if client, err := panel.PanelFactory(panelModel); err == nil {
				ctx := context.Background()
				if err := client.Authenticate(ctx); err == nil {
					if pu, err := client.GetUser(ctx, row.Username); err == nil && pu != nil {
						status = pu.Status
						if pu.ExpireTime > 0 {
							expire = time.Unix(pu.ExpireTime, 0).Format("2006/01/02")
						} else {
							expire = "نامحدود"
						}
					}
				}
			}
		}
		out = append(out, map[string]interface{}{
			"username": row.Username,
			"status":   status,
			"expire":   expire,
			"note":     row.Note,
		})
	}

	totalPages := 0
	if totalItems > 0 {
		totalPages = int((totalItems + int64(limit) - 1) / int64(limit))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj":    out,
		"meta": map[string]interface{}{
			"currentPage": page,
			"totalPages":  totalPages,
			"totalItems":  totalItems,
			"limit":       limit,
		},
	})
}

func (h *MiniAppHandler) service(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}
	username := strings.TrimSpace(getStringField(body, "username"))
	if username == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"status": false,
			"msg":    "username required",
			"obj":    []interface{}{},
		})
	}

	var invoice models.Invoice
	err := h.repos.Setting.DB().Model(&models.Invoice{}).
		Where("id_user = ? AND username = ? AND Status IN ?", user.ID, username, miniappActiveStatuses).
		First(&invoice).Error
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"status": false,
			"msg":    "Service Not  Found",
			"obj":    []interface{}{},
		})
	}

	panelModel, err := h.repos.Panel.FindByName(invoice.ServiceLocation)
	if err != nil || panelModel == nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"status": false,
			"msg":    "Panel Not Found",
			"obj":    []interface{}{},
		})
	}
	client, err := panel.PanelFactory(panelModel)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]interface{}{
			"status": false,
			"msg":    "Service data unavailable",
			"obj":    []interface{}{},
		})
	}
	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]interface{}{
			"status": false,
			"msg":    "Service data unavailable",
			"obj":    []interface{}{},
		})
	}
	panelUser, err := client.GetUser(ctx, invoice.Username)
	if err != nil || panelUser == nil {
		return c.JSON(http.StatusBadGateway, map[string]interface{}{
			"status": false,
			"msg":    "Service data unavailable",
			"obj":    []interface{}{},
		})
	}

	totalGB := float64(panelUser.DataLimit) / float64(1024*1024*1024)
	usedGB := float64(panelUser.UsedTraffic) / float64(1024*1024*1024)
	remainGB := totalGB - usedGB
	if remainGB < 0 {
		remainGB = 0
	}

	serviceOutput := make([]map[string]interface{}, 0)
	switch strings.ToLower(strings.TrimSpace(panelModel.Type)) {
	case "marzban", "marzneshin", "alireza_single", "x-ui_single", "xui", "hiddify", "eylanpanel":
		if panelModel.SubLink == "onsublink" && strings.TrimSpace(panelUser.SubLink) != "" {
			serviceOutput = append(serviceOutput, map[string]interface{}{
				"type":  "link",
				"value": panelUser.SubLink,
			})
		}
		if panelModel.Config == "onconfig" && len(panelUser.Links) > 0 {
			serviceOutput = append(serviceOutput, map[string]interface{}{
				"type":  "config",
				"value": panelUser.Links,
			})
		}
	case "wgdashboard":
		serviceOutput = append(serviceOutput, map[string]interface{}{
			"type":     "file",
			"value":    panelUser.SubLink,
			"filename": panelModel.InboundID + "_" + invoice.IDUser + "_" + invoice.IDInvoice + ".config",
		})
	case "mikrotik", "ibsng":
		serviceOutput = append(serviceOutput, map[string]interface{}{
			"type":  "password",
			"value": "",
		})
	}

	expirationDate := "نامحدود"
	if panelUser.ExpireTime > 0 {
		expirationDate = time.Unix(panelUser.ExpireTime, 0).Format("2006/01/02")
	}
	lastOnline := "متصل نشده"
	if panelUser.OnlineAt > 0 {
		lastOnline = time.Unix(panelUser.OnlineAt, 0).Format("2006/01/02 15:04:05")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj": map[string]interface{}{
			"status":                   panelUser.Status,
			"username":                 panelUser.Username,
			"product_name":             invoice.NameProduct,
			"total_traffic_gb":         roundFloat(totalGB, 2),
			"used_traffic_gb":          roundFloat(usedGB, 2),
			"remaining_traffic_gb":     roundFloat(remainGB, 2),
			"expiration_time":          expirationDate,
			"last_subscription_update": nil,
			"online_at":                lastOnline,
			"service_output":           serviceOutput,
		},
	})
}

func (h *MiniAppHandler) userInfo(c echo.Context) error {
	user, httpCode, resp := h.authenticateMiniAppUser(c, "")
	if user == nil {
		return c.JSON(httpCode, resp)
	}

	if !user.CodeInvitation.Valid || strings.TrimSpace(user.CodeInvitation.String) == "" {
		newCode := utils.RandomCode(8)
		_ = h.repos.User.Update(user.ID, map[string]interface{}{
			"codeInvitation": sql.NullString{String: newCode, Valid: true},
		})
		user.CodeInvitation = sql.NullString{String: newCode, Valid: true}
	}

	phone := user.Number
	if phone == "" || phone == "none" {
		phone = "🔴 ارسال نشده است 🔴"
	}
	if phone == "confrim number by admin" {
		phone = "✅ تایید شده توسط ادمین"
	}

	var countOrder int64
	_ = h.repos.Setting.DB().Model(&models.Invoice{}).
		Where("id_user = ? AND name_product != ? AND Status IN ?", user.ID, "سرویس تست", miniappActiveStatuses).
		Count(&countOrder).Error

	var countPayment int64
	_ = h.repos.Setting.DB().Model(&models.PaymentReport{}).
		Where("id_user = ? AND payment_Status = ?", user.ID, "paid").
		Count(&countPayment).Error

	groupType := "عادی"
	switch strings.TrimSpace(user.Agent) {
	case "n":
		groupType = "نماینده"
	case "n2":
		groupType = "نمایندگی پیشرفته"
	}

	timeJoin := ""
	if ts, err := strconv.ParseInt(strings.TrimSpace(user.Register), 10, 64); err == nil && ts > 0 {
		timeJoin = time.Unix(ts, 0).Format("2006/01/02")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj": map[string]interface{}{
			"codeInvitation":  user.CodeInvitation.String,
			"balance":         user.Balance,
			"phone":           phone,
			"count_order":     countOrder,
			"count_payment":   countPayment,
			"group_type":      groupType,
			"time_join":       timeJoin,
			"affiliatescount": user.AffiliatesCount,
		},
	})
}

func (h *MiniAppHandler) countries(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}

	panels := make([]models.Panel, 0)
	if err := h.repos.Setting.DB().Model(&models.Panel{}).
		Where("status = ? AND (agent = ? OR agent = ?) AND type != ?", "active", user.Agent, "all", "Manualsale").
		Find(&panels).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "Database error",
		})
	}

	setting, _ := h.repos.Setting.GetSettings()
	isNote := false
	if setting != nil && setting.StatusNameCustom == "onnamecustom" {
		isNote = true
	}
	if setting != nil && setting.StatusNoteForF == "0" && user.Agent == "f" {
		isNote = false
	}

	out := make([]map[string]interface{}, 0, len(panels))
	for _, p := range panels {
		if hiddenForUser(p.HideUser, user.ID) {
			continue
		}
		method := strings.TrimSpace(p.MethodUsername)
		isUsername := strings.Contains(method, "دلخواه")
		isCustom := agentJSONInt(p.CustomVolume, user.Agent) == 1 && !strings.EqualFold(strings.TrimSpace(p.Type), "Manualsale")
		out = append(out, map[string]interface{}{
			"id":          p.CodePanel,
			"name":        p.NamePanel,
			"is_custom":   isCustom,
			"is_username": isUsername,
			"is_note":     isNote,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj":    out,
	})
}

func (h *MiniAppHandler) categories(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}
	setting, _ := h.repos.Setting.GetSettings()
	if setting != nil && setting.StatusCategoryGeneral == "offcategorys" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": true,
			"msg":    "Successful",
			"obj":    []interface{}{},
		})
	}

	panelCode := getStringField(body, "id_panel")
	panelModel, err := h.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": false,
			"msg":    "panel not fonud!(invalid id_panel)",
		})
	}

	var categories []models.Category
	_ = h.repos.Setting.DB().Model(&models.Category{}).Find(&categories).Error
	out := make([]map[string]interface{}, 0, len(categories))
	for _, cat := range categories {
		var count int64
		_ = h.repos.Setting.DB().Model(&models.Product{}).
			Where("(Location = ? OR Location = ?) AND category = ? AND agent = ?", panelModel.NamePanel, "/all", cat.Remark, user.Agent).
			Count(&count).Error
		if count == 0 {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":   cat.ID,
			"name": cat.Remark,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj":    out,
	})
}

func (h *MiniAppHandler) timeRanges(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}
	setting, _ := h.repos.Setting.GetSettings()
	if setting != nil && setting.StatusCategory == "offcategory" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": true,
			"msg":    "Successful",
			"obj":    []interface{}{},
		})
	}

	panelCode := getStringField(body, "id_panel")
	panelModel, err := h.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": false,
			"msg":    "panel not fonud!(invalid id_panel)",
		})
	}

	var serviceTimes []string
	_ = h.repos.Setting.DB().Model(&models.Product{}).
		Where("(Location = ? OR Location = ?) AND agent = ?", panelModel.NamePanel, "/all", user.Agent).
		Distinct("Service_time").
		Pluck("Service_time", &serviceTimes).Error

	available := map[int]bool{}
	for _, s := range serviceTimes {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			available[n] = true
		}
	}

	candidates := []int{1, 7, 31, 30, 61, 60, 91, 90, 121, 120, 181, 180, 365, 0}
	out := make([]map[string]interface{}, 0)
	for _, day := range candidates {
		if !available[day] {
			continue
		}
		name := fmt.Sprintf("%d days", day)
		if day == 0 {
			name = "Unlimited"
		}
		out = append(out, map[string]interface{}{
			"id":   0,
			"name": name,
			"day":  day,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj":    out,
	})
}

func (h *MiniAppHandler) services(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}

	panelCode := getStringField(body, "id_panel")
	panelModel, err := h.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": false,
			"msg":    "panel not fonud!(invalid id_panel)",
		})
	}

	db := h.repos.Setting.DB().Model(&models.Product{}).
		Where("(Location = ? OR Location = ?) AND agent = ?", panelModel.NamePanel, "/all", user.Agent)

	if categoryID := getIntField(body, "category_id", 0); categoryID > 0 {
		category, err := h.repos.Setting.FindCategoryByID(categoryID)
		if err != nil || category == nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"status": false,
				"msg":    "category not found!(invalid category_id)",
			})
		}
		db = db.Where("category = ?", category.Remark)
	}
	if timeRange := getIntField(body, "time_range_day", 0); timeRange > 0 {
		db = db.Where("Service_time = ?", fmt.Sprintf("%d", timeRange))
	}

	var products []models.Product
	if err := db.Find(&products).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "Database error",
		})
	}

	var userInvoicesCount int64
	_ = h.repos.Setting.DB().Model(&models.Invoice{}).
		Where("id_user = ? AND Status != ?", user.ID, "Unpaid").
		Count(&userInvoicesCount).Error

	discountPct := 0
	if user.PriceDiscount.Valid {
		discountPct, _ = strconv.Atoi(strings.TrimSpace(user.PriceDiscount.String))
	}

	out := make([]map[string]interface{}, 0, len(products))
	for _, p := range products {
		if hiddenForPanel(p.HidePanel, panelModel.NamePanel) {
			continue
		}
		if strings.TrimSpace(p.OneBuyStatus) == "1" && userInvoicesCount > 0 {
			continue
		}
		price := parseIntSafe(p.PriceProduct)
		if discountPct > 0 {
			price = price - (price*discountPct)/100
		}
		out = append(out, map[string]interface{}{
			"id":            p.CodeProduct,
			"name":          p.NameProduct,
			"description":   p.Note,
			"price":         price,
			"traffic_gb":    parseIntSafe(p.VolumeConstraint),
			"time_days":     parseIntSafe(p.ServiceTime),
			"country_id":    panelModel.CodePanel,
			"time_range_id": parseIntSafe(p.ServiceTime),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj":    out,
	})
}

func (h *MiniAppHandler) customPrice(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}

	panelCode := getStringField(body, "id_panel")
	panelModel, err := h.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": false,
			"msg":    "panel not fonud!(invalid id_panel)",
		})
	}

	trafficGB := getIntField(body, "traffic_gb", 0)
	timeDays := getIntField(body, "time_days", 0)

	statusCustom := agentJSONInt(panelModel.CustomVolume, user.Agent)
	trafficMin := agentJSONInt(panelModel.MainVolume, user.Agent)
	trafficMax := agentJSONInt(panelModel.MaxVolume, user.Agent)
	timeMin := agentJSONInt(panelModel.MainTime, user.Agent)
	timeMax := agentJSONInt(panelModel.MaxTime, user.Agent)
	trafficPrice := agentJSONFloat(panelModel.PriceCustomVolume, user.Agent)
	timePrice := agentJSONFloat(panelModel.PriceCustomTime, user.Agent)

	var price interface{} = false
	if statusCustom == 1 && !strings.EqualFold(strings.TrimSpace(panelModel.Type), "Manualsale") {
		price = int((float64(trafficGB) * trafficPrice) + (float64(timeDays) * timePrice))
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "Successful",
		"obj": map[string]interface{}{
			"price":       price,
			"traffic_min": trafficMin,
			"traffic_max": trafficMax,
			"time_min":    timeMin,
			"time_max":    timeMax,
		},
	})
}

func (h *MiniAppHandler) purchase(c echo.Context, body map[string]interface{}) error {
	userID := getStringField(body, "user_id")
	user, httpCode, resp := h.authenticateMiniAppUser(c, userID)
	if user == nil {
		return c.JSON(httpCode, resp)
	}

	panelCode := getStringField(body, "country_id")
	panelModel, err := h.repos.Panel.FindByCode(panelCode)
	if err != nil || panelModel == nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "پنل انتخابی موجود نیست.",
		})
	}
	if strings.EqualFold(strings.TrimSpace(panelModel.Status), "disable") {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "پنل انتخابی درحال حاضر فعال نیست",
		})
	}

	product, price, volumeGB, serviceDays, err := h.resolveMiniappProduct(user, panelModel, body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    err.Error(),
		})
	}

	discountPct := 0
	if user.PriceDiscount.Valid {
		discountPct, _ = strconv.Atoi(strings.TrimSpace(user.PriceDiscount.String))
	}
	if discountPct > 0 {
		price = price - (price*discountPct)/100
	}
	if user.Balance < price {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "موجودی کمتر از قیمت محصول است",
		})
	}

	username := h.generateMiniappUsername(panelModel, user, getStringField(body, "custom_username"))
	if strings.TrimSpace(username) == "" {
		username = fmt.Sprintf("%s_%s", user.ID, utils.RandomCode(5))
	}
	username = strings.ToLower(strings.TrimSpace(username))

	// Avoid username collisions in invoice table.
	var exists int64
	_ = h.repos.Setting.DB().Model(&models.Invoice{}).Where("username = ?", username).Count(&exists).Error
	if exists > 0 {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "نام کاربری وجود دارد مراحل را از اول طی کنید",
		})
	}

	client, err := panel.PanelFactory(panelModel)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "خطایی در ساخت اشتراک رخ داده است با پشتیبانی در ارتباط باشید",
		})
	}
	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "خطایی در ساخت اشتراک رخ داده است با پشتیبانی در ارتباط باشید",
		})
	}
	if _, err := client.GetUser(ctx, username); err == nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "نام کاربری وجود دارد مراحل را از اول طی کنید",
		})
	}

	inbounds := parseInboundsMap(product.Inbounds)
	if len(inbounds) == 0 {
		inbounds = parseInboundsMap(panelModel.Inbounds)
	}
	proxies := parseProxiesMap(product.Proxies)
	if len(proxies) == 0 {
		proxies = parseProxiesMap(panelModel.Proxies)
	}

	dataLimitBytes := int64(volumeGB) * 1024 * 1024 * 1024
	createReq := panel.CreateUserRequest{
		Username:       username,
		DataLimit:      dataLimitBytes,
		ExpireDays:     serviceDays,
		Inbounds:       inbounds,
		Proxies:        proxies,
		Note:           getStringField(body, "custom_note"),
		DataLimitReset: product.DataLimitReset,
	}
	panelUser, err := client.CreateUser(ctx, createReq)
	if err != nil || panelUser == nil || strings.TrimSpace(panelUser.Username) == "" {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "خطایی در ساخت اشتراک رخ داده است با پشتیبانی در ارتباط باشید",
		})
	}

	idBytes := make([]byte, 4)
	_, _ = rand.Read(idBytes)
	orderID := hex.EncodeToString(idBytes)
	if orderID == "" {
		orderID = utils.RandomCode(8)
	}
	nowUnix := time.Now().Unix()
	notifications, _ := json.Marshal(map[string]bool{"volume": false, "time": false})
	invoice := &models.Invoice{
		IDInvoice:       orderID,
		IDUser:          user.ID,
		Username:        username,
		TimeSell:        fmt.Sprintf("%d", nowUnix),
		ServiceLocation: panelModel.NamePanel,
		NameProduct:     product.NameProduct,
		PriceProduct:    fmt.Sprintf("%d", price),
		Volume:          fmt.Sprintf("%d", volumeGB),
		ServiceTime:     fmt.Sprintf("%d", serviceDays),
		Status:          "active",
		Note:            getStringField(body, "custom_note"),
		Referral:        user.Affiliates,
		Notifications:   string(notifications),
	}
	if panelUser.SubLink != "" {
		invoice.UUID = panelUser.SubLink
	}
	if err := h.repos.Invoice.Create(invoice); err != nil {
		h.logger.Error("miniapp purchase create invoice failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "خطایی در ساخت اشتراک رخ داده است با پشتیبانی در ارتباط باشید",
		})
	}

	if price > 0 {
		_ = h.repos.User.UpdateBalance(user.ID, -price)
	}

	expire := int64(0)
	if serviceDays > 0 {
		expire = nowUnix + int64(serviceDays)*86400
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":  true,
		"message":  "ok",
		"order_id": orderID,
		"service": map[string]interface{}{
			"id":       orderID,
			"username": username,
			"status":   "active",
			"expire":   expire,
		},
	})
}

func (h *MiniAppHandler) resolveMiniappProduct(
	user *models.User,
	panelModel *models.Panel,
	body map[string]interface{},
) (*models.Product, int, int, int, error) {
	if customRaw, ok := body["custom_service"]; ok && customRaw != nil {
		customMap, ok := customRaw.(map[string]interface{})
		if !ok {
			return nil, 0, 0, 0, fmt.Errorf("custom_service invalid")
		}
		traffic := getIntField(customMap, "traffic_gb", 0)
		days := getIntField(customMap, "time_days", 0)

		statusCustom := agentJSONInt(panelModel.CustomVolume, user.Agent)
		mainVol := agentJSONInt(panelModel.MainVolume, user.Agent)
		maxVol := agentJSONInt(panelModel.MaxVolume, user.Agent)
		mainTime := agentJSONInt(panelModel.MainTime, user.Agent)
		maxTime := agentJSONInt(panelModel.MaxTime, user.Agent)
		priceVol := agentJSONFloat(panelModel.PriceCustomVolume, user.Agent)
		priceTime := agentJSONFloat(panelModel.PriceCustomTime, user.Agent)
		if statusCustom != 1 || strings.EqualFold(strings.TrimSpace(panelModel.Type), "Manualsale") {
			return nil, 0, 0, 0, fmt.Errorf("خرید سرویس دلخواه برای این پنل فعال نیست")
		}
		if traffic > maxVol || traffic < mainVol {
			return nil, 0, 0, 0, fmt.Errorf("حجم نامعتبر است خرید را از اول انجام دهید")
		}
		if days > maxTime || days < mainTime {
			return nil, 0, 0, 0, fmt.Errorf("زمان نامعتبر است خرید را از اول انجام دهید")
		}
		price := int((float64(traffic) * priceVol) + (float64(days) * priceTime))
		product := &models.Product{
			CodeProduct:      "customvolume",
			NameProduct:      "سرویس دلخواه",
			VolumeConstraint: fmt.Sprintf("%d", traffic),
			ServiceTime:      fmt.Sprintf("%d", days),
			Location:         panelModel.NamePanel,
			PriceProduct:     fmt.Sprintf("%d", price),
		}
		return product, price, traffic, days, nil
	}

	serviceID := getStringField(body, "service_id")
	if strings.TrimSpace(serviceID) == "" {
		return nil, 0, 0, 0, fmt.Errorf("محصول انتخابی پیدا نشد")
	}
	product, err := h.repos.Product.FindByCode(serviceID)
	if err != nil || product == nil {
		return nil, 0, 0, 0, fmt.Errorf("محصول انتخابی پیدا نشد")
	}
	price := parseIntSafe(product.PriceProduct)
	traffic := parseIntSafe(product.VolumeConstraint)
	days := parseIntSafe(product.ServiceTime)
	return product, price, traffic, days, nil
}

func (h *MiniAppHandler) generateMiniappUsername(panelModel *models.Panel, user *models.User, customUsername string) string {
	method := strings.TrimSpace(panelModel.MethodUsername)
	cleanCustom := utils.SanitizeUsername(strings.TrimSpace(customUsername))
	if cleanCustom != "" && (strings.Contains(method, "دلخواه") || strings.Contains(strings.ToLower(method), "custom")) {
		return cleanCustom
	}

	switch method {
	case "آیدی عددی + حروف و عدد رندوم", "1":
		return fmt.Sprintf("%s_%s", user.ID, utils.RandomCode(5))
	case "نام کاربری + عدد به ترتیب", "2":
		counter := parseIntSafe(panelModel.Connection) + 1
		_ = h.repos.Panel.IncrementCounter(panelModel.CodePanel)
		name := user.Username
		if strings.TrimSpace(name) == "" {
			name = user.ID
		}
		return fmt.Sprintf("%s%d", utils.SanitizeUsername(name), counter)
	case "متن دلخواه + عدد رندوم", "5":
		prefix := strings.TrimSpace(panelModel.NameCustom)
		if prefix == "" {
			prefix = "user"
		}
		return fmt.Sprintf("%s_%s", utils.SanitizeUsername(prefix), utils.RandomCode(5))
	case "متن دلخواه + عدد ترتیبی", "6":
		prefix := strings.TrimSpace(panelModel.NameCustom)
		if prefix == "" {
			prefix = "user"
		}
		counter := parseIntSafe(panelModel.Connection) + 1
		_ = h.repos.Panel.IncrementCounter(panelModel.CodePanel)
		return fmt.Sprintf("%s%d", utils.SanitizeUsername(prefix), counter)
	case "آیدی عددی+عدد ترتیبی", "7":
		counter := parseIntSafe(panelModel.Connection) + 1
		_ = h.repos.Panel.IncrementCounter(panelModel.CodePanel)
		return fmt.Sprintf("%s%d", user.ID, counter)
	default:
		if cleanCustom != "" {
			return cleanCustom
		}
		return fmt.Sprintf("%s_%s", user.ID, utils.RandomCode(5))
	}
}

func (h *MiniAppHandler) authenticateMiniAppUser(c echo.Context, userID string) (*models.User, int, map[string]interface{}) {
	token := bearerToken(c.Request().Header.Get("Authorization"))
	if token == "" {
		return nil, http.StatusForbidden, map[string]interface{}{
			"status": false,
			"msg":    "Token invalid",
		}
	}

	var user models.User
	db := h.repos.Setting.DB().Model(&models.User{})
	if strings.TrimSpace(userID) != "" {
		if err := db.Where("id = ?", userID).First(&user).Error; err != nil {
			return nil, http.StatusNotFound, map[string]interface{}{
				"status": false,
				"msg":    "User Not  Found",
			}
		}
	} else {
		if err := db.Where("token = ?", token).First(&user).Error; err != nil {
			return nil, http.StatusNotFound, map[string]interface{}{
				"status": false,
				"msg":    "User Not  Found",
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(user.UserStatus), "block") ||
		strings.EqualFold(strings.TrimSpace(user.UserStatus), "blocked") {
		return nil, 402, map[string]interface{}{
			"status": false,
			"msg":    "user blocked",
		}
	}

	if !user.Token.Valid || strings.TrimSpace(user.Token.String) != token {
		return nil, http.StatusForbidden, map[string]interface{}{
			"status": false,
			"msg":    "Token invalid",
		}
	}
	return &user, 0, nil
}

func bearerToken(authorization string) string {
	raw := strings.TrimSpace(authorization)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		raw = strings.TrimSpace(raw[7:])
	}
	return raw
}

func hiddenForUser(raw, userID string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var arr []interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return false
	}
	for _, v := range arr {
		if strings.TrimSpace(fmt.Sprintf("%v", v)) == strings.TrimSpace(userID) {
			return true
		}
	}
	return false
}

func hiddenForPanel(raw, panelName string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return false
	}
	for _, v := range arr {
		if strings.TrimSpace(v) == strings.TrimSpace(panelName) {
			return true
		}
	}
	return false
}

func agentJSONInt(raw, agent string) int {
	if raw == "" {
		return 0
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return 0
	}
	if v, ok := obj[agent]; ok {
		return parseIntSafe(fmt.Sprintf("%v", v))
	}
	if v, ok := obj["all"]; ok {
		return parseIntSafe(fmt.Sprintf("%v", v))
	}
	return 0
}

func agentJSONFloat(raw, agent string) float64 {
	if raw == "" {
		return 0
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return 0
	}
	if v, ok := obj[agent]; ok {
		return parseFloatSafe(fmt.Sprintf("%v", v))
	}
	if v, ok := obj["all"]; ok {
		return parseFloatSafe(fmt.Sprintf("%v", v))
	}
	return 0
}

func parseIntSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func parseFloatSafe(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

func roundFloat(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return float64(int(v*pow+0.5)) / pow
}

func parseInboundsMap(raw string) map[string][]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string][]string{}
	if err := json.Unmarshal([]byte(raw), &out); err == nil && len(out) > 0 {
		return out
	}
	var asList []string
	if err := json.Unmarshal([]byte(raw), &asList); err == nil && len(asList) > 0 {
		for _, proto := range asList {
			p := strings.TrimSpace(proto)
			if p != "" {
				out[p] = []string{}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func parseProxiesMap(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err == nil && len(out) > 0 {
		return out
	}
	return nil
}
