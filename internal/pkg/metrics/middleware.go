package metrics

import (
	"net/http"
	"time"
)

// HTTPMetrics is a middleware that records request count and latency.
func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ActiveConnections.Inc()
		defer ActiveConnections.Dec()

		HTTPRequestsTotal.Inc()

		next.ServeHTTP(w, r)

		duration := time.Since(start).Seconds()
		HTTPRequestDuration.Observe(duration)
	})
}
