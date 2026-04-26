package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
)

// Test basic request rate limiting behavior
func TestRateLimiter_RequestLimit(t *testing.T) {
	cfg := config.RateLimiterConfig{
		RequestsPerMinute: 2,
		MaxErrors:         0,
		BlockMinutes:      1,
	}
	rl := NewRateLimiter(cfg)
	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/foo", nil)
	req.RemoteAddr = "1.2.3.4:1234"

	// first request should pass
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	entry := rl.getEntry("1.2.3.4")
	t.Logf("after first request: requests=%d errors=%d blockedUntil=%v", len(entry.requests), len(entry.errors), entry.blockedUntil)

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	// log again
	entry = rl.getEntry("1.2.3.4")
	t.Logf("after second request: requests=%d errors=%d blockedUntil=%v", len(entry.requests), len(entry.errors), entry.blockedUntil)
	assert.Equal(t, http.StatusOK, w.Code)

	// third request exceeds limit and returns 429
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "Rate limit exceeded", w.Body.String())

	// entry should be marked blocked
	entry = rl.getEntry("1.2.3.4")
	assert.True(t, time.Now().Before(entry.blockedUntil))

	// manually expire the block and clear historical timestamps so a new
	// request will not immediately re‑trigger the limit.
	entry.mu.Lock()
	entry.blockedUntil = time.Now().Add(-time.Minute)
	entry.requests = nil
	entry.errors = nil
	entry.mu.Unlock()

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Test that repeated 404/401 errors trigger blocking
func TestRateLimiter_ErrorLimit(t *testing.T) {
	cfg := config.RateLimiterConfig{
		RequestsPerMinute: 0,
		MaxErrors:         2,
		BlockMinutes:      1,
	}
	rl := NewRateLimiter(cfg)

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // simulate missing resource
	}))

	req := httptest.NewRequest("GET", "/bar", nil)
	req.RemoteAddr = "5.6.7.8:4321"

	// first two calls return 404 but are not blocked yet
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// third call should still return 404; limiter blocks subsequent requests
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// fourth call should now be blocked with 429
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// check that errors slice contains at least three timestamps
	entry := rl.getEntry("5.6.7.8")
	assert.Greater(t, len(entry.errors), 2)
}

// Test that an empty configuration disables all limiting and is a no-op.
func TestRateLimiter_Disabled(t *testing.T) {
	cfg := config.RateLimiterConfig{} // zero values
	assert.False(t, cfg.IsEnabled(), "empty config should report disabled")

	rl := NewRateLimiter(cfg)
	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("passed"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "9.9.9.9:1111"

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "should pass through when disabled")
	}

	// also ensure the exported helper behaves the same way
	handler2 := RateLimiterMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	handler2.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Test combined limits: request count and error count share same block
func TestRateLimiter_Combined(t *testing.T) {
	cfg := config.RateLimiterConfig{
		RequestsPerMinute: 3,
		MaxErrors:         1,
		BlockMinutes:      1,
	}
	rl := NewRateLimiter(cfg)

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// alternate between OK and unauthorized
		if r.URL.Path == "/login" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/login", nil)
	req.RemoteAddr = "9.9.9.9:9999"

	// first unauthorized counts toward error limit
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// next two successful requests count toward rate limit
	req2 := httptest.NewRequest("GET", "/foo", nil)
	req2.RemoteAddr = "9.9.9.9:9999"

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req2)
	assert.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req2)
	assert.Equal(t, http.StatusOK, w.Code)

	// fourth request should hit rate limit because of previous three actions
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req2)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// Test vulnerability scan detection via configured URLs
func TestRateLimiter_VulnerabilityScan(t *testing.T) {
	cfg := config.RateLimiterConfig{
		RequestsPerMinute: 0,
		MaxErrors:         0,
		BlockMinutes:      1,
		VulnerabilityScan: config.VulnerabilityScanConfig{
			URLs:         []string{"/admin.php", "/.env"},
			Max404:       2,
			BlockMinutes: 1,
		},
	}
	rl := NewRateLimiter(cfg)

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/admin.php", nil)
	req.RemoteAddr = "11.11.11.11:1111"

	// first two hits should return 404
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	entry := rl.getEntry("11.11.11.11")
	t.Logf("scan count after first: %d", len(entry.scan404))

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	entry = rl.getEntry("11.11.11.11")
	t.Logf("scan count after second: %d", len(entry.scan404))

	// third hit on monitored path should still return 404 (block applies next)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	entry = rl.getEntry("11.11.11.11")
	t.Logf("scan count after third: %d", len(entry.scan404))

	// fourth hit should now be rate limited
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// check scan404 list length
	assert.Greater(t, len(entry.scan404), 2)

	// non-watched path should not count towards scan
	rq2 := httptest.NewRequest("GET", "/not-watched", nil)
	rq2.RemoteAddr = "11.11.11.11:1111"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, rq2)
	// still blocked
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// Test that the trim function correctly prunes old timestamps.
func TestRateLimiter_Trim(t *testing.T) {
	cfg := config.RateLimiterConfig{
		RequestsPerMinute: 10,
		MaxErrors:         10,
		BlockMinutes:      1,
		VulnerabilityScan: config.VulnerabilityScanConfig{
			URLs:         []string{"/foo"},
			Max404:       10,
			BlockMinutes: 1,
		},
	}

	entry := &rateEntry{}
	now := time.Now()

	// Add requests, errors, and scan404s that should be trimmed and some that should remain
	// Requests (1 minute cutoff)
	entry.requests = []time.Time{
		now.Add(-2 * time.Minute),  // should be trimmed
		now.Add(-30 * time.Second), // should remain
	}
	// Errors (BlockMinutes cutoff = 1 minute)
	entry.errors = []time.Time{
		now.Add(-2 * time.Minute),  // should be trimmed
		now.Add(-45 * time.Second), // should remain
	}
	// Scan404 (VulnerabilityScan.BlockMinutes cutoff = 1 minute)
	entry.scan404 = []time.Time{
		now.Add(-2 * time.Minute),  // should be trimmed
		now.Add(-15 * time.Second), // should remain
	}

	entry.trim(now, cfg)

	assert.Len(t, entry.requests, 1)
	assert.True(t, entry.requests[0].After(now.Add(-1*time.Minute)))

	assert.Len(t, entry.errors, 1)
	assert.True(t, entry.errors[0].After(now.Add(-1*time.Minute)))

	assert.Len(t, entry.scan404, 1)
	assert.True(t, entry.scan404[0].After(now.Add(-1*time.Minute)))
}

