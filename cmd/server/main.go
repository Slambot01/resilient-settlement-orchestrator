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

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/config"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/handler"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/middleware"
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

	// ── Create router ───────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recoverer(logger))
	r.Use(chimw.Timeout(30 * time.Second))

	// ── Register routes ─────────────────────────────────────────────
	healthHandler := handler.NewHealthHandler(cfg)

	r.Get("/healthz", healthHandler.Health)
	r.Get("/readyz", healthHandler.Ready)

	// API v1 routes (placeholder — will be populated in later commits)
	r.Route("/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"service":"payment-orchestrator","version":"v1"}`))
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

	// Channel to listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
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

	// Block until shutdown signal
	sig := <-shutdown
	logger.Info("shutdown signal received", slog.String("signal", sig.String()))

	// Graceful shutdown with 10-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}
