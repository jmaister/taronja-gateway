package handlers

import (
	"context"
	"errors" // Added for errors.Is
	"fmt"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
	openapi_types "github.com/oapi-codegen/runtime/types" // Added for openapi_types.Email
	"gorm.io/gorm"                                        // Added for gorm.ErrRecordNotFound
)

// dbUserToAPIUserResponse converts a db.User object to an API UserResponse object.
// It handles the conversion of ID to string and formats timestamps to RFC3339.
// Nullable fields (Name, Picture, Provider) are converted to pointers to strings.
func dbUserToAPIUserResponse(dbUser *db.User) api.UserResponse {
	// ID is already string in db.User and api.UserResponse
	// CreatedAt and UpdatedAt are time.Time in db.User and api.UserResponse

	var namePtr *string
	if dbUser.Name != "" {
		namePtr = &dbUser.Name
	}
	var picturePtr *string
	if dbUser.Picture != "" {
		picturePtr = &dbUser.Picture
	}
	var providerPtr *string
	if dbUser.Provider != "" {
		providerPtr = &dbUser.Provider
	}

	// Handle email conversion safely - only include if it's a valid email
	var emailPtr *openapi_types.Email
	if dbUser.Email != "" {
		email := openapi_types.Email(dbUser.Email)
		// Test if the email is valid by trying to marshal it
		if _, err := email.MarshalJSON(); err == nil {
			emailPtr = &email
		}
		// If email is invalid, we leave emailPtr as nil
	}

	return api.UserResponse{
		Id:        dbUser.ID, // Corrected: Directly use dbUser.ID
		Username:  dbUser.Username,
		Email:     emailPtr,
		Name:      namePtr,
		Picture:   picturePtr,
		Provider:  providerPtr,
		CreatedAt: dbUser.CreatedAt, // Corrected: Assign directly
		UpdatedAt: dbUser.UpdatedAt, // Corrected: Assign directly
	}
}

// CreateUser handles the HTTP request for creating a new user.
// It implements the createUser operation defined in the OpenAPI specification.
func (s *StrictApiServer) CreateUser(ctx context.Context, request api.CreateUserRequestObject) (api.CreateUserResponseObject, error) {
	// Validate input (basic check, OpenAPI spec should enforce most of this)
	if request.Body.Username == "" || string(request.Body.Email) == "" || request.Body.Password == "" {
		log.Printf("CreateUser: Missing required fields.")
		return api.CreateUser400JSONResponse{
			Code:    http.StatusBadRequest,
			Message: "Username, email, and password are required",
		}, nil
	}

	// Check if user already exists by username or email
	// Corrected: s.userRepo instead of s.UserRepo
	existingUser, err := s.userRepo.FindUserByIdOrUsername("", request.Body.Username, string(request.Body.Email))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) { // Allow gorm.ErrRecordNotFound
		log.Printf("CreateUser: Error checking if user exists (username '%s', email '%s'): %v", request.Body.Username, string(request.Body.Email), err)
		return api.CreateUser500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}
	if existingUser != nil {
		var conflictField string
		if existingUser.Username == request.Body.Username {
			conflictField = "username"
		} else {
			conflictField = "email"
		}
		log.Printf("CreateUser: Attempt to create user with existing %s: username '%s', email '%s'", conflictField, request.Body.Username, string(request.Body.Email))
		return api.CreateUser409JSONResponse{
			Code:    http.StatusConflict,
			Message: fmt.Sprintf("User with this %s already exists", conflictField),
		}, nil
	}

	// Hash the password
	hashedPassword, err := encryption.GeneratePasswordHash(request.Body.Password)
	if err != nil {
		log.Printf("CreateUser: Error hashing password for user '%s': %v", request.Body.Username, err)
		return api.CreateUser500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	// Create the user
	newUser := &db.User{
		Username: request.Body.Username,
		Email:    string(request.Body.Email), // Cast types.Email to string
		Password: hashedPassword,             // Corrected: Use Password field (GORM hook handles hashing if not already hashed)
	}

	// Corrected: s.userRepo instead of s.UserRepo
	err = s.userRepo.CreateUser(newUser)
	if err != nil {
		log.Printf("CreateUser: Error creating user '%s': %v", request.Body.Username, err)
		return api.CreateUser500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error",
		}, nil
	}

	log.Printf("CreateUser: Successfully created user: %s, email: %s, ID: %s", newUser.Username, newUser.Email, newUser.ID)
	apiUserResponse := dbUserToAPIUserResponse(newUser)
	return api.CreateUser201JSONResponse(apiUserResponse), nil
}

// ListUsers handles the HTTP request for listing all users.
// It implements the listUsers operation defined in the OpenAPI specification.
func (s *StrictApiServer) ListUsers(ctx context.Context, request api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	// Corrected: s.userRepo instead of s.UserRepo
	dbUsers, err := s.userRepo.GetAllUsers()
	if err != nil {
		log.Printf("ListUsers: Error fetching all users: %v", err)
		return api.ListUsers500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error while fetching user list",
		}, nil
	}

	apiUsers := make([]api.UserResponse, 0, len(dbUsers))
	for _, dbUser := range dbUsers { // Corrected: Iterate directly over the slice of pointers
		apiUsers = append(apiUsers, dbUserToAPIUserResponse(dbUser))
	}

	log.Printf("ListUsers: Successfully retrieved %d users", len(apiUsers))
	return api.ListUsers200JSONResponse(apiUsers), nil
}

// GetUserById handles the HTTP request for retrieving a user by their ID.
// It implements the getUserById operation defined in the OpenAPI specification.
func (s *StrictApiServer) GetUserById(ctx context.Context, request api.GetUserByIdRequestObject) (api.GetUserByIdResponseObject, error) {
	if request.UserId == "" { // UserId is a string (CUID)
		log.Printf("GetUserById: User ID is required and was not found in path")
		return api.GetUserById400JSONResponse{
			Code:    http.StatusBadRequest,
			Message: "User ID path parameter is required",
		}, nil
	}

	// Corrected: s.userRepo instead of s.UserRepo
	dbUser, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("GetUserById: User with ID '%s' not found: %v", request.UserId, err)
			return api.GetUserById404JSONResponse{
				Code:    http.StatusNotFound,
				Message: "User not found",
			}, nil
		}
		log.Printf("GetUserById: Error fetching user with ID '%s': %v", request.UserId, err)
		return api.GetUserById500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error while fetching user data",
		}, nil
	}

	// This check might be redundant if gorm.ErrRecordNotFound is always returned and handled above.
	// However, it can serve as a safeguard if FindUserByIdOrUsername could return (nil, nil) for "not found".
	if dbUser == nil {
		log.Printf("GetUserById: User with ID '%s' not found (dbUser is nil after non-error return)", request.UserId)
		return api.GetUserById404JSONResponse{
			Code:    http.StatusNotFound,
			Message: "User not found",
		}, nil
	}

	apiUserResponse := dbUserToAPIUserResponse(dbUser)
	log.Printf("GetUserById: Successfully retrieved user with ID '%s'", request.UserId)
	return api.GetUserById200JSONResponse(apiUserResponse), nil
}
