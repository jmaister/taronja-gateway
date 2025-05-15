package handlers

import (
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/session"
)

// RegisterLogoutHandler registers the logout route.
// This function is exported so it can be used in tests or other packages if necessary.
func RegisterLogoutHandler(mux *http.ServeMux, sessionStore session.SessionStore, managementPrefix string) {
	// Define the logout route path
	logoutPath := managementPrefix + "/logout"

	mux.HandleFunc(logoutPath, func(w http.ResponseWriter, r *http.Request) {
		// Get the session cookie
		cookie, err := r.Cookie(session.SessionCookieName)
		if err != nil {
			// No session cookie, redirect to home
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Get redirect URL from query parameters, default to "/"
		redirectURL := r.URL.Query().Get("redirect")
		if redirectURL == "" {
			redirectURL = "/"
		}

		// Delete the session from the store
		if err := sessionStore.EndSession(cookie.Value); err != nil {
			log.Printf("Error deleting session: %v", err)
			// Continue with logout even if there's an error
		}

		// Expire the session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     session.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			MaxAge:   -1, // Delete immediately
		})

		// Add cache control headers to prevent browser caching
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Redirect to the original URL or home page
		http.Redirect(w, r, redirectURL, http.StatusFound)
	})
	// Log statement was moved to the caller g.registerLogout to keep context of GatewayConfig
}
