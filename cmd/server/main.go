package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/adapter"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/config"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/database"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/handler"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/middleware"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

func main() {
	// ── Load configuration ──────────────────────────────────────────
	cfg := config.Load()

	// ── Set up structured logger ────────────────────────────────────
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()

	// ── Run database migrations ─────────────────────────────────────
	if err := database.RunMigrations(cfg.Database, "migrations"); err != nil {
		logger.Error("failed to run migrations", slog.Any("error", err))
		os.Exit(1)
	}

	// ── Connect to PostgreSQL ───────────────────────────────────────
	dbPool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbPool.Close()

	// ── Connect to Redis ────────────────────────────────────────────
	redisClient, err := database.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		logger.Warn("failed to connect to redis, continuing without cache", slog.Any("error", err))
		// Redis is non-critical for startup — continue without it
	} else {
		defer redisClient.Close()
	}

	// ── Initialize Services & Adapters ──────────────────────────────
	
	ledgerService := service.NewLedgerService(dbPool)

	paymentRouter := service.NewPaymentRouter()
	
	// Register Mock PSP
	mockPSP := adapter.NewMockPSP(adapter.DefaultMockConfig())
	paymentRouter.RegisterAdapter(models.PSPMock, mockPSP)
	
	// Set default rule to route everything to Mock PSP for now
	paymentRouter.LoadRules([]models.RoutingRule{
		{
			Name:       "default_mock_rule",
			PrimaryPSP: models.PSPMock,
			Priority:   100,
		},
	})

	paymentService := service.NewPaymentService(dbPool, paymentRouter, ledgerService)

	// PSP adapter registry for webhook signature verification
	pspAdapters := map[string]adapter.PSPAdapter{
		"mock": mockPSP,
	}
	webhookService := service.NewWebhookService(dbPool, ledgerService, pspAdapters)
	reconService := service.NewReconciliationService(dbPool, pspAdapters)
	dlqService := service.NewDLQService(redisClient, webhookService)
	dashboardService := service.NewDashboardService(dbPool, paymentRouter)

	// ── Initialize Handlers ─────────────────────────────────────────
	
	healthHandler := handler.NewHealthHandler(cfg)
	paymentHandler := handler.NewPaymentHandler(paymentService)
	ledgerHandler := handler.NewLedgerHandler(ledgerService)
	webhookHandler := handler.NewWebhookHandler(webhookService)
	reconHandler := handler.NewReconciliationHandler(reconService)
	dlqHandler := handler.NewDLQHandler(dlqService)
	dashboardHandler := handler.NewDashboardHandler(dashboardService)

	// ── Create router ───────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recoverer(logger))
	r.Use(chimw.Timeout(30 * time.Second))

	// ── Register routes ─────────────────────────────────────────────
	r.Get("/healthz", healthHandler.Health)
	r.Get("/readyz", healthHandler.Ready)

	// API v1 routes
	r.Route("/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"service":"payment-orchestrator","version":"v1"}`))
		})
		
		r.Route("/payments", func(r chi.Router) {
			r.Use(middleware.Idempotency(redisClient))
			r.Post("/", paymentHandler.CreatePayment)
			r.Get("/{id}", paymentHandler.GetPayment)
		})
		
		r.Route("/ledger", func(r chi.Router) {
			r.Get("/accounts/{code}/balance", ledgerHandler.GetAccountBalance)
		})

		r.Post("/webhooks/{psp}", webhookHandler.HandleWebhook)

		// Admin routes — protected by API key
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.APIKeyAuth(cfg.Auth.APIKeys))

			r.Route("/reconciliation", func(r chi.Router) {
				r.Post("/{psp}", reconHandler.TriggerReconciliation)
				r.Get("/{id}/discrepancies", reconHandler.GetDiscrepancies)
			})

			r.Route("/dlq", func(r chi.Router) {
				r.Get("/", dlqHandler.ListEntries)
				r.Post("/retry", dlqHandler.RetryEntry)
				r.Delete("/", dlqHandler.PurgeQueue)
			})

			r.Route("/dashboard", func(r chi.Router) {
				r.Get("/stats", dashboardHandler.GetStats)
				r.Get("/volume", dashboardHandler.GetDailyVolume)
				r.Get("/psp-health", dashboardHandler.GetPSPHealth)
			})
		})
	})

	// ── Start server with graceful shutdown ──────────────────────────
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("server starting",
			slog.Int("port", cfg.Server.Port),
			slog.String("env", cfg.Server.Env),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed to start", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	sig := <-shutdown
	logger.Info("shutdown signal received", slog.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}
