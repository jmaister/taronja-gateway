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
		ClosedOn:        value.ClosedOn,
		// Client information
		IPAddress:    value.IPAddress,
		UserAgent:    value.UserAgent,
		Browser:      value.Browser,
		OS:           value.OS,
		DeviceType:   value.DeviceType,
		Referrer:     value.Referrer,
		LastActivity: value.LastActivity,
		SessionName:  value.SessionName,
		GeoLocation:  value.GeoLocation,
		CreatedFrom:  value.CreatedFrom,
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

	// Check if the session has been closed
	if session.ClosedOn != nil && !session.ClosedOn.IsZero() {
		return SessionObject{}, errors.New("session has been closed")
	}

	return SessionObject{
		UserID:          session.UserID,
		Username:        session.Username,
		Email:           session.Email,
		IsAuthenticated: session.IsAuthenticated,
		ValidUntil:      session.ValidUntil,
		Provider:        session.Provider,
		ClosedOn:        session.ClosedOn,
		// Client information
		IPAddress:    session.IPAddress,
		UserAgent:    session.UserAgent,
		Browser:      session.Browser,
		OS:           session.OS,
		DeviceType:   session.DeviceType,
		Referrer:     session.Referrer,
		LastActivity: session.LastActivity,
		SessionName:  session.SessionName,
		GeoLocation:  session.GeoLocation,
		CreatedFrom:  session.CreatedFrom,
	}, nil
}

func (s *SessionStoreDB) Delete(key string) error {
	now := time.Now()
	result := s.dbConn.Model(&db.Session{}).Where("token = ?", key).Update("closed_on", now)
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

	// Check if session is expired or has been closed
	now := time.Now()
	if session.ValidUntil.Before(now) || (session.ClosedOn != nil && !session.ClosedOn.IsZero()) {
		// Session expired or closed, mark it as closed if not already
		if session.ClosedOn == nil {
			s.dbConn.Model(&session).Update("closed_on", now)
		}
		return SessionObject{}, false
	}

	// Update last activity time
	s.dbConn.Model(&session).Updates(map[string]interface{}{
		"last_activity": now,
		"ip_address":    r.RemoteAddr, // Or use X-Forwarded-For if available
	})

	sessionObj := SessionObject{
		UserID:          session.UserID,
		Username:        session.Username,
		Email:           session.Email,
		IsAuthenticated: session.IsAuthenticated,
		ValidUntil:      session.ValidUntil,
		Provider:        session.Provider,
		ClosedOn:        session.ClosedOn,
		IPAddress:       session.IPAddress,
		UserAgent:       session.UserAgent,
		Browser:         session.Browser,
		OS:              session.OS,
		DeviceType:      session.DeviceType,
		Referrer:        session.Referrer,
		LastActivity:    now,
		SessionName:     session.SessionName,
		GeoLocation:     session.GeoLocation,
		CreatedFrom:     session.CreatedFrom,
	}

	return sessionObj, true
}

// GetSessionsByUserID retrieves all sessions, open and closed, for a specific user
func (s *SessionStoreDB) GetSessionsByUserID(userID string) ([]SessionObject, error) {
	var dbSessions []db.Session
	result := s.dbConn.Where("user_id = ?", userID).Find(&dbSessions)
	if result.Error != nil {
		return nil, result.Error
	}

	sessions := make([]SessionObject, 0, len(dbSessions))
	for _, session := range dbSessions {
		sessions = append(sessions, SessionObject{
			UserID:          session.UserID,
			Username:        session.Username,
			Email:           session.Email,
			IsAuthenticated: session.IsAuthenticated,
			ValidUntil:      session.ValidUntil,
			Provider:        session.Provider,
			ClosedOn:        session.ClosedOn,
			// Client information
			IPAddress:    session.IPAddress,
			UserAgent:    session.UserAgent,
			Browser:      session.Browser,
			OS:           session.OS,
			DeviceType:   session.DeviceType,
			Referrer:     session.Referrer,
			LastActivity: session.LastActivity,
			SessionName:  session.SessionName,
			GeoLocation:  session.GeoLocation,
			CreatedFrom:  session.CreatedFrom,
		})
	}

	return sessions, nil
}
