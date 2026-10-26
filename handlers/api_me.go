package handlers

import (
	"context"
	"log"
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

	if s.organizations != nil {
		orgs, err := s.myOrganizations(sessionObject.UserID)
		if err != nil {
			log.Printf("Error listing organizations of %s: %v", sessionObject.UserID, err)
		} else {
			response.Organizations = &orgs
		}
	}

	// Add entitlements if billing is configured
	if s.billing != nil {
		userEntitlements, err := s.billing.ForUser(sessionObject.UserID)
		if err != nil {
			log.Printf("Error getting entitlements for %s: %v", sessionObject.UserID, err)
		} else {
			header := userEntitlements.Header(time.Now())
			response.Entitlements = &api.EntitlementHeader{
				User: header.User,
				Orgs: header.Orgs,
			}
			if header.Grace != nil {
				grace := &api.EntitlementHeaderGrace{}
				if len(header.Grace.User) > 0 {
					grace.User = &header.Grace.User
				}
				if len(header.Grace.Orgs) > 0 {
					grace.Orgs = &header.Grace.Orgs
				}
				response.Entitlements.Grace = grace
			}
		}
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
