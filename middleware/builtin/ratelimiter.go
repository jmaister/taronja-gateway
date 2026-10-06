package builtin

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
	yaml "gopkg.in/yaml.v3"
)

// RateLimiter implements an in‑memory rate limiter keyed by client IP.
// It's safe for concurrent use and maintains its own cleanup goroutine —
// call Close when a RateLimiter is being discarded (e.g. gateway/setup.go
// building a fresh one during setup) so that goroutine actually stops
// instead of running forever alongside its replacement.
type RateLimiter struct {
	cfg             RateLimiterConfig
	entries         sync.Map // map[string]*rateEntry
	cleanupInterval time.Duration
	// stop, closed by Close, tells cleanupLoop to exit. Close is safe to
	// call more than once (sync.Once).
	stop     chan struct{}
	stopOnce sync.Once
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
	// blockedClientRetention bounds how long a row recordBlock wrote stays
	// in the database before pruneBlockedClientsLoop deletes it — see
	// RateLimiterConfig.BlockedClientRetentionDays and
	// defaultBlockedClientRetentionDays. Unused when blockedClientRepo is
	// nil.
	blockedClientRetention time.Duration
}

// defaultBlockedClientRetentionDays is how long a persisted block event is
// kept when RateLimiterConfig.BlockedClientRetentionDays isn't set
// (0). 90 days comfortably covers the kind of incident review or abuse
// pattern analysis this history exists for, without keeping every scanner's
// visit forever on a gateway that's been up for years.
const defaultBlockedClientRetentionDays = 90

// blockedClientPruneInterval is how often pruneBlockedClientsLoop checks
// for rows past their retention window. Independent of cleanupInterval
// (which can be as short as one minute, driven by BlockMinutes) — pruning
// old history doesn't need anywhere near that frequency, and running it on
// a fixed, coarser cadence keeps it from adding a DELETE query to a busy
// gateway's tightest cleanup loop.
const blockedClientPruneInterval = time.Hour

// scanPattern holds the per-pattern forms matches needs (the backslash-normalized
// pattern and, where applicable, the expanded form), computed once at construction
// instead of on every 404.
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

// RateLimiterMiddleware creates a middleware from cfg; it is a no-op if both
// RequestsPerMinute and MaxErrors are zero. It has no BlockedClientRepository, so
// blocks are not persisted; the gateway uses NewRateLimiter instead.
func RateLimiterMiddleware(cfg RateLimiterConfig) func(http.Handler) http.Handler {
	rl := NewRateLimiter(cfg, nil)
	return rl.Handler
}

// NewRateLimiter constructs a RateLimiter and starts the cleanup goroutine.
// blockedClientRepo may be nil (see RateLimiter.blockedClientRepo's doc
// comment for what that means).
func NewRateLimiter(cfg RateLimiterConfig, blockedClientRepo db.BlockedClientRepository) *RateLimiter {
	// determine cleanup interval: use block minutes or default one minute
	interval := time.Minute
	if cfg.BlockMinutes > 0 {
		interval = time.Duration(cfg.BlockMinutes) * time.Minute
	}
	scanPatterns := make([]scanPattern, len(cfg.VulnerabilityScan.URLs))
	for i, url := range cfg.VulnerabilityScan.URLs {
		scanPatterns[i] = newScanPattern(url)
	}

	retentionDays := cfg.BlockedClientRetentionDays
	if retentionDays <= 0 {
		retentionDays = defaultBlockedClientRetentionDays
	}

	rl := &RateLimiter{
		cfg:                    cfg,
		cleanupInterval:        interval,
		scanPatterns:           scanPatterns,
		blockedClientRepo:      blockedClientRepo,
		blockedClientRetention: time.Duration(retentionDays) * 24 * time.Hour,
		stop:                   make(chan struct{}),
	}
	go rl.cleanupLoop()
	if blockedClientRepo != nil {
		go rl.pruneBlockedClientsLoop()
	}
	return rl
}

// Close stops the cleanup goroutine started by NewRateLimiter. Safe to call
// more than once, and safe to call on a nil *RateLimiter (a no-op) so a
// call site holding a possibly-not-yet-set RateLimiter reference (e.g. a
// gateway's very first config load, before any limiter has been built)
// doesn't need its own nil check.
func (rl *RateLimiter) Close() {
	if rl == nil {
		return
	}
	rl.stopOnce.Do(func() {
		close(rl.stop)
	})
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

// recordBlock persists one block event if a repository was supplied. The
// ClientInfo is built synchronously (only the write is deferred), which costs
// nothing because callers have already written the response.
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
func (e *rateEntry) trim(now time.Time, cfg RateLimiterConfig) {
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
func (rl *RateLimiter) Config() RateLimiterConfig {
	// cfg is a value type so copying is cheap, and it is immutable after
	// limiter creation which means we can safely return it directly.
	return rl.cfg
}

// cleanupLoop periodically removes stale entries from the map, until Close
// is called — without this exit path, a RateLimiter discarded by a test or
// other short-lived caller (a real gateway process only ever builds one, in
// gateway/setup.go) would leak this goroutine and its ticker forever, and
// its still-running cleanup would keep expiring entries an operator might
// expect to remain read-only history at that point.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case now := <-ticker.C:
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
}

// pruneBlockedClientsLoop deletes db.BlockedClient rows older than
// rl.blockedClientRetention until Close; only started when blockedClientRepo is set.
func (rl *RateLimiter) pruneBlockedClientsLoop() {
	ticker := time.NewTicker(blockedClientPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case now := <-ticker.C:
			rl.pruneBlockedClients(now)
		}
	}
}

