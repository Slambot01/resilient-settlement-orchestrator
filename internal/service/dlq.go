package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	dlqKey    = "dlq:webhooks"
	dlqMaxLen = 10000
)

// DLQEntry represents a failed webhook that has been parked for manual review.
type DLQEntry struct {
	ID        string    `json:"id"`
	PSP       string    `json:"psp"`
	EventType string    `json:"event_type"`
	Payload   string    `json:"payload"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

type DLQService struct {
	rdb     *redis.Client
	webhook *WebhookService
}

func NewDLQService(rdb *redis.Client, webhook *WebhookService) *DLQService {
	return &DLQService{
		rdb:     rdb,
		webhook: webhook,
	}
}

// Enqueue adds a failed webhook to the dead letter queue.
func (s *DLQService) Enqueue(ctx context.Context, entry DLQEntry) error {
	if s.rdb == nil {
		slog.Warn("dlq: redis unavailable, entry dropped",
			slog.String("psp", entry.PSP),
			slog.String("event_type", entry.EventType),
		)
		return nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("dlq marshal: %w", err)
	}

	pipe := s.rdb.Pipeline()
	pipe.LPush(ctx, dlqKey, data)
	pipe.LTrim(ctx, dlqKey, 0, dlqMaxLen-1) // cap queue size
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("dlq enqueue: %w", err)
	}

	slog.Info("webhook moved to dead letter queue",
		slog.String("psp", entry.PSP),
		slog.String("event_type", entry.EventType),
		slog.String("error", entry.Error),
	)
	return nil
}

// List returns the most recent entries from the DLQ.
func (s *DLQService) List(ctx context.Context, offset, limit int64) ([]DLQEntry, int64, error) {
	if s.rdb == nil {
		return nil, 0, fmt.Errorf("dlq: redis unavailable")
	}

	total, err := s.rdb.LLen(ctx, dlqKey).Result()
	if err != nil {
		return nil, 0, err
	}

	raw, err := s.rdb.LRange(ctx, dlqKey, offset, offset+limit-1).Result()
	if err != nil {
		return nil, 0, err
	}

	entries := make([]DLQEntry, 0, len(raw))
	for _, item := range raw {
		var e DLQEntry
		if err := json.Unmarshal([]byte(item), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	return entries, total, nil
}

// Retry re-processes a DLQ entry by calling the webhook service again.
func (s *DLQService) Retry(ctx context.Context, index int64) error {
	if s.rdb == nil {
		return fmt.Errorf("dlq: redis unavailable")
	}

	raw, err := s.rdb.LIndex(ctx, dlqKey, index).Result()
	if err != nil {
		return fmt.Errorf("dlq fetch at index %d: %w", index, err)
	}

	var entry DLQEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return fmt.Errorf("dlq unmarshal: %w", err)
	}

	// Re-process through the webhook service
	err = s.webhook.IngestWebhook(ctx, entry.PSP, entry.EventType, []byte(entry.Payload), "")
	if err != nil {
		return fmt.Errorf("dlq retry failed: %w", err)
	}

	// Remove from queue on success (mark as tombstone, then clean up)
	s.rdb.LSet(ctx, dlqKey, index, "__DELETED__")
	s.rdb.LRem(ctx, dlqKey, 1, "__DELETED__")

	slog.Info("dlq entry retried successfully",
		slog.String("psp", entry.PSP),
		slog.String("event_type", entry.EventType),
	)
	return nil
}

// Purge clears the entire dead letter queue.
func (s *DLQService) Purge(ctx context.Context) error {
	if s.rdb == nil {
		return fmt.Errorf("dlq: redis unavailable")
	}
	return s.rdb.Del(ctx, dlqKey).Err()
}
