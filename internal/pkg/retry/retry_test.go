package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	err := Do(context.Background(), DefaultConfig(), "test-op", func(ctx context.Context) error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDo_SuccessOnRetry(t *testing.T) {
	cfg := Config{MaxAttempts: 3, InitialWait: 10 * time.Millisecond, MaxWait: 100 * time.Millisecond, Multiplier: 2.0}
	callCount := 0

	err := Do(context.Background(), cfg, "test-op", func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDo_ExhaustsAllAttempts(t *testing.T) {
	cfg := Config{MaxAttempts: 3, InitialWait: 10 * time.Millisecond, MaxWait: 50 * time.Millisecond, Multiplier: 2.0}
	callCount := 0

	err := Do(context.Background(), cfg, "test-op", func(ctx context.Context) error {
		callCount++
		return errors.New("persistent failure")
	})

	if err == nil {
		t.Error("expected error after exhausting attempts")
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	cfg := Config{MaxAttempts: 5, InitialWait: 10 * time.Millisecond, MaxWait: 50 * time.Millisecond, Multiplier: 2.0}
	callCount := 0

	err := Do(context.Background(), cfg, "test-op", func(ctx context.Context) error {
		callCount++
		return NonRetryable(errors.New("invalid input"))
	})

	if err == nil {
		t.Error("expected error for non-retryable")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retries), got %d", callCount)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	cfg := Config{MaxAttempts: 10, InitialWait: 100 * time.Millisecond, MaxWait: time.Second, Multiplier: 2.0}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callCount := 0
	err := Do(ctx, cfg, "test-op", func(ctx context.Context) error {
		callCount++
		return errors.New("keep failing")
	})

	if err == nil {
		t.Error("expected error from context cancellation")
	}
	// Should have stopped early due to context timeout
	if callCount >= 10 {
		t.Errorf("expected fewer than 10 calls due to timeout, got %d", callCount)
	}
}

func TestNonRetryable_Wrapping(t *testing.T) {
	original := errors.New("bad request")
	wrapped := NonRetryable(original)

	if !IsNonRetryable(wrapped) {
		t.Error("expected IsNonRetryable to return true")
	}

	if wrapped.Error() != "bad request" {
		t.Errorf("expected error message 'bad request', got %s", wrapped.Error())
	}
}

func TestIsNonRetryable_RegularError(t *testing.T) {
	err := errors.New("regular error")

	if IsNonRetryable(err) {
		t.Error("expected IsNonRetryable to return false for regular error")
	}
}

func TestIsNonRetryable_NilError(t *testing.T) {
	if IsNonRetryable(nil) {
		t.Error("expected IsNonRetryable to return false for nil")
	}
}

func TestBackoffDuration_Increases(t *testing.T) {
	cfg := Config{InitialWait: 100 * time.Millisecond, MaxWait: 5 * time.Second, Multiplier: 2.0}

	d0 := backoffDuration(0, cfg)
	d1 := backoffDuration(1, cfg)
	d2 := backoffDuration(2, cfg)

	if d1 <= d0 {
		t.Errorf("expected d1 (%v) > d0 (%v)", d1, d0)
	}
	if d2 <= d1 {
		t.Errorf("expected d2 (%v) > d1 (%v)", d2, d1)
	}
}

func TestBackoffDuration_CapsAtMaxWait(t *testing.T) {
	cfg := Config{InitialWait: 100 * time.Millisecond, MaxWait: 500 * time.Millisecond, Multiplier: 10.0}

	d := backoffDuration(5, cfg) // would be huge without cap

	if d > cfg.MaxWait {
		t.Errorf("expected backoff capped at %v, got %v", cfg.MaxWait, d)
	}
}

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("expected 3 max attempts, got %d", cfg.MaxAttempts)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected multiplier 2.0, got %f", cfg.Multiplier)
	}
}
