package providers

import (
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
	// For session.ExtractClientInfo, session.SessionCookieName
)

// RegisterBasicAuth registers basic authentication handlers for login.
// It now uses db.SessionRepository.
func RegisterBasicAuth(mux *http.ServeMux, sessionRepo db.SessionRepository, managementPrefix string, userRepo db.UserRepository) {
	basicLoginPath := managementPrefix + "/auth/basic/login"

	checkSessionAndRedirect := func(w http.ResponseWriter, r *http.Request) bool {
		_, isValid := sessionRepo.ValidateSession(r) // Use ValidateSession from db.SessionRepository
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

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form data", http.StatusBadRequest)
			return
		}

		username := r.Form.Get("username")
		password := r.Form.Get("password")

		user, err := userRepo.FindUserByIdOrUsername("", username, username)
		if err != nil {
			log.Printf("Error finding user: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if user == nil {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
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

		token, err := sessionRepo.GenerateToken() // Use GenerateToken from db.SessionRepository
		if err != nil {
			http.Error(w, "Internal Server Error: Could not generate session token", http.StatusInternalServerError)
			return
		}

		so := db.NewSession(r, user, "basic", 24*time.Hour) // NewSession returns *db.Session

		if err := sessionRepo.CreateSession(token, so); err != nil { // Use CreateSession from db.SessionRepository
			http.Error(w, "Internal Server Error: Could not create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     db.SessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			MaxAge:   86400, // 24 hours
		})

		redirectURL := r.Form.Get("redirect")
		if redirectURL == "" {
			redirectURL = r.URL.Query().Get("redirect")
		}
		if redirectURL == "" {
			redirectURL = "/" // Default redirect
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
	})

	log.Printf("Registered Login Route: %-25s | Path: %s (POST)", "Basic Auth Login", basicLoginPath)
}
