package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test cases for improved Authorization header handling
func TestAuthorizationHeaderEdgeCases(t *testing.T) {
	// Setup test database
	db.ResetConnection()
	db.SetupTestDB("TestAuthorizationHeaderEdgeCases")
	defer db.ResetConnection()

	sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
	sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
	tokenService := createMockTokenService()

	tests := []struct {
		name         string
		authHeader   string
		expectAuth   bool
		expectMethod string
	}{
		{
			name:         "Valid Bearer token",
			authHeader:   "Bearer valid-bearer-token",
			expectAuth:   true,
			expectMethod: "token",
		},
		{
			name:         "Bearer token with leading space",
			authHeader:   " Bearer valid-bearer-token",
			expectAuth:   true,
			expectMethod: "token",
		},
		{
			name:         "Bearer token with trailing space",
			authHeader:   "Bearer valid-bearer-token ",
			expectAuth:   true,
			expectMethod: "token",
		},
		{
			name:         "Bearer token with spaces around token",
			authHeader:   "Bearer  valid-bearer-token  ",
			expectAuth:   true,
			expectMethod: "token",
		},
		{
			name:         "Bearer with lowercase",
			authHeader:   "bearer valid-bearer-token",
			expectAuth:   true,
			expectMethod: "token",
		},
		{
			name:         "Bearer with mixed case",
			authHeader:   "BeArEr valid-bearer-token",
			expectAuth:   true,
			expectMethod: "token",
		},
		{
			name:         "Just Bearer with no token",
			authHeader:   "Bearer",
			expectAuth:   false,
			expectMethod: "",
		},
		{
			name:         "Bearer with only spaces",
			authHeader:   "Bearer   ",
			expectAuth:   false,
			expectMethod: "",
		},
		{
			name:         "Empty Authorization header",
			authHeader:   "",
			expectAuth:   false,
			expectMethod: "",
		},
		{
			name:         "Only spaces in Authorization header",
			authHeader:   "   ",
			expectAuth:   false,
			expectMethod: "",
		},
		{
			name:         "Basic auth instead of Bearer",
			authHeader:   "Basic dXNlcjpwYXNz",
			expectAuth:   false,
			expectMethod: "",
		},
		{
			name:         "Invalid format",
			authHeader:   "InvalidFormat",
			expectAuth:   false,
			expectMethod: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			result := ValidateSessionFromRequest(req, sessionStore, tokenService)

			assert.Equal(t, tt.expectAuth, result.IsAuthenticated,
				"Authentication result mismatch for header: '%s'", tt.authHeader)
			assert.Equal(t, tt.expectMethod, result.AuthMethod,
				"Auth method mismatch for header: '%s'", tt.authHeader)

			if tt.expectAuth {
				require.NotNil(t, result.Session, "Session should not be nil for valid auth")
			}
		})
	}
}
