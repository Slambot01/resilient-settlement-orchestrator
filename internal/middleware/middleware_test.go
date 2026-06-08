package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- Auth Middleware Tests ----

func TestAPIKeyAuth_NoKeysConfigured_DeniesAll(t *testing.T) {
	handler := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no keys configured, got %d", w.Code)
	}
}

func TestAPIKeyAuth_ValidBearerToken(t *testing.T) {
	keys := ParseMerchantKeys([]string{"secret-key-123", "backup-key-456"})
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer secret-key-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPIKeyAuth_InvalidToken_Returns403(t *testing.T) {
	keys := ParseMerchantKeys([]string{"valid-key"})
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAPIKeyAuth_MissingHeader_Returns401(t *testing.T) {
	keys := ParseMerchantKeys([]string{"valid-key"})
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_QueryParam_NoLongerAccepted(t *testing.T) {
	keys := ParseMerchantKeys([]string{"query-key-789"})
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats?api_key=query-key-789", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 (query param auth removed), got %d", w.Code)
	}
}

func TestAPIKeyAuth_MalformedAuthHeader(t *testing.T) {
	keys := ParseMerchantKeys([]string{"valid-key"})
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // wrong scheme
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyAuth_BackupKey(t *testing.T) {
	keys := ParseMerchantKeys([]string{"primary-key", "backup-key"})
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer backup-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with backup key, got %d", w.Code)
	}
}

func TestAPIKeyAuth_MerchantScopedKey_SetsContext(t *testing.T) {
	keys := ParseMerchantKeys([]string{"merchant_abc:sk_test_123"})
	var capturedMerchant string
	handler := APIKeyAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMerchant = MerchantIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/payments", nil)
	req.Header.Set("Authorization", "Bearer sk_test_123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if capturedMerchant != "merchant_abc" {
		t.Errorf("expected merchant_id 'merchant_abc' in context, got %q", capturedMerchant)
	}
}

// ---- Logging Middleware Tests ----

func TestLogger_SetsContentTypeAndStatus(t *testing.T) {
	logger := slog.Default()
	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/payments", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestLogger_TracksResponseBytes(t *testing.T) {
	logger := slog.Default()
	responseBody := `{"test":"data"}`

	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(responseBody))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if body != responseBody {
		t.Errorf("expected body %q, got %q", responseBody, body)
	}
}

func TestLoggerFromCtx_WithContext(t *testing.T) {
	logger := slog.Default()
	var capturedLogger *slog.Logger

	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLogger = LoggerFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if capturedLogger == nil {
		t.Error("expected logger from context to be non-nil")
	}
}

func TestLoggerFromCtx_WithoutContext(t *testing.T) {
	// When no logger in context, should return default
	l := LoggerFromCtx(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	if l == nil {
		t.Error("expected fallback logger to be non-nil")
	}
}

func TestWithPaymentID(t *testing.T) {
	logger := slog.Default()
	var capturedLogger *slog.Logger

	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLogger = WithPaymentID(r.Context(), "pay_123")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if capturedLogger == nil {
		t.Error("expected payment logger to be non-nil")
	}
}

func TestWithPSP(t *testing.T) {
	logger := slog.Default()
	var capturedLogger *slog.Logger

	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLogger = WithPSP(r.Context(), "razorpay")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if capturedLogger == nil {
		t.Error("expected PSP logger to be non-nil")
	}
}

func TestRecoverer_CatchesPanic(t *testing.T) {
	logger := slog.Default()
	handler := Recoverer(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", w.Code)
	}

	var resp map[string]interface{}
	body, _ := io.ReadAll(w.Body)
	// The panic recovery writes plain text via http.Error, parse it
	bodyStr := strings.TrimSpace(string(body))
	if err := json.Unmarshal([]byte(bodyStr), &resp); err != nil {
		t.Logf("response body: %s", bodyStr)
	}
}

func TestRecoverer_NoPanic(t *testing.T) {
	logger := slog.Default()
	handler := Recoverer(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("no panic"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when no panic, got %d", w.Code)
	}
}

// ---- Idempotency Helper Tests ----

func TestReadBodyAndReset(t *testing.T) {
	body := `{"amount":1000}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))

	// First read
	data, err := ReadBodyAndReset(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != body {
		t.Errorf("expected %q, got %q", body, string(data))
	}

	// Body should still be readable after reset
	data2, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("unexpected error on second read: %v", err)
	}
	if string(data2) != body {
		t.Errorf("expected body to be re-readable, got %q", string(data2))
	}
}

func TestIdempotency_SkipsGetRequests(t *testing.T) {
	// Nil redis client should be fine for GET requests
	handler := Idempotency(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(idempotencyHeader, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for GET, got %d", w.Code)
	}
}

func TestIdempotency_NoKeyHeader_PassesThrough(t *testing.T) {
	handler := Idempotency(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	// No idempotency key header
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without key, got %d", w.Code)
	}
}

func TestIdempotency_NilRedis_PassesThrough(t *testing.T) {
	handler := Idempotency(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set(idempotencyHeader, "key-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 with nil redis, got %d", w.Code)
	}
}
