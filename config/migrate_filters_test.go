package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func migratedFilters(t *testing.T, raw string) (string, []FilterConfig) {
	t.Helper()
	out := string(migrateLegacyToV1([]byte(raw)))
	var cfg struct {
		Management map[string]any `yaml:"management"`
		Filters    []FilterConfig `yaml:"filters"`
		Middleware any            `yaml:"middleware"`
		Tracing    any            `yaml:"tracing"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &cfg), out)
	for _, key := range []string{"logging", "compression", "analytics", "excludeStaticAssets", "rateLimiter", "cors"} {
		assert.NotContains(t, cfg.Management, key)
	}
	assert.Nil(t, cfg.Middleware)
	assert.Nil(t, cfg.Tracing)
	return out, cfg.Filters
}

func filterNames(filters []FilterConfig) []string {
	names := make([]string, len(filters))
	for i, f := range filters {
		names[i] = f.Name
	}
	return names
}

func TestMoveFiltersToRoot_ManagementFlagsKeepTheOldOrder(t *testing.T) {
	out, filters := migratedFilters(t, `name: Test
management:
  prefix: /_
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
	assert.Equal(t, []string{"tracing", "compression", "cors", "rate_limiter", "ja4_fingerprint", "session_extraction", "traffic_metrics", "logging"}, filterNames(filters))
	assert.Contains(t, out, "prefix: /_", "other management keys stay")
	assert.Contains(t, out, "# per IP", "comments inside moved blocks survive")
	assert.Contains(t, out, "  - name: rate_limiter\n    config:\n      requestsPerMinute: 100", "2-space indentation")

	var tracing struct {
		Endpoint string `yaml:"endpoint"`
		Enabled  *bool  `yaml:"enabled"`
	}
	require.NoError(t, filters[0].Config.Decode(&tracing))
	assert.Equal(t, "localhost:4318", tracing.Endpoint)
	assert.Nil(t, tracing.Enabled, "listing the filter replaces tracing.enabled")

	var metrics struct {
		ExcludeStaticAssets bool `yaml:"excludeStaticAssets"`
	}
	require.NoError(t, filters[6].Config.Decode(&metrics))
	assert.True(t, metrics.ExcludeStaticAssets)
}

func TestMoveFiltersToRoot_DisabledFlagsProduceNoFilters(t *testing.T) {
	out, filters := migratedFilters(t, `name: Test
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
	assert.Empty(t, filters)
	assert.NotContains(t, out, "filters:")
}

func TestMoveFiltersToRoot_ExplicitMiddlewareListKeepsItsOrderAndFallbacks(t *testing.T) {
	_, filters := migratedFilters(t, `name: Test
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
	require.Equal(t, []string{"logging", "rate_limiter", "cors"}, filterNames(filters))
	assert.False(t, filters[0].IsEnabled())

	var rl struct {
		RequestsPerMinute int `yaml:"requestsPerMinute"`
	}
	require.NoError(t, filters[1].Config.Decode(&rl))
	assert.Equal(t, 7, rl.RequestsPerMinute, "an entry without options falls back to management.rateLimiter")

	var cors struct {
		AllowedOrigins []string `yaml:"allowedOrigins"`
	}
	require.NoError(t, filters[2].Config.Decode(&cors))
	assert.Equal(t, []string{"*"}, cors.AllowedOrigins)
}

func TestMoveFiltersToRoot_LeavesConfigsWithoutFilterSettingsByteForByte(t *testing.T) {
	raw := "name: Test   # keep spacing\nserver:\n    port: 8080\n"
	assert.Equal(t, "version: 1.0\n"+raw, string(migrateLegacyToV1([]byte(raw))))
}
