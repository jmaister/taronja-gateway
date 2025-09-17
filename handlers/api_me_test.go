package handlers_test

import (
	"context"
	"math/big"
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

	"crypto/rand"
)

func RndStr(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		nBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			b[i] = letters[0]
		} else {
			b[i] = letters[nBig.Int64()]
		}
	}
	return string(b)
}

func TestGetCurrentUser(t *testing.T) {
	// Setup isolated test database
	db.SetupTestDB("TestGetCurrentUser")
	testDB := db.GetConnection()

	// Setup test server with database repositories
	userRepo := db.NewDBUserRepository(testDB)
	sessionRepo := db.NewSessionRepositoryDB(testDB)
	sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
	trafficMetricRepo := db.NewTrafficMetricRepository(testDB)
	tokenRepo := db.NewTokenRepositoryDB(testDB)
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	startTime := time.Now()

	creditsRepo := db.NewDBCreditsRepository(testDB)
	s := handlers.NewStrictApiServer(sessionStore, userRepo, trafficMetricRepo, tokenRepo, creditsRepo, tokenService, startTime)

	t.Run("AuthenticatedUser", func(t *testing.T) {
		// Setup: Create a test user in the repository
		rnd := RndStr(5)
		testUser := &db.User{
			Username: "testuser" + rnd,
			Email:    "testuser" + rnd + "@example.com",
			Name:     "Test User",
			Picture:  "https://example.com/avatar.jpg",
			Provider: "testprovider",
		}
		err := userRepo.CreateUser(testUser)
		assert.NoError(t, err)

		// Setup: Create a valid session
		validSession := &db.Session{
			Token:           "valid-session-id",
			UserID:          testUser.ID,
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
		assert.Equal(t, testUser.Username, *userResp.Username)
		assert.NotNil(t, userResp.Email)
		assert.Equal(t, openapi_types.Email(testUser.Email), openapi_types.Email(*userResp.Email))
		assert.NotNil(t, userResp.Provider)
		assert.Equal(t, testUser.Provider, *userResp.Provider)
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
		// Setup: Create a test admin user in the repository
		rnd := time.Now().String()
		testAdmin := &db.User{
			Username: "testuser" + rnd,
			Email:    "",
			Name:     "Test User",
			Picture:  "https://example.com/avatar.jpg",
			Provider: db.AdminProvider,
		}
		err := userRepo.CreateUser(testAdmin)
		assert.NoError(t, err)

		// Setup: Create a valid admin session
		adminSession := &db.Session{
			Token:           "admin-session-id",
			UserID:          testAdmin.ID,
			Username:        testAdmin.Username,
			Email:           "",
			Provider:        db.AdminProvider,
			IsAuthenticated: true,
			IsAdmin:         true,
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
		assert.Equal(t, testAdmin.Username, *userResp.Username)
		// Email type is openapi_types.Email, but empty string should be handled as empty email
		assert.Nil(t, userResp.Email)
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
