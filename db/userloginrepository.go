package db

// UserLoginRepository interface for abstracting user login database operations
type UserLoginRepository interface {
	// Find login method by provider and provider ID
	FindUserLoginByProvider(provider, providerId string) (*UserLogin, error)
	
	// Find all login methods for a user
	FindUserLoginsByUserID(userID string) ([]*UserLogin, error)
	
	// Create a new login method for a user
	CreateUserLogin(userLogin *UserLogin) error
	
	// Update an existing login method
	UpdateUserLogin(userLogin *UserLogin) error
	
	// Delete a login method
	DeleteUserLogin(id string) error
	
	// Deactivate a login method instead of deleting
	DeactivateUserLogin(id string) error
	
	// Find user by provider login (returns the associated user)
	FindUserByProviderLogin(provider, providerId string) (*User, error)
}
