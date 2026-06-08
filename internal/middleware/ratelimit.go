package middleware

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides a simple in-memory token bucket rate limiter per IP.
//
// WARNING (V-011): This limiter is per-process. In multi-pod Kubernetes deployments,
// each pod has independent rate state, so an attacker hitting N pods gets N× the limit.
// For production multi-instance deployments, replace with a Redis-based distributed
// limiter (e.g., go-redis/redis_rate using GCRA algorithm).
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*bucket
	rate     int           // tokens per interval
	interval time.Duration // refill interval
	burst    int           // max tokens (bucket capacity)
}

type bucket struct {
	tokens   int
	lastFill time.Time
}

// NewRateLimiter creates a rate limiter.
// rate: requests allowed per interval. burst: max burst capacity.
func NewRateLimiter(rate int, interval time.Duration, burst int) *RateLimiter {
	slog.Warn("using in-memory rate limiter — not suitable for multi-pod deployments (V-011)",
		slog.Int("rate", rate), slog.Int("burst", burst))

	rl := &RateLimiter{
		visitors: make(map[string]*bucket),
		rate:     rate,
		interval: interval,
		burst:    burst,
	}

	// Background cleanup of stale entries every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			rl.cleanup()
		}
	}()

	return rl
}

// Handler returns middleware that rate-limits by client IP.
func (rl *RateLimiter) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr

			if !rl.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"success":false,"error":{"code":"RATE_LIMITED","message":"too many requests, please retry later"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, exists := rl.visitors[key]
	if !exists {
		rl.visitors[key] = &bucket{tokens: rl.burst - 1, lastFill: time.Now()}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := time.Since(b.lastFill)
	refill := int(elapsed / rl.interval) * rl.rate
	if refill > 0 {
		b.tokens += refill
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.lastFill = time.Now()
	}

	if b.tokens <= 0 {
		return false
	}

	b.tokens--
	return true
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for key, b := range rl.visitors {
		if b.lastFill.Before(cutoff) {
			delete(rl.visitors, key)
		}
	}
}
