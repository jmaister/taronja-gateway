package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetCurrentUser implements the GetCurrentUser operation for the api.StrictServerInterface.
// It relies on SessionMiddleware to validate the session and put *db.Session into the context.
func (s *StrictApiServer) GetCurrentUser(ctx context.Context, request api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error) {
	sessionObject, ok := ctx.Value(session.SessionKey).(*db.Session)

	if !ok || sessionObject == nil || !sessionObject.IsAuthenticated || sessionObject.ValidUntil.Before(time.Now()) {
		// No valid authenticated session found by the middleware,
		// or the type assertion failed, or session is not valid.
		return api.GetCurrentUser401JSONResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized",
		}, nil
	}

	// Get full user information from the database
	user, err := s.userRepo.FindUserByIdOrUsername(sessionObject.UserID, "", "")
	if err != nil || user == nil {
		// If user not found in database, still return session info
		// This can happen if the user was deleted but session is still valid
		authenticated := true
		email := sessionObject.Email
		username := sessionObject.Username
		provider := sessionObject.Provider
		isAdmin := sessionObject.IsAdmin
		timestamp := time.Now().UTC()

		var emailPointer *openapi_types.Email
		if email != "" {
			emailType := openapi_types.Email(email)
			emailPointer = &emailType
		}

		response := api.GetCurrentUser200JSONResponse{
			Authenticated: &authenticated,
			Username:      &username,
			Email:         emailPointer,
			Provider:      &provider,
			IsAdmin:       &isAdmin,
			Timestamp:     &timestamp,
		}
		return response, nil
	}

	// User found, return their information including additional fields
	authenticated := true
	email := user.Email
	username := user.Username
	provider := sessionObject.Provider // Get provider from session instead of user
	isAdmin := sessionObject.IsAdmin
	timestamp := time.Now().UTC()
	name := user.Name
	picture := user.Picture
	givenName := "" // These fields are now in UserLogin table
	familyName := "" // These fields are now in UserLogin table

	var emailPointer *openapi_types.Email
	if email != "" {
		emailType := openapi_types.Email(email)
		emailPointer = &emailType
	}

	response := api.GetCurrentUser200JSONResponse{
		Authenticated: &authenticated,
		Username:      &username,
		Email:         emailPointer,
		Name:          stringToPointer(name),
		Picture:       stringToPointer(picture),
		GivenName:     stringToPointer(givenName),
		FamilyName:    stringToPointer(familyName),
		Provider:      &provider,
		IsAdmin:       &isAdmin,
		Timestamp:     &timestamp,
	}
	return response, nil
}

// Helper function to convert string to pointer, returns nil for empty strings
func stringToPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
