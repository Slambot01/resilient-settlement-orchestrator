package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cloud.google.com/go/pubsub/v2"
)

// WebhookProcessor defines the interface the subscriber uses to process webhooks.
// This decouples the pubsub package from the service layer.
type WebhookProcessor interface {
	IngestWebhook(ctx context.Context, psp, eventType string, payload []byte, signature string) error
}

// deliveryAttempt safely dereferences the DeliveryAttempt pointer.
func deliveryAttempt(msg *pubsub.Message) int {
	if msg.DeliveryAttempt != nil {
		return *msg.DeliveryAttempt
	}
	return 0
}

// StartWebhookSubscriber starts a blocking subscriber that pulls messages from
// the webhook subscription and processes them via the WebhookProcessor.
// Messages are Ack'd on success; Nack'd on failure (Pub/Sub retries with backoff,
// then routes to the dead-letter topic after MaxDeliveryAttempts).
func (c *Client) StartWebhookSubscriber(ctx context.Context, processor WebhookProcessor) error {
	sub := c.client.Subscriber(c.config.SubscriptionID)

	// Concurrency settings — tune for webhook processing load
	sub.ReceiveSettings.MaxOutstandingMessages = 100
	sub.ReceiveSettings.NumGoroutines = 10

	slog.Info("starting webhook subscriber",
		slog.String("subscription", c.config.SubscriptionID),
	)

	return sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		var webhook WebhookMessage
		if err := json.Unmarshal(msg.Data, &webhook); err != nil {
			slog.Error("failed to unmarshal webhook message — acking to prevent infinite retry",
				slog.String("message_id", msg.ID),
				slog.Any("error", err),
			)
			msg.Ack()
			return
		}

		attempt := deliveryAttempt(msg)

		slog.Debug("processing webhook from pubsub",
			slog.String("message_id", msg.ID),
			slog.String("psp", webhook.PSP),
			slog.String("event_type", webhook.EventType),
			slog.Int("attempt", attempt),
		)

		err := processor.IngestWebhook(ctx, webhook.PSP, webhook.EventType, []byte(webhook.Payload), webhook.Signature)
		if err != nil {
			slog.Error("webhook processing failed, nacking for retry",
				slog.String("message_id", msg.ID),
				slog.String("psp", webhook.PSP),
				slog.String("event_type", webhook.EventType),
				slog.Int("attempt", attempt),
				slog.Any("error", err),
			)
			msg.Nack() // Pub/Sub retries with exponential backoff → DLT after max attempts
			return
		}

		msg.Ack()
		slog.Info("webhook processed successfully via pubsub",
			slog.String("message_id", msg.ID),
			slog.String("psp", webhook.PSP),
			slog.String("event_type", webhook.EventType),
		)
	})
}

// DLQEntry represents a message pulled from the dead-letter subscription.
type DLQEntry struct {
	MessageID     string `json:"message_id"`
	PSP           string `json:"psp"`
	EventType     string `json:"event_type"`
	Payload       string `json:"payload"`
	DeliveryCount int    `json:"delivery_count"`
	PublishTime   string `json:"publish_time"`
	RawData       []byte `json:"-"` // For re-publishing on retry
}

// PullDLQEntries pulls up to maxMessages from the dead-letter subscription.
func (c *Client) PullDLQEntries(ctx context.Context, maxMessages int) ([]DLQEntry, error) {
	sub := c.client.Subscriber(c.config.DLQSubID)
	sub.ReceiveSettings.MaxOutstandingMessages = maxMessages

	var entries []DLQEntry

	pullCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := sub.Receive(pullCtx, func(_ context.Context, msg *pubsub.Message) {
		var webhook WebhookMessage
		entry := DLQEntry{
			MessageID:     msg.ID,
			DeliveryCount: deliveryAttempt(msg),
			PublishTime:   msg.PublishTime.String(),
			RawData:       msg.Data,
		}

		if err := json.Unmarshal(msg.Data, &webhook); err == nil {
			entry.PSP = webhook.PSP
			entry.EventType = webhook.EventType
			entry.Payload = webhook.Payload
		} else {
			entry.Payload = string(msg.Data)
		}

		entries = append(entries, entry)
		msg.Nack() // Keep in queue — just peeking

		if len(entries) >= maxMessages {
			cancel()
		}
	})

	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("pulling DLQ entries: %w", err)
	}

	return entries, nil
}

// RetryDLQMessage re-publishes a message from the DLQ back to the webhook topic.
func (c *Client) RetryDLQMessage(ctx context.Context, messageData []byte) error {
	result := c.webhookPublisher.Publish(ctx, &pubsub.Message{
		Data: messageData,
	})

	id, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("republishing DLQ message: %w", err)
	}

	slog.Info("DLQ message republished for retry", slog.String("new_message_id", id))
	return nil
}
