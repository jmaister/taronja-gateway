package handlers

import (
	"context"
	"errors" // Added for errors.Is
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/notification"
	openapi_types "github.com/oapi-codegen/runtime/types" // Added for openapi_types.Email
	"gorm.io/gorm"                                        // Added for gorm.ErrRecordNotFound
)

// dbUserToAPIUserResponse converts a db.User object to an API UserResponse object.
// It handles the conversion of ID to string and formats timestamps to RFC3339.
// Nullable fields (Name, Picture, Provider, CreatedAt, UpdatedAt) are converted
// to pointers, omitted (left nil) when the underlying value is unset.
func dbUserToAPIUserResponse(dbUser *db.User) api.UserResponse {
	// ID is already string in db.User and api.UserResponse
	// CreatedAt and UpdatedAt are time.Time in db.User (via the embedded
	// gorm.Model, populated by GORM on create/update) and *time.Time in
	// api.UserResponse.

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
	var createdAtPtr *time.Time
	if !dbUser.CreatedAt.IsZero() {
		createdAtPtr = &dbUser.CreatedAt
	}
	var updatedAtPtr *time.Time
	if !dbUser.UpdatedAt.IsZero() {
		updatedAtPtr = &dbUser.UpdatedAt
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
		Id:        dbUser.ID,
		Username:  dbUser.Username,
		Email:     emailPtr,
		Name:      namePtr,
		Picture:   picturePtr,
		Provider:  providerPtr,
		CreatedAt: createdAtPtr,
		UpdatedAt: updatedAtPtr,
	}
}

// withPhone adds a stored phone number to resp.
func withPhone(resp *api.UserResponse, phone *db.UserPhone) {
	if phone == nil || phone.Phone == "" {
		return
	}
	resp.Phone = &phone.Phone
	verified := phone.VerifiedAt != nil
	resp.PhoneVerified = &verified
}

// userResponse is dbUserToAPIUserResponse plus the user's phone number.
func (s *StrictApiServer) userResponse(dbUser *db.User) api.UserResponse {
	resp := dbUserToAPIUserResponse(dbUser)
	if s.userPhones != nil {
		phone, err := s.userPhones.Get(dbUser.ID)
		if err == nil {
			withPhone(&resp, phone)
		}
	}
	return resp
}

