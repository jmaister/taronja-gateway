package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/session"
)

// RateLimiter implements an in‑memory rate limiter keyed by client IP.
// It's safe for concurrent use and maintains its own cleanup goroutine.
type RateLimiter struct {
	cfg             config.RateLimiterConfig
	entries         sync.Map // map[string]*rateEntry
	cleanupInterval time.Duration
}

// rateEntry stores the state for a single IP address.
type rateEntry struct {
	mu           sync.Mutex
	requests     []time.Time // timestamps of all requests in the last minute
	errors       []time.Time // timestamps of 401/404 responses in the block window
	scan404      []time.Time // timestamps of 404s for watched vulnerability paths
	blockedUntil time.Time   // if in the future, requests should be rejected
}

// RateLimiterMiddleware creates a middleware function configured with the
// supplied settings. If both RequestsPerMinute and MaxErrors are zero the
// returned middleware is a no-op and simply invokes the next handler.
func RateLimiterMiddleware(cfg config.RateLimiterConfig) func(http.Handler) http.Handler {
	rl := NewRateLimiter(cfg)
	return rl.Handler
}

// NewRateLimiter constructs a RateLimiter and starts the cleanup goroutine.
func NewRateLimiter(cfg config.RateLimiterConfig) *RateLimiter {
	// determine cleanup interval: use block minutes or default one minute
	interval := time.Minute
	if cfg.BlockMinutes > 0 {
		interval = time.Duration(cfg.BlockMinutes) * time.Minute
	}
	rl := &RateLimiter{
		cfg:             cfg,
		cleanupInterval: interval,
	}
	go rl.cleanupLoop()
	return rl
}

// Handler is the middleware implementation.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	// if no limits are configured simply pass through
	if !rl.cfg.IsEnabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := session.GetClientIP(r)
		now := time.Now()
		entry := rl.getEntry(ip)

		entry.mu.Lock()
		// check existing block
		if now.Before(entry.blockedUntil) {
			retry := int(entry.blockedUntil.Sub(now).Seconds())
			entry.mu.Unlock()
			header := w.Header()
			header.Set("Retry-After", fmt.Sprintf("%d", retry))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
			return
		}

		// record the request timestamp and prune old records
		entry.requests = append(entry.requests, now)
		entry.trim(now, rl.cfg)

		// enforce request rate limit
		if rl.cfg.RequestsPerMinute > 0 && len(entry.requests) > rl.cfg.RequestsPerMinute {
			// block the IP
			entry.blockedUntil = now.Add(time.Duration(rl.cfg.BlockMinutes) * time.Minute)
			retry := int(entry.blockedUntil.Sub(now).Seconds())
			entry.mu.Unlock()
			header := w.Header()
			header.Set("Retry-After", fmt.Sprintf("%d", retry))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
			return
		}
		entry.mu.Unlock()

		// wrap response to capture status
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		// after response, update error counts if necessary
		if rw.status == http.StatusNotFound || rw.status == http.StatusUnauthorized {
			entry := rl.getEntry(ip)
			now := time.Now()
			entry.mu.Lock()
			entry.errors = append(entry.errors, now)
			entry.trim(now, rl.cfg)
			if rl.cfg.MaxErrors > 0 && len(entry.errors) > rl.cfg.MaxErrors {
				entry.blockedUntil = now.Add(time.Duration(rl.cfg.BlockMinutes) * time.Minute)
			}
			entry.mu.Unlock()
		}

		// vulnerability scan paths: count only 404s on configured urls
		if rw.status == http.StatusNotFound && len(rl.cfg.VulnerabilityScan.URLs) > 0 {
			for _, pattern := range rl.cfg.VulnerabilityScan.URLs {
				matched, _ := doublestar.PathMatch(pattern, r.URL.Path)
				if matched {
					entry := rl.getEntry(ip)
					now := time.Now()
					entry.mu.Lock()
					entry.scan404 = append(entry.scan404, now)
					entry.trim(now, rl.cfg)
					if rl.cfg.VulnerabilityScan.Max404 > 0 && len(entry.scan404) > rl.cfg.VulnerabilityScan.Max404 {
						entry.blockedUntil = now.Add(time.Duration(rl.cfg.VulnerabilityScan.BlockMinutes) * time.Minute)
					}
					entry.mu.Unlock()
					break
				}
			}
		}
	})
}

