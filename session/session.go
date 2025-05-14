package session

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

const TokenLength = 32
const SessionCookieName = "tg_session_token"

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// SessionKey is the key used to store session in context
const SessionKey contextKey = "session"

type SessionStore interface {
	GenerateKey() (string, error)
	Set(key string, value SessionObject) error
	Get(key string) (SessionObject, error)
	Delete(key string) error
	Validate(r *http.Request) (SessionObject, bool)
	GetSessionsByUserID(userID string) ([]SessionObject, error)
}

type SessionObject struct {
	UserID          string
	Username        string
	Email           string
	IsAuthenticated bool
	ValidUntil      time.Time
	Provider        string
	ClosedOn        *time.Time
	// Client information
	IPAddress    string
	UserAgent    string
	Browser      string
	OS           string
	DeviceType   string
	Referrer     string
	LastActivity time.Time
	SessionName  string
	GeoLocation  string
	CreatedFrom  string
	// Detailed geo information
	Latitude    float64
	Longitude   float64
	City        string
	Country     string
	CountryCode string
	Region      string
	Continent   string
	ZipCode     string
}

type MemorySessionStore struct {
	store map[string]SessionObject
}

func NewMemorySessionStore() SessionStore {
	store := make(map[string]SessionObject, 10)
	return &MemorySessionStore{
		store: store,
	}
}

func (s MemorySessionStore) GenerateKey() (string, error) {
	tokenBytes := make([]byte, TokenLength)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(tokenBytes), nil
}

func (s MemorySessionStore) Set(key string, value SessionObject) error {
	s.store[key] = value
	return nil
}

func (s MemorySessionStore) Get(key string) (SessionObject, error) {
	value, found := s.store[key]
	if !found {
		return SessionObject{}, errors.New("not found")
	}

	// Check if session is closed
	if value.ClosedOn != nil && !value.ClosedOn.IsZero() {
		return SessionObject{}, errors.New("session has been closed")
	}

	return value, nil
}

func (s MemorySessionStore) Delete(key string) error {
	session, exists := s.store[key]
	if exists {
		now := time.Now()
		session.ClosedOn = &now
		s.store[key] = session
	}
	return nil
}

func (s MemorySessionStore) Validate(r *http.Request) (SessionObject, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return SessionObject{}, false
	}

	sessionObject, exists := s.store[cookie.Value]
	if !exists {
		return SessionObject{}, false
	}

	now := time.Now()
	// Check if session is expired or closed
	if sessionObject.ValidUntil.Before(now) || (sessionObject.ClosedOn != nil && !sessionObject.ClosedOn.IsZero()) {
		if sessionObject.ClosedOn == nil {
			// Mark as closed if not already
			sessionObject.ClosedOn = &now
			s.store[cookie.Value] = sessionObject
		}
		return SessionObject{}, false
	}

	// Update last activity time and client info if changed
	sessionObject.LastActivity = now

	// Update client information that might have changed
	ipAddress := r.RemoteAddr
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ipAddress = forwardedFor
	}
	sessionObject.IPAddress = ipAddress

	// Update the session in the store
	s.store[cookie.Value] = sessionObject

	return sessionObject, true
}

// GetSessionsByUserID retrieves all active sessions for a specific user from memory store
func (s MemorySessionStore) GetSessionsByUserID(userID string) ([]SessionObject, error) {
	var sessions []SessionObject

	for _, session := range s.store {
		// Only include sessions that belong to this user and haven't been closed
		if session.UserID == userID && (session.ClosedOn == nil || session.ClosedOn.IsZero()) {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// ExtractClientInfo extracts client information from an HTTP request
// and adds it to a SessionObject
func ExtractClientInfo(r *http.Request, obj *SessionObject) {
	// Get IP address
	ipAddress := r.RemoteAddr
	// Check for forwarded IP if behind proxy
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ipAddress = forwardedFor
	}
	obj.IPAddress = ipAddress

	// Get User Agent information
	userAgent := r.UserAgent()
	obj.UserAgent = userAgent

	// Extract browser and OS information from user agent
	// Note: In a production environment, you might want to use a library
	// like github.com/mssola/user_agent for more accurate parsing
	if userAgent != "" {
		// Simplified browser detection
		switch {
		case contains(userAgent, "Chrome") && !contains(userAgent, "Edg/"):
			obj.Browser = "Chrome"
		case contains(userAgent, "Firefox"):
			obj.Browser = "Firefox"
		case contains(userAgent, "Safari") && !contains(userAgent, "Chrome"):
			obj.Browser = "Safari"
		case contains(userAgent, "Edg/"):
			obj.Browser = "Edge"
		case contains(userAgent, "MSIE") || contains(userAgent, "Trident/"):
			obj.Browser = "Internet Explorer"
		default:
			obj.Browser = "Other"
		}

		// Simplified OS detection
		switch {
		case contains(userAgent, "Windows"):
			obj.OS = "Windows"
		case contains(userAgent, "Mac OS X"):
			obj.OS = "macOS"
		case contains(userAgent, "Linux"):
			obj.OS = "Linux"
		case contains(userAgent, "Android"):
			obj.OS = "Android"
		case contains(userAgent, "iOS") || contains(userAgent, "iPhone") || contains(userAgent, "iPad"):
			obj.OS = "iOS"
		default:
			obj.OS = "Other"
		}

		// Device type detection
		switch {
		case contains(userAgent, "Mobile"):
			obj.DeviceType = "Mobile"
		case contains(userAgent, "Tablet") || contains(userAgent, "iPad"):
			obj.DeviceType = "Tablet"
		default:
			obj.DeviceType = "Desktop"
		}
	}

	// Get referrer
	obj.Referrer = r.Referer()

	// Set last activity to current time
	obj.LastActivity = time.Now()

	// Default to Web interface
	obj.CreatedFrom = "Web"
	// Detect API requests based on Accept header
	if r.Header.Get("Accept") == "application/json" {
		obj.CreatedFrom = "API"
	}

	// Extract just the IP without port if it has one
	ip := ipAddress
	if idx := strings.Index(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}

	// Get detailed geo data from IP address and store in session object
	geoData, err := GetGeoDataFromIP(ip)
	if err == nil {
		obj.Latitude = geoData.Latitude
		obj.Longitude = geoData.Longitude
		obj.City = geoData.City
		obj.Country = geoData.Country
		obj.CountryCode = geoData.CountryCode
		obj.Region = geoData.Region
		obj.Continent = geoData.Continent
		obj.ZipCode = geoData.ZipCode
		obj.GeoLocation = geoData.FormattedLoc
	} else {
		log.Printf("Failed to get geolocation for IP %s: %v", ip, err)
		obj.GeoLocation = "Unknown"
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
