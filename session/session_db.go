package session

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// SessionStoreDB implements the SessionStore interface using a database
type SessionStoreDB struct {
	dbConn *gorm.DB
}

// NewSessionStoreDB creates a new SessionStoreDB with the provided database connection
func NewSessionStoreDB() SessionStore {
	return &SessionStoreDB{
		dbConn: db.GetConnection(),
	}
}

func (s *SessionStoreDB) GenerateKey() (string, error) {
	tokenBytes := make([]byte, TokenLength)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(tokenBytes), nil
}

func (s *SessionStoreDB) Set(key string, value SessionObject) error {
	session := db.Session{
		Token:           key,
		UserID:          value.UserID, // Store the user ID for future reference
		Username:        value.Username,
		Email:           value.Email,
		IsAuthenticated: value.IsAuthenticated,
		ValidUntil:      value.ValidUntil,
		Provider:        value.Provider,
	}

	result := s.dbConn.Create(&session)
	return result.Error
}

func (s *SessionStoreDB) Get(key string) (SessionObject, error) {
	var session db.Session
	result := s.dbConn.Where("token = ?", key).First(&session)
	if result.Error != nil {
		return SessionObject{}, errors.New("session not found")
	}
	return SessionObject{
		UserID:          session.UserID,
		Username:        session.Username,
		Email:           session.Email,
		IsAuthenticated: session.IsAuthenticated,
		ValidUntil:      session.ValidUntil,
		Provider:        session.Provider,
	}, nil
}

func (s *SessionStoreDB) Delete(key string) error {
	result := s.dbConn.Where("token = ?", key).Delete(&db.Session{})
	return result.Error
}

func (s *SessionStoreDB) Validate(r *http.Request) (SessionObject, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return SessionObject{}, false
	}

	var session db.Session
	result := s.dbConn.Where("token = ?", cookie.Value).First(&session)
	if result.Error != nil {
		return SessionObject{}, false
	}

	if session.ValidUntil.Before(time.Now()) {
		// Session expired, delete it
		s.dbConn.Delete(&session)
		return SessionObject{}, false
	}
	return SessionObject{
		UserID:          session.UserID,
		Username:        session.Username,
		Email:           session.Email,
		IsAuthenticated: session.IsAuthenticated,
		ValidUntil:      session.ValidUntil,
		Provider:        session.Provider,
	}, true
}
