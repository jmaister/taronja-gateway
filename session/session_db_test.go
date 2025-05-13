package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
)

func TestSessionStoreDB(t *testing.T) {
	// Initialize the test DB
	db.InitForTest()

	// Create a new SessionStoreDB
	store := NewSessionStoreDB()

	// Generate a session key
	key, err := store.GenerateKey()
	if err != nil {
		t.Fatalf("Error generating session key: %v", err)
	}

	// Create session object
	session := SessionObject{
		UserID:          "test-user-id",
		Username:        "test-user",
		Email:           "test@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(time.Hour),
		Provider:        "test-provider",
	}

	// Test Set
	err = store.Set(key, session)
	if err != nil {
		t.Fatalf("Error storing session: %v", err)
	}

	// Test Get
	retrievedSession, err := store.Get(key)
	if err != nil {
		t.Fatalf("Error retrieving session: %v", err)
	}

	// Check if retrieved session matches the original
	if retrievedSession.UserID != session.UserID {
		t.Errorf("Expected UserID %s, got %s", session.UserID, retrievedSession.UserID)
	}
	if retrievedSession.Username != session.Username {
		t.Errorf("Expected Username %s, got %s", session.Username, retrievedSession.Username)
	}
	if retrievedSession.Email != session.Email {
		t.Errorf("Expected Email %s, got %s", session.Email, retrievedSession.Email)
	}
	if retrievedSession.IsAuthenticated != session.IsAuthenticated {
		t.Errorf("Expected IsAuthenticated %t, got %t", session.IsAuthenticated, retrievedSession.IsAuthenticated)
	}
	if retrievedSession.Provider != session.Provider {
		t.Errorf("Expected Provider %s, got %s", session.Provider, retrievedSession.Provider)
	}

	// Test Delete
	err = store.Delete(key)
	if err != nil {
		t.Fatalf("Error deleting session: %v", err)
	}

	// Verify session was deleted
	_, err = store.Get(key)
	if err == nil {
		t.Error("Expected an error when getting deleted session, got nil")
	}
}

func TestSessionStoreDBValidate(t *testing.T) {
	// Initialize the test DB
	db.InitForTest()

	// Create a new SessionStoreDB
	store := NewSessionStoreDB()

	// Generate a session key
	key, err := store.GenerateKey()
	if err != nil {
		t.Fatalf("Error generating session key: %v", err)
	}

	// Create session object with future expiry (valid)
	validSession := SessionObject{
		UserID:          "valid-user-id",
		Username:        "valid-user",
		Email:           "valid@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(time.Hour),
		Provider:        "test-provider",
	}

	// Store the valid session
	err = store.Set(key, validSession)
	if err != nil {
		t.Fatalf("Error storing valid session: %v", err)
	}

	// Create expired session object
	expiredKey, err := store.GenerateKey()
	if err != nil {
		t.Fatalf("Error generating expired session key: %v", err)
	}

	expiredSession := SessionObject{
		UserID:          "expired-user-id",
		Username:        "expired-user",
		Email:           "expired@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(-time.Hour), // Set to 1 hour in the past
		Provider:        "test-provider",
	}

	// Store the expired session
	err = store.Set(expiredKey, expiredSession)
	if err != nil {
		t.Fatalf("Error storing expired session: %v", err)
	}

	// Test 1: Request with valid session cookie
	r1 := httptest.NewRequest("GET", "http://example.com", nil)
	r1.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: key,
	})

	session1, isValid1 := store.Validate(r1)
	if !isValid1 {
		t.Error("Expected valid session, got invalid")
	}

	if session1.UserID != validSession.UserID {
		t.Errorf("Expected UserID %s, got %s", validSession.UserID, session1.UserID)
	}

	if session1.Username != validSession.Username {
		t.Errorf("Expected Username %s, got %s", validSession.Username, session1.Username)
	}

	// Test 2: Request with expired session cookie
	r2 := httptest.NewRequest("GET", "http://example.com", nil)
	r2.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: expiredKey,
	})

	_, isValid2 := store.Validate(r2)
	if isValid2 {
		t.Error("Expected invalid session for expired token, got valid")
	}

	// Test 3: Request with non-existent session cookie
	r3 := httptest.NewRequest("GET", "http://example.com", nil)
	r3.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: "non-existent-session-key",
	})

	_, isValid3 := store.Validate(r3)
	if isValid3 {
		t.Error("Expected invalid session for non-existent token, got valid")
	}

	// Test 4: Request with no session cookie
	r4 := httptest.NewRequest("GET", "http://example.com", nil)

	_, isValid4 := store.Validate(r4)
	if isValid4 {
		t.Error("Expected invalid session for request with no cookie, got valid")
	}
}
