package middleware

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	idempotencyHeader = "Idempotency-Key"
	idempotencyTTL    = 24 * time.Hour
	processingMarker  = "__PROCESSING__"
)

// Idempotency enforces at-most-once semantics for mutating API requests.
//
// Flow:
//  1. Client sends Idempotency-Key header
//  2. Redis SET NX atomically claims the key with a "processing" marker
//  3. If key already exists:
//     - If value is "__PROCESSING__" → another request is in-flight, return 409
//     - Otherwise → return the cached response (status + body)
//  4. If SET NX succeeded → process the request, cache the response, return it
func Idempotency(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only enforce on mutating methods
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(idempotencyHeader)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip if Redis is unavailable
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()

			// Namespace by merchant to prevent cross-merchant cache poisoning (V-007 fix)
			merchantID := MerchantIDFromContext(ctx)
			if merchantID == "" {
				merchantID = "_anonymous"
			}
			redisKey := "idempotency:" + merchantID + ":" + key

			// Atomic claim: SET key "processing" NX EX ttl
			claimed, err := rdb.SetNX(ctx, redisKey, processingMarker, idempotencyTTL).Result()
			if err != nil {
				slog.Warn("idempotency: redis error, proceeding without check", slog.Any("error", err))
				next.ServeHTTP(w, r)
				return
			}

			if !claimed {
				// Key exists — either in-flight or completed
				cached, err := rdb.Get(ctx, redisKey).Result()
				if err != nil {
					http.Error(w, `{"success":false,"error":{"code":"IDEMPOTENCY_ERROR","message":"failed to read cached response"}}`, http.StatusInternalServerError)
					return
				}

				if cached == processingMarker {
					// Another request with the same key is still being processed
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					w.Write([]byte(`{"success":false,"error":{"code":"REQUEST_IN_PROGRESS","message":"a request with this idempotency key is already being processed"}}`))
					return
				}

				// Return the cached response with the original status code
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Idempotent-Replayed", "true")
				statusCode, body := parseCachedResponse(cached)
				w.WriteHeader(statusCode)
				w.Write([]byte(body))
				return
			}

			// We claimed the key — process the request and cache the result
			rec := &responseRecorder{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(rec, r)

			// Cache the response body + status code (only for successful responses)
			if rec.statusCode >= 200 && rec.statusCode < 300 {
				cacheResponse(ctx, rdb, redisKey, rec.statusCode, rec.body.String())
			} else {
				// Failed request — release the key so client can retry
				rdb.Del(ctx, redisKey)
			}
		})
	}
}

// cacheResponse stores "statusCode:body" in Redis so replayed responses use the original status.
func cacheResponse(ctx context.Context, rdb *redis.Client, key string, statusCode int, body string) {
	value := strconv.Itoa(statusCode) + ":" + body
	if err := rdb.Set(ctx, key, value, idempotencyTTL).Err(); err != nil {
		slog.Warn("idempotency: failed to cache response", slog.Any("error", err))
	}
}

// parseCachedResponse extracts the status code and body from a cached "statusCode:body" string.
func parseCachedResponse(cached string) (int, string) {
	idx := strings.Index(cached, ":")
	if idx <= 0 {
		return http.StatusOK, cached // fallback for old format
	}
	code, err := strconv.Atoi(cached[:idx])
	if err != nil {
		return http.StatusOK, cached
	}
	return code, cached[idx+1:]
}

// responseRecorder captures the response so we can cache it.
type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// ReadBodyAndReset reads the request body and resets it for downstream handlers.
func ReadBodyAndReset(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	return body, nil
}
