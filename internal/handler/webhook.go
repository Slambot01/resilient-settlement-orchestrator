package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	appPubSub "github.com/Slambot01/resilient-settlement-orchestrator/internal/pubsub"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/response"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/service"
)

type WebhookHandler struct {
	svc    *service.WebhookService
	pubsub *appPubSub.Client // nil when Pub/Sub is disabled — falls back to sync
}

func NewWebhookHandler(svc *service.WebhookService, pubsubClient *appPubSub.Client) *WebhookHandler {
	return &WebhookHandler{svc: svc, pubsub: pubsubClient}
}

// maxWebhookBodySize caps incoming webhook payloads to prevent OOM from oversized requests.
const maxWebhookBodySize = 1 << 20 // 1 MB

// HandleWebhook receives POST /v1/webhooks/{psp}
// When Pub/Sub is enabled: publishes to the webhook topic and returns 200 immediately.
// When Pub/Sub is disabled: processes synchronously (original behavior).
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	psp := chi.URLParam(r, "psp")
	if psp == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "psp path parameter required")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodySize))
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

	// Pub/Sub path: publish and return immediately (async processing by subscriber)
	if h.pubsub != nil {
		if err := h.pubsub.PublishWebhook(r.Context(), psp, eventType, body, signature); err != nil {
			slog.Error("failed to publish webhook to pubsub",
				slog.Any("error", err),
				slog.String("psp", psp),
				slog.String("event_type", eventType),
			)
			response.Error(w, http.StatusInternalServerError, "WEBHOOK_PUBLISH_FAILED", "failed to queue webhook")
			return
		}

		slog.Info("webhook queued via pubsub",
			slog.String("psp", psp),
			slog.String("event_type", eventType),
		)
		response.JSON(w, http.StatusOK, map[string]string{"status": "queued"})
		return
	}

	// Fallback: synchronous processing (Pub/Sub disabled)
	if err := h.svc.IngestWebhook(r.Context(), psp, eventType, body, signature); err != nil {
		slog.Error("failed to process webhook", slog.Any("error", err), slog.String("psp", psp), slog.String("event_type", eventType))
		response.Error(w, http.StatusInternalServerError, "WEBHOOK_PROCESSING_FAILED", "failed to process webhook")
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
