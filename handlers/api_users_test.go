package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer() *StrictApiServer {
	userRepo := db.NewMemoryUserRepository()
	sessionRepo := db.NewMemorySessionRepository()
	sessionStore := session.NewSessionStore(sessionRepo)

	return NewStrictApiServer(sessionStore, userRepo, nil)
}

func TestCreateUser(t *testing.T) {
	s := setupTestServer()
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		userRequest := api.CreateUserJSONRequestBody{
			Username: "testuser",
			Email:    openapi_types.Email("testuser@example.com"),
			Password: "password123",
		}
		req := api.CreateUserRequestObject{
			Body: &userRequest,
		}

		resp, err := s.CreateUser(ctx, req)
		require.NoError(t, err)

		createUserResp, ok := resp.(api.CreateUser201JSONResponse)
		require.True(t, ok, "Expected CreateUser201JSONResponse")
		assert.Equal(t, "testuser", createUserResp.Username)
		require.NotNil(t, createUserResp.Email)
		assert.Equal(t, openapi_types.Email("testuser@example.com"), *createUserResp.Email)
		assert.NotEmpty(t, createUserResp.Id)
		//assert.NotZero(t, createUserResp.CreatedAt)
		//assert.NotZero(t, createUserResp.UpdatedAt)

		// Verify user was actually created in the repo
		createdUser, dbErr := s.userRepo.FindUserByIdOrUsername(createUserResp.Id, "", "")
		require.NoError(t, dbErr)
		require.NotNil(t, createdUser)
		assert.Equal(t, "testuser", createdUser.Username)
	})

	t.Run("MissingUsername", func(t *testing.T) {
		userRequest := api.CreateUserJSONRequestBody{
			Email:    openapi_types.Email("nousername@example.com"),
			Password: "password123",
		}
		req := api.CreateUserRequestObject{
			Body: &userRequest,
		}

		resp, err := s.CreateUser(ctx, req)
		require.NoError(t, err)
		errResp, ok := resp.(api.CreateUser400JSONResponse)
		require.True(t, ok, "Expected CreateUser400JSONResponse")
		assert.Equal(t, http.StatusBadRequest, errResp.Code)
		assert.Contains(t, errResp.Message, "Username, email, and password are required")
	})

	t.Run("MissingEmail", func(t *testing.T) {
		userRequest := api.CreateUserJSONRequestBody{
			Username: "noemailuser",
			Password: "password123",
		}
		req := api.CreateUserRequestObject{
			Body: &userRequest,
		}

		resp, err := s.CreateUser(ctx, req)
		require.NoError(t, err)
		errResp, ok := resp.(api.CreateUser400JSONResponse)
		require.True(t, ok, "Expected CreateUser400JSONResponse")
		assert.Equal(t, http.StatusBadRequest, errResp.Code)
		assert.Contains(t, errResp.Message, "Username, email, and password are required")
	})

	t.Run("MissingPassword", func(t *testing.T) {
		userRequest := api.CreateUserJSONRequestBody{
			Username: "nopassworduser",
			Email:    openapi_types.Email("nopassword@example.com"),
		}
		req := api.CreateUserRequestObject{
			Body: &userRequest,
		}

		resp, err := s.CreateUser(ctx, req)
		require.NoError(t, err)
		errResp, ok := resp.(api.CreateUser400JSONResponse)
		require.True(t, ok, "Expected CreateUser400JSONResponse")
		assert.Equal(t, http.StatusBadRequest, errResp.Code)
		assert.Contains(t, errResp.Message, "Username, email, and password are required")
	})

	t.Run("ConflictUsername", func(t *testing.T) {
		// First, create a user
		initialUserRequest := api.CreateUserJSONRequestBody{
			Username: "conflictuser",
			Email:    openapi_types.Email("conflict1@example.com"),
			Password: "password123",
		}
		initialReq := api.CreateUserRequestObject{Body: &initialUserRequest}
		_, err := s.CreateUser(ctx, initialReq)
		require.NoError(t, err)

		// Attempt to create another user with the same username
		conflictUserRequest := api.CreateUserJSONRequestBody{
			Username: "conflictuser", // Same username
			Email:    openapi_types.Email("conflict2@example.com"),
			Password: "password456",
		}
		conflictReq := api.CreateUserRequestObject{Body: &conflictUserRequest}
		resp, err := s.CreateUser(ctx, conflictReq)
		require.NoError(t, err)

		errResp, ok := resp.(api.CreateUser409JSONResponse)
		require.True(t, ok, "Expected CreateUser409JSONResponse")
		assert.Equal(t, http.StatusConflict, errResp.Code)
		assert.Contains(t, errResp.Message, "User with this username already exists")
	})

	t.Run("ConflictEmail", func(t *testing.T) {
		// First, create a user
		initialUserRequest := api.CreateUserJSONRequestBody{
			Username: "emailconflictuser",
			Email:    openapi_types.Email("conflict@example.com"), // This email will be conflicted
			Password: "password123",
		}
		initialReq := api.CreateUserRequestObject{Body: &initialUserRequest}
		_, err := s.CreateUser(ctx, initialReq)
		require.NoError(t, err)

		// Attempt to create another user with the same email
		conflictUserRequest := api.CreateUserJSONRequestBody{
			Username: "anotheruser",
			Email:    openapi_types.Email("conflict@example.com"), // Same email
			Password: "password456",
		}
		conflictReq := api.CreateUserRequestObject{Body: &conflictUserRequest}
		resp, err := s.CreateUser(ctx, conflictReq)
		require.NoError(t, err)

		errResp, ok := resp.(api.CreateUser409JSONResponse)
		require.True(t, ok, "Expected CreateUser409JSONResponse")
		assert.Equal(t, http.StatusConflict, errResp.Code)
		assert.Contains(t, errResp.Message, "User with this email already exists")
	})
}

