package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type ctxKey string

const loggerCtxKey ctxKey = "slog_logger"

// Logger returns middleware that logs every HTTP request with structured fields
// including request ID, response size, and log-level based on status code.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := chimw.GetReqID(r.Context())

			// Attach request-scoped logger to context
			// V-024: sanitize user-controlled path to prevent log injection
			reqLogger := logger.With(
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", sanitizeLogValue(r.URL.Path)),
			)
			ctx := context.WithValue(r.Context(), loggerCtxKey, reqLogger)

			// Wrap response writer to capture status code and bytes written
			wrapped := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			duration := time.Since(start)

			// Choose log level based on status code
			// V-024: sanitize all user-controlled values before logging
			attrs := []slog.Attr{
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", sanitizeLogValue(r.URL.Path)),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.Int64("response_bytes", wrapped.bytesWritten),
				slog.String("remote_addr", sanitizeLogValue(r.RemoteAddr)),
				slog.String("user_agent", sanitizeLogValue(r.UserAgent())),
			}

			// Add query params if present
			if r.URL.RawQuery != "" {
				attrs = append(attrs, slog.String("query", sanitizeLogValue(r.URL.RawQuery)))
			}

			msg := "http request"
			level := slog.LevelInfo
			if wrapped.statusCode >= 500 {
				level = slog.LevelError
				msg = "http request error"
			} else if wrapped.statusCode >= 400 {
				level = slog.LevelWarn
				msg = "http request warning"
			}

			logger.LogAttrs(r.Context(), level, msg, attrs...)
		})
	}
}

// Recoverer recovers from panics and returns a 500 error.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					reqID := chimw.GetReqID(r.Context())
					logger.Error("panic recovered",
						slog.Any("error", err),
						slog.String("request_id", reqID),
						slog.String("method", r.Method),
						slog.String("path", sanitizeLogValue(r.URL.Path)),
					)
					http.Error(w, `{"success":false,"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// LoggerFromCtx retrieves the request-scoped logger from context.
// Falls back to a default logger if none is found.
func LoggerFromCtx(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerCtxKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithPaymentID returns a logger enriched with the payment ID for tracing payment flows.
func WithPaymentID(ctx context.Context, paymentID string) *slog.Logger {
	return LoggerFromCtx(ctx).With(slog.String("payment_id", paymentID))
}

// WithPSP returns a logger enriched with PSP name for tracing PSP interactions.
func WithPSP(ctx context.Context, psp string) *slog.Logger {
	return LoggerFromCtx(ctx).With(slog.String("psp", psp))
}

// statusResponseWriter wraps http.ResponseWriter to capture the status code and bytes.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

// sanitizeLogValue strips control characters (newlines, tabs, ANSI escape codes)
// from user-controlled strings before they are written to logs.
// This prevents log injection attacks where crafted paths/headers could:
//   - Forge fake log entries via injected newlines
//   - Corrupt terminal output via ANSI escape sequences
//   - Hide malicious activity by overwriting previous log lines
func sanitizeLogValue(s string) string {
	return strings.Map(func(r rune) rune {
		// Strip all C0 control characters (0x00-0x1F) and DEL (0x7F)
		if r < 0x20 || r == 0x7F {
			return -1 // drop the character
		}
		return r
	}, s)
}
