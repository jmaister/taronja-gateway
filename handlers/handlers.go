package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/session"
)

// --- Meta API Handlers ---

// handleHealth is a simple health check endpoint.
// It indicates the gateway process is running.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	// Basic response
	response := map[string]string{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		// This error is unlikely but possible if the connection is closed
		log.Printf("Error encoding health response: %v", err)
	}
}

// handleMe returns information about the currently authenticated user.
// It retrieves the session from the session store using the cookie.
func HandleMe(w http.ResponseWriter, r *http.Request, sessionStore session.SessionStore) {
	// Retrieve the session object from the session store using the request's cookie
	sessionObject, valid := sessionStore.Validate(r)
	if !valid {
		// No valid authenticated session found
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// User found, return their information
	response := map[string]interface{}{
		"authenticated": true,
		"username":      sessionObject.Username,
		"email":         sessionObject.Email,
		"provider":      sessionObject.Provider,

		// Add more fields as needed (e.g., email, name if fetched during OAuth)
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("Error encoding /me response: %v", err)
	}
}
