package v0_test

import (
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	v0 "github.com/jmaister/taronja-gateway/config/v0"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func migratedMiddleware(t *testing.T, raw string) (string, []config.MiddlewareConfig) {
	t.Helper()
	out := string(v0.UpgradeToV1([]byte(raw)))
	var cfg struct {
		Management map[string]any            `yaml:"management"`
		Middleware []config.MiddlewareConfig `yaml:"middleware"`
		Tracing    any                       `yaml:"tracing"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &cfg), out)
	for _, key := range []string{"logging", "compression", "analytics", "excludeStaticAssets", "rateLimiter", "cors"} {
		assert.NotContains(t, cfg.Management, key)
	}
	assert.Nil(t, cfg.Tracing)
	return out, cfg.Middleware
}

func entryNames(entries []config.MiddlewareConfig) []string {
	names := make([]string, len(entries))
	for i, f := range entries {
		names[i] = f.Name
	}
	return names
}

func TestMoveMiddlewareToRoot_ManagementFlagsKeepTheOldOrder(t *testing.T) {
	out, entries := migratedMiddleware(t, `name: Test
management:
  admin:
    enabled: false
  logging: true
  compression: true
  analytics: true
  excludeStaticAssets: true
  rateLimiter:
    requestsPerMinute: 100 # per IP
    blockMinutes: 5
  cors:
    allowedOrigins: ["https://app.example.com"]
tracing:
  enabled: true
  endpoint: localhost:4318
`)
	assert.Equal(t, []string{"tracing", "compression", "cors", "rate_limiter", "ja4_fingerprint", "session_extraction", "traffic_metrics", "logging"}, entryNames(entries))
	assert.Contains(t, out, "admin:\n    enabled: false", "other management keys stay")
	assert.Contains(t, out, "# per IP", "comments inside moved blocks survive")
	assert.Contains(t, out, "  - name: rate_limiter\n    config:\n      requestsPerMinute: 100", "2-space indentation")

	var tracing struct {
		Endpoint string `yaml:"endpoint"`
		Enabled  *bool  `yaml:"enabled"`
	}
	require.NoError(t, entries[0].Config.Decode(&tracing))
	assert.Equal(t, "localhost:4318", tracing.Endpoint)
	assert.Nil(t, tracing.Enabled, "listing the middleware replaces tracing.enabled")

	var metrics struct {
		ExcludeStaticAssets bool `yaml:"excludeStaticAssets"`
	}
	require.NoError(t, entries[6].Config.Decode(&metrics))
	assert.True(t, metrics.ExcludeStaticAssets)
}

func TestMoveMiddlewareToRoot_DisabledFlagsProduceNoMiddleware(t *testing.T) {
	out, entries := migratedMiddleware(t, `name: Test
management:
  logging: false
  analytics: false
  rateLimiter:
    requestsPerMinute: 0
  cors:
    allowedOrigins: []
tracing:
  enabled: false
`)
	assert.Empty(t, entries)
	assert.NotContains(t, out, "middleware:")
}

func TestMoveMiddlewareToRoot_ExplicitMiddlewareListKeepsItsOrderAndFallbacks(t *testing.T) {
	_, entries := migratedMiddleware(t, `name: Test
management:
  analytics: true
  rateLimiter:
    requestsPerMinute: 7
    blockMinutes: 1
middleware:
  global:
    - name: logging
      enabled: false
    - name: rate_limiter
    - name: cors
      cors:
        allowedOrigins: ["*"]
`)
	require.Equal(t, []string{"logging", "rate_limiter", "cors"}, entryNames(entries))
	assert.False(t, entries[0].IsEnabled())

	var rl struct {
		RequestsPerMinute int `yaml:"requestsPerMinute"`
	}
	require.NoError(t, entries[1].Config.Decode(&rl))
	assert.Equal(t, 7, rl.RequestsPerMinute, "an entry without options falls back to management.rateLimiter")

	var cors struct {
		AllowedOrigins []string `yaml:"allowedOrigins"`
	}
	require.NoError(t, entries[2].Config.Decode(&cors))
	assert.Equal(t, []string{"*"}, cors.AllowedOrigins)
}

func TestMoveMiddlewareToRoot_LeavesConfigsWithoutMiddlewareSettingsByteForByte(t *testing.T) {
	raw := "name: Test   # keep spacing\nserver:\n    port: 8080\n"
	assert.Equal(t, raw, string(v0.UpgradeToV1([]byte(raw))), "nothing to move: returned byte for byte")
}
