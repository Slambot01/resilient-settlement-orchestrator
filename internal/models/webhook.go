package models

import (
	"time"
)

type WebhookStatus string

const (
	WebhookStatusReceived   WebhookStatus = "received"
	WebhookStatusProcessing WebhookStatus = "processing"
	WebhookStatusProcessed  WebhookStatus = "processed"
	WebhookStatusFailed     WebhookStatus = "failed"
)

type WebhookEvent struct {
	ID                string        `json:"id"`
	PSP               string        `json:"psp"`
	EventType         string        `json:"event_type"`
	RawPayload        string        `json:"raw_payload"`
	Signature         string        `json:"signature,omitempty"`
	PSPPaymentID      string        `json:"psp_payment_id,omitempty"`
	InternalPaymentID string        `json:"internal_payment_id,omitempty"`
	Status            WebhookStatus `json:"status"`
	IdempotencyKey    string        `json:"idempotency_key"`
	ProcessedAt       *time.Time    `json:"processed_at,omitempty"`
	RetryCount        int           `json:"retry_count"`
	MaxRetries        int           `json:"max_retries"`
	ErrorMessage      string        `json:"error_message,omitempty"`
	NextRetryAt       *time.Time    `json:"next_retry_at,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
}
