package main

import (
	"strings"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway"
)

func TestNewGateway(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.GatewayConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "Valid config",
			config: &config.GatewayConfig{
				Server: config.ServerConfig{
					Host: "localhost",
					Port: 8080,
				},
				Management: config.ManagementConfig{
					Prefix: "/admin",
				},
				Routes: []config.RouteConfig{
					{
						Name: "Test Route",
						From: "/",
						To:   "http://localhost:8081",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateway.NewGateway(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("NewGateway() error = nil, wantErr true")
					return
				}
				if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("NewGateway() error = %v, want error containing %v", err, tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("NewGateway() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got == nil {
				t.Error("NewGateway() returned nil gateway")
				return
			}

			// Check that the gateway was properly initialized
			if got.GatewayConfig != tt.config {
				t.Error("NewGateway() gateway.config not properly set")
			}
			if got.Mux == nil {
				t.Error("NewGateway() gateway.mux is nil")
			}
			if got.Server == nil {
				t.Error("NewGateway() gateway.server is nil")
			}
			if got.SessionStore == nil {
				t.Error("NewGateway() gateway.sessionStore is nil")
			}

			// Check server configuration
			if got.Server.Addr != "localhost:8080" {
				t.Errorf("NewGateway() server.Addr = %v, want %v", got.Server.Addr, "localhost:8080")
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return substr != "" && s != substr && s != "" && strings.Contains(s, substr)
}
