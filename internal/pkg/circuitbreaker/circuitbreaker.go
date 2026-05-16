package circuitbreaker

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"    // healthy, requests flow through
	StateOpen     State = "open"      // tripped, requests are blocked
	StateHalfOpen State = "half_open" // recovery probe, limited requests allowed
)

type Config struct {
	FailureThreshold int           // consecutive failures before opening
	SuccessThreshold int           // consecutive successes in half-open before closing
	OpenTimeout      time.Duration // how long to stay open before probing
}

func DefaultConfig() Config {
	return Config{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	}
}

type CircuitBreaker struct {
	mu               sync.RWMutex
	name             string
	state            State
	cfg              Config
	consecutiveFails int
	consecutiveOK    int
	totalRequests    int64
	totalFailures    int64
	lastFailureAt    time.Time
	openedAt         time.Time
}

func New(name string, cfg Config) *CircuitBreaker {
	return &CircuitBreaker{
		name:  name,
		state: StateClosed,
		cfg:   cfg,
	}
}

// Allow checks if a request should be permitted through the breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		if time.Since(cb.openedAt) >= cb.cfg.OpenTimeout {
			cb.state = StateHalfOpen
			cb.consecutiveOK = 0
			slog.Info("circuit breaker transitioning to half-open",
				slog.String("psp", cb.name),
			)
			return true
		}
		return false

	case StateHalfOpen:
		return true
	}

	return false
}

// RecordSuccess records a successful PSP call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.consecutiveFails = 0

	if cb.state == StateHalfOpen {
		cb.consecutiveOK++
		if cb.consecutiveOK >= cb.cfg.SuccessThreshold {
			cb.state = StateClosed
			slog.Info("circuit breaker closed, PSP recovered",
				slog.String("psp", cb.name),
			)
		}
	}
}

// RecordFailure records a failed PSP call.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.totalFailures++
	cb.consecutiveFails++
	cb.consecutiveOK = 0
	cb.lastFailureAt = time.Now()

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.openedAt = time.Now()
		slog.Warn("circuit breaker re-opened from half-open",
			slog.String("psp", cb.name),
		)
		return
	}

	if cb.state == StateClosed && cb.consecutiveFails >= cb.cfg.FailureThreshold {
		cb.state = StateOpen
		cb.openedAt = time.Now()
		slog.Warn("circuit breaker opened",
			slog.String("psp", cb.name),
			slog.Int("consecutive_failures", cb.consecutiveFails),
		)
	}
}

// State returns current breaker state.
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns a snapshot for monitoring.
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var successRate float64
	if cb.totalRequests > 0 {
		successRate = float64(cb.totalRequests-cb.totalFailures) / float64(cb.totalRequests)
	}

	return Stats{
		Name:             cb.name,
		State:            cb.state,
		TotalRequests:    cb.totalRequests,
		TotalFailures:    cb.totalFailures,
		ConsecutiveFails: cb.consecutiveFails,
		SuccessRate:      successRate,
		LastFailureAt:    cb.lastFailureAt,
	}
}

func (cb *CircuitBreaker) String() string {
	s := cb.Stats()
	return fmt.Sprintf("CircuitBreaker[%s] state=%s requests=%d failures=%d rate=%.2f",
		s.Name, s.State, s.TotalRequests, s.TotalFailures, s.SuccessRate)
}

type Stats struct {
	Name             string    `json:"name"`
	State            State     `json:"state"`
	TotalRequests    int64     `json:"total_requests"`
	TotalFailures    int64     `json:"total_failures"`
	ConsecutiveFails int       `json:"consecutive_failures"`
	SuccessRate      float64   `json:"success_rate"`
	LastFailureAt    time.Time `json:"last_failure_at,omitempty"`
}