// CreateUser handles the HTTP request for creating a new user.
// It implements the createUser operation defined in the OpenAPI specification.
func (s *StrictApiServer) CreateUser(ctx context.Context, request api.CreateUserRequestObject) (api.CreateUserResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		log.Printf("CreateUser: rejected — caller is not an admin")
		return api.CreateUser401JSONResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

	// Validate input (basic check, OpenAPI spec should enforce most of this)
	if request.Body.Username == "" || string(request.Body.Email) == "" {
		log.Printf("CreateUser: Missing required fields.")
		return api.CreateUser400JSONResponse{
			Code:    http.StatusBadRequest,
			Message: "Username and email are required",
		}, nil
	}
	var phone string
	if request.Body.Phone != nil && *request.Body.Phone != "" {
		normalized, err := notification.NormalizePhone(*request.Body.Phone)
		if err != nil {
			return api.CreateUser400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
		}
		phone = normalized
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

	// Hash the password. A user without one (a contact who is only
	// notified) cannot log in with a password.
	var hashedPassword string
	if request.Body.Password != nil && *request.Body.Password != "" {
		hashedPassword, err = encryption.GeneratePasswordHash(*request.Body.Password)
		if err != nil {
			log.Printf("CreateUser: Error hashing password for user '%s': %v", request.Body.Username, err)
			return api.CreateUser500JSONResponse{
				Code:    http.StatusInternalServerError,
				Message: "Internal Server Error",
			}, nil
		}
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

	if phone != "" {
		if s.userPhones == nil {
			return api.CreateUser500JSONResponse{Code: http.StatusInternalServerError, Message: "Phone numbers are not available"}, nil
		}
		err = s.userPhones.Set(newUser.ID, phone, nil)
		if err != nil {
			log.Printf("CreateUser: Error storing phone for user '%s': %v", newUser.ID, err)
			return api.CreateUser500JSONResponse{Code: http.StatusInternalServerError, Message: "Internal Server Error"}, nil
		}
	}

	log.Printf("CreateUser: Successfully created user: %s, email: %s, ID: %s", newUser.Username, newUser.Email, newUser.ID)
	return api.CreateUser201JSONResponse(s.userResponse(newUser)), nil
}

// ListUsers handles the HTTP request for listing all users.
// It implements the listUsers operation defined in the OpenAPI specification.
func (s *StrictApiServer) ListUsers(ctx context.Context, request api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		log.Printf("ListUsers: rejected — caller is not an admin")
		return api.ListUsers401JSONResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

	// Corrected: s.userRepo instead of s.UserRepo
	dbUsers, err := s.userRepo.GetAllUsers()
	if err != nil {
		log.Printf("ListUsers: Error fetching all users: %v", err)
		return api.ListUsers500JSONResponse{
			Code:    http.StatusInternalServerError,
			Message: "Internal Server Error while fetching user list",
		}, nil
	}

	phones := map[string]db.UserPhone{}
	if s.userPhones != nil {
		ids := make([]string, 0, len(dbUsers))
		for _, u := range dbUsers {
			ids = append(ids, u.ID)
		}
		phones, err = s.userPhones.GetMany(ids)
		if err != nil {
			log.Printf("ListUsers: Error fetching phone numbers: %v", err)
			return api.ListUsers500JSONResponse{Code: http.StatusInternalServerError, Message: "Internal Server Error while fetching user list"}, nil
		}
	}
	apiUsers := make([]api.UserResponse, 0, len(dbUsers))
	for _, dbUser := range dbUsers { // Corrected: Iterate directly over the slice of pointers
		resp := dbUserToAPIUserResponse(dbUser)
		if phone, ok := phones[dbUser.ID]; ok {
			withPhone(&resp, &phone)
		}
		apiUsers = append(apiUsers, resp)
	}

	log.Printf("ListUsers: Successfully retrieved %d users", len(apiUsers))
	return api.ListUsers200JSONResponse(apiUsers), nil
}

// GetUserById handles the HTTP request for retrieving a user by their ID.
// It implements the getUserById operation defined in the OpenAPI specification.
func (s *StrictApiServer) GetUserById(ctx context.Context, request api.GetUserByIdRequestObject) (api.GetUserByIdResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		log.Printf("GetUserById: rejected — caller is not an admin")
		return api.GetUserById401JSONResponse{
			Code:    http.StatusUnauthorized,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

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

	apiUserResponse := s.userResponse(dbUser)
	log.Printf("GetUserById: Successfully retrieved user with ID '%s'", request.UserId)
	return api.GetUserById200JSONResponse(apiUserResponse), nil
}

// UpdateUser handles PATCH /api/users/{userId}: the name and phone number.
func (s *StrictApiServer) UpdateUser(ctx context.Context, request api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.UpdateUser401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized: Admin access required"}, nil
	}
	user, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.UpdateUser404JSONResponse{Code: http.StatusNotFound, Message: "User not found"}, nil
		}
		log.Printf("UpdateUser: Error finding user %s: %v", request.UserId, err)
		return api.UpdateUser500JSONResponse{Code: http.StatusInternalServerError, Message: "Internal Server Error"}, nil
	}
	if request.Body == nil {
		return api.UpdateUser200JSONResponse(s.userResponse(user)), nil
	}

	if request.Body.Phone != nil {
		if s.userPhones == nil {
			return api.UpdateUser500JSONResponse{Code: http.StatusInternalServerError, Message: "Phone numbers are not available"}, nil
		}
		if *request.Body.Phone == "" {
			err = s.userPhones.Delete(user.ID)
		} else {
			phone, normErr := notification.NormalizePhone(*request.Body.Phone)
			if normErr != nil {
				return api.UpdateUser400JSONResponse{Code: http.StatusBadRequest, Message: normErr.Error()}, nil
			}
			// A changed number is no longer the one the user may have proven.
			var verifiedAt *time.Time
			current, getErr := s.userPhones.Get(user.ID)
			if getErr == nil && current.Phone == phone {
				verifiedAt = current.VerifiedAt
			}
			err = s.userPhones.Set(user.ID, phone, verifiedAt)
		}
		if err != nil {
			log.Printf("UpdateUser: Error storing phone of %s: %v", user.ID, err)
			return api.UpdateUser500JSONResponse{Code: http.StatusInternalServerError, Message: "Internal Server Error"}, nil
		}
	}
	if request.Body.Name != nil {
		user.Name = *request.Body.Name
		err = s.userRepo.UpdateUser(user)
		if err != nil {
			log.Printf("UpdateUser: Error updating %s: %v", user.ID, err)
			return api.UpdateUser500JSONResponse{Code: http.StatusInternalServerError, Message: "Internal Server Error"}, nil
		}
	}
	return api.UpdateUser200JSONResponse(s.userResponse(user)), nil
}
