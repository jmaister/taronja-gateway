package middleware

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// RateLimiter implements an in‑memory rate limiter keyed by client IP.
// It's safe for concurrent use and maintains its own cleanup goroutine.
type RateLimiter struct {
	cfg             config.RateLimiterConfig
	entries         sync.Map // map[string]*rateEntry
	cleanupInterval time.Duration
	// scanPatterns is cfg.VulnerabilityScan.URLs, preprocessed once here
	// instead of on every 404 — see scanPattern's doc comment.
	scanPatterns []scanPattern
	// blockedClientRepo persists every block event (see recordBlock) so
	// it survives past cleanupLoop discarding the in-memory entry once
	// the block expires — nil when no repository was supplied (e.g.
	// RateLimiterMiddleware's standalone constructor, used where there's
	// no database to write to), in which case blocks are still enforced
	// exactly as before, just never recorded to the persistent registry.
	blockedClientRepo db.BlockedClientRepository
}

// scanPattern holds the precomputed forms of one
// config.VulnerabilityScanConfig.URLs entry that matches actually needs:
// the backslash-normalized pattern, and — only when applicable — the
// "expanded" form that also matches the bare pattern at any nesting depth
// (see matches' doc comment). Both depend only on the pattern string, never
// on the request, so computing them here, once per pattern when the
// RateLimiter is built, replaces work matchesVulnerabilityScanPath used to
// redo on every single matching attempt: normalizing the pattern's
// backslashes, checking it for "**", and building the expanded string via
// strings.ReplaceAll, all from scratch, for every configured pattern, on
// every 404 response. That's exactly the traffic this feature exists to
// handle efficiently — a scanner generating many rapid 404s — so redoing
// static, pattern-only work on that hot path was self-defeating.
type scanPattern struct {
	normalized string
	expanded   string // "" if this pattern has no bare "*" to expand (see newScanPattern)
}

// normalizeScanPath normalizes a request path's separators the same way
// newScanPattern normalizes a pattern's, so the two are comparable
// regardless of platform.
func normalizeScanPath(requestPath string) string {
	return strings.ReplaceAll(requestPath, "\\", "/")
}

// newScanPattern precomputes the forms of pattern that matches(requestPath)
// needs. Mirrors matchesVulnerabilityScanPath's normalization/expansion
// rules exactly — that function is now a thin per-call wrapper around this
// same logic, kept for its existing direct unit test coverage.
func newScanPattern(pattern string) scanPattern {
	normalized := strings.ReplaceAll(pattern, "\\", "/")
	sp := scanPattern{normalized: normalized}
	// Expand a pattern with a bare "*" (but no "**") so it also matches the
	// pattern nested at any depth, e.g. "/*.php" -> "/**/*.php" matches
	// "/dir/admin.php" too, not just a top-level "/admin.php".
	if !strings.Contains(normalized, "**") && strings.Contains(normalized, "*") {
		sp.expanded = strings.ReplaceAll(normalized, "*", "**/*")
	}
	return sp
}

