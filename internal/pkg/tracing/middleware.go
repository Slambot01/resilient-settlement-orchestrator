package tracing

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware returns a Chi-compatible middleware that creates a root span
// for every incoming HTTP request using OpenTelemetry.
//
// It also injects the trace ID into the response header (X-Trace-ID) so that
// merchants can reference it when reporting issues — e.g., "payment xyz was slow,
// trace ID was abc-123".
func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// otelhttp.NewMiddleware handles span creation, propagation, and
		// recording http.method, http.status_code, etc.
		otelHandler := otelhttp.NewMiddleware(serviceName,
			otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
				return fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
			}),
		)

		return otelHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace ID and add to response header.
			span := trace.SpanFromContext(r.Context())
			if span.SpanContext().HasTraceID() {
				w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			}
			next.ServeHTTP(w, r)
		}))
	}
}
