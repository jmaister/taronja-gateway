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

	// User found, return their information
	authenticated := true
	email := sessionObject.Email
	username := sessionObject.Username
	provider := sessionObject.Provider
	isAdmin := sessionObject.IsAdmin
	timestamp := time.Now().UTC() // Consistent with APIGetMe logic

	var emailPointer *openapi_types.Email
	if email != "" {
		emailType := openapi_types.Email(email)
		emailPointer = &emailType
	}
	// If email is empty, emailPointer remains nil

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
