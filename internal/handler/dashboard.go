package handler

import (
	"net/http"
	"time"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// GetStats handles GET /v1/admin/dashboard/stats?from=2026-05-01&to=2026-05-23
// Defaults to last 30 days if no range is provided.
func (h *DashboardHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	from, to := parseDateRange(r)

	stats, err := h.svc.GetStats(r.Context(), from, to)
	if err != nil {
		response.ErrorWithDetails(w, http.StatusInternalServerError, "STATS_ERROR", "failed to fetch dashboard stats", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, stats)
}

// GetDailyVolume handles GET /v1/admin/dashboard/volume?from=2026-05-01&to=2026-05-23
func (h *DashboardHandler) GetDailyVolume(w http.ResponseWriter, r *http.Request) {
	from, to := parseDateRange(r)

	volume, err := h.svc.GetDailyVolume(r.Context(), from, to)
	if err != nil {
		response.ErrorWithDetails(w, http.StatusInternalServerError, "VOLUME_ERROR", "failed to fetch daily volume", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": volume,
		"from": from.Format("2006-01-02"),
		"to":   to.Format("2006-01-02"),
	})
}

func parseDateRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	from := to.Add(-30 * 24 * time.Hour)

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if parsed, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = parsed
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if parsed, err := time.Parse("2006-01-02", toStr); err == nil {
			to = parsed.Add(24 * time.Hour) // inclusive end date
		}
	}

	return from, to
}
