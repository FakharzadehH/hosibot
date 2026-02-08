package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"hosibot/internal/repository"
)

// APIAuth validates the Token header against hash.txt file or APIKEY.
// Matches PHP's validateToken() behavior from api/*.php files.
func APIAuth(apiKey string, hashFilePath string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := c.Request().Header.Get("Token")
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"status": false,
					"msg":    "Token is required",
					"obj":    nil,
				})
			}

			// Check against API key
			if token == apiKey {
				return next(c)
			}

			// Check against hash.txt file (PHP behavior)
			hashData, err := os.ReadFile(hashFilePath)
			if err == nil {
				hash := strings.TrimSpace(string(hashData))
				if token == hash {
					return next(c)
				}
				// Also check SHA256 of token
				h := sha256.Sum256([]byte(token))
				if hex.EncodeToString(h[:]) == hash {
					return next(c)
				}
			}

			return c.JSON(http.StatusUnauthorized, map[string]interface{}{
				"status": false,
				"msg":    "Invalid token",
				"obj":    nil,
			})
		}
	}
}

// APILogger logs API requests to the logs_api table.
// Matches PHP's log behavior in api/*.php files.
func APILogger(settingRepo *repository.SettingRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Execute the handler
			err := next(c)

			// Log the request
			ip := c.RealIP()

			// Get the actions field from context (set by handler)
			actions, _ := c.Get("api_actions").(string)

			// Log to database (async, non-blocking)
			go func() {
				headers := map[string]string{
					"Token":        c.Request().Header.Get("Token"),
					"Content-Type": c.Request().Header.Get("Content-Type"),
				}
				_ = settingRepo.CreateAPILog(headers, nil, ip, actions)
			}()

			return err
		}
	}
}

// TelegramIPCheck ensures requests come from Telegram's IP range.
// Matches PHP's checktelegramip() function.
func TelegramIPCheck() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			// Telegram webhook IPs: 149.154.160.0/20 and 91.108.4.0/22
			if !strings.HasPrefix(ip, "149.154.") &&
				!strings.HasPrefix(ip, "91.108.") &&
				ip != "127.0.0.1" &&
				ip != "::1" {
				return c.String(http.StatusForbidden, "Forbidden")
			}
			return next(c)
		}
	}
}

// CORS configures CORS headers.
func CORS() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("Access-Control-Allow-Origin", "*")
			c.Response().Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type, Token, Authorization")
			if c.Request().Method == "OPTIONS" {
				return c.NoContent(http.StatusOK)
			}
			return next(c)
		}
	}
}
