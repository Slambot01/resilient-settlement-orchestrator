package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/config"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	cfg       *config.Config
	startTime time.Time
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(cfg *config.Config) *HealthHandler {
	return &HealthHandler{
		cfg:       cfg,
		startTime: time.Now(),
	}
}

// Health returns basic application health status.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"status":     "healthy",
		"service":    "payment-orchestrator",
		"version":    "0.1.0",
		"env":        h.cfg.Server.Env,
		"uptime":     time.Since(h.startTime).String(),
		"go_version": runtime.Version(),
	})
}

// Ready returns readiness status (checks dependencies).
// For now, returns OK. Will check DB/Redis connectivity later.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"status": "ready",
		"checks": map[string]string{
			"database": "not_configured",
			"redis":    "not_configured",
		},
	})
}
