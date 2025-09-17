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

// GetCurrentUser returns the current authenticated user info, using session from context.
func (s *StrictApiServer) GetCurrentUser(ctx context.Context, request api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error) {
	sessionObject, ok := ctx.Value(session.SessionKey).(*db.Session)

	if !ok || sessionObject == nil || !sessionObject.IsAuthenticated || sessionObject.ValidUntil.Before(time.Now()) {
		// No valid authenticated session found
		return api.GetCurrentUser401JSONResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized",
		}, nil
	}

	// Get full user information from the database
	user, err := s.userRepo.FindUserByIdOrUsername(sessionObject.UserID, "", "")
	if err != nil {
		return nil, err
	}
	if user == nil {
		return api.GetCurrentUser401JSONResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized",
		}, nil
	}

	// User found, return their information including additional fields
	authenticated := true
	email := user.Email
	username := user.Username
	provider := user.Provider
	isAdmin := sessionObject.IsAdmin
	timestamp := time.Now().UTC()
	name := user.Name
	picture := user.Picture
	givenName := user.GivenName
	familyName := user.FamilyName

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

// Converts string to pointer, returns nil for empty strings
func stringToPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
