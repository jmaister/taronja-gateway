// filepath: c:\dev\workspace\taronja-gateway\db\mockdb.go
package db

// UserRepository interface for abstracting user database operations
type UserRepository interface {
	FindUserByIdOrUsername(id, username, email string) (*User, error)
	CreateUser(user *User) error
	GetAllUsers() ([]*User, error)
	UpdateUser(user *User) error
	DeleteUser(id string) error
	EnsureAdminUser(username, email, password string) error
}
