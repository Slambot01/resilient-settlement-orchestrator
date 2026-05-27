package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ── Metric Types ────────────────────────────────────────────────

// Counter is a monotonically increasing counter.
type Counter struct {
	name   string
	labels map[string]string
	value  atomic.Int64
}

// Histogram tracks value distributions with predefined buckets.
type Histogram struct {
	name    string
	labels  map[string]string
	mu      sync.Mutex
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

// Gauge is a value that can go up or down.
type Gauge struct {
	name   string
	labels map[string]string
	value  atomic.Int64
}

// ── Registry ────────────────────────────────────────────────────

var (
	registry     = make(map[string]interface{})
	registryLock sync.RWMutex
)

// ── Counter Operations ──────────────────────────────────────────

func NewCounter(name string, labels map[string]string) *Counter {
	c := &Counter{name: name, labels: labels}
	key := metricKey(name, labels)
	registryLock.Lock()
	registry[key] = c
	registryLock.Unlock()
	return c
}

func (c *Counter) Inc() {
	c.value.Add(1)
}

func (c *Counter) Add(v int64) {
	c.value.Add(v)
}

func (c *Counter) Value() int64 {
	return c.value.Load()
}

// ── Gauge Operations ────────────────────────────────────────────

func NewGauge(name string, labels map[string]string) *Gauge {
	g := &Gauge{name: name, labels: labels}
	key := metricKey(name, labels)
	registryLock.Lock()
	registry[key] = g
	registryLock.Unlock()
	return g
}

func (g *Gauge) Set(v int64) {
	g.value.Store(v)
}

func (g *Gauge) Inc() {
	g.value.Add(1)
}

func (g *Gauge) Dec() {
	g.value.Add(-1)
}

func (g *Gauge) Value() int64 {
	return g.value.Load()
}

// ── Histogram Operations ────────────────────────────────────────

func NewHistogram(name string, labels map[string]string, buckets []float64) *Histogram {
	h := &Histogram{
		name:    name,
		labels:  labels,
		buckets: buckets,
		counts:  make([]int64, len(buckets)+1), // +1 for +Inf
	}
	key := metricKey(name, labels)
	registryLock.Lock()
	registry[key] = h
	registryLock.Unlock()
	return h
}

func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += v
	h.count++

	for i, b := range h.buckets {
		if v <= b {
			h.counts[i]++
		}
	}
	h.counts[len(h.buckets)]++ // +Inf always increments
}

// DefaultHTTPBuckets are standard latency buckets in seconds.
var DefaultHTTPBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// ── Well-Known Application Metrics ──────────────────────────────

var (
	HTTPRequestsTotal    = NewCounter("http_requests_total", nil)
	HTTPRequestDuration  = NewHistogram("http_request_duration_seconds", nil, DefaultHTTPBuckets)
	PaymentsCreated      = NewCounter("payments_created_total", nil)
	PaymentsFailed       = NewCounter("payments_failed_total", nil)
	PaymentsCaptured     = NewCounter("payments_captured_total", nil)
	WebhooksReceived     = NewCounter("webhooks_received_total", nil)
	WebhooksFailed       = NewCounter("webhooks_failed_total", nil)
	CircuitBreakerTrips  = NewCounter("circuit_breaker_trips_total", nil)
	RetryAttempts        = NewCounter("retry_attempts_total", nil)
	DLQEnqueued          = NewCounter("dlq_enqueued_total", nil)
	ActiveConnections    = NewGauge("active_connections", nil)
)

// ── Prometheus-Compatible Text Output ───────────────────────────

func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		registryLock.RLock()
		defer registryLock.RUnlock()

		for _, metric := range registry {
			switch m := metric.(type) {
			case *Counter:
				writeCounter(w, m)
			case *Gauge:
				writeGauge(w, m)
			case *Histogram:
				writeHistogram(w, m)
			}
		}

		// Runtime metrics
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		fmt.Fprintf(w, "# TYPE go_goroutines gauge\n")
		fmt.Fprintf(w, "go_goroutines %d\n\n", runtime.NumGoroutine())

		fmt.Fprintf(w, "# TYPE go_memstats_alloc_bytes gauge\n")
		fmt.Fprintf(w, "go_memstats_alloc_bytes %d\n\n", memStats.Alloc)

		fmt.Fprintf(w, "# TYPE go_memstats_sys_bytes gauge\n")
		fmt.Fprintf(w, "go_memstats_sys_bytes %d\n\n", memStats.Sys)

		fmt.Fprintf(w, "# TYPE go_memstats_heap_inuse_bytes gauge\n")
		fmt.Fprintf(w, "go_memstats_heap_inuse_bytes %d\n\n", memStats.HeapInuse)

		fmt.Fprintf(w, "# TYPE go_gc_duration_seconds gauge\n")
		fmt.Fprintf(w, "go_gc_duration_seconds %f\n\n", float64(memStats.PauseTotalNs)/1e9)

		fmt.Fprintf(w, "# TYPE process_uptime_seconds gauge\n")
		fmt.Fprintf(w, "process_uptime_seconds %f\n\n", time.Since(startTime).Seconds())
	}
}

var startTime = time.Now()

// ── Helper Functions ────────────────────────────────────────────

func metricKey(name string, labels map[string]string) string {
	key := name
	for k, v := range labels {
		key += fmt.Sprintf("_%s_%s", k, v)
	}
	return key
}

func labelStr(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	s := "{"
	first := true
	for k, v := range labels {
		if !first {
			s += ","
		}
		s += fmt.Sprintf(`%s="%s"`, k, v)
		first = false
	}
	return s + "}"
}

func writeCounter(w http.ResponseWriter, c *Counter) {
	fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
	fmt.Fprintf(w, "%s%s %d\n\n", c.name, labelStr(c.labels), c.value.Load())
}

func writeGauge(w http.ResponseWriter, g *Gauge) {
	fmt.Fprintf(w, "# TYPE %s gauge\n", g.name)
	fmt.Fprintf(w, "%s%s %d\n\n", g.name, labelStr(g.labels), g.value.Load())
}

func writeHistogram(w http.ResponseWriter, h *Histogram) {
	h.mu.Lock()
	defer h.mu.Unlock()

	fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)
	ls := labelStr(h.labels)

	cumulative := int64(0)
	for i, b := range h.buckets {
		cumulative += h.counts[i]
		if len(h.labels) == 0 {
			fmt.Fprintf(w, "%s_bucket{le=\"%g\"} %d\n", h.name, b, cumulative)
		} else {
			fmt.Fprintf(w, "%s_bucket%s{le=\"%g\"} %d\n", h.name, ls, b, cumulative)
		}
	}
	cumulative += h.counts[len(h.buckets)]
	if len(h.labels) == 0 {
		fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.name, cumulative)
	} else {
		fmt.Fprintf(w, "%s_bucket%s{le=\"+Inf\"} %d\n", h.name, ls, cumulative)
	}
	fmt.Fprintf(w, "%s_sum%s %f\n", h.name, ls, h.sum)
	fmt.Fprintf(w, "%s_count%s %d\n\n", h.name, ls, h.count)
}