// Test the various matching scenarios for vulnerability scan paths
func TestMatchesVulnerabilityScanPath(t *testing.T) {
	tests := []struct {
		pattern     string
		requestPath string
		expected    bool
	}{
		// Direct matches
		{"/foo/bar", "/foo/bar", true},
		{"/foo/baz", "/foo/bar", false},

		// Backslash normalization
		{"\\foo\\bar", "/foo/bar", true},
		{"/foo/bar", "\\foo\\bar", true},

		// Root wildcard patterns (path.Base matching)
		{"/*.php", "/admin.php", true},
		{"/*.php", "/dir/admin.php", true},
		{"/*.php", "/dir/sub/admin.php", true},
		{"/*.js", "/app.js", true},
		{"/*.js", "/dir/app.js", true},
		{"/*.php", "/admin/test.php", true},
		{"/*.php", "/admin.jpg", false},
		{"/*", "/anything", true},
		{"/*", "/dir/anything", true}, // Should match base name

		// Single segment wildcard (doublestar)
		{"/foo/*", "/foo/bar", true},
		{"/foo/*", "/foo/bar/baz", true},
		{"/foo/*", "/bar/baz", false},

		// Multi segment wildcard (doublestar)
		{"/foo/**", "/foo/bar", true},
		{"/foo/**", "/foo/bar/baz", true},
		{"/foo/**", "/bar/baz", false},
		{"/**/bar", "/foo/bar", true},
		{"/**/bar", "/foo/baz/bar", true},
		{"/**/bar", "/bar", true},

		// Recursive expansion of single segment wildcards (e.g., /*.php -> **/*.php)
		{"/*.php", "/nested/admin.php", true}, // originally handled by path.Base, but also by expansion
		{"/foo/*.js", "/foo/bar/app.js", true},
		{"/foo/*.js", "/foo/bar/baz/app.js", true},
		{"/foo/*", "/foo/bar/baz", true},
		{"/user/*/profile", "/user/123/profile", true},
		{"/user/*/profile", "/user/123/nested/profile", true},

		// Patterns with existing /**/ should not be re-expanded (placeholder logic)
		{"/api/**/status", "/api/v1/status", true},
		{"/api/**/status", "/api/status", true},           // Should still match
		{"/api/**/status", "/api/v1/nested/status", true}, // Should still match

		// Complex scenarios
		{"/download/*/*.zip", "/download/files/archive.zip", true},
		{"/download/*/*.zip", "/download/temp/subdir/archive.zip", true},
		{"/download/**/*.zip", "/download/temp/subdir/archive.zip", true},
		{"/download/*/*.zip", "/download/archive.zip", false}, // Requires two segments before .zip
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Pattern: %s, Path: %s", tt.pattern, tt.requestPath), func(t *testing.T) {
			actual := matchesVulnerabilityScanPath(tt.pattern, tt.requestPath)
			assert.Equal(t, tt.expected, actual, "for pattern %s and path %s", tt.pattern, tt.requestPath)
		})
	}
}

func TestRateLimiter_VulnerabilityScanWildcardMatchesNestedPaths(t *testing.T) {
	cfg := config.RateLimiterConfig{
		RequestsPerMinute: 0,
		MaxErrors:         0,
		BlockMinutes:      1,
		VulnerabilityScan: config.VulnerabilityScanConfig{
			URLs:         []string{"/*.php", "/*.yml"},
			Max404:       2,
			BlockMinutes: 1,
		},
	}
	rl := NewRateLimiter(cfg)

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	paths := []string{
		"/mailchimp_keys.php",
		"/config/mandrill.local.php",
		"/config/mailchimp.yml",
	}

	for _, requestPath := range paths {
		req := httptest.NewRequest("GET", requestPath, nil)
		req.RemoteAddr = "12.12.12.12:1212"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	}

	entry := rl.getEntry("12.12.12.12")
	assert.Len(t, entry.scan404, 3)
	assert.True(t, entry.blockedUntil.After(time.Now()))

	req := httptest.NewRequest("GET", "/lib/sparkpost.php", nil)
	req.RemoteAddr = "12.12.12.12:1212"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
