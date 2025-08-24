package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLogoutTestServer() (*StrictApiServer, db.SessionRepository) {
	userRepo := db.NewMemoryUserRepository()
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo)
	trafficMetricRepo := db.NewMemoryTrafficMetricRepository(userRepo)
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	// For tests, we can use a nil database connection since we're using memory repositories
	startTime := time.Now()

	return NewStrictApiServer(sessionStore, userRepo, trafficMetricRepo, tokenRepo, tokenService, startTime), sessionRepo
}

func TestLogoutUser(t *testing.T) {
	t.Run("LogoutWithValidSession", func(t *testing.T) {
		s, sessionRepo := setupLogoutTestServer()
		ctx := context.Background()

		// Create a valid session in the repository
		sessionToken := "valid-session-token"
		testSession := &db.Session{
			UserID:          "user123",
			Username:        "testuser",
			Email:           "test@example.com",
			Provider:        "test-provider",
			IsAuthenticated: true,
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}

		err := sessionRepo.CreateSession(sessionToken, testSession)
		require.NoError(t, err)

		// Verify session exists before logout
		_, err = sessionRepo.FindSessionByToken(sessionToken)
		assert.NoError(t, err, "Session should exist before logout")

		// Create request with session token
		req := api.LogoutUserRequestObject{
			Params: api.LogoutUserParams{
				TgSessionToken: &sessionToken,
			},
		}

		// Execute logout
		resp, err := s.LogoutUser(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response is redirect with correct headers
		logoutResp, ok := resp.(api.LogoutUser302Response)
		assert.True(t, ok, "Response should be LogoutUser302Response")

		// Check redirect location defaults to "/"
		assert.Equal(t, "/", logoutResp.Headers.Location)

		// Check cache control header
		assert.Equal(t, "no-store, no-cache, must-revalidate, post-check=0, pre-check=0", logoutResp.Headers.CacheControl)

		// Check that Set-Cookie header is present to clear the session cookie
		assert.NotEmpty(t, logoutResp.Headers.SetCookie)
		assert.Contains(t, logoutResp.Headers.SetCookie, session.SessionCookieName)
		assert.Contains(t, logoutResp.Headers.SetCookie, "Max-Age=0")

		// Verify session was deleted from store
		foundSession, err := sessionRepo.FindSessionByToken(sessionToken)
		assert.Error(t, err, "Session should be closed after logout")
		assert.Nil(t, foundSession, "Session should be nil after logout")
	})

	t.Run("LogoutWithCustomRedirectURL", func(t *testing.T) {
		s, sessionRepo := setupLogoutTestServer()
		ctx := context.Background()

		// Create a valid session in the repository
		sessionToken := "valid-session-token-2"
		testSession := &db.Session{
			UserID:          "user456",
			Username:        "testuser2",
			Email:           "test2@example.com",
			Provider:        "test-provider",
			IsAuthenticated: true,
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}

		err := sessionRepo.CreateSession(sessionToken, testSession)
		require.NoError(t, err)

		// Create request with session token and custom redirect
		redirectURL := "/custom-redirect"
		req := api.LogoutUserRequestObject{
			Params: api.LogoutUserParams{
				TgSessionToken: &sessionToken,
				Redirect:       &redirectURL,
			},
		}

		// Execute logout
		resp, err := s.LogoutUser(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response is redirect with custom URL
		logoutResp, ok := resp.(api.LogoutUser302Response)
		assert.True(t, ok, "Response should be LogoutUser302Response")

		// Check redirect location is custom URL
		assert.Equal(t, "/custom-redirect", logoutResp.Headers.Location)

		// Verify session was deleted from store
		foundSession, err := sessionRepo.FindSessionByToken(sessionToken)
		assert.Error(t, err, "Session should be closed after logout")
		assert.Nil(t, foundSession, "Session should be nil after logout")
	})

	t.Run("LogoutWithNoSession", func(t *testing.T) {
		s, _ := setupLogoutTestServer()
		ctx := context.Background()

		// Create request without session token
		req := api.LogoutUserRequestObject{
			Params: api.LogoutUserParams{
				TgSessionToken: nil,
			},
		}

		// Execute logout
		resp, err := s.LogoutUser(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response is redirect with correct headers
		logoutResp, ok := resp.(api.LogoutUser302Response)
		assert.True(t, ok, "Response should be LogoutUser302Response")

		// Check redirect location defaults to "/"
		assert.Equal(t, "/", logoutResp.Headers.Location)

		// Check cache control header
		assert.Equal(t, "no-store, no-cache, must-revalidate, post-check=0, pre-check=0", logoutResp.Headers.CacheControl)

		// Check that Set-Cookie header is present to clear the session cookie
		assert.NotEmpty(t, logoutResp.Headers.SetCookie)
		assert.Contains(t, logoutResp.Headers.SetCookie, session.SessionCookieName)
		assert.Contains(t, logoutResp.Headers.SetCookie, "Max-Age=0")
	})

	t.Run("LogoutWithEmptySessionToken", func(t *testing.T) {
		s, _ := setupLogoutTestServer()
		ctx := context.Background()

		// Create request with empty session token
		emptyToken := ""
		req := api.LogoutUserRequestObject{
			Params: api.LogoutUserParams{
				TgSessionToken: &emptyToken,
			},
		}

		// Execute logout
		resp, err := s.LogoutUser(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response is redirect with correct headers
		logoutResp, ok := resp.(api.LogoutUser302Response)
		assert.True(t, ok, "Response should be LogoutUser302Response")

		// Check redirect location defaults to "/"
		assert.Equal(t, "/", logoutResp.Headers.Location)

		// Check that Set-Cookie header is present to clear the session cookie
		assert.NotEmpty(t, logoutResp.Headers.SetCookie)
		assert.Contains(t, logoutResp.Headers.SetCookie, session.SessionCookieName)
	})

	t.Run("LogoutWithInvalidSessionToken", func(t *testing.T) {
		s, sessionRepo := setupLogoutTestServer()
		ctx := context.Background()

		// Use a session token that doesn't exist in the repository
		invalidToken := "invalid-session-token"
		req := api.LogoutUserRequestObject{
			Params: api.LogoutUserParams{
				TgSessionToken: &invalidToken,
			},
		}

		// Execute logout
		resp, err := s.LogoutUser(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response is redirect (logout should proceed even if session doesn't exist)
		logoutResp, ok := resp.(api.LogoutUser302Response)
		assert.True(t, ok, "Response should be LogoutUser302Response")

		// Check redirect location defaults to "/"
		assert.Equal(t, "/", logoutResp.Headers.Location)

		// Check that Set-Cookie header is present to clear the session cookie
		assert.NotEmpty(t, logoutResp.Headers.SetCookie)
		assert.Contains(t, logoutResp.Headers.SetCookie, session.SessionCookieName)

		// Verify the invalid token still doesn't exist (nothing should have been created)
		foundSession, err := sessionRepo.FindSessionByToken(invalidToken)
		assert.NoError(t, err)
		assert.Nil(t, foundSession, "Invalid session should not exist")
	})

	t.Run("LogoutWithEmptyRedirectURL", func(t *testing.T) {
		s, _ := setupLogoutTestServer()
		ctx := context.Background()

		// Create request with empty redirect URL
		emptyRedirect := ""
		req := api.LogoutUserRequestObject{
			Params: api.LogoutUserParams{
				TgSessionToken: nil,
				Redirect:       &emptyRedirect,
			},
		}

		// Execute logout
		resp, err := s.LogoutUser(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response is redirect with default URL
		logoutResp, ok := resp.(api.LogoutUser302Response)
		assert.True(t, ok, "Response should be LogoutUser302Response")

		// Check redirect location defaults to "/" when empty
		assert.Equal(t, "/", logoutResp.Headers.Location)
	})
}
