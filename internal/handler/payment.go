package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type PaymentHandler struct {
	svc *service.PaymentService
}

func NewPaymentHandler(svc *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

// CreatePayment handles POST /v1/payments
func (h *PaymentHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	// Idempotency-Key is required for payment creation to prevent duplicate charges
	if r.Header.Get("Idempotency-Key") == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required for payment creation")
		return
	}

	var req models.CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed json payload")
		return
	}

	// Basic validation (in a real app, use the validator library here)
	if req.Amount <= 0 {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "amount must be greater than zero")
		return
	}
	if req.Currency == "" || req.OrderID == "" || req.MerchantID == "" {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "currency, order_id, and merchant_id are required")
		return
	}

	res, err := h.svc.CreatePayment(r.Context(), req)
	if err != nil {
		slog.Error("failed to create payment", slog.Any("error", err))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create payment")
		return
	}

	response.JSON(w, http.StatusCreated, res)
}

// GetPayment handles GET /v1/payments/{id}
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "payment ID is required")
		return
	}

	res, err := h.svc.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
			return
		}
		slog.Error("failed to fetch payment", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch payment")
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// CapturePayment handles POST /v1/payments/{id}/capture
func (h *PaymentHandler) CapturePayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "payment ID is required")
		return
	}

	res, err := h.svc.CapturePayment(r.Context(), id)
	if err != nil {
		slog.Error("failed to capture payment", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to capture payment")
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// RefundPayment handles POST /v1/payments/{id}/refund
func (h *PaymentHandler) RefundPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "payment ID is required")
		return
	}

	var req models.RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed json payload")
		return
	}

	if req.Amount <= 0 {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "amount must be greater than zero")
		return
	}

	res, err := h.svc.RefundPayment(r.Context(), id, req.Amount)
	if err != nil {
		slog.Error("failed to refund payment", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to refund payment")
		return
	}

	response.JSON(w, http.StatusOK, res)
}

// CancelPayment handles POST /v1/payments/{id}/cancel
func (h *PaymentHandler) CancelPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "payment ID is required")
		return
	}

	res, err := h.svc.CancelPayment(r.Context(), id)
	if err != nil {
		slog.Error("failed to cancel payment", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel payment")
		return
	}

	response.JSON(w, http.StatusOK, res)
}
