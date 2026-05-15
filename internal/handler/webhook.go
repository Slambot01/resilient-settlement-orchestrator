package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type WebhookHandler struct {
	svc *service.WebhookService
}

func NewWebhookHandler(svc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

// HandleWebhook receives POST /v1/webhooks/{psp}
// PSPs expect 200 OK on success and will retry on non-200.
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	psp := chi.URLParam(r, "psp")
	if psp == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "psp path parameter required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "failed to read request body")
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "empty request body")
		return
	}

	signature := extractSignatureHeader(psp, r)
	eventType := extractEventType(psp, r, body)
	if eventType == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "could not determine event type")
		return
	}

	if err := h.svc.IngestWebhook(r.Context(), psp, eventType, body, signature); err != nil {
		response.ErrorWithDetails(w, http.StatusInternalServerError, "WEBHOOK_PROCESSING_FAILED", "failed to process webhook", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func extractSignatureHeader(psp string, r *http.Request) string {
	switch psp {
	case "stripe":
		return r.Header.Get("Stripe-Signature")
	case "razorpay":
		return r.Header.Get("X-Razorpay-Signature")
	default:
		return r.Header.Get("X-Webhook-Signature")
	}
}

func extractEventType(psp string, r *http.Request, body []byte) string {
	switch psp {
	case "stripe":
		return extractJSONField(body, "type")
	case "razorpay":
		return extractJSONField(body, "event")
	default:
		if t := r.Header.Get("X-Event-Type"); t != "" {
			return t
		}
		return extractJSONField(body, "event_type")
	}
}

func extractJSONField(data []byte, key string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
