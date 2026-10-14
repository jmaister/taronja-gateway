package middleware

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// MiddlewareMetricsSnapshot is a middleware's in-memory request metrics.
type MiddlewareMetricsSnapshot struct {
	Name         string `json:"name"`
	RequestCount int64  `json:"requestCount"`
	ErrorCount   int64  `json:"errorCount"` // requests where the eventual response status was >= 500
	// AverageDurationMs includes everything after this middleware in the chain.
	AverageDurationMs float64    `json:"averageDurationMs"`
	LastInvokedAt     *time.Time `json:"lastInvokedAt,omitempty"`
}

// Counter backs one MiddlewareMetricsSnapshot; safe for concurrent requests.
type Counter struct {
	requestCount    int64
	errorCount      int64
	totalDurationNs int64

	mu            sync.Mutex
	lastInvokedAt time.Time
}

func (c *Counter) record(status int, elapsed time.Duration) {
	atomic.AddInt64(&c.requestCount, 1)
	atomic.AddInt64(&c.totalDurationNs, int64(elapsed))
	if status >= http.StatusInternalServerError {
		atomic.AddInt64(&c.errorCount, 1)
	}
	c.mu.Lock()
	c.lastInvokedAt = time.Now()
	c.mu.Unlock()
}

// Snapshot reads the counters.
func (c *Counter) Snapshot(name string) MiddlewareMetricsSnapshot {
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

// metricsResponseRecorder captures the status code written (200 by default).
type metricsResponseRecorder struct {
	http.ResponseWriter
	status int
}

func (m *metricsResponseRecorder) WriteHeader(code int) {
	m.status = code
	m.ResponseWriter.WriteHeader(code)
}

// Instrument wraps mw so every request through it updates c.
func (c *Counter) Instrument(mw Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &metricsResponseRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			wrapped.ServeHTTP(rec, r)
			c.record(rec.status, time.Since(start))
		})
	}
}
