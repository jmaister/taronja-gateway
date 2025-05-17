package db

// SessionRepository defines the interface for session persistence and operations.
type SessionRepository interface {
	CreateSession(token string, session *Session) error // Changed from CreateSession(session *Session) to include token
	FindSessionByToken(token string) (*Session, error)
	UpdateSession(session *Session) error
	GetSessionsByUserID(userID string) ([]Session, error)
	CloseSession(token string) error
}
