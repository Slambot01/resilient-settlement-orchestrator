package middleware

import (
	"net/http"
)

// SecurityHeaders adds essential HTTP security headers to every response.
// These defend against clickjacking, MIME confusion, XSS, HTTPS downgrade,
// and browser-level data leakage.
//
// Must be placed early in the middleware chain (before any handler writes).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent HTTPS downgrade attacks — browsers will refuse HTTP for 1 year
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Prevent MIME-type sniffing — stops browsers from guessing Content-Type
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking — disallow embedding in any iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// Content Security Policy — restrict resource loading to same origin
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'")

		// Control Referrer leakage
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable browser features not needed by a payment API
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

		// Payment data must never be cached by proxies or browsers
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")

		next.ServeHTTP(w, r)
	})
}
