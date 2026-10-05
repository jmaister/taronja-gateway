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
// same filter options the chain uses.
func TestBuildRuntime_RateLimiterUsesTheFilterOptions(t *testing.T) {
	cfg := &config.GatewayConfig{
		Name:       "rate-limiter-test",
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Management: config.ManagementConfig{Prefix: "/_"},
		Filters: []config.FilterConfig{
			config.NewFilterConfig(builtin.FilterRateLimiter, builtin.RateLimiterConfig{RequestsPerMinute: 5, BlockMinutes: 1}),
		},
	}

	rt, err := buildRuntime(cfg, deps.NewTest())
	require.NoError(t, err)
	require.NotNil(t, rt.rateLimiter)
	assert.Equal(t, 5, rt.rateLimiter.Config().RequestsPerMinute)
	assert.True(t, rt.registry.GetStatus()[builtin.FilterRateLimiter].Enabled)
}

func TestBuildRuntime_AnalyticsFiltersGetTheirDependencies(t *testing.T) {
	cfg := &config.GatewayConfig{
		Name:       "analytics-test",
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 8080},
		Management: config.ManagementConfig{Prefix: "/_"},
		Filters: []config.FilterConfig{
			config.NewFilterConfig(builtin.FilterJA4Fingerprint, nil),
			config.NewFilterConfig(builtin.FilterSessionExtraction, nil),
			config.NewFilterConfig(builtin.FilterTrafficMetrics, nil),
		},
	}

	rt, err := buildRuntime(cfg, deps.NewTest())
	require.NoError(t, err)
	assert.True(t, rt.registry.GetStatus()[builtin.FilterTrafficMetrics].Enabled)
}
