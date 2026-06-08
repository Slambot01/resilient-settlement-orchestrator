package middleware

import (
	"net/http"
	"strings"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int // preflight cache duration in seconds
}

// DefaultCORSConfig returns a restrictive CORS configuration suitable for API servers.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{}, // no origins allowed by default — configure per environment
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type", "X-Idempotency-Key", "X-Request-ID"},
		MaxAge:         3600,
	}
}

// CORS returns middleware that sets Cross-Origin Resource Sharing headers.
// An empty AllowedOrigins list blocks all cross-origin requests.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	origins := strings.Join(cfg.AllowedOrigins, ", ")
	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origins != "" {
				w.Header().Set("Access-Control-Allow-Origin", origins)
			}
			w.Header().Set("Access-Control-Allow-Methods", methods)
			w.Header().Set("Access-Control-Allow-Headers", headers)
			w.Header().Set("Access-Control-Max-Age", http.StatusText(cfg.MaxAge))

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
