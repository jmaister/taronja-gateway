package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// --- Meta API Handlers ---

// handleHealth is a simple health check endpoint.
// It indicates the gateway process is running.
func handleHealth(w http.ResponseWriter, r *http.Request) {
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
func handleMe(w http.ResponseWriter, r *http.Request) {
	// Attempt to retrieve user information from the context
	// The auth middleware (Basic, OAuth2) should add this upon successful authentication.
	user, ok := r.Context().Value(userContextKey).(*AuthenticatedUser)

	if !ok || user == nil {
		// No authenticated user found in context
		log.Println("meta_api.go: handleMe called but no authenticated user found in context.")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// User found, return their information
	response := map[string]interface{}{
		"authenticated": true,
		"user": map[string]string{
			"id":     user.ID,     // e.g., username for basic, provider ID for oauth
			"source": user.Source, // e.g., "basic", "google", "github"
			// Add more fields as needed (e.g., email, name if fetched during OAuth)
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("meta_api.go: Error encoding /me response: %v", err)
	}
}

// --- Context Handling ---
// (Moved context key and user struct definition here for clarity)

// userContextKey is the key used to store user information in the request context.
// Using a custom type avoids collisions.
type contextKey string

const userContextKey contextKey = "authenticatedUser"

// AuthenticatedUser holds basic information about the logged-in user.
// This struct is added to the request context by authenticators.
type AuthenticatedUser struct {
	ID     string // Username, email, or provider-specific ID
	Source string // Authentication source ("basic", "google", "github", etc.)
	// Add more fields like Email, Name if available from provider
}
