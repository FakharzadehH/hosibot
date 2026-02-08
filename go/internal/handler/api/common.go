package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"hosibot/internal/models"
	"hosibot/internal/repository"
)

// Response helpers matching PHP's sendJsonResponse().
func successResponse(c echo.Context, msg string, obj interface{}) error {
	return c.JSON(http.StatusOK, models.APIResponse{
		Status: true,
		Msg:    msg,
		Obj:    obj,
	})
}

func errorResponse(c echo.Context, msg string) error {
	return c.JSON(http.StatusOK, models.APIResponse{
		Status: false,
		Msg:    msg,
		Obj:    nil,
	})
}

func paginatedResponse(data interface{}, total int64, page, limit int) models.PaginatedResponse {
	totalPages := totalPages(total, limit)
	return models.PaginatedResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}
}

func totalPages(total int64, limit int) int {
	if limit <= 0 {
		limit = 50
	}
	pages := int(total) / limit
	if int(total)%limit != 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	return pages
}

// paginatedNamedResponse returns PHP-compatible list payloads:
// { "<key>": [...], "pagination": {...} }.
func paginatedNamedResponse(key string, data interface{}, total int64, page, limit int) map[string]interface{} {
	pagination := map[string]interface{}{
		"total_pages":  totalPages(total, limit),
		"current_page": page,
		"per_page":     limit,
	}
	switch key {
	case "users":
		pagination["total_users"] = total
	case "panels":
		pagination["total_panel"] = total
	case "discount":
		pagination["total_discount"] = total
	default:
		pagination["total_record"] = total
	}
	return map[string]interface{}{
		key:          data,
		"pagination": pagination,
	}
}

// parseBodyAction extracts the "actions" field from request body.
// This is the core PHP routing mechanism: all API requests have an "actions" field.
func parseBodyAction(c echo.Context) (string, map[string]interface{}, error) {
	body := make(map[string]interface{})
	if err := c.Bind(&body); err != nil {
		// Try reading as JSON manually
		return "", nil, err
	}
	action, _ := body["actions"].(string)
	c.Set("api_actions", action) // for logging middleware
	return action, body, nil
}

// getStringField gets a string field from the body map.
func getStringField(body map[string]interface{}, key string) string {
	if v, ok := body[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		// Handle numbers that should be strings
		if f, ok := v.(float64); ok {
			return fmt.Sprintf("%.0f", f)
		}
	}
	return ""
}

// getIntField gets an int field from the body map.
func getIntField(body map[string]interface{}, key string, defaultVal int) int {
	if v, ok := body[key]; ok {
		switch t := v.(type) {
		case float64:
			return int(t)
		case int:
			return t
		case string:
			if i, err := strconv.Atoi(t); err == nil {
				return i
			}
		}
	}
	return defaultVal
}

// Repos bundles all repositories needed by API handlers.
type Repos struct {
	User         *repository.UserRepository
	Product      *repository.ProductRepository
	Invoice      *repository.InvoiceRepository
	Payment      *repository.PaymentRepository
	Panel        *repository.PanelRepository
	ServicePanel *repository.ServicePanelRepository
	Setting      *repository.SettingRepository
}
