package main

import (
	"strings"
	"testing"
)

func TestNewGateway(t *testing.T) {
	tests := []struct {
		name      string
		config    *GatewayConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "Valid config",
			config: &GatewayConfig{
				Server: ServerConfig{
					Host: "localhost",
					Port: 8080,
				},
				Management: ManagementConfig{
					Prefix: "/admin",
				},
				Routes: []RouteConfig{
					{
						Name: "Test Route",
						From: "/",
						To:   "http://localhost:8081",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Error configuring user routes",
			config: &GatewayConfig{
				Server: ServerConfig{
					Host: "localhost",
					Port: 8080,
				},
				Management: ManagementConfig{
					Prefix: "/admin",
				},
				Routes: []RouteConfig{
					{
						Name:   "Invalid Route",
						From:   "/",
						Static: true,
						// Missing ToFolder which should cause an error
					},
				},
			},
			wantErr:   true,
			errSubstr: "error configuring user routes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGateway(tt.config)

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
			if got.config != tt.config {
				t.Error("NewGateway() gateway.config not properly set")
			}
			if got.mux == nil {
				t.Error("NewGateway() gateway.mux is nil")
			}
			if got.server == nil {
				t.Error("NewGateway() gateway.server is nil")
			}
			if got.sessionStore == nil {
				t.Error("NewGateway() gateway.sessionStore is nil")
			}
			if got.authManager == nil {
				t.Error("NewGateway() gateway.authManager is nil")
			}

			// Check server configuration
			if got.server.Addr != "localhost:8080" {
				t.Errorf("NewGateway() server.Addr = %v, want %v", got.server.Addr, "localhost:8080")
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return substr != "" && s != substr && s != "" && strings.Contains(s, substr)
}
