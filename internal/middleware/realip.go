package middleware

import (
	"net"
	"net/http"
	"strings"
)

// TrustedProxies holds CIDR ranges of trusted reverse proxies.
// Only X-Forwarded-For values from these IPs are trusted.
//
// V-027: The default chi.RealIP middleware blindly trusts X-Forwarded-For
// from any client, allowing attackers to spoof their IP and bypass rate
// limiting. This middleware only trusts known infrastructure proxies.
var defaultTrustedProxies = []string{
	"10.0.0.0/8",     // GKE internal pods
	"172.16.0.0/12",  // Docker/K8s bridge networks
	"192.168.0.0/16", // Local development
	"127.0.0.0/8",    // Loopback
	"::1/128",        // IPv6 loopback
	"35.191.0.0/16",  // GCP health check probes
	"130.211.0.0/22", // GCP load balancer ranges
}

// RealIPFromTrustedProxy replaces r.RemoteAddr with the client IP from
// X-Forwarded-For, but ONLY if the direct connection is from a trusted proxy.
// If the connection is from an untrusted IP, X-Forwarded-For is ignored.
func RealIPFromTrustedProxy(next http.Handler) http.Handler {
	// Parse trusted CIDRs once at init
	trustedNets := make([]*net.IPNet, 0, len(defaultTrustedProxies))
	for _, cidr := range defaultTrustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			trustedNets = append(trustedNets, network)
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the direct peer IP (strip port)
		peerIP := extractIP(r.RemoteAddr)
		if peerIP == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Only trust X-Forwarded-For if the direct peer is a known proxy
		if !isTrustedProxy(peerIP, trustedNets) {
			// Untrusted peer — use their direct IP, ignore X-Forwarded-For
			next.ServeHTTP(w, r)
			return
		}

		// Trusted proxy — extract real client IP from X-Forwarded-For (leftmost)
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			// X-Forwarded-For: client, proxy1, proxy2
			// The leftmost entry is the original client
			parts := strings.SplitN(xff, ",", 2)
			clientIP := strings.TrimSpace(parts[0])
			if clientIP != "" && net.ParseIP(clientIP) != nil {
				r.RemoteAddr = clientIP
			}
		} else if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if net.ParseIP(xri) != nil {
				r.RemoteAddr = xri
			}
		}

		next.ServeHTTP(w, r)
	})
}

func extractIP(remoteAddr string) string {
	// Try host:port format
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	// Bare IP (no port)
	if net.ParseIP(remoteAddr) != nil {
		return remoteAddr
	}
	return ""
}

func isTrustedProxy(ip string, trustedNets []*net.IPNet) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, network := range trustedNets {
		if network.Contains(parsedIP) {
			return true
		}
	}
	return false
}
