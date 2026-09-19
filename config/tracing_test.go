package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_TracingDisabledByDefault(t *testing.T) {
	path := writeTestConfig(t, "")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.False(t, cfg.Tracing.Enabled, "tracing must default to disabled when the section is omitted entirely")
}

func TestLoadConfig_TracingEnabledRequiresEndpoint(t *testing.T) {
	path := writeTestConfig(t, "tracing:\n  enabled: true\n")
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tracing.endpoint")
}

func TestLoadConfig_TracingEnabledWithEndpointSucceeds(t *testing.T) {
	path := writeTestConfig(t, "tracing:\n  enabled: true\n  endpoint: localhost:4318\n  insecure: true\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.True(t, cfg.Tracing.Enabled)
	assert.Equal(t, "localhost:4318", cfg.Tracing.Endpoint)
	assert.True(t, cfg.Tracing.Insecure)
}

func TestLoadConfig_TracingDisabledIgnoresMissingEndpoint(t *testing.T) {
	// enabled: false (or omitted) never needs an endpoint, regardless of
	// what else is (or isn't) set under tracing:.
	path := writeTestConfig(t, "tracing:\n  enabled: false\n")
	_, err := LoadConfig(path)
	require.NoError(t, err)
}
