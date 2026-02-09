package router

import (
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"hosibot/internal/handler"
	"hosibot/internal/handler/api"
	"hosibot/internal/middleware"
	"hosibot/internal/pkg/telegram"
	"hosibot/internal/repository"

	"gorm.io/gorm"
)

// Setup configures all routes for the Echo server.
func Setup(
	e *echo.Echo,
	db *gorm.DB,
	botAPI *telegram.BotAPI,
	logger *zap.Logger,
	apiKey string,
	hashFilePath string,
	updateDeduper middleware.UpdateDeduper,
	webhookHandler http.Handler,
) {
	// Global middleware
	e.Use(echomw.Recover())
	e.Use(middleware.CORS())

	// Repositories
	repos := &api.Repos{
		User:         repository.NewUserRepository(db),
		Product:      repository.NewProductRepository(db),
		Invoice:      repository.NewInvoiceRepository(db),
		Payment:      repository.NewPaymentRepository(db),
		Panel:        repository.NewPanelRepository(db),
		ServicePanel: repository.NewServicePanelRepository(db),
		Setting:      repository.NewSettingRepository(db),
	}

	// Handlers
	userHandler := api.NewUserHandler(repos, botAPI, logger)
	productHandler := api.NewProductHandler(repos, logger)
	invoiceHandler := api.NewInvoiceHandler(repos, logger)
	paymentHandler := api.NewPaymentHandler(repos, logger)
	panelHandler := api.NewPanelHandler(repos, logger)
	miniAppHandler := api.NewMiniAppHandler(repos, logger)
	discountHandler := api.NewDiscountHandler(repos, logger)
	categoryHandler := api.NewCategoryHandler(repos, logger)
	settingsHandler := api.NewSettingsHandler(repos, logger)
	serviceHandler := api.NewServiceHandler(repos, logger)
	legacyHandler := api.NewLegacyHandler(repos, logger, apiKey)

	// Payment callback handler
	callbackRepos := &handler.CallbackRepos{
		User:    repos.User,
		Product: repos.Product,
		Invoice: repos.Invoice,
		Payment: repos.Payment,
		Panel:   repos.Panel,
		Setting: repos.Setting,
	}
	paymentCallbackHandler := handler.NewPaymentCallbackHandler(callbackRepos, botAPI, logger)

	// API group with auth + logging middleware
	apiGroup := e.Group("/api")
	apiGroup.Use(middleware.APIAuth(apiKey, hashFilePath))
	apiGroup.Use(middleware.APILogger(repos.Setting))

	// API routes — all POST, matching PHP behavior
	apiGroup.POST("/users", userHandler.Handle)
	apiGroup.GET("/users", userHandler.Handle)
	apiGroup.POST("/products", productHandler.Handle)
	apiGroup.GET("/products", productHandler.Handle)
	apiGroup.POST("/invoices", invoiceHandler.Handle)
	apiGroup.GET("/invoices", invoiceHandler.Handle)
	apiGroup.POST("/payments", paymentHandler.Handle)
	apiGroup.GET("/payments", paymentHandler.Handle)
	apiGroup.POST("/panels", panelHandler.Handle)
	apiGroup.GET("/panels", panelHandler.Handle)
	apiGroup.POST("/discounts", discountHandler.Handle)
	apiGroup.GET("/discounts", discountHandler.Handle)
	apiGroup.POST("/categories", categoryHandler.Handle)
	apiGroup.GET("/categories", categoryHandler.Handle)
	apiGroup.POST("/settings", settingsHandler.Handle)
	apiGroup.GET("/settings", settingsHandler.Handle)
	apiGroup.POST("/services", serviceHandler.Handle)
	apiGroup.GET("/services", serviceHandler.Handle)
	apiGroup.GET("/log", legacyHandler.LogStats)
	apiGroup.GET("/statbot", legacyHandler.StatBot)

	// Legacy endpoints without APIAuth (matching original PHP behavior).
	e.GET("/api/keyboard", legacyHandler.Keyboard)
	e.POST("/api/verify", legacyHandler.Verify)
	e.GET("/api/verify", legacyHandler.Verify)
	e.GET("/api/miniapp", miniAppHandler.Handle)
	e.POST("/api/miniapp", miniAppHandler.Handle)

	// Telegram webhook (protected by IP check + deduplication)
	if webhookHandler != nil {
		botWebhookGroup := e.Group("/bot")
		botWebhookGroup.Use(middleware.TelegramIPCheck())
		botWebhookGroup.Use(middleware.TelegramUpdateDedup(updateDeduper))
		botWebhookGroup.POST("/webhook", echo.WrapHandler(webhookHandler))

		// Legacy webhook route for backward compatibility.
		webhookGroup := e.Group("/webhook")
		webhookGroup.Use(middleware.TelegramIPCheck())
		webhookGroup.Use(middleware.TelegramUpdateDedup(updateDeduper))
		webhookGroup.POST("/:token", echo.WrapHandler(webhookHandler))
	} else {
		logger.Info("Telegram webhook routes disabled (bot update mode is polling)")
	}

	// Payment callback routes
	paymentGroup := e.Group("/payment")
	paymentGroup.GET("/zarinpal/callback", paymentCallbackHandler.ZarinPalCallback)
	paymentGroup.POST("/nowpayments/callback", paymentCallbackHandler.NOWPaymentsCallback)
	paymentGroup.POST("/nowpayment/callback", paymentCallbackHandler.NOWPaymentsCallback) // legacy alias
	paymentGroup.POST("/tronado/callback", paymentCallbackHandler.TronadoCallback)
	paymentGroup.POST("/iranpay/callback", paymentCallbackHandler.IranPayCallback)
	paymentGroup.POST("/aqayepardakht/callback", paymentCallbackHandler.AqayePardakhtCallback)

	// Subscription endpoint
	e.GET("/sub/:uuid", paymentCallbackHandler.SubscriptionHandler)

	// Mini-app (serve static SPA)
	e.Static("/app", "app")

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})
}
