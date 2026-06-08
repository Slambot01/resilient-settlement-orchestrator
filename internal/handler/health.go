package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/config"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
)

// HealthHandler handles liveness and readiness endpoints.
type HealthHandler struct {
	cfg       *config.Config
	db        *pgxpool.Pool
	redis     *redis.Client
	startTime time.Time
}

// NewHealthHandler creates a health handler with dependency references.
func NewHealthHandler(cfg *config.Config, db *pgxpool.Pool, rdb *redis.Client) *HealthHandler {
	return &HealthHandler{
		cfg:       cfg,
		db:        db,
		redis:     rdb,
		startTime: time.Now(),
	}
}

// Health is the liveness probe. Returns 200 if the process is alive.
// Kubernetes uses this to decide whether to restart the pod.
// Intentionally minimal — no internal details exposed (V-010 fix).
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"service": "payment-orchestrator",
	})
}

// Ready is the readiness probe. Returns 200 only if all dependencies are reachable.
// Kubernetes uses this to decide whether to route traffic to the pod.
// Only exposes up/down status — no pool stats, error messages, or internal details (V-010 fix).
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := make(map[string]string)
	allHealthy := true

	// PostgreSQL check — up/down only
	if h.db != nil {
		if err := h.db.Ping(ctx); err != nil {
			checks["database"] = "down"
			allHealthy = false
		} else {
			checks["database"] = "up"
		}
	} else {
		checks["database"] = "not_configured"
		allHealthy = false
	}

	// Redis check — up/down only
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "down"
			allHealthy = false
		} else {
			checks["redis"] = "up"
		}
	} else {
		checks["redis"] = "not_configured"
	}

	status := http.StatusOK
	statusText := "ready"
	if !allHealthy {
		status = http.StatusServiceUnavailable
		statusText = "not_ready"
	}

	response.JSON(w, status, map[string]interface{}{
		"status": statusText,
		"checks": checks,
	})
}