// matches reports whether normalizedRequestPath (already backslash-
// normalized by the caller — see the Handler's vulnerability-scan block)
// matches this pattern, trying the expanded form (if any) as a fallback.
func (sp scanPattern) matches(normalizedRequestPath string) bool {
	if matched, _ := doublestar.Match(sp.normalized, normalizedRequestPath); matched {
		return true
	}
	if sp.expanded != "" {
		if matched, _ := doublestar.Match(sp.expanded, normalizedRequestPath); matched {
			return true
		}
	}
	return false
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
//
// Built with no BlockedClientRepository — this standalone constructor has
// no database connection to hand it one, so block events it enforces are
// never persisted to the registry (see RateLimiter.blockedClientRepo).
// The real gateway runtime always goes through NewRateLimiter directly
// instead (see gateway/reload.go's buildRuntime), supplying one.
func RateLimiterMiddleware(cfg config.RateLimiterConfig) func(http.Handler) http.Handler {
	rl := NewRateLimiter(cfg, nil)
	return rl.Handler
}

// NewRateLimiter constructs a RateLimiter and starts the cleanup goroutine.
// blockedClientRepo may be nil (see RateLimiter.blockedClientRepo's doc
// comment for what that means).
func NewRateLimiter(cfg config.RateLimiterConfig, blockedClientRepo db.BlockedClientRepository) *RateLimiter {
	// determine cleanup interval: use block minutes or default one minute
	interval := time.Minute
	if cfg.BlockMinutes > 0 {
		interval = time.Duration(cfg.BlockMinutes) * time.Minute
	}
	scanPatterns := make([]scanPattern, len(cfg.VulnerabilityScan.URLs))
	for i, url := range cfg.VulnerabilityScan.URLs {
		scanPatterns[i] = newScanPattern(url)
	}

	rl := &RateLimiter{
		cfg:               cfg,
		cleanupInterval:   interval,
		scanPatterns:      scanPatterns,
		blockedClientRepo: blockedClientRepo,
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
			blockedUntil := entry.blockedUntil
			triggerCount := len(entry.requests)
			retry := int(entry.blockedUntil.Sub(now).Seconds())
			entry.mu.Unlock()
			header := w.Header()
			header.Set("Retry-After", fmt.Sprintf("%d", retry))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
			// After the response, not before: recordBlock builds a
			// ClientInfo (User-Agent parse, geolocation lookup) that has
			// no business adding latency to a response that's already
			// been written.
			rl.recordBlock(r, db.BlockReasonRateLimit, "", triggerCount, now, blockedUntil)
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
			var justBlocked bool
			var blockedUntil time.Time
			var triggerCount int
			if rl.cfg.MaxErrors > 0 && len(entry.errors) > rl.cfg.MaxErrors {
				entry.blockedUntil = now.Add(time.Duration(rl.cfg.BlockMinutes) * time.Minute)
				justBlocked, blockedUntil, triggerCount = true, entry.blockedUntil, len(entry.errors)
			}
			entry.mu.Unlock()
			if justBlocked {
				rl.recordBlock(r, db.BlockReasonMaxErrors, "", triggerCount, now, blockedUntil)
			}
		}

		// vulnerability scan paths: count only 404s on configured urls.
		// The request path is normalized once here, not once per pattern
		// inside the loop — see scanPattern's doc comment for why that
		// matters specifically on this path.
		if rw.status == http.StatusNotFound && len(rl.scanPatterns) > 0 {
			normalizedPath := normalizeScanPath(r.URL.Path)
			for _, sp := range rl.scanPatterns {
				if sp.matches(normalizedPath) {
					entry := rl.getEntry(ip)
					now := time.Now()
					entry.mu.Lock()
					entry.scan404 = append(entry.scan404, now)
					entry.trim(now, rl.cfg)
					var justBlocked bool
					var blockedUntil time.Time
					var triggerCount int
					if rl.cfg.VulnerabilityScan.Max404 > 0 && len(entry.scan404) > rl.cfg.VulnerabilityScan.Max404 {
						entry.blockedUntil = now.Add(time.Duration(rl.cfg.VulnerabilityScan.BlockMinutes) * time.Minute)
						justBlocked, blockedUntil, triggerCount = true, entry.blockedUntil, len(entry.scan404)
					}
					entry.mu.Unlock()
					if justBlocked {
						rl.recordBlock(r, db.BlockReasonVulnerabilityScan, r.URL.Path, triggerCount, now, blockedUntil)
					}
					break
				}
			}
		}
	})
}

// recordBlock persists one block event to the registry (blockedClientRepo)
// — a no-op if none was supplied (see that field's doc comment). Building
// the db.BlockedClient (including session.NewClientInfo's User-Agent
// parse and geolocation lookup) happens synchronously, on the same
// goroutine handling this request — mirroring exactly how
// middleware/trafficmetric.go's TrafficMetricMiddleware already builds
// its own per-request record for every single request, not just blocked
// ones, deferring only the database write itself to a goroutine. Every
// call site above only calls this after the response has already been
// written, so none of this synchronous work adds latency to what the
// client actually experiences.
func (rl *RateLimiter) recordBlock(r *http.Request, reason, path string, triggerCount int, blockedAt, blockedUntil time.Time) {
	if rl.blockedClientRepo == nil {
		return
	}
	clientInfo := session.NewClientInfo(r)
	bc := &db.BlockedClient{
		Reason:       reason,
		Path:         path,
		TriggerCount: triggerCount,
		BlockedAt:    blockedAt,
		BlockedUntil: blockedUntil,
		ClientInfo:   *clientInfo,
	}
	go func() {
		if err := rl.blockedClientRepo.Create(bc); err != nil {
			log.Printf("rate limiter: failed to record blocked client %s: %v", bc.IPAddress, err)
		}
	}()
}

// matchesVulnerabilityScanPath reports whether requestPath matches pattern,
// applying the same backslash-normalization and nested-path expansion
// rules as scanPattern.matches. Kept as a per-call convenience (and its
// existing exhaustive test coverage) for callers matching a one-off
// pattern; RateLimiter.Handler's hot path uses precomputed scanPatterns
// instead — see scanPattern's doc comment for why that distinction matters
// here specifically.
func matchesVulnerabilityScanPath(pattern string, requestPath string) bool {
	return newScanPattern(pattern).matches(normalizeScanPath(requestPath))
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
