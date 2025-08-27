package providers

import (
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/session"
	"gorm.io/gorm"
	// For session.ExtractClientInfo, session.SessionCookieName
)

// parseLoginCredentials parses username and password from form data (both URL-encoded and multipart)
func parseLoginCredentials(r *http.Request) (username, password string) {
	// First try to parse as URL-encoded form
	if err := r.ParseForm(); err == nil {
		username = r.Form.Get("username")
		password = r.Form.Get("password")
	}

	// If that didn't work or values are empty, try multipart form
	if (username == "" || password == "") && r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		if err := r.ParseMultipartForm(32 << 20); err == nil { // 32MB max
			if r.MultipartForm != nil && r.MultipartForm.Value != nil {
				if usernames := r.MultipartForm.Value["username"]; len(usernames) > 0 {
					username = usernames[0]
				}
				if passwords := r.MultipartForm.Value["password"]; len(passwords) > 0 {
					password = passwords[0]
				}
			}
		}
	}
	return username, password
}

// getRedirectURL extracts the redirect URL from various sources in the request
func getRedirectURL(r *http.Request) string {
	redirectURL := r.Form.Get("redirect")
	if redirectURL == "" && r.MultipartForm != nil && r.MultipartForm.Value != nil {
		if redirects := r.MultipartForm.Value["redirect"]; len(redirects) > 0 {
			redirectURL = redirects[0]
		}
	}
	if redirectURL == "" {
		redirectURL = r.URL.Query().Get("redirect")
	}
	if redirectURL == "" {
		redirectURL = "/" // Default redirect
	}
	return redirectURL
}

// createSessionAndRedirect creates a session for the user, sets the session cookie, and redirects
func createSessionAndRedirect(w http.ResponseWriter, r *http.Request, user *db.User, sessionStore session.SessionStore) {
	sessionObject, err := sessionStore.NewSession(r, user, "basic", 24*time.Hour)
	if err != nil {
		http.Error(w, "Internal Server Error: Could not create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     session.SessionCookieName,
		Value:    sessionObject.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   int((24 * time.Hour).Seconds()), // 24 hours
	})

	redirectURL := getRedirectURL(r)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// RegisterBasicAuth registers basic authentication handlers for login.
// It now uses db.SessionRepository.
func RegisterBasicAuth(mux *http.ServeMux, sessionStore session.SessionStore, managementPrefix string, userRepo db.UserRepository, gatewayConfig *config.GatewayConfig) {
	basicLoginPath := managementPrefix + "/auth/basic/login"

	checkSessionAndRedirect := func(w http.ResponseWriter, r *http.Request) bool {
		_, isValid := sessionStore.ValidateSession(r) // Use ValidateSession from db.SessionRepository
		if isValid {
			redirectURL := r.URL.Query().Get("redirect")
			if redirectURL == "" {
				redirectURL = "/"
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return true
		}
		return false
	}

	mux.HandleFunc("POST "+basicLoginPath, func(w http.ResponseWriter, r *http.Request) {
		if checkSessionAndRedirect(w, r) {
			return
		}

		username, password := parseLoginCredentials(r)

		log.Printf("Login attempt for user: %s", username)
		log.Printf("Password received: %t (length: %d)", password != "", len(password))

		if username == "" || password == "" {
			log.Printf("Empty username or password received")
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		// Find user from database (this now includes admin users)
		user, err := userRepo.FindUserByIdOrUsername("", username, username)
		if err != nil && err != gorm.ErrRecordNotFound {
			log.Printf("Error finding user: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if user == nil {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Check if this is an admin user from config
		isAdminFromConfig := gatewayConfig.Management.Admin.Enabled &&
			gatewayConfig.Management.Admin.Username == username

		if isAdminFromConfig {
			log.Printf("Admin user login attempt for: %s", username)
		}

		matches, err := encryption.ComparePassword(password, user.Password)
		if err != nil {
			log.Printf("Password comparison failed: %v", err)
			// Log actual error but return generic message to user
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		if !matches {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		createSessionAndRedirect(w, r, user, sessionStore)
	})

	log.Printf("Registered Login Route: %-25s | Path: %s (POST)", "Basic Auth Login", basicLoginPath)
}
