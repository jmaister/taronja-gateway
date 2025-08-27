package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/session"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
)

func TestGetCurrentUser(t *testing.T) {
	// Setup test server with proper repositories
	userRepo := db.NewMemoryUserRepository()
	userLoginRepo := db.NewUserLoginRepositoryMemory(userRepo)
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo)
	trafficMetricRepo := db.NewMemoryTrafficMetricRepository(userRepo)
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	// For tests, we can use a nil database connection since we're using memory repositories
	startTime := time.Now()

	s := handlers.NewStrictApiServer(sessionStore, userRepo, userLoginRepo, trafficMetricRepo, tokenRepo, tokenService, startTime)

	t.Run("AuthenticatedUser", func(t *testing.T) {
		// Setup: Create a test user in the repository
		testUser := &db.User{
			ID:       "1",
			Username: "testuser",
			Email:    "testuser@example.com",
			Name:     "Test User",
			Picture:  "https://example.com/avatar.jpg",
			IsAdmin:  false,
		}
		err := userRepo.CreateUser(testUser)
		assert.NoError(t, err)

		// Setup: Create a valid session
		validSession := &db.Session{
			Token:           "valid-session-id",
			UserID:          "1",
			Username:        "testuser",
			Email:           "testuser@example.com",
			Provider:        "testprovider",
			IsAuthenticated: true,
			IsAdmin:         false, // Regular user
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}
		ctxWithSession := context.WithValue(context.Background(), session.SessionKey, validSession)
		req := api.GetCurrentUserRequestObject{}

		// Execute
		resp, err := s.GetCurrentUser(ctxWithSession, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		userResp, ok := resp.(api.GetCurrentUser200JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser200JSONResponse")
		assert.NotNil(t, userResp.Authenticated)
		assert.True(t, *userResp.Authenticated)
		assert.NotNil(t, userResp.Username)
		assert.Equal(t, "testuser", *userResp.Username)
		assert.NotNil(t, userResp.Email)
		// Ensure comparison is type-consistent, assuming *userResp.Email might be a base string
		// due to the casting in api_me.go.
		assert.Equal(t, openapi_types.Email("testuser@example.com"), openapi_types.Email(*userResp.Email))
		assert.NotNil(t, userResp.Provider)
		assert.Equal(t, "testprovider", *userResp.Provider)
		assert.NotNil(t, userResp.IsAdmin)
		assert.False(t, *userResp.IsAdmin)
		assert.NotNil(t, userResp.Timestamp)
		// Check new fields
		assert.NotNil(t, userResp.Name)
		assert.Equal(t, "Test User", *userResp.Name)
		assert.NotNil(t, userResp.Picture)
		assert.Equal(t, "https://example.com/avatar.jpg", *userResp.Picture)
	})

	t.Run("AuthenticatedAdminUser", func(t *testing.T) {
		// Setup: Create a valid admin session
		adminSession := &db.Session{
			Token:           "admin-session-id",
			UserID:          "admin",
			Username:        "admin",
			Email:           "",
			Provider:        db.AdminProvider,
			IsAuthenticated: true,
			IsAdmin:         true, // Admin user
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}
		ctxWithAdminSession := context.WithValue(context.Background(), session.SessionKey, adminSession)
		req := api.GetCurrentUserRequestObject{}

		// Execute
		resp, err := s.GetCurrentUser(ctxWithAdminSession, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		userResp, ok := resp.(api.GetCurrentUser200JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser200JSONResponse")
		assert.NotNil(t, userResp.Authenticated)
		assert.True(t, *userResp.Authenticated)
		assert.NotNil(t, userResp.Username)
		assert.Equal(t, "admin", *userResp.Username)
		assert.Nil(t, userResp.Email) // Admin user has no email, so email should be nil
		assert.NotNil(t, userResp.Provider)
		assert.Equal(t, db.AdminProvider, *userResp.Provider)
		assert.NotNil(t, userResp.IsAdmin)
		assert.True(t, *userResp.IsAdmin)
		assert.NotNil(t, userResp.Timestamp)
	})

	t.Run("UnauthenticatedUser_NoSessionInContext", func(t *testing.T) {
		// Setup: Context without session
		ctxWithoutSession := context.Background()
		req := api.GetCurrentUserRequestObject{}

		// Execute
		resp, err := s.GetCurrentUser(ctxWithoutSession, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		errResp, ok := resp.(api.GetCurrentUser401JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser401JSONResponse")
		assert.Equal(t, http.StatusUnauthorized, errResp.Code)
		assert.Equal(t, "Unauthorized", errResp.Message)
	})

	t.Run("UnauthenticatedUser_NilSessionInContext", func(t *testing.T) {
		// Setup: Context with nil session
		ctxWithNilSession := context.WithValue(context.Background(), session.SessionKey, (*db.Session)(nil))
		req := api.GetCurrentUserRequestObject{}

		// Execute
		resp, err := s.GetCurrentUser(ctxWithNilSession, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		errResp, ok := resp.(api.GetCurrentUser401JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser401JSONResponse")
		assert.Equal(t, http.StatusUnauthorized, errResp.Code)
		assert.Equal(t, "Unauthorized", errResp.Message)
	})

	t.Run("UnauthenticatedUser_SessionNotAuthenticated", func(t *testing.T) {
		// Setup: Session is not authenticated
		notAuthSession := &db.Session{
			Token:           "not-auth-session-id",
			IsAuthenticated: false, // Key difference
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}
		ctxWithNotAuthSession := context.WithValue(context.Background(), session.SessionKey, notAuthSession)
		req := api.GetCurrentUserRequestObject{}

		// Execute
		resp, err := s.GetCurrentUser(ctxWithNotAuthSession, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		errResp, ok := resp.(api.GetCurrentUser401JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser401JSONResponse")
		assert.Equal(t, http.StatusUnauthorized, errResp.Code)
		assert.Equal(t, "Unauthorized", errResp.Message)
	})

	t.Run("UnauthenticatedUser_SessionExpired", func(t *testing.T) {
		// Setup: Session is expired
		expiredSession := &db.Session{
			Token:           "not-auth-session-id",
			IsAuthenticated: true,
			ValidUntil:      time.Now().Add(-1 * time.Hour), // Expired
		}

		// Execute
		ctxWithExpiredSession := context.WithValue(context.Background(), session.SessionKey, expiredSession)
		req := api.GetCurrentUserRequestObject{}
		resp, err := s.GetCurrentUser(ctxWithExpiredSession, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		errResp, ok := resp.(api.GetCurrentUser401JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser401JSONResponse")
		assert.Equal(t, http.StatusUnauthorized, errResp.Code)
		assert.Equal(t, "Unauthorized", errResp.Message)
	})

	t.Run("UnauthenticatedUser_WrongSessionTypeInContext", func(t *testing.T) {
		// Setup: Context with wrong session type
		ctxWithWrongSessionType := context.WithValue(context.Background(), session.SessionKey, "not-a-session-object")
		req := api.GetCurrentUserRequestObject{}

		// Execute
		resp, err := s.GetCurrentUser(ctxWithWrongSessionType, req)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		errResp, ok := resp.(api.GetCurrentUser401JSONResponse)
		assert.True(t, ok, "Response should be GetCurrentUser401JSONResponse")
		assert.Equal(t, http.StatusUnauthorized, errResp.Code)
		assert.Equal(t, "Unauthorized", errResp.Message)
	})
}
