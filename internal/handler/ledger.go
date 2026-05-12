package handler

import (
	"net/http"

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
		if err.Error() == "account not found: "+code {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "account not found")
			return
		}
		response.ErrorWithDetails(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch balance", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, res)
}
