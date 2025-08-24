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

func TestTokenHandlersAdmin(t *testing.T) {
	// Setup repositories and services
	userRepo := db.NewMemoryUserRepository()
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)

	// Create test server
	server := &StrictApiServer{
		userRepo:     userRepo,
		tokenRepo:    tokenRepo,
		tokenService: tokenService,
	}

	// Create test user
	testUser := &db.User{
		ID:       "user-123",
		Username: "testuser",
		Email:    "test@example.com",
		Name:     "Test User",
		Provider: "test",
	}
	err := userRepo.CreateUser(testUser)
	require.NoError(t, err)

	// Create admin session
	adminSession := &db.Session{
		Token:           "admin-session-token",
		UserID:          "admin-123",
		Username:        "admin",
		Email:           "admin@example.com",
		IsAuthenticated: true,
		IsAdmin:         true, // Admin required for token operations
		Provider:        "test",
	}

	// Create non-admin session
	nonAdminSession := &db.Session{
		Token:           "nonadmin-session-token",
		UserID:          testUser.ID,
		Username:        testUser.Username,
		Email:           testUser.Email,
		IsAuthenticated: true,
		IsAdmin:         false, // Non-admin
		Provider:        "test",
	}

	t.Run("ListTokens_NoSession", func(t *testing.T) {
		ctx := context.Background()
		request := api.ListTokensRequestObject{
			UserId: testUser.ID,
		}

		response, err := server.ListTokens(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.ListTokens401JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 401, errorResponse.Code)
	})

	t.Run("ListTokens_NonAdmin", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, nonAdminSession)
		request := api.ListTokensRequestObject{
			UserId: testUser.ID,
		}

		response, err := server.ListTokens(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.ListTokens401JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 401, errorResponse.Code)
		assert.Contains(t, errorResponse.Message, "Admin access required")
	})

	t.Run("ListTokens_AdminSuccess", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
		request := api.ListTokensRequestObject{
			UserId: testUser.ID,
		}

		response, err := server.ListTokens(ctx, request)
		require.NoError(t, err)

		successResponse, ok := response.(api.ListTokens200JSONResponse)
		assert.True(t, ok)
		assert.Len(t, []api.TokenResponse(successResponse), 0) // No tokens initially
	})

	t.Run("ListTokens_UserNotFound", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
		request := api.ListTokensRequestObject{
			UserId: "nonexistent-user",
		}

		response, err := server.ListTokens(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.ListTokens404JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 404, errorResponse.Code)
		assert.Contains(t, errorResponse.Message, "User not found")
	})

	t.Run("CreateToken_AdminSuccess", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

		request := api.CreateTokenRequestObject{
			UserId: testUser.ID,
			Body: &api.TokenCreateRequest{
				Name:   "Test Token",
				Scopes: &[]string{"read", "write"},
			},
		}

		response, err := server.CreateToken(ctx, request)
		require.NoError(t, err)

		successResponse, ok := response.(api.CreateToken201JSONResponse)
		assert.True(t, ok)
		assert.NotEmpty(t, successResponse.Token)
		assert.Contains(t, successResponse.Token, "tg_")
		assert.Equal(t, "Test Token", successResponse.TokenInfo.Name)
		assert.True(t, successResponse.TokenInfo.IsActive)
	})

	t.Run("CreateToken_NonAdmin", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, nonAdminSession)

		request := api.CreateTokenRequestObject{
			UserId: testUser.ID,
			Body: &api.TokenCreateRequest{
				Name:   "Test Token",
				Scopes: &[]string{"read", "write"},
			},
		}

		response, err := server.CreateToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.CreateToken401JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 401, errorResponse.Code)
		assert.Contains(t, errorResponse.Message, "Admin access required")
	})

	t.Run("CreateToken_UserNotFound", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

		request := api.CreateTokenRequestObject{
			UserId: "nonexistent-user",
			Body: &api.TokenCreateRequest{
				Name:   "Test Token",
				Scopes: &[]string{"read", "write"},
			},
		}

		response, err := server.CreateToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.CreateToken404JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 404, errorResponse.Code)
		assert.Contains(t, errorResponse.Message, "User not found")
	})

	t.Run("CreateToken_EmptyName", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

		request := api.CreateTokenRequestObject{
			UserId: testUser.ID,
			Body: &api.TokenCreateRequest{
				Name: "",
			},
		}

		response, err := server.CreateToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.CreateToken400JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 400, errorResponse.Code)
	})

	t.Run("CreateToken_ExpiredDate", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)

		pastTime := time.Now().Add(-1 * time.Hour)
		request := api.CreateTokenRequestObject{
			UserId: testUser.ID,
			Body: &api.TokenCreateRequest{
				Name:      "Expired Token",
				ExpiresAt: &pastTime,
			},
		}

		response, err := server.CreateToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.CreateToken400JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 400, errorResponse.Code)
	})

	t.Run("GetToken_AdminSuccess", func(t *testing.T) {
		// First create a token
		tokenString, tokenData, err := tokenService.GenerateToken(
			testUser.ID,
			"Get Test Token",
			nil,
			[]string{"read"},
			"test",
			nil,
		)
		require.NoError(t, err)
		require.NotEmpty(t, tokenString)

		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
		request := api.GetTokenRequestObject{
			TokenId: tokenData.ID,
		}

		response, err := server.GetToken(ctx, request)
		require.NoError(t, err)

		successResponse, ok := response.(api.GetToken200JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, tokenData.ID, successResponse.Id)
		assert.Equal(t, "Get Test Token", successResponse.Name)
	})

	t.Run("GetToken_NonAdmin", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, nonAdminSession)
		request := api.GetTokenRequestObject{
			TokenId: "some-token",
		}

		response, err := server.GetToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.GetToken401JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 401, errorResponse.Code)
		assert.Contains(t, errorResponse.Message, "Admin access required")
	})

	t.Run("GetToken_NotFound", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
		request := api.GetTokenRequestObject{
			TokenId: "nonexistent-token",
		}

		response, err := server.GetToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.GetToken404JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 404, errorResponse.Code)
	})

	t.Run("DeleteToken_AdminSuccess", func(t *testing.T) {
		// First create a token
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
		createRequest := api.CreateTokenRequestObject{
			UserId: testUser.ID,
			Body: &api.TokenCreateRequest{
				Name: "Token to Delete",
			},
		}

		createResponse, err := server.CreateToken(ctx, createRequest)
		require.NoError(t, err)

		createSuccessResponse, ok := createResponse.(api.CreateToken201JSONResponse)
		require.True(t, ok)
		tokenID := createSuccessResponse.TokenInfo.Id

		// Now delete the token
		deleteRequest := api.DeleteTokenRequestObject{
			TokenId: tokenID,
		}

		deleteResponse, err := server.DeleteToken(ctx, deleteRequest)
		require.NoError(t, err)

		_, ok = deleteResponse.(api.DeleteToken204Response)
		assert.True(t, ok)

		// Verify token is no longer active
		tokens, err := server.tokenService.GetUserTokens(testUser.ID)
		require.NoError(t, err)
		for _, token := range tokens {
			if token.ID == tokenID {
				assert.False(t, token.IsActive, "Deleted token should not be active")
			}
		}
	})

	t.Run("DeleteToken_NonAdmin", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, nonAdminSession)
		request := api.DeleteTokenRequestObject{
			TokenId: "some-token",
		}

		response, err := server.DeleteToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.DeleteToken401JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 401, errorResponse.Code)
		assert.Contains(t, errorResponse.Message, "Admin access required")
	})

	t.Run("DeleteToken_NotFound", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, adminSession)
		request := api.DeleteTokenRequestObject{
			TokenId: "nonexistent-token",
		}

		response, err := server.DeleteToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.DeleteToken404JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 404, errorResponse.Code)
	})
}
