package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
)

// GetUserLogins handles the HTTP request for retrieving user login methods.
func (s *StrictApiServer) GetUserLogins(ctx context.Context, request api.GetUserLoginsRequestObject) (api.GetUserLoginsResponseObject, error) {
	// Check if user exists
	_, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("GetUserLogins: User with ID '%s' not found", request.UserId)
			return api.GetUserLogins404JSONResponse{
				Code:    http.StatusNotFound,
				Message: "User not found",
			}, nil
		}
		log.Printf("GetUserLogins: Error finding user with ID '%s': %v", request.UserId, err)
		return api.GetUserLogins500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	// TODO: Add authorization check - users should only be able to access their own login methods
	// For now, we'll implement basic authorization in a future iteration

	// Get user login methods
	userLogins, err := s.userLoginRepo.FindUserLoginsByUserID(request.UserId)
	if err != nil {
		log.Printf("GetUserLogins: Error fetching login methods for user '%s': %v", request.UserId, err)
		return api.GetUserLogins500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	// Convert to API response format
	apiUserLogins := make([]api.UserLoginResponse, 0, len(userLogins))
	for _, userLogin := range userLogins {
		apiUserLogin := dbUserLoginToAPIUserLoginResponse(userLogin)
		apiUserLogins = append(apiUserLogins, apiUserLogin)
	}

	log.Printf("GetUserLogins: Successfully retrieved %d login methods for user '%s'", len(apiUserLogins), request.UserId)
	return api.GetUserLogins200JSONResponse(apiUserLogins), nil
}

// LinkUserLogin handles the HTTP request for linking a new social login method to a user.
func (s *StrictApiServer) LinkUserLogin(ctx context.Context, request api.LinkUserLoginRequestObject) (api.LinkUserLoginResponseObject, error) {
	// Validate input
	if request.Body.Provider == "" || request.Body.ProviderId == "" || string(request.Body.ProviderEmail) == "" {
		log.Printf("LinkUserLogin: Missing required fields")
		return api.LinkUserLogin400JSONResponse{
			Code:    http.StatusBadRequest,
			Message: "Provider, provider_id, and provider_email are required",
		}, nil
	}

	// Check if user exists
	_, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("LinkUserLogin: User with ID '%s' not found", request.UserId)
			return api.LinkUserLogin400JSONResponse{
				Code:    http.StatusBadRequest,
				Message: "User not found",
			}, nil
		}
		log.Printf("LinkUserLogin: Error finding user with ID '%s': %v", request.UserId, err)
		return api.LinkUserLogin500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	// TODO: Add authorization check - users should only be able to manage their own login methods

	// Check if this provider login already exists
	existingLogin, err := s.userLoginRepo.FindUserLoginByProvider(string(request.Body.Provider), request.Body.ProviderId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("LinkUserLogin: Error checking existing login: %v", err)
		return api.LinkUserLogin500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	if existingLogin != nil {
		log.Printf("LinkUserLogin: Provider login already exists for provider '%s', ID '%s'", request.Body.Provider, request.Body.ProviderId)
		return api.LinkUserLogin409JSONResponse{
			Code:    http.StatusConflict,
			Message: "This social login method is already linked to an account",
		}, nil
	}

	// Create new user login
	userLogin := &db.UserLogin{
		UserID:     request.UserId,
		Provider:   string(request.Body.Provider),
		ProviderId: request.Body.ProviderId,
		Email:      string(request.Body.ProviderEmail),
		IsActive:   true,
	}

	// Set optional fields
	if request.Body.ProviderUsername != nil {
		userLogin.Username = *request.Body.ProviderUsername
	}

	err = s.userLoginRepo.CreateUserLogin(userLogin)
	if err != nil {
		log.Printf("LinkUserLogin: Error creating user login: %v", err)
		return api.LinkUserLogin500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Failed to link login method",
		}, nil
	}

	log.Printf("LinkUserLogin: Successfully linked %s login to user %s", request.Body.Provider, request.UserId)
	apiUserLogin := dbUserLoginToAPIUserLoginResponse(userLogin)
	return api.LinkUserLogin201JSONResponse(apiUserLogin), nil
}

// UnlinkUserLogin handles the HTTP request for unlinking a social login method from a user.
func (s *StrictApiServer) UnlinkUserLogin(ctx context.Context, request api.UnlinkUserLoginRequestObject) (api.UnlinkUserLoginResponseObject, error) {
	// Check if user exists
	user, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("UnlinkUserLogin: User with ID '%s' not found", request.UserId)
			return api.UnlinkUserLogin404JSONResponse{
				Code:    http.StatusNotFound,
				Message: "User not found",
			}, nil
		}
		log.Printf("UnlinkUserLogin: Error finding user with ID '%s': %v", request.UserId, err)
		return api.UnlinkUserLogin500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	// TODO: Add authorization check - users should only be able to manage their own login methods

	// Get all user login methods to check if this is the last one
	userLogins, err := s.userLoginRepo.FindUserLoginsByUserID(request.UserId)
	if err != nil {
		log.Printf("UnlinkUserLogin: Error fetching login methods for user '%s': %v", request.UserId, err)
		return api.UnlinkUserLogin500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	// Find the specific login method to unlink
	var targetLogin *db.UserLogin
	for _, login := range userLogins {
		if login.ID == request.LoginId {
			targetLogin = login
			break
		}
	}

	if targetLogin == nil {
		log.Printf("UnlinkUserLogin: Login method with ID '%s' not found for user '%s'", request.LoginId, request.UserId)
		return api.UnlinkUserLogin404JSONResponse{
			Code:    http.StatusNotFound,
			Message: "Login method not found",
		}, nil
	}

	// Check if user has password or other login methods (prevent locking out)
	hasPassword := user.Password != ""
	hasOtherLogins := len(userLogins) > 1

	if !hasPassword && !hasOtherLogins {
		log.Printf("UnlinkUserLogin: Cannot unlink last login method for user '%s'", request.UserId)
		return api.UnlinkUserLogin400JSONResponse{
			Code:    http.StatusBadRequest,
			Message: "Cannot unlink the last login method. Set a password or link another social account first.",
		}, nil
	}

	// Deactivate the login method (soft delete)
	err = s.userLoginRepo.DeactivateUserLogin(request.LoginId)
	if err != nil {
		log.Printf("UnlinkUserLogin: Error deactivating login method '%s': %v", request.LoginId, err)
		return api.UnlinkUserLogin500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Failed to unlink login method",
		}, nil
	}

	log.Printf("UnlinkUserLogin: Successfully unlinked login method '%s' from user '%s'", request.LoginId, request.UserId)
	return api.UnlinkUserLogin204Response{}, nil
}

// dbUserLoginToAPIUserLoginResponse converts a db.UserLogin to api.UserLoginResponse
func dbUserLoginToAPIUserLoginResponse(userLogin *db.UserLogin) api.UserLoginResponse {
	var providerUsername *string
	if userLogin.Username != "" {
		providerUsername = &userLogin.Username
	}

	var picture *string
	if userLogin.Picture != "" {
		picture = &userLogin.Picture
	}

	return api.UserLoginResponse{
		Id:               userLogin.ID,
		Provider:         userLogin.Provider,
		ProviderEmail:    types.Email(userLogin.Email),
		ProviderUsername: providerUsername,
		Picture:          picture,
		CreatedAt:        userLogin.CreatedAt,
		UpdatedAt:        userLogin.UpdatedAt,
		IsActive:         userLogin.IsActive,
	}
}
