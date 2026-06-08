package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// APIKeyAuth validates requests against a set of valid API keys.
// Keys are checked via constant-time comparison to prevent timing attacks.
//
// Usage:
//
//	r.Route("/admin", func(r chi.Router) {
//	    r.Use(middleware.APIKeyAuth(validKeys))
//	    r.Get("/stats", handler.Stats)
//	})
func APIKeyAuth(validKeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(validKeys) == 0 {
				// No keys configured — deny all requests (misconfiguration protection)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"success":false,"error":{"code":"UNAUTHORIZED","message":"API authentication not configured"}}`))
				return
			}

			token := extractBearerToken(r)
			if token == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"success":false,"error":{"code":"UNAUTHORIZED","message":"missing or invalid Authorization header"}}`))
				return
			}

			if !validateKey(token, validKeys) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"success":false,"error":{"code":"FORBIDDEN","message":"invalid API key"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func validateKey(token string, validKeys []string) bool {
	for _, key := range validKeys {
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
			return true
		}
	}
	return false
}