// statusRecorder is a minimal response writer that remembers the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// getEntry retrieves or creates the rateEntry for the given IP.
func (rl *RateLimiter) getEntry(ip string) *rateEntry {
	if v, ok := rl.entries.Load(ip); ok {
		return v.(*rateEntry)
	}
	e := &rateEntry{}
	actual, _ := rl.entries.LoadOrStore(ip, e)
	return actual.(*rateEntry)
}

// trim removes outdated timestamps from the entry.
func (e *rateEntry) trim(now time.Time, cfg config.RateLimiterConfig) {
	// prune requests older than one minute
	cutoff := now.Add(-1 * time.Minute)
	i := 0
	for ; i < len(e.requests); i++ {
		if e.requests[i].After(cutoff) {
			break
		}
	}
	if i > 0 {
		e.requests = e.requests[i:]
	}

	// prune errors older than block window
	if cfg.BlockMinutes > 0 {
		cutoffErr := now.Add(-time.Duration(cfg.BlockMinutes) * time.Minute)
		j := 0
		for ; j < len(e.errors); j++ {
			if e.errors[j].After(cutoffErr) {
				break
			}
		}
		if j > 0 {
			e.errors = e.errors[j:]
		}
	}

	// prune vulnerability scan timestamps
	if cfg.VulnerabilityScan.BlockMinutes > 0 {
		cutoffScan := now.Add(-time.Duration(cfg.VulnerabilityScan.BlockMinutes) * time.Minute)
		k := 0
		for ; k < len(e.scan404); k++ {
			if e.scan404[k].After(cutoffScan) {
				break
			}
		}
		if k > 0 {
			e.scan404 = e.scan404[k:]
		}
	}
}

// RateLimiterStat is a snapshot of a single IP's rate limiter state.
type RateLimiterStat struct {
	IP           string    `json:"ip"`
	Requests     int       `json:"requests"`
	Errors       int       `json:"errors"`
	Scan404      int       `json:"scan404"`
	BlockedUntil time.Time `json:"blockedUntil"`
}

// Stats returns a copy of the current entries suitable for reporting.
func (rl *RateLimiter) Stats() []RateLimiterStat {
	var stats []RateLimiterStat
	rl.entries.Range(func(key, val interface{}) bool {
		ip := key.(string)
		e := val.(*rateEntry)
		e.mu.Lock()
		stats = append(stats, RateLimiterStat{
			IP:           ip,
			Requests:     len(e.requests),
			Errors:       len(e.errors),
			Scan404:      len(e.scan404),
			BlockedUntil: e.blockedUntil,
		})
		e.mu.Unlock()
		return true
	})
	return stats
}

// Config returns a snapshot of the limiter's configuration.
// A copy is returned to avoid callers mutating internal state.
func (rl *RateLimiter) Config() config.RateLimiterConfig {
	// cfg is a value type so copying is cheap, and it is immutable after
	// limiter creation which means we can safely return it directly.
	return rl.cfg
}

// cleanupLoop periodically removes stale entries from the map.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	for now := range ticker.C {
		rl.entries.Range(func(key, val interface{}) bool {
			entry := val.(*rateEntry)
			entry.mu.Lock()
			entry.trim(now, rl.cfg)
			if entry.blockedUntil.Before(now) && len(entry.requests) == 0 && len(entry.errors) == 0 && len(entry.scan404) == 0 {
				rl.entries.Delete(key)
			}
			entry.mu.Unlock()
			return true
		})
	}
}
