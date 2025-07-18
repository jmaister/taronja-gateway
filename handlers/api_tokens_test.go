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

func TestTokenHandlers(t *testing.T) {
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

	// Create test session
	testSession := &db.Session{
		Token:           "session-token",
		UserID:          testUser.ID,
		Username:        testUser.Username,
		Email:           testUser.Email,
		IsAuthenticated: true,
		IsAdmin:         false,
		Provider:        "test",
	}

	t.Run("ListTokens_NoSession", func(t *testing.T) {
		ctx := context.Background()
		request := api.ListTokensRequestObject{}

		response, err := server.ListTokens(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.ListTokens401JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 401, errorResponse.Code)
	})

	t.Run("ListTokens_WithSession", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, testSession)
		request := api.ListTokensRequestObject{}

		response, err := server.ListTokens(ctx, request)
		require.NoError(t, err)

		successResponse, ok := response.(api.ListTokens200JSONResponse)
		assert.True(t, ok)
		assert.Len(t, []api.TokenResponse(successResponse), 0) // No tokens initially
	})

	t.Run("CreateToken_Success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, testSession)

		request := api.CreateTokenRequestObject{
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

	t.Run("CreateToken_EmptyName", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, testSession)

		request := api.CreateTokenRequestObject{
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
		ctx := context.WithValue(context.Background(), session.SessionKey, testSession)

		pastTime := time.Now().Add(-1 * time.Hour)
		request := api.CreateTokenRequestObject{
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

	t.Run("GetToken_Success", func(t *testing.T) {
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

		ctx := context.WithValue(context.Background(), session.SessionKey, testSession)
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

	t.Run("GetToken_NotFound", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), session.SessionKey, testSession)
		request := api.GetTokenRequestObject{
			TokenId: "nonexistent-token",
		}

		response, err := server.GetToken(ctx, request)
		require.NoError(t, err)

		errorResponse, ok := response.(api.GetToken404JSONResponse)
		assert.True(t, ok)
		assert.Equal(t, 404, errorResponse.Code)
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("parseScopes", func(t *testing.T) {
		// Test empty string
		scopes := parseScopes("")
		assert.Len(t, scopes, 0)

		// Test valid scopes
		scopes = parseScopes("[read,write,admin]")
		assert.Len(t, scopes, 3)
		assert.Contains(t, scopes, "read")
		assert.Contains(t, scopes, "write")
		assert.Contains(t, scopes, "admin")

		// Test empty brackets
		scopes = parseScopes("[]")
		assert.Len(t, scopes, 0)

		// Test single scope
		scopes = parseScopes("[read]")
		assert.Len(t, scopes, 1)
		assert.Equal(t, "read", scopes[0])

		// Test invalid format
		scopes = parseScopes("read,write")
		assert.Len(t, scopes, 0)

		// Test scopes with spaces
		scopes = parseScopes("[read , write , admin]")
		assert.Len(t, scopes, 3)
		assert.Contains(t, scopes, "read")
		assert.Contains(t, scopes, "write")
		assert.Contains(t, scopes, "admin")
	})

	t.Run("convertTokenToResponse", func(t *testing.T) {
		now := time.Now()
		future := now.Add(1 * time.Hour)
		past := now.Add(-1 * time.Hour)

		token := &db.Token{
			ID:         "token-123",
			Name:       "Test Token",
			IsActive:   true,
			ExpiresAt:  &future,
			LastUsedAt: &past,
			UsageCount: 5,
			Scopes:     "[read,write]",
		}
		token.CreatedAt = now

		response := convertTokenToResponse(token)

		assert.Equal(t, "token-123", response.Id)
		assert.Equal(t, "Test Token", response.Name)
		assert.True(t, response.IsActive)
		assert.Equal(t, now, response.CreatedAt)
		assert.Equal(t, &future, response.ExpiresAt)
		assert.Equal(t, &past, response.LastUsedAt)
		assert.Equal(t, 5, response.UsageCount)
		assert.Len(t, response.Scopes, 2)
		assert.Contains(t, response.Scopes, "read")
		assert.Contains(t, response.Scopes, "write")
	})
}
