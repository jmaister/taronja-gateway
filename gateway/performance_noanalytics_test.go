package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/static"
)

// BenchmarkStaticRequestNoAnalytics benchmarks static requests without analytics middleware
func BenchmarkStaticRequestNoAnalytics(b *testing.B) {
	cfg := createTestConfigNoAnalytics()
	gw, err := NewGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create test request for static content
	req := httptest.NewRequest("GET", "/_/static/style.css", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)

		// Static files might return 404 if not found, that's ok for performance testing
		if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
			b.Errorf("Expected status 200 or 404, got %d", rr.Code)
		}
	}
}

// createTestConfigNoAnalytics creates a configuration with analytics disabled
func createTestConfigNoAnalytics() *config.GatewayConfig {
	return &config.GatewayConfig{
		Name: "test-gateway",
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Management: config.ManagementConfig{
			Prefix:    "/_",
			Analytics: false, // Disabled analytics
			Logging:   false, // Disabled logging
			Admin: config.AdminConfig{
				Enabled:  false,
				Username: "admin",
				Email:    "admin@example.com",
				Password: "password",
			},
		},
		AuthenticationProviders: config.AuthenticationProviders{
			Basic: config.BasicAuthenticationConfig{
				Enabled: false,
			},
			Google: config.AuthProviderCredentials{
				ClientId:     "",
				ClientSecret: "",
			},
			Github: config.AuthProviderCredentials{
				ClientId:     "",
				ClientSecret: "",
			},
		},
		Routes: []config.RouteConfig{
			{
				Name:   "test-api",
				From:   "/api/*",
				To:     "http://localhost:3000",
				Static: false,
				Authentication: config.AuthenticationConfig{
					Enabled: false,
				},
			},
		},
	}
}
