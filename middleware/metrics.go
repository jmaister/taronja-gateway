package middleware

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// MiddlewareMetricsSnapshot is a point-in-time read of a middleware's
// request metrics since the process started (counters are in-memory only).
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

// Counter holds the counters backing one MiddlewareMetricsSnapshot.
// Count/duration/error fields use atomics since they're updated from
// whatever goroutine is serving the request; lastInvokedAt is guarded by mu
// since time.Time isn't atomic-friendly.
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

// Instrument wraps mw so every request through it updates c: request count,
// error count (status >= 500), and elapsed wall-clock time.
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
