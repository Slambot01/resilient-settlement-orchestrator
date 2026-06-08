package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type DLQHandler struct {
	svc *service.DLQService
}

func NewDLQHandler(svc *service.DLQService) *DLQHandler {
	return &DLQHandler{svc: svc}
}

// ListEntries handles GET /v1/dlq?offset=0&limit=20
func (h *DLQHandler) ListEntries(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	entries, total, err := h.svc.List(r.Context(), offset, limit)
	if err != nil {
		slog.Error("failed to list dead letter queue", slog.Any("error", err))
		response.Error(w, http.StatusInternalServerError, "DLQ_ERROR", "failed to list dead letter queue")
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

// RetryEntry handles POST /v1/dlq/retry
// Body: {"index": 0}
func (h *DLQHandler) RetryEntry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Index int64 `json:"index"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "expected json body with index field")
		return
	}

	if err := h.svc.Retry(r.Context(), req.Index); err != nil {
		slog.Error("dlq retry failed", slog.Any("error", err), slog.Int64("index", req.Index))
		response.Error(w, http.StatusInternalServerError, "DLQ_RETRY_FAILED", "retry failed")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "retried"})
}

// PurgeQueue handles DELETE /v1/dlq
func (h *DLQHandler) PurgeQueue(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Purge(r.Context()); err != nil {
		slog.Error("dlq purge failed", slog.Any("error", err))
		response.Error(w, http.StatusInternalServerError, "DLQ_PURGE_FAILED", "purge failed")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "purged"})
}
