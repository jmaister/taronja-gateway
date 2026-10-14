package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
)

func TestRequestHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"[::1]:8080", "::1"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = tt.host
		assert.Equal(t, tt.want, requestHost(req), "Host: %q", tt.host)
	}
}

func TestAllowedRedirectHosts(t *testing.T) {
	t.Run("server.url's hostname, lowercased", func(t *testing.T) {
		cfg := &config.GatewayConfig{}
		cfg.Server.URL = "https://Example.com:8443/base"
		assert.Equal(t, map[string]bool{"example.com": true}, allowedRedirectHosts(cfg))
	})

	t.Run("empty when server.url is unset", func(t *testing.T) {
		assert.Empty(t, allowedRedirectHosts(&config.GatewayConfig{}))
	})
}
