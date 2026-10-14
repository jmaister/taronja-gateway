package builtin

import (
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfigOnly_ValidConfigPasses(t *testing.T) {
	cfg := &config.GatewayConfig{Server: config.ServerConfig{Port: 8080}}
	assert.NoError(t, ValidateConfigOnly(cfg))
}

func TestValidateConfigOnly_CatchesInvalidMiddlewareOptions(t *testing.T) {
	cfg := &config.GatewayConfig{Server: config.ServerConfig{Port: 8080}, Middleware: []config.MiddlewareConfig{
		config.NewMiddlewareConfig(NameCORS, CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: true}),
	}}
	err := ValidateConfigOnly(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowCredentials")
}

func TestValidateConfigOnly_CatchesBadDependencyGraph(t *testing.T) {
	cfg := &config.GatewayConfig{Server: config.ServerConfig{Port: 8080}, Middleware: []config.MiddlewareConfig{config.NewMiddlewareConfig(NameSessionExtraction, nil)}}
	assert.Error(t, ValidateConfigOnly(cfg))
}

func TestCORSConfig_Validate(t *testing.T) {
	assert.NoError(t, CORSConfig{}.Validate())
	assert.NoError(t, CORSConfig{AllowedOrigins: []string{"https://example.com"}, AllowCredentials: true}.Validate())
	assert.ErrorContains(t, CORSConfig{AllowedOrigins: []string{"https://example.com"}, MaxAgeSeconds: -1}.Validate(), "maxAgeSeconds")
}

func TestCORSConfig_IsEnabledAndAllowsAnyOrigin(t *testing.T) {
	assert.False(t, CORSConfig{}.IsEnabled())
	assert.True(t, CORSConfig{AllowedOrigins: []string{"https://a"}}.IsEnabled())
	assert.False(t, CORSConfig{AllowedOrigins: []string{"https://a"}}.AllowsAnyOrigin())
	assert.True(t, CORSConfig{AllowedOrigins: []string{"https://a", "*"}}.AllowsAnyOrigin())
}

func TestRateLimiterConfig_Validate(t *testing.T) {
	assert.NoError(t, RateLimiterConfig{}.Validate(), "all zero means disabled")
	assert.NoError(t, RateLimiterConfig{RequestsPerMinute: 10, BlockMinutes: 1}.Validate())
	assert.ErrorContains(t, RateLimiterConfig{RequestsPerMinute: -1}.Validate(), "requestsPerMinute")
	assert.ErrorContains(t, RateLimiterConfig{MaxErrors: 3}.Validate(), "blockMinutes")
	assert.ErrorContains(t, RateLimiterConfig{BlockMinutes: 1, VulnerabilityScan: VulnerabilityScanConfig{Max404: 2, BlockMinutes: 1}}.Validate(), "urls")
	assert.ErrorContains(t, RateLimiterConfig{BlockMinutes: 1, VulnerabilityScan: VulnerabilityScanConfig{Max404: 2, URLs: []string{"/x"}}}.Validate(), "vulnerabilityScan.blockMinutes")
}

func TestTracingConfig_Validate(t *testing.T) {
	assert.ErrorContains(t, TracingConfig{}.Validate(), "endpoint")
	assert.NoError(t, TracingConfig{Endpoint: "localhost:4318"}.Validate())
}
