package session

import "testing"

func TestSanitizeRedirectPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty falls back to root", "", "/"},
		{"ordinary relative path is kept", "/dashboard", "/dashboard"},
		{"relative path with query string is kept", "/dashboard?tab=settings", "/dashboard?tab=settings"},
		{"absolute URL is rejected", "https://evil.example/phish", "/"},
		{"protocol-relative URL is rejected", "//evil.example/phish", "/"},
		{"backslash-leading value is rejected", "/\\evil.example", "/"},
		{"scheme-relative with uppercase scheme is rejected", "HTTPS://evil.example", "/"},
		{"no leading slash at all is rejected", "dashboard", "/"},
		{"javascript scheme is rejected", "javascript:alert(1)", "/"},
		{"bare double-slash is rejected", "//", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeRedirectPath(tt.raw)
			if got != tt.want {
				t.Fatalf("SanitizeRedirectPath(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
