package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type authContextKey string

const merchantIDKey authContextKey = "merchant_id"

// MerchantKey maps an API key to a merchant identity.
type MerchantKey struct {
	MerchantID string
	APIKey     string
}

// ParseMerchantKeys parses API keys in "merchantID:apiKey" format.
// Keys without a colon are treated as admin keys with merchant_id="__admin__".
func ParseMerchantKeys(rawKeys []string) []MerchantKey {
	var keys []MerchantKey
	for _, raw := range rawKeys {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if idx := strings.Index(raw, ":"); idx > 0 {
			keys = append(keys, MerchantKey{
				MerchantID: raw[:idx],
				APIKey:     raw[idx+1:],
			})
		} else {
			// Legacy format: no merchant scoping — treat as admin
			keys = append(keys, MerchantKey{
				MerchantID: "__admin__",
				APIKey:     raw,
			})
		}
	}
	return keys
}

// APIKeyAuth validates requests against a set of merchant-scoped API keys.
// On success, injects the merchant_id into the request context.
// Keys are checked via constant-time comparison to prevent timing attacks.
//
// Key format: "merchantID:apiKey" — e.g., "merchant_abc:sk_live_xxxxx"
// Legacy format: "apiKey" — treated as admin with merchant_id="__admin__"
//
// Usage:
//
//	r.Route("/v1", func(r chi.Router) {
//	    r.Use(middleware.APIKeyAuth(merchantKeys))
//	    r.Post("/payments", handler.CreatePayment)
//	})
func APIKeyAuth(keys []MerchantKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(keys) == 0 {
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

			merchantID := validateKeyAndGetMerchant(token, keys)
			if merchantID == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"success":false,"error":{"code":"FORBIDDEN","message":"invalid API key"}}`))
				return
			}

			// Inject merchant_id into context for downstream ownership checks
			ctx := context.WithValue(r.Context(), merchantIDKey, merchantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MerchantIDFromContext extracts the authenticated merchant ID from the request context.
// Returns empty string if not authenticated (should never happen behind APIKeyAuth middleware).
func MerchantIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(merchantIDKey).(string); ok {
		return id
	}
	return ""
}

// IsAdmin returns true if the authenticated identity is the admin (legacy key format).
func IsAdmin(ctx context.Context) bool {
	return MerchantIDFromContext(ctx) == "__admin__"
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

// validateKeyAndGetMerchant checks the token against all keys using constant-time comparison.
// Returns the merchant_id on match, empty string on failure.
// Iterates all keys to prevent timing leaks about which key failed.
func validateKeyAndGetMerchant(token string, keys []MerchantKey) string {
	var matched string
	for _, k := range keys {
		if subtle.ConstantTimeCompare([]byte(token), []byte(k.APIKey)) == 1 {
			matched = k.MerchantID
		}
	}
	return matched
}