func TestListUsers(t *testing.T) {
	s := setupTestServer()
	ctx := context.Background()

	t.Run("NoUsers", func(t *testing.T) {
		req := api.ListUsersRequestObject{}
		resp, err := s.ListUsers(ctx, req)
		require.NoError(t, err)

		listResp, ok := resp.(api.ListUsers200JSONResponse)
		require.True(t, ok, "Expected ListUsers200JSONResponse")
		assert.Empty(t, listResp)
	})

	t.Run("WithUsers", func(t *testing.T) {
		// Create a couple of users
		user1Req := api.CreateUserJSONRequestBody{Username: "user1", Email: "user1@example.com", Password: "p1"}
		_, _ = s.CreateUser(ctx, api.CreateUserRequestObject{Body: &user1Req})
		user2Req := api.CreateUserJSONRequestBody{Username: "user2", Email: "user2@example.com", Password: "p2"}
		_, _ = s.CreateUser(ctx, api.CreateUserRequestObject{Body: &user2Req})

		req := api.ListUsersRequestObject{}
		resp, err := s.ListUsers(ctx, req)
		require.NoError(t, err)

		listResp, ok := resp.(api.ListUsers200JSONResponse)
		require.True(t, ok, "Expected ListUsers200JSONResponse")
		assert.Len(t, listResp, 2)
		assert.Equal(t, "user1", listResp[0].Username)
		assert.Equal(t, "user2", listResp[1].Username)
	})
}

func TestGetUserById(t *testing.T) {
	s := setupTestServer()
	ctx := context.Background()

	// Create a user to be fetched
	userRequest := api.CreateUserJSONRequestBody{
		Username: "getmeuser",
		Email:    openapi_types.Email("getme@example.com"),
		Password: "password123",
	}
	createReq := api.CreateUserRequestObject{Body: &userRequest}
	createResp, err := s.CreateUser(ctx, createReq)
	require.NoError(t, err)
	createdUserResp, ok := createResp.(api.CreateUser201JSONResponse)
	require.True(t, ok)
	userID := createdUserResp.Id

	t.Run("Success", func(t *testing.T) {
		req := api.GetUserByIdRequestObject{
			UserId: userID,
		}
		resp, err := s.GetUserById(ctx, req)
		require.NoError(t, err)

		getResp, ok := resp.(api.GetUserById200JSONResponse)
		require.True(t, ok, "Expected GetUserById200JSONResponse")
		assert.Equal(t, userID, getResp.Id)
		assert.Equal(t, "getmeuser", getResp.Username)
		require.NotNil(t, getResp.Email)
		assert.Equal(t, openapi_types.Email("getme@example.com"), *getResp.Email)
	})

	t.Run("NotFound", func(t *testing.T) {
		req := api.GetUserByIdRequestObject{
			UserId: "nonexistentid",
		}
		resp, err := s.GetUserById(ctx, req)
		require.NoError(t, err)

		errResp, ok := resp.(api.GetUserById404JSONResponse)
		require.True(t, ok, "Expected GetUserById404JSONResponse")
		assert.Equal(t, http.StatusNotFound, errResp.Code)
		assert.Equal(t, "User not found", errResp.Message)
	})

	t.Run("EmptyUserId", func(t *testing.T) {
		req := api.GetUserByIdRequestObject{
			UserId: "",
		}
		resp, err := s.GetUserById(ctx, req)
		require.NoError(t, err)

		errResp, ok := resp.(api.GetUserById400JSONResponse)
		require.True(t, ok, "Expected GetUserById400JSONResponse")
		assert.Equal(t, http.StatusBadRequest, errResp.Code)
		assert.Equal(t, "User ID path parameter is required", errResp.Message)
	})
}
