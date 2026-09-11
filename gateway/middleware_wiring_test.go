package gateway

import (
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildRuntime_RateLimiterHonorsPerEntryOverride guards against a
// bug where the shared *RateLimiter instance was always built from
// management.rateLimiter, silently ignoring a per-entry
// `middleware.global[].rateLimiter` override — because RateLimiterFactory
// reuses that shared instance's Handler whenever one exists, the override
// computed by ResolveGlobalChainSpecs never actually took effect. See
// middleware.EffectiveRateLimiterConfig.
func TestBuildRuntime_RateLimiterHonorsPerEntryOverride(t *testing.T) {
	testDeps := deps.NewTest()

	override := config.RateLimiterConfig{RequestsPerMinute: 5, MaxErrors: 1, BlockMinutes: 1}
	cfg := &config.GatewayConfig{
		Name:   "override-test",
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Management: config.ManagementConfig{
			Prefix:      "/_",
			RateLimiter: config.RateLimiterConfig{RequestsPerMinute: 1000}, // legacy field says 1000
		},
	}
	cfg.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameRateLimiter, RateLimiter: &override}, // per-entry override says 5
	}

	rt, err := buildRuntime(cfg, testDeps)
	require.NoError(t, err)
	require.NotNil(t, rt.rateLimiter)

	assert.Equal(t, 5, rt.rateLimiter.Config().RequestsPerMinute, "the per-entry override should win over management.rateLimiter")
}

// TestBuildRuntime_RateLimiterFallsBackToManagementConfig covers the
// companion case: no per-entry override present (either no explicit
// middleware: section, or an entry with no rateLimiter: block) should still
// use management.rateLimiter, exactly as before this fix.
func TestBuildRuntime_RateLimiterFallsBackToManagementConfig(t *testing.T) {
	testDeps := deps.NewTest()

	cfg := &config.GatewayConfig{
		Name:   "fallback-test",
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Management: config.ManagementConfig{
			Prefix:      "/_",
			RateLimiter: config.RateLimiterConfig{RequestsPerMinute: 42},
		},
	}

	rt, err := buildRuntime(cfg, testDeps)
	require.NoError(t, err)
	require.NotNil(t, rt.rateLimiter)

	assert.Equal(t, 42, rt.rateLimiter.Config().RequestsPerMinute)
}
