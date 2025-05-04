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
		log.Printf("meta_api.go: Error encoding health response: %v", err)
	}
}

// handleMe returns information about the currently authenticated user.
// It relies on the authentication middleware having populated the request context.
func HandleMe(w http.ResponseWriter, r *http.Request) {
	// Attempt to retrieve sessionObject information from the context
	// The auth middleware (Basic, OAuth2) should add this upon successful authentication.
	sessionObject, ok := r.Context().Value(session.SessionKey).(*session.SessionObject)

	if !ok || sessionObject == nil {
		// No authenticated user found in context
		log.Println("meta_api.go: handleMe called but no authenticated user found in context.")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if the session is still valid
	if sessionObject.ValidUntil.Before(time.Now()) {
		http.Error(w, "Session expired", http.StatusUnauthorized)
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
