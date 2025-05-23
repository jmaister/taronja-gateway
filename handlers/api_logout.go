package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/session"
)

// LogoutUser handles the GET /logout request.
func (s *StrictApiServer) LogoutUser(ctx context.Context, request api.LogoutUserRequestObject) (api.LogoutUserResponseObject, error) {

	sessionToken := request.Params.TgSessionToken
	if sessionToken != nil && *sessionToken != "" {
		err := s.sessionStore.EndSession(*sessionToken)
		if err != nil {
			log.Printf("Error deleting session from store: %v. Proceeding with cookie expiration.", err)
			// Non-fatal for client, main goal is cookie removal.
		}
	}

	// Redirect URL from query parameters, default to "/"
	redirectURL := request.Params.Redirect
	if redirectURL == nil || *redirectURL == "" {
		redirectURL = new(string)
		*redirectURL = "/"
	}

	// Create the Set-Cookie header value for clearing the session
	clearCookie := &http.Cookie{
		Name:     session.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	}
	cookieValue := clearCookie.String()

	// Check if we're on HTTPS and add Secure flag if needed
	// Note: We can't access the request.TLS directly here as we only have the context
	// If the gateway always sets Secure flag on cookies when behind HTTPS, we should follow that pattern
	// Or we could add the Secure flag based on a configuration setting

	// Return a 302 response with the Set-Cookie header
	return api.LogoutUser302Response{
		Headers: api.LogoutUser302ResponseHeaders{
			Location:     *redirectURL,
			CacheControl: "no-store, no-cache, must-revalidate, post-check=0, pre-check=0",
			SetCookie:    cookieValue,
		},
	}, nil
}
