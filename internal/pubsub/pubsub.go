package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Config holds Pub/Sub connection settings.
type Config struct {
	ProjectID      string
	WebhookTopicID string // Topic for incoming webhook events
	EventTopicID   string // Topic for payment state change events
	SubscriptionID string // Subscription for webhook processing
	DLQTopicID     string // Dead-letter topic for failed messages
	DLQSubID       string // Dead-letter subscription
	MaxRetries     int    // Max delivery attempts before dead-lettering
	Enabled        bool   // Feature flag — false = skip Pub/Sub entirely
}

// Client wraps the Google Cloud Pub/Sub client with payment-specific publishers.
type Client struct {
	client           *pubsub.Client
	webhookPublisher *pubsub.Publisher
	eventPublisher   *pubsub.Publisher
	config           Config
}

// WebhookMessage is the envelope published to the webhook topic.
type WebhookMessage struct {
	PSP       string `json:"psp"`
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// PaymentEvent is published when a payment transitions state.
type PaymentEvent struct {
	PaymentID  string    `json:"payment_id"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Trigger    string    `json:"trigger"`
	Timestamp  time.Time `json:"timestamp"`
}

// topicPath returns the fully-qualified topic resource name.
func topicPath(projectID, topicID string) string {
	return fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
}

// subPath returns the fully-qualified subscription resource name.
func subPath(projectID, subID string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subID)
}

// NewClient creates a Pub/Sub client and ensures topics/subscriptions exist.
// When PUBSUB_EMULATOR_HOST is set, it connects to the local emulator.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if host := os.Getenv("PUBSUB_EMULATOR_HOST"); host != "" {
		slog.Info("connecting to Pub/Sub emulator", slog.String("host", host))
	}

	client, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}

	// Ensure topics exist (creates if not present — idempotent)
	for _, topicID := range []string{cfg.WebhookTopicID, cfg.EventTopicID, cfg.DLQTopicID} {
		if err := ensureTopic(ctx, client, cfg.ProjectID, topicID); err != nil {
			client.Close()
			return nil, fmt.Errorf("ensuring topic %s: %w", topicID, err)
		}
	}

	// Ensure the webhook processing subscription (with dead-letter policy)
	dlqPath := topicPath(cfg.ProjectID, cfg.DLQTopicID)
	if err := ensureSubscription(ctx, client, cfg.ProjectID, cfg.SubscriptionID, cfg.WebhookTopicID, dlqPath, cfg.MaxRetries); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring webhook subscription: %w", err)
	}

	// Ensure the dead-letter subscription (for DLQ listing/retry)
	if err := ensureSubscription(ctx, client, cfg.ProjectID, cfg.DLQSubID, cfg.DLQTopicID, "", 0); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring DLQ subscription: %w", err)
	}

	// Create publishers
	webhookPub := client.Publisher(cfg.WebhookTopicID)
	eventPub := client.Publisher(cfg.EventTopicID)

	slog.Info("pub/sub client initialized",
		slog.String("project", cfg.ProjectID),
		slog.String("webhook_topic", cfg.WebhookTopicID),
		slog.String("event_topic", cfg.EventTopicID),
	)

	return &Client{
		client:           client,
		webhookPublisher: webhookPub,
		eventPublisher:   eventPub,
		config:           cfg,
	}, nil
}

// PublishWebhook publishes an incoming webhook for async processing.
func (c *Client) PublishWebhook(ctx context.Context, psp, eventType string, payload []byte, signature string) error {
	msg := &WebhookMessage{
		PSP:       psp,
		EventType: eventType,
		Payload:   string(payload),
		Signature: signature,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling webhook message: %w", err)
	}

	result := c.webhookPublisher.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"psp":        psp,
			"event_type": eventType,
		},
	})

	id, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("publishing webhook to pubsub: %w", err)
	}

	slog.Debug("webhook published to pubsub",
		slog.String("message_id", id),
		slog.String("psp", psp),
		slog.String("event_type", eventType),
	)
	return nil
}

// PublishPaymentEvent publishes a payment state change event (fire-and-forget).
func (c *Client) PublishPaymentEvent(ctx context.Context, paymentID, fromStatus, toStatus, trigger string) {
	event := &PaymentEvent{
		PaymentID:  paymentID,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		Trigger:    trigger,
		Timestamp:  time.Now().UTC(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal payment event", slog.Any("error", err))
		return
	}

	result := c.eventPublisher.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"payment_id": paymentID,
			"from":       fromStatus,
			"to":         toStatus,
		},
	})

	// Fire-and-forget — don't block payment flow
	go func() {
		if _, err := result.Get(context.Background()); err != nil {
			slog.Error("failed to publish payment event",
				slog.String("payment_id", paymentID),
				slog.Any("error", err),
			)
		}
	}()
}

// WebhookPublisher returns the webhook publisher for re-publishing retries.
func (c *Client) WebhookPublisher() *pubsub.Publisher {
	return c.webhookPublisher
}

// GetConfig returns the client configuration.
func (c *Client) GetConfig() Config {
	return c.config
}

// Close flushes pending messages and closes the client.
func (c *Client) Close() {
	c.webhookPublisher.Stop()
	c.eventPublisher.Stop()
	c.client.Close()
}

// ensureTopic creates a topic if it doesn't already exist (idempotent).
func ensureTopic(ctx context.Context, client *pubsub.Client, projectID, topicID string) error {
	_, err := client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{
		Name: topicPath(projectID, topicID),
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil // Already exists — idempotent
		}
		return fmt.Errorf("creating topic %s: %w", topicID, err)
	}
	slog.Info("created pubsub topic", slog.String("topic", topicID))
	return nil
}

// ensureSubscription creates a subscription with optional dead-lettering.
func ensureSubscription(ctx context.Context, client *pubsub.Client, projectID, subID, topicID, dlqTopicPath string, maxRetries int) error {
	sub := &pubsubpb.Subscription{
		Name:               subPath(projectID, subID),
		Topic:              topicPath(projectID, topicID),
		AckDeadlineSeconds: 60,
		RetryPolicy: &pubsubpb.RetryPolicy{
			MinimumBackoff: durationpb.New(1 * time.Second),
			MaximumBackoff: durationpb.New(60 * time.Second),
		},
	}

	if dlqTopicPath != "" && maxRetries > 0 {
		sub.DeadLetterPolicy = &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     dlqTopicPath,
			MaxDeliveryAttempts: int32(maxRetries),
		}
	}

	_, err := client.SubscriptionAdminClient.CreateSubscription(ctx, sub)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil // Already exists — idempotent
		}
		return fmt.Errorf("creating subscription %s: %w", subID, err)
	}
	slog.Info("created pubsub subscription", slog.String("subscription", subID))
	return nil
}
