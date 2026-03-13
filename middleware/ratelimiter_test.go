package middleware

import (
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

func TestMatchesVulnerabilityScanPath(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		requestPath string
		expected    bool
	}{
		{name: "exact match", pattern: "/.env", requestPath: "/.env", expected: true},
		{name: "exact match", pattern: "/.env.*", requestPath: "/.env.prod", expected: true},
		{name: "root wildcard matches top level file", pattern: "/*.php", requestPath: "/mailchimp_keys.php", expected: true},
		{name: "root wildcard matches nested file", pattern: "/*.php", requestPath: "/config/mandrill.local.php", expected: true},
		{name: "root wildcard matches nested yaml file", pattern: "/*.yml", requestPath: "/config/mailchimp.yml", expected: true},
		{name: "pattern under fixed directory still matches direct child", pattern: "/admin/*.php", requestPath: "/admin/index.php", expected: true},
		{name: "different extension does not match", pattern: "/*.php", requestPath: "/config/mailchimp.yml", expected: false},
		{name: "fixed directory pattern does not overmatch nested folders", pattern: "/admin/*.php", requestPath: "/config/admin/index.php", expected: false},
		{name: "fixed directory pattern does not overmatch nested folders", pattern: "/admin/*.php", requestPath: "/admin/index.php", expected: true},
		{name: "nested wildcard pattern does not match deeper nested path", pattern: "/*/admin/*.php", requestPath: "/config/admin/index.php", expected: true},
	}

	for _, testCase := range tests {
		assert.Equal(t, testCase.expected, matchesVulnerabilityScanPath(testCase.pattern, testCase.requestPath), testCase.name)
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
