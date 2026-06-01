package circuitbreaker

import (
	"testing"
	"time"
)

func TestNew_DefaultState(t *testing.T) {
	cb := New("test-psp", DefaultConfig())

	if cb.GetState() != StateClosed {
		t.Errorf("expected initial state closed, got %s", cb.GetState())
	}

	stats := cb.Stats()
	if stats.TotalRequests != 0 {
		t.Errorf("expected 0 total requests, got %d", stats.TotalRequests)
	}
}

func TestAllow_ClosedState(t *testing.T) {
	cb := New("test-psp", DefaultConfig())

	if !cb.Allow() {
		t.Error("expected Allow() to return true in closed state")
	}
}

func TestRecordSuccess_ResetsConsecutiveFails(t *testing.T) {
	cb := New("test-psp", Config{FailureThreshold: 5, SuccessThreshold: 2, OpenTimeout: time.Second})

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	stats := cb.Stats()
	if stats.ConsecutiveFails != 0 {
		t.Errorf("expected 0 consecutive fails after success, got %d", stats.ConsecutiveFails)
	}
	if stats.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", stats.TotalRequests)
	}
}

func TestCircuitOpens_AfterThreshold(t *testing.T) {
	cfg := Config{FailureThreshold: 3, SuccessThreshold: 1, OpenTimeout: time.Second}
	cb := New("test-psp", cfg)

	// Record failures up to threshold
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.GetState() != StateClosed {
		t.Errorf("expected closed after 2 failures, got %s", cb.GetState())
	}

	cb.RecordFailure() // 3rd failure = threshold

	if cb.GetState() != StateOpen {
		t.Errorf("expected open after 3 failures, got %s", cb.GetState())
	}
}

func TestCircuitOpen_BlocksRequests(t *testing.T) {
	cfg := Config{FailureThreshold: 2, SuccessThreshold: 1, OpenTimeout: 10 * time.Second}
	cb := New("test-psp", cfg)

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.Allow() {
		t.Error("expected Allow() to return false in open state")
	}
}

func TestCircuitHalfOpen_AfterTimeout(t *testing.T) {
	cfg := Config{FailureThreshold: 2, SuccessThreshold: 1, OpenTimeout: 50 * time.Millisecond}
	cb := New("test-psp", cfg)

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.GetState() != StateOpen {
		t.Fatalf("expected open state, got %s", cb.GetState())
	}

	// Wait for open timeout to expire
	time.Sleep(60 * time.Millisecond)

	// Next Allow() should transition to half-open
	if !cb.Allow() {
		t.Error("expected Allow() to return true after open timeout")
	}

	if cb.GetState() != StateHalfOpen {
		t.Errorf("expected half_open state, got %s", cb.GetState())
	}
}

func TestCircuitCloses_FromHalfOpen(t *testing.T) {
	cfg := Config{FailureThreshold: 2, SuccessThreshold: 2, OpenTimeout: 50 * time.Millisecond}
	cb := New("test-psp", cfg)

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout, transition to half-open
	time.Sleep(60 * time.Millisecond)
	cb.Allow()

	// Record enough successes to close
	cb.RecordSuccess()
	if cb.GetState() != StateHalfOpen {
		t.Errorf("expected still half_open after 1 success, got %s", cb.GetState())
	}

	cb.RecordSuccess()
	if cb.GetState() != StateClosed {
		t.Errorf("expected closed after 2 successes in half-open, got %s", cb.GetState())
	}
}

func TestCircuitReopens_FromHalfOpen(t *testing.T) {
	cfg := Config{FailureThreshold: 2, SuccessThreshold: 2, OpenTimeout: 50 * time.Millisecond}
	cb := New("test-psp", cfg)

	// Trip, wait, half-open
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	cb.Allow()

	// Failure in half-open should re-open immediately
	cb.RecordFailure()

	if cb.GetState() != StateOpen {
		t.Errorf("expected open after failure in half-open, got %s", cb.GetState())
	}
}

func TestStats_SuccessRate(t *testing.T) {
	cfg := Config{FailureThreshold: 10, SuccessThreshold: 1, OpenTimeout: time.Second}
	cb := New("test-psp", cfg)

	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats.TotalRequests != 4 {
		t.Errorf("expected 4 total requests, got %d", stats.TotalRequests)
	}
	if stats.TotalFailures != 1 {
		t.Errorf("expected 1 total failure, got %d", stats.TotalFailures)
	}

	expectedRate := 0.75
	if stats.SuccessRate != expectedRate {
		t.Errorf("expected success rate %.2f, got %.2f", expectedRate, stats.SuccessRate)
	}
}

func TestString_Format(t *testing.T) {
	cb := New("razorpay", DefaultConfig())
	cb.RecordSuccess()

	s := cb.String()
	if s == "" {
		t.Error("expected non-empty string representation")
	}
}
