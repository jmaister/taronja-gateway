package handlers

import (
	"encoding/json" // Added for HandleHealth
	"fmt"
	"log"
	"net/http"
	"time"

	"html/template"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
)

// --- Meta API Handlers ---

// HandleHealth checks the health of the gateway.
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

// HandleMe provides information about the currently authenticated user.
func HandleMe(w http.ResponseWriter, r *http.Request, sessionRepo db.SessionRepository) {
	// Retrieve the session object from the session repository using the request's cookie
	sessionObject, valid := sessionRepo.ValidateSession(r)
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
		// This error is unlikely but possible if the connection is closed
		log.Printf("Error encoding user info response: %v", err)
	}
}

// CreateUserRequest defines the expected JSON structure for the create user request
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// HandleCreateUser handles the creation of a new user.
// It expects a POST request with a JSON body containing "username", "email", and "password".
func HandleCreateUser(w http.ResponseWriter, r *http.Request, userRepo db.UserRepository) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode the JSON request body
	var req CreateUserRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("Error decoding JSON for user creation: %v", err)
		http.Error(w, "Bad Request: Could not decode JSON data", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	username := req.Username
	email := req.Email
	password := req.Password

	if username == "" || email == "" || password == "" { // Validate email
		http.Error(w, "Bad Request: Username, email, and password are required", http.StatusBadRequest)
		return
	}

	// Check if user already exists by username or email
	existingUser, err := userRepo.FindUserByIdOrUsername("", username, email)
	if err != nil {
		// An actual error occurred during the database query
		log.Printf("Error checking if user exists (username '%s', email '%s'): %v", username, email, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if existingUser != nil {
		// User found with either the same username or email
		var conflictField string
		if existingUser.Username == username {
			conflictField = "username"
		} else if existingUser.Email == email {
			conflictField = "email"
		} else {
			conflictField = "unknown field" // Should not happen if query is correct
		}
		log.Printf("Attempt to create user with existing %s: username '%s', email '%s'", conflictField, username, email)
		http.Error(w, fmt.Sprintf("Conflict: User with this %s already exists", conflictField), http.StatusConflict)
		return
	}

	// Hash the password
	hashedPassword, err := encryption.GeneratePasswordHash(password)
	if err != nil {
		log.Printf("Error hashing password for user '%s': %v", username, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create the user
	newUser := &db.User{
		Username: username,
		Email:    email,          // Add email to the new user struct
		Password: hashedPassword, // Assuming schema.go uses Password for input, hooks handle hashing to PasswordHash if applicable
		// PasswordHash: hashedPassword, // If your User struct directly uses PasswordHash
	}

	err = userRepo.CreateUser(newUser) // Changed to CreateUser
	if err != nil {
		log.Printf("Error creating user '%s': %v", username, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully created user: %s, email: %s", username, email)
	w.WriteHeader(http.StatusCreated)
	// Return a simple success message for the JavaScript to handle
	fmt.Fprintf(w, "User %s created successfully.", username)
}

// HandleGetUser retrieves and returns a user by their ID as an HTML page.
// It ensures that sensitive information like the password is not exposed.
func HandleGetUser(w http.ResponseWriter, r *http.Request, userRepo db.UserRepository, templates map[string]*template.Template, managementPrefix string, sessionRepo db.SessionRepository) {
	userID := r.PathValue("user_id")

	tmpl, ok := templates["user_info.html"]
	if !ok || tmpl == nil {
		log.Printf("Error: User info template 'user_info.html' not found in cache")
		http.Error(w, "Internal Server Error: Template not found", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"ManagementPrefix": managementPrefix,
	}

	if userID == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		data["Error"] = "User ID is required and was not found in path"
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("Error executing user_info template for bad request: %v", err)
		}
		return
	}

	user, err := userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		log.Printf("Error fetching user with ID '%s': %v", userID, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		data["Error"] = "Internal Server Error while fetching user data."
		if errEx := tmpl.Execute(w, data); errEx != nil {
			log.Printf("Error executing user_info template for server error: %v", errEx)
		}
		return
	}

	if user == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		data["Error"] = "User not found."
		if errEx := tmpl.Execute(w, data); errEx != nil {
			log.Printf("Error executing user_info template for not found: %v", errEx)
		}
		return
	}

	// Prepare user data for response, excluding sensitive fields like password
	userData := map[string]interface{}{
		"id":        user.ID,
		"username":  user.Username,
		"email":     user.Email,
		"name":      user.Name,
		"picture":   user.Picture,
		"provider":  user.Provider,
		"createdAt": user.CreatedAt.Format(time.RFC1123), // Format dates for display
		"updatedAt": user.UpdatedAt.Format(time.RFC1123),
	}
	data["User"] = userData

	// Get all sessions for this user
	userSessions, err := sessionRepo.GetSessionsByUserID(userID)
	if err != nil {
		log.Printf("Warning: Error fetching sessions for user %s: %v", userID, err)
		// Continue without sessions, don't fail the entire page
	} else {
		// Format session data for display
		sessionData := make([]map[string]interface{}, 0, len(userSessions))
		for _, sess := range userSessions {
			sessionData = append(sessionData, map[string]interface{}{
				"provider":   sess.Provider,
				"validUntil": sess.ValidUntil.Format(time.RFC1123),
				"active":     sess.ValidUntil.After(time.Now()) && sess.ClosedOn == nil,
				"closedOn":   sess.ClosedOn,
			})
		}
		data["Sessions"] = sessionData
		data["SessionCount"] = len(sessionData)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing user_info template: %v", err)
		// Attempt to send a simple error if template execution fails mid-way
		// This might not work if headers are already sent.
		http.Error(w, "Failed to render user information page.", http.StatusInternalServerError)
	}
}

// HandleListUsers retrieves all users and displays them in a list.
func HandleListUsers(w http.ResponseWriter, r *http.Request, userRepo db.UserRepository, templates map[string]*template.Template, managementPrefix string) {
	tmpl, ok := templates["users_list.html"]
	if !ok || tmpl == nil {
		log.Printf("Error: User list template 'users_list.html' not found in cache")
		http.Error(w, "Internal Server Error: Template not found", http.StatusInternalServerError)
		return
	}

	users, err := userRepo.GetAllUsers()
	if err != nil {
		log.Printf("Error fetching all users: %v", err)
		data := map[string]interface{}{
			"ManagementPrefix": managementPrefix,
			"Error":            "Internal Server Error while fetching user list.",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		if errEx := tmpl.Execute(w, data); errEx != nil {
			log.Printf("Error executing users_list template for server error: %v", errEx)
		}
		return
	}

	data := map[string]interface{}{
		"ManagementPrefix": managementPrefix,
		"Users":            users,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing users_list template: %v", err)
		http.Error(w, "Failed to render user list page.", http.StatusInternalServerError)
	}
}