// pruneBlockedClients does the actual delete for pruneBlockedClientsLoop,
// split out so tests can trigger a prune deterministically instead of
// waiting on blockedClientPruneInterval's real-time ticker.
func (rl *RateLimiter) pruneBlockedClients(now time.Time) {
	cutoff := now.Add(-rl.blockedClientRetention)
	if _, err := rl.blockedClientRepo.DeleteOlderThan(cutoff); err != nil {
		log.Printf("rate limiter: failed to prune blocked_clients older than %s: %v", cutoff, err)
	}
}

// NameRateLimiter limits requests per client IP and blocks scanners.
const NameRateLimiter = "rate_limiter"

// RateLimiterConfig holds the rate_limiter middleware options, applied per client
// IP. Zero values turn the matching check off.
type RateLimiterConfig struct {
	RequestsPerMinute int `yaml:"requestsPerMinute"` // Max requests per IP per 60s window. 0 = no limit.
	MaxErrors         int `yaml:"maxErrors"`         // Max 401/404 responses before blocking. 0 = no limit.
	BlockMinutes      int `yaml:"blockMinutes"`      // How long an offending IP is blocked. Required when any check is on.
	// BlockedClientRetentionDays is how long persisted block events are kept.
	// 0 means the default (90 days).
	BlockedClientRetentionDays int `yaml:"blockedClientRetentionDays"`

	VulnerabilityScan VulnerabilityScanConfig `yaml:"vulnerabilityScan"` // Optional scanner detector.
}

// VulnerabilityScanConfig lists paths scanners probe; an IP that gets too many
// 404s on them is blocked.
type VulnerabilityScanConfig struct {
	URLs         []string `yaml:"urls"`         // Paths to watch (wildcards supported).
	Max404       int      `yaml:"max404"`       // 404s on watched paths before blocking.
	BlockMinutes int      `yaml:"blockMinutes"` // How long a scanner is blocked.
}

// IsEnabled reports whether any rate-limiting or scan check is active.
func (r RateLimiterConfig) IsEnabled() bool {
	return r.RequestsPerMinute > 0 || r.MaxErrors > 0 ||
		r.VulnerabilityScan.Max404 > 0 || len(r.VulnerabilityScan.URLs) > 0
}

// Validate checks the limits are coherent.
func (r RateLimiterConfig) Validate() error {
	switch {
	case r.RequestsPerMinute < 0:
		return fmt.Errorf("requestsPerMinute cannot be negative")
	case r.MaxErrors < 0:
		return fmt.Errorf("maxErrors cannot be negative")
	case r.IsEnabled() && r.BlockMinutes <= 0:
		return fmt.Errorf("blockMinutes must be positive when rate limiting is enabled")
	}
	v := r.VulnerabilityScan
	switch {
	case v.Max404 < 0:
		return fmt.Errorf("vulnerabilityScan.max404 cannot be negative")
	case v.Max404 > 0 && len(v.URLs) == 0:
		return fmt.Errorf("vulnerabilityScan.urls must be provided when max404 > 0")
	case v.Max404 > 0 && v.BlockMinutes <= 0:
		return fmt.Errorf("vulnerabilityScan.blockMinutes must be positive when scan detection is enabled")
	}
	return nil
}

// rateLimiterFactory reuses the gateway's shared RateLimiter, so the chain
// and the management API see the same state. Without one it builds a
// standalone limiter from the config.
type rateLimiterFactory struct {
	middleware.ConcreteFactory
	rateLimiter *RateLimiter
}

func newRateLimiterFactory(rateLimiter *RateLimiter) *rateLimiterFactory {
	return &rateLimiterFactory{
		ConcreteFactory: middleware.NewConcreteFactory(NameRateLimiter, "Limits request rate per client IP and detects vulnerability scans"),
		rateLimiter:     rateLimiter,
	}
}

func (f *rateLimiterFactory) DecodeConfig(raw *yaml.Node) (interface{}, error) {
	return middleware.DecodeConfigInto(raw, RateLimiterConfig{})
}

func (f *rateLimiterFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	if f.rateLimiter != nil {
		return f.rateLimiter.Handler, nil
	}
	c, err := middleware.ConfigAs[RateLimiterConfig](NameRateLimiter, cfg)
	if err != nil {
		return nil, err
	}
	return RateLimiterMiddleware(c), nil
}

// HealthCheck reports how many client IPs are tracked and blocked.
func (f *rateLimiterFactory) HealthCheck() middleware.MiddlewareHealth {
	if f.rateLimiter == nil {
		return middleware.MiddlewareHealth{Status: "unknown", Message: "no rate limiter instance configured"}
	}
	stats := f.rateLimiter.Stats()
	now := time.Now()
	blocked := 0
	for _, s := range stats {
		if s.BlockedUntil.After(now) {
			blocked++
		}
	}
	return middleware.MiddlewareHealth{
		Status:  "healthy",
		Message: fmt.Sprintf("tracking %d client IP(s), %d currently blocked", len(stats), blocked),
	}
}
