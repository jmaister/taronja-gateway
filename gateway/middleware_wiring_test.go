package gateway

import (
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shared rate limiter is built outside the chain, so it must read the
// same middleware options the chain uses.
func TestBuildRuntime_RateLimiterUsesTheMiddlewareOptions(t *testing.T) {
	cfg := &config.GatewayConfig{
		Name:       "rate-limiter-test",
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Management: config.ManagementConfig{},
		Middleware: []config.MiddlewareConfig{
			config.NewMiddlewareConfig(builtin.NameRateLimiter, builtin.RateLimiterConfig{RequestsPerMinute: 5, BlockMinutes: 1}),
		},
	}

	rt, err := buildRuntime(cfg, deps.NewTest())
	require.NoError(t, err)
	require.NotNil(t, rt.rateLimiter)
	assert.Equal(t, 5, rt.rateLimiter.Config().RequestsPerMinute)
	assert.True(t, rt.middlewareChain.Enabled(builtin.NameRateLimiter))
}

func TestBuildRuntime_AnalyticsMiddlewareGetTheirDependencies(t *testing.T) {
	cfg := &config.GatewayConfig{
		Name:       "analytics-test",
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Management: config.ManagementConfig{},
		Middleware: []config.MiddlewareConfig{
			config.NewMiddlewareConfig(builtin.NameJA4Fingerprint, nil),
			config.NewMiddlewareConfig(builtin.NameSessionExtraction, nil),
			config.NewMiddlewareConfig(builtin.NameTrafficMetrics, nil),
		},
	}

	rt, err := buildRuntime(cfg, deps.NewTest())
	require.NoError(t, err)
	assert.True(t, rt.middlewareChain.Enabled(builtin.NameTrafficMetrics))
}
