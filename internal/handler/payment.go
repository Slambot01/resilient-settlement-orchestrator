package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/middleware"
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

	// Input validation
	if req.Amount <= 0 {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "amount must be greater than zero")
		return
	}
	// V-016: prevent integer overflow in ledger aggregates (max ~₹9.99B / $9.99B in minor units)
	const maxAmount int64 = 999_999_999_999
	if req.Amount > maxAmount {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "amount exceeds maximum allowed value")
		return
	}
	if req.Currency == "" || req.OrderID == "" || req.MerchantID == "" {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "currency, order_id, and merchant_id are required")
		return
	}
	// V-015: validate currency against supported ISO 4217 codes
	if !isValidCurrency(req.Currency) {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "unsupported currency; must be a valid ISO 4217 code (e.g., INR, USD, EUR)")
		return
	}

	// Multi-tenant enforcement: non-admin merchants can only create payments for themselves
	authMerchant := middleware.MerchantIDFromContext(r.Context())
	if !middleware.IsAdmin(r.Context()) && req.MerchantID != authMerchant {
		response.Error(w, http.StatusForbidden, "FORBIDDEN", "cannot create payments for a different merchant")
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

	// Multi-tenant enforcement: non-admin merchants can only view their own payments
	if !middleware.IsAdmin(r.Context()) && res.MerchantID != middleware.MerchantIDFromContext(r.Context()) {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
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

	// Ownership check: fetch first, verify merchant, then capture
	existing, err := h.svc.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
			return
		}
		slog.Error("failed to fetch payment for capture", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to capture payment")
		return
	}
	if !middleware.IsAdmin(r.Context()) && existing.MerchantID != middleware.MerchantIDFromContext(r.Context()) {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
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
	const maxRefundAmount int64 = 999_999_999_999
	if req.Amount > maxRefundAmount {
		response.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "refund amount exceeds maximum allowed value")
		return
	}

	// Ownership check: fetch first, verify merchant, then refund
	existing, err := h.svc.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
			return
		}
		slog.Error("failed to fetch payment for refund", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to refund payment")
		return
	}
	if !middleware.IsAdmin(r.Context()) && existing.MerchantID != middleware.MerchantIDFromContext(r.Context()) {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
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

	// Ownership check: fetch first, verify merchant, then cancel
	existing, err := h.svc.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPaymentNotFound) {
			response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
			return
		}
		slog.Error("failed to fetch payment for cancel", slog.Any("error", err), slog.String("payment_id", id))
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel payment")
		return
	}
	if !middleware.IsAdmin(r.Context()) && existing.MerchantID != middleware.MerchantIDFromContext(r.Context()) {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "payment not found")
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

// supportedCurrencies is the allowlist of ISO 4217 currency codes this system supports.
// Add currencies here as PSP integrations expand.
var supportedCurrencies = map[string]bool{
	"INR": true, // Indian Rupee
	"USD": true, // US Dollar
	"EUR": true, // Euro
	"GBP": true, // British Pound
	"SGD": true, // Singapore Dollar
	"AUD": true, // Australian Dollar
	"CAD": true, // Canadian Dollar
	"JPY": true, // Japanese Yen
	"AED": true, // UAE Dirham
	"MYR": true, // Malaysian Ringgit
}

func isValidCurrency(code string) bool {
	return supportedCurrencies[code]
}
