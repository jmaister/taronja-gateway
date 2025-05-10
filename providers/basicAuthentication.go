package providers

import (
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/session"
)

// RegisterBasicAuth registers a basic authentication handler
func RegisterBasicAuth(mux *http.ServeMux, sessionStore session.SessionStore, managementPrefix string, userRepo db.UserRepository) {
	// Basic Auth Login Route
	basicLoginPath := managementPrefix + "/auth/basic/login"
	mux.HandleFunc(basicLoginPath, func(w http.ResponseWriter, r *http.Request) {
		// First, check if the user already has a valid session
		_, isValid := sessionStore.Validate(r)
		if isValid {
			// If session is valid, redirect to the requested URL or home
			redirectURL := r.URL.Query().Get("redirect")
			if redirectURL == "" {
				redirectURL = "/"
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		// Handle form submission for POST requests
		if r.Method == "POST" {
			// Parse the form data
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Error parsing form data", http.StatusBadRequest)
				return
			}

			// Extract username and password from form
			username := r.Form.Get("username")
			password := r.Form.Get("password")

			// Load user from database and validate password
			user, err := userRepo.FindUserByIdOrUsername("", username, username) // Try both username and email fields
			if err != nil {
				log.Printf("Error finding user: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if user == nil {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}

			// Compare the provided password with the stored hash
			matches, err := encryption.ComparePassword(password, user.Password)
			if err != nil {
				log.Printf("Password comparison failed: %v", err)
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}
			if !matches {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}

			// User found and password validated, create session
			token, err := sessionStore.GenerateKey()
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Create session object with all required fields
			so := session.SessionObject{
				Username:        user.Username,
				Email:           user.Email,
				IsAuthenticated: true,
				ValidUntil:      time.Now().Add(24 * time.Hour), // 24-hour session
				Provider:        "basic",
			}
			sessionStore.Set(token, so)

			// Set session token in a cookie
			http.SetCookie(w, &http.Cookie{
				Name:     session.SessionCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				MaxAge:   86400, // 24 hours in seconds
			})

			// Check if there's a redirect URL in the query parameters
			// First check form data (for POST requests), then URL query parameters
			redirectURL := r.Form.Get("redirect")
			if redirectURL == "" {
				redirectURL = r.URL.Query().Get("redirect")
			}
			if redirectURL != "" {
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}

			// Redirect to home page if no redirect URL
			http.Redirect(w, r, "/", http.StatusFound)
			return
		} else {
			// For GET requests, serve the login page
			http.ServeFile(w, r, "./static/login.html")
		}
	})
	log.Printf("Registered Login Route: %-25s | Path: %s", "Basic Auth Login", basicLoginPath)
}
