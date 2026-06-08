package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type LedgerHandler struct {
	svc *service.LedgerService
}

func NewLedgerHandler(svc *service.LedgerService) *LedgerHandler {
	return &LedgerHandler{svc: svc}
}

// GetAccountBalance handles GET /v1/ledger/accounts/{code}/balance
func (h *LedgerHandler) GetAccountBalance(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "account code is required")
		return
	}

	res, err := h.svc.GetAccountBalance(r.Context(), code)
	if err != nil {
		if errors.Is(err, service.ErrAccountNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "account not found")
			return
		}
		slog.Error("failed to fetch balance", slog.Any("error", err), slog.String("account_code", code))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch balance")
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// GetRecentEntries handles GET /v1/ledger/entries?limit=50&offset=0
func (h *LedgerHandler) GetRecentEntries(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	entries, err := h.svc.GetRecentEntries(r.Context(), limit, offset)
	if err != nil {
		slog.Error("failed to fetch ledger entries", slog.Any("error", err))
		response.Error(w, http.StatusInternalServerError, "LEDGER_ERROR", "failed to fetch entries")
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"limit":   limit,
		"offset":  offset,
	})
}
