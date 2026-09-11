package middleware

import (
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCORSMiddleware_DisabledSkipsValidation(t *testing.T) {
	cfg := &config.GatewayConfig{}
	assert.NoError(t, ValidateCORSMiddleware(nil, cfg))
}

func TestValidateCORSMiddleware_WildcardWithCredentialsRejected(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Management.CORS = config.CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}

	err := ValidateCORSMiddleware(nil, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowCredentials")
}

func TestValidateCORSMiddleware_ExplicitOriginsWithCredentialsAccepted(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Management.CORS = config.CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
	}

	assert.NoError(t, ValidateCORSMiddleware(nil, cfg))
}

func TestValidateCORSMiddleware_NegativeMaxAgeRejected(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Management.CORS = config.CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		MaxAgeSeconds:  -1,
	}

	err := ValidateCORSMiddleware(nil, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxAgeSeconds")
}

func TestValidateConfigOnly_ValidConfigPasses(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Management.Prefix = "/_"
	assert.NoError(t, ValidateConfigOnly(cfg))
}

func TestValidateConfigOnly_CatchesInvalidCORS(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Management.CORS = config.CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}

	err := ValidateConfigOnly(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowCredentials")
}

func TestValidateConfigOnly_CatchesBadMiddlewareDependencyGraph(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameSessionExtraction}, // missing ja4_fingerprint
	}

	err := ValidateConfigOnly(cfg)
	require.Error(t, err)
}

func TestValidateConfigOnly_DoesNotPanicWithNilDeps(t *testing.T) {
	// ValidateConfigOnly must never dereference deps — every check it calls
	// is passed nil explicitly. This just confirms none of them panic.
	cfg := &config.GatewayConfig{}
	cfg.Management.Admin.Enabled = false
	assert.NotPanics(t, func() { _ = ValidateConfigOnly(cfg) })
}
