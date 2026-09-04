package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_TrustedProxiesDefaultsToEmpty(t *testing.T) {
	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8080\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Server.TrustedProxies, "omitting the key must leave nothing trusted — secure by default")
}

func TestLoadConfig_TrustedProxiesAcceptsCIDRAndBareIPs(t *testing.T) {
	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8080\n  trustedProxies:\n    - 10.0.0.0/8\n    - 127.0.0.1\n    - ::1\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "127.0.0.1", "::1"}, cfg.Server.TrustedProxies)
}

func TestLoadConfig_TrustedProxiesRejectsInvalidEntry(t *testing.T) {
	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8080\n  trustedProxies:\n    - not-an-ip-or-cidr\n")
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.trustedProxies")
	assert.Contains(t, err.Error(), "not-an-ip-or-cidr")
}

func TestIsValidIPOrCIDR(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.0/8", true},
		{"::1", true},
		{"fd00::/8", true},
		{"not-an-ip", false},
		{"", false},
		{"999.999.999.999", false},
		{"10.0.0.0/99", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidIPOrCIDR(tt.input))
		})
	}
}
