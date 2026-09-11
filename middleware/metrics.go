package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// MiddlewareMetricsSnapshot is a point-in-time read of a middleware's
// request metrics, as tracked by MiddlewareRegistryV2 since the chain was
// built (counters are in-memory only and reset on process restart).
type MiddlewareMetricsSnapshot struct {
	Name         string `json:"name"`
	RequestCount int64  `json:"requestCount"`
	ErrorCount   int64  `json:"errorCount"` // requests where the eventual response status was >= 500
	// AverageDurationMs is the mean wall-clock time, in milliseconds, spent
	// from entering this middleware to the response being written. Because
	// middlewares are nested (each wraps everything after it in the chain),
	// this includes every downstream middleware and the final handler too —
	// it is not this middleware's isolated cost.
	AverageDurationMs float64    `json:"averageDurationMs"`
	LastInvokedAt     *time.Time `json:"lastInvokedAt,omitempty"`
}

// middlewareMetricsCounter holds the mutable counters backing one
// MiddlewareMetricsSnapshot. Count/duration/error fields use atomics since
// they're updated from whatever goroutine is serving the request;
// lastInvokedAt is guarded by mu since time.Time isn't atomic-friendly.
type middlewareMetricsCounter struct {
	requestCount    int64
	errorCount      int64
	totalDurationNs int64

	mu            sync.Mutex
	lastInvokedAt time.Time
}

func (c *middlewareMetricsCounter) record(status int, elapsed time.Duration) {
	atomic.AddInt64(&c.requestCount, 1)
	atomic.AddInt64(&c.totalDurationNs, int64(elapsed))
	if status >= http.StatusInternalServerError {
		atomic.AddInt64(&c.errorCount, 1)
	}
	c.mu.Lock()
	c.lastInvokedAt = time.Now()
	c.mu.Unlock()
}

func (c *middlewareMetricsCounter) snapshot(name string) MiddlewareMetricsSnapshot {
	count := atomic.LoadInt64(&c.requestCount)
	totalNs := atomic.LoadInt64(&c.totalDurationNs)
	errCount := atomic.LoadInt64(&c.errorCount)

	var avgMs float64
	if count > 0 {
		avgMs = float64(totalNs) / float64(count) / float64(time.Millisecond)
	}

	c.mu.Lock()
	last := c.lastInvokedAt
	c.mu.Unlock()

	snap := MiddlewareMetricsSnapshot{
		Name:              name,
		RequestCount:      count,
		ErrorCount:        errCount,
		AverageDurationMs: avgMs,
	}
	if !last.IsZero() {
		snap.LastInvokedAt = &last
	}
	return snap
}

// metricsResponseRecorder captures the status code the wrapped handler
// eventually writes, defaulting to 200 to match net/http's own behavior when
// WriteHeader is never called explicitly.
type metricsResponseRecorder struct {
	http.ResponseWriter
	status int
}

func (m *metricsResponseRecorder) WriteHeader(code int) {
	m.status = code
	m.ResponseWriter.WriteHeader(code)
}

// instrumentMiddleware wraps mw so every request through it updates counter:
// request count, error count (status >= 500), and elapsed wall-clock time.
func instrumentMiddleware(mw Middleware, counter *middlewareMetricsCounter) Middleware {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &metricsResponseRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			wrapped.ServeHTTP(rec, r)
			counter.record(rec.status, time.Since(start))
		})
	}
}

// GetMetrics returns a snapshot of the named middleware's request metrics.
// Returns an error if name is not a registered factory. A registered
// middleware that has never been built into a chain (or has been built but
// not yet seen a request) returns a zero-valued snapshot, not an error.
func (r *MiddlewareRegistryV2) GetMetrics(name string) (MiddlewareMetricsSnapshot, error) {
	if _, exists := r.factories[name]; !exists {
		return MiddlewareMetricsSnapshot{}, fmt.Errorf("unknown middleware: %s", name)
	}
	counter, ok := r.metrics[name]
	if !ok {
		return MiddlewareMetricsSnapshot{Name: name}, nil
	}
	return counter.snapshot(name), nil
}

// GetAllMetrics returns a snapshot of every middleware that has been built
// into a chain at least once (see BuildChain), keyed by name.
func (r *MiddlewareRegistryV2) GetAllMetrics() map[string]MiddlewareMetricsSnapshot {
	result := make(map[string]MiddlewareMetricsSnapshot, len(r.metrics))
	for name, counter := range r.metrics {
		result[name] = counter.snapshot(name)
	}
	return result
}
