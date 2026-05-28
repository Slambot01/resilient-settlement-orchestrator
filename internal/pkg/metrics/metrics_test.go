package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCounter_IncAndAdd(t *testing.T) {
	c := NewCounter("test_counter_inc", map[string]string{"env": "test"})

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("expected 1, got %d", c.Value())
	}

	c.Add(5)
	if c.Value() != 6 {
		t.Errorf("expected 6, got %d", c.Value())
	}

	c.Inc()
	if c.Value() != 7 {
		t.Errorf("expected 7, got %d", c.Value())
	}
}

func TestGauge_SetIncDec(t *testing.T) {
	g := NewGauge("test_gauge_ops", map[string]string{"env": "test"})

	g.Set(10)
	if g.Value() != 10 {
		t.Errorf("expected 10, got %d", g.Value())
	}

	g.Inc()
	if g.Value() != 11 {
		t.Errorf("expected 11, got %d", g.Value())
	}

	g.Dec()
	g.Dec()
	if g.Value() != 9 {
		t.Errorf("expected 9, got %d", g.Value())
	}
}

func TestHistogram_Observe(t *testing.T) {
	h := NewHistogram("test_histogram_obs", map[string]string{"env": "test"}, []float64{0.1, 0.5, 1.0})

	h.Observe(0.05)
	h.Observe(0.3)
	h.Observe(0.8)
	h.Observe(2.0)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count != 4 {
		t.Errorf("expected count 4, got %d", h.count)
	}

	expectedSum := 0.05 + 0.3 + 0.8 + 2.0
	if h.sum != expectedSum {
		t.Errorf("expected sum %f, got %f", expectedSum, h.sum)
	}

	// Bucket le=0.1: 0.05 fits (1 value)
	if h.counts[0] != 1 {
		t.Errorf("bucket le=0.1: expected 1, got %d", h.counts[0])
	}
	// Bucket le=0.5: 0.05 and 0.3 fit (2 values)
	if h.counts[1] != 2 {
		t.Errorf("bucket le=0.5: expected 2, got %d", h.counts[1])
	}
	// Bucket le=1.0: 0.05, 0.3, 0.8 fit (3 values)
	if h.counts[2] != 3 {
		t.Errorf("bucket le=1.0: expected 3, got %d", h.counts[2])
	}
	// +Inf: all 4 values
	if h.counts[3] != 4 {
		t.Errorf("bucket le=+Inf: expected 4, got %d", h.counts[3])
	}
}

func TestHandler_OutputFormat(t *testing.T) {
	// Create fresh metrics for this test
	NewCounter("test_handler_req", nil)
	NewGauge("test_handler_conns", nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	Handler()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %s", ct)
	}

	body, _ := io.ReadAll(w.Body)
	output := string(body)

	// Check that runtime metrics are present
	if !strings.Contains(output, "go_goroutines") {
		t.Error("expected go_goroutines in output")
	}
	if !strings.Contains(output, "go_memstats_alloc_bytes") {
		t.Error("expected go_memstats_alloc_bytes in output")
	}
	if !strings.Contains(output, "process_uptime_seconds") {
		t.Error("expected process_uptime_seconds in output")
	}

	// Check TYPE annotations
	if !strings.Contains(output, "# TYPE") {
		t.Error("expected TYPE annotations in output")
	}
}

func TestHTTPMetrics_Middleware(t *testing.T) {
	handler := HTTPMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	initialCount := HTTPRequestsTotal.Value()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if HTTPRequestsTotal.Value() != initialCount+1 {
		t.Errorf("expected request counter to increment by 1")
	}

	// Active connections should be back to its value before the request
	// (it was incremented then decremented during request)
}

func TestLabelStr(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"nil labels", nil, ""},
		{"empty labels", map[string]string{}, ""},
		{"single label", map[string]string{"env": "prod"}, `{env="prod"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelStr(tt.labels)
			if got != tt.want {
				t.Errorf("labelStr(%v) = %q, want %q", tt.labels, got, tt.want)
			}
		})
	}
}
