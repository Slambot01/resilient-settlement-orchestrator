package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type ReconciliationHandler struct {
	svc *service.ReconciliationService
}

func NewReconciliationHandler(svc *service.ReconciliationService) *ReconciliationHandler {
	return &ReconciliationHandler{svc: svc}
}

// TriggerReconciliation handles POST /v1/reconciliation/{psp}?date=2026-05-15
func (h *ReconciliationHandler) TriggerReconciliation(w http.ResponseWriter, r *http.Request) {
	psp := chi.URLParam(r, "psp")
	if psp == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "psp is required")
		return
	}

	dateStr := r.URL.Query().Get("date")
	date := time.Now().UTC()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "date must be YYYY-MM-DD format")
			return
		}
		date = parsed
	}

	record, err := h.svc.RunBatchReconciliation(r.Context(), psp, date)
	if err != nil {
		slog.Error("batch reconciliation failed", slog.Any("error", err), slog.String("psp", psp))
		response.Error(w, http.StatusInternalServerError, "RECONCILIATION_FAILED", "batch reconciliation failed")
		return
	}

	response.JSON(w, http.StatusOK, record)
}

// GetDiscrepancies handles GET /v1/reconciliation/{id}/discrepancies
func (h *ReconciliationHandler) GetDiscrepancies(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "reconciliation ID is required")
		return
	}

	discrepancies, err := h.svc.GetDiscrepancies(r.Context(), id)
	if err != nil {
		slog.Error("failed to fetch discrepancies", slog.Any("error", err), slog.String("reconciliation_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch discrepancies")
		return
	}

	response.JSON(w, http.StatusOK, discrepancies)
}
