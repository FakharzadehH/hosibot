package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/bootstrap"
	"hosibot/internal/bot"
	"hosibot/internal/config"
	cronpkg "hosibot/internal/cron"
	"hosibot/internal/middleware"
	"hosibot/internal/pkg/telegram"
	"hosibot/internal/repository"
	"hosibot/internal/router"
)

func main() {
	// --- Logger ---
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	if hasArg("--bootstrap-db") {
		if err := runDBBootstrap(logger); err != nil {
			logger.Fatal("Database bootstrap failed", zap.Error(err))
		}
		logger.Info("Database bootstrap completed")
		return
	}

	// --- Config ---
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// --- Database ---
	db, err := config.NewDatabase(&cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	if err := bootstrap.MigrateAndSeed(db); err != nil {
		logger.Fatal("Failed to bootstrap database schema", zap.Error(err))
	}

	// --- Telegram Bot API (direct HTTP client) ---
	botAPI := telegram.NewBotAPI(cfg.Bot.Token)

	// --- Echo ---
	e := echo.New()
	e.HideBanner = true

	// --- Webhook Deduper (Redis with in-memory fallback) ---
	updateDeduper, dedupeErr := middleware.NewUpdateDeduper(
		cfg.Redis.Addr,
		cfg.Redis.Pass,
		cfg.Redis.DB,
		10*time.Minute,
	)
	if dedupeErr != nil {
		logger.Warn("Redis unavailable for webhook dedup, using in-memory fallback", zap.Error(dedupeErr))
	}

	// --- Bot ---
	botRepos := &bot.BotRepos{
		User:    repository.NewUserRepository(db),
		Product: repository.NewProductRepository(db),
		Invoice: repository.NewInvoiceRepository(db),
		Payment: repository.NewPaymentRepository(db),
		Panel:   repository.NewPanelRepository(db),
		Setting: repository.NewSettingRepository(db),
		CronJob: repository.NewCronJobRepository(db),
	}
	teleBot, err := bot.New(cfg, botRepos, botAPI, logger)
	if err != nil {
		logger.Fatal("Failed to create bot", zap.Error(err))
	}

	// --- Routes ---
	router.Setup(e, db, botAPI, logger, cfg.API.Key, cfg.API.HashFile, updateDeduper, teleBot.WebhookHandler())

	// --- Cron Scheduler ---
	cronRepos := &cronpkg.CronRepos{
		User:    repository.NewUserRepository(db),
		Product: repository.NewProductRepository(db),
		Invoice: repository.NewInvoiceRepository(db),
		Payment: repository.NewPaymentRepository(db),
		Panel:   repository.NewPanelRepository(db),
		Setting: repository.NewSettingRepository(db),
		CronJob: repository.NewCronJobRepository(db),
	}
	scheduler := cronpkg.New(cfg, cronRepos, botAPI, teleBot, logger)
	scheduler.Start()

	// --- Start Server ---
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	go func() {
		logger.Info("Starting Hosibot server", zap.String("addr", addr))
		if err := e.Start(addr); err != nil {
			logger.Info("Server stopped", zap.Error(err))
		}
	}()

	// Start bot (webhook polling — telebot registers webhook with Telegram
	// and waits for updates via the Echo-mounted handler)
	go teleBot.Start()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down...")

	// Stop bot
	teleBot.Stop()

	// Stop cron
	ctx := scheduler.Stop()
	<-ctx.Done()

	// Stop HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

func hasArg(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

func runDBBootstrap(logger *zap.Logger) error {
	dbCfg, err := config.LoadDatabaseOnly()
	if err != nil {
		return err
	}
	db, err := config.NewDatabase(dbCfg)
	if err != nil {
		return err
	}
	if err := bootstrap.MigrateAndSeed(db); err != nil {
		return err
	}
	logger.Info("Schema migration and default seed completed")
	return nil
}
