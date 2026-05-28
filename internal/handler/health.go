package handler

import (
	"context"
	"net/http"
	"runtime"
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
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"status":     "healthy",
		"service":    "payment-orchestrator",
		"version":    "0.1.0",
		"env":        h.cfg.Server.Env,
		"uptime":     time.Since(h.startTime).String(),
		"go_version": runtime.Version(),
		"goroutines": runtime.NumGoroutine(),
	})
}

// Ready is the readiness probe. Returns 200 only if all dependencies are reachable.
// Kubernetes uses this to decide whether to route traffic to the pod.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := make(map[string]interface{})
	allHealthy := true

	// Deep PostgreSQL check
	dbCheck := h.checkDatabase(ctx)
	checks["database"] = dbCheck
	if dbCheck["status"] != "up" {
		allHealthy = false
	}

	// Deep Redis check
	redisCheck := h.checkRedis(ctx)
	checks["redis"] = redisCheck
	if redisCheck["status"] != "up" {
		allHealthy = false
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

// checkDatabase performs a deep PostgreSQL health check.
func (h *HealthHandler) checkDatabase(ctx context.Context) map[string]interface{} {
	result := map[string]interface{}{
		"status": "down",
	}

	if h.db == nil {
		result["error"] = "not configured"
		return result
	}

	start := time.Now()

	// Ping the database
	if err := h.db.Ping(ctx); err != nil {
		result["error"] = err.Error()
		result["latency_ms"] = time.Since(start).Milliseconds()
		return result
	}

	// Check connection pool stats
	stats := h.db.Stat()
	result["status"] = "up"
	result["latency_ms"] = time.Since(start).Milliseconds()
	result["pool"] = map[string]interface{}{
		"total_conns":   stats.TotalConns(),
		"idle_conns":    stats.IdleConns(),
		"acquired":      stats.AcquiredConns(),
		"max_conns":     stats.MaxConns(),
	}

	return result
}

// checkRedis performs a deep Redis health check.
func (h *HealthHandler) checkRedis(ctx context.Context) map[string]interface{} {
	result := map[string]interface{}{
		"status": "down",
	}

	if h.redis == nil {
		result["error"] = "not configured"
		return result
	}

	start := time.Now()

	// Ping Redis
	if err := h.redis.Ping(ctx).Err(); err != nil {
		result["error"] = err.Error()
		result["latency_ms"] = time.Since(start).Milliseconds()
		return result
	}

	result["status"] = "up"
	result["latency_ms"] = time.Since(start).Milliseconds()

	// Get pool stats
	poolStats := h.redis.PoolStats()
	result["pool"] = map[string]interface{}{
		"total_conns": poolStats.TotalConns,
		"idle_conns":  poolStats.IdleConns,
		"stale_conns": poolStats.StaleConns,
		"hits":        poolStats.Hits,
		"misses":      poolStats.Misses,
	}

	return result
}
