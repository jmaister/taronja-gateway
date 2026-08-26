package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestMiddlewareEntryConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		entry    MiddlewareEntryConfig
		expected bool
	}{
		{"absent Enabled defaults to true", MiddlewareEntryConfig{Name: "logging"}, true},
		{"explicit true", MiddlewareEntryConfig{Name: "logging", Enabled: boolPtr(true)}, true},
		{"explicit false", MiddlewareEntryConfig{Name: "logging", Enabled: boolPtr(false)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.entry.IsEnabled())
		})
	}
}

func TestIsMiddlewareNameKnown(t *testing.T) {
	for _, name := range KnownMiddlewareNames {
		assert.True(t, IsMiddlewareNameKnown(name), "expected %s to be known", name)
	}
	assert.False(t, IsMiddlewareNameKnown("not_a_real_middleware"))
	assert.False(t, IsMiddlewareNameKnown(""))
}

// writeTestConfig writes a minimal valid gateway config, augmented with extraYAML
// appended at the end, to a temp file and returns its path.
func writeTestConfig(t *testing.T, extraYAML string) string {
	t.Helper()
	base := `
version: 2
name: Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  admin:
    enabled: false
routes:
  - name: root
    from: /
    to: http://localhost:9999
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base+extraYAML), 0o644))
	return path
}

func TestLoadConfig_NoMiddlewareSection(t *testing.T) {
	path := writeTestConfig(t, "")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Middleware.Global, "no middleware: section should leave Global empty")
	assert.Nil(t, cfg.Middleware.Global, "an absent middleware: section must leave Global nil, not an empty slice — "+
		"middleware.ResolveGlobalChainSpecs distinguishes the two to let a config explicitly declare zero global middleware")
}

// TestLoadConfig_ExplicitEmptyMiddlewareGlobalIsNotNil guards the nil-vs-empty
// distinction ResolveGlobalChainSpecs relies on: `middleware: {global: []}`
// must NOT unmarshal the same way as an absent middleware: section (nil),
// otherwise a config author has no way to explicitly disable every global
// middleware without deleting the section entirely — it would silently fall
// back to the legacy management.analytics/logging/rateLimiter flags instead.
func TestLoadConfig_ExplicitEmptyMiddlewareGlobalIsNotNil(t *testing.T) {
	path := writeTestConfig(t, "middleware:\n  global: []\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.NotNil(t, cfg.Middleware.Global, "an explicit empty global: [] must unmarshal to a non-nil empty slice")
	assert.Empty(t, cfg.Middleware.Global)
}

func TestLoadConfig_MiddlewareSectionParsed(t *testing.T) {
	path := writeTestConfig(t, `
middleware:
  global:
    - name: rate_limiter
      rateLimiter:
        requestsPerMinute: 42
        maxErrors: 5
        blockMinutes: 10
    - name: ja4_fingerprint
    - name: session_extraction
    - name: traffic_metrics
    - name: logging
      enabled: false
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	require.Len(t, cfg.Middleware.Global, 5)

	rl := cfg.Middleware.Global[0]
	assert.Equal(t, MiddlewareNameRateLimiter, rl.Name)
	assert.True(t, rl.IsEnabled())
	require.NotNil(t, rl.RateLimiter)
	assert.Equal(t, 42, rl.RateLimiter.RequestsPerMinute)
	assert.Equal(t, 5, rl.RateLimiter.MaxErrors)
	assert.Equal(t, 10, rl.RateLimiter.BlockMinutes)

	logging := cfg.Middleware.Global[4]
	assert.Equal(t, MiddlewareNameLogging, logging.Name)
	assert.False(t, logging.IsEnabled())
}

func TestLoadConfig_UnknownMiddlewareNameRejected(t *testing.T) {
	path := writeTestConfig(t, `
middleware:
  global:
    - name: not_a_real_middleware
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware")
}

func TestLoadConfig_MissingMiddlewareNameRejected(t *testing.T) {
	path := writeTestConfig(t, `
middleware:
  global:
    - enabled: true
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'name'")
}

func TestLoadConfig_DuplicateMiddlewareNameRejected(t *testing.T) {
	path := writeTestConfig(t, `
middleware:
  global:
    - name: logging
    - name: logging
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}
