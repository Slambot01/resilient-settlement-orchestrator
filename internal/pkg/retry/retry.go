package retry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/tracing"
)

type Config struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

func DefaultConfig() Config {
	return Config{
		MaxAttempts: 3,
		InitialWait: 200 * time.Millisecond,
		MaxWait:     5 * time.Second,
		Multiplier:  2.0,
	}
}

// RetryableFunc is any operation that can be retried.
type RetryableFunc func(ctx context.Context) error

var tracer = tracing.Tracer("retry")

// Do executes fn with exponential backoff. Returns nil on first success.
// Non-retryable errors (wrapped with ErrNonRetryable) abort immediately.
// Each attempt is wrapped in an OpenTelemetry span for distributed tracing.
func Do(ctx context.Context, cfg Config, operationName string, fn RetryableFunc) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Create a span for each retry attempt.
		attemptCtx, span := tracer.Start(ctx, fmt.Sprintf("retry.attempt.%d", attempt+1),
			trace.WithAttributes(
				attribute.Int("retry.attempt_number", attempt+1),
				attribute.Int("retry.max_attempts", cfg.MaxAttempts),
				attribute.String("retry.operation", operationName),
			),
		)

		lastErr = fn(attemptCtx)

		if lastErr == nil {
			span.SetAttributes(attribute.Bool("retry.success", true))
			span.End()
			return nil
		}

		// Record the error on the span.
		span.RecordError(lastErr)
		span.SetAttributes(attribute.Bool("retry.success", false))

		// Check if error is explicitly non-retryable
		if IsNonRetryable(lastErr) {
			span.SetStatus(codes.Error, "non-retryable error")
			span.End()
			slog.Warn("non-retryable error, aborting",
				slog.String("operation", operationName),
				slog.Int("attempt", attempt+1),
				slog.Any("error", lastErr),
			)
			return lastErr
		}

		span.SetStatus(codes.Error, "attempt failed, will retry")
		span.End()

		// Don't wait after the last attempt
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		wait := backoffDuration(attempt, cfg)
		slog.Info("retrying operation",
			slog.String("operation", operationName),
			slog.Int("attempt", attempt+1),
			slog.Int("max_attempts", cfg.MaxAttempts),
			slog.Duration("wait", wait),
			slog.Any("error", lastErr),
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operationName, cfg.MaxAttempts, lastErr)
}

func backoffDuration(attempt int, cfg Config) time.Duration {
	wait := float64(cfg.InitialWait) * math.Pow(cfg.Multiplier, float64(attempt))
	if time.Duration(wait) > cfg.MaxWait {
		wait = float64(cfg.MaxWait)
	}
	// Add ±25% jitter to prevent thundering herd
	jitter := 1.0 + (rand.Float64()-0.5)*0.5 // 0.75 to 1.25
	return time.Duration(wait * jitter)
}

// nonRetryableError wraps errors that should not be retried.
type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

// NonRetryable wraps an error to signal that it should not be retried.
// Use for: invalid input, auth failures, duplicate keys, etc.
func NonRetryable(err error) error {
	return &nonRetryableError{err: err}
}

func IsNonRetryable(err error) bool {
	target := &nonRetryableError{}
	return errors.As(err, &target)
}
