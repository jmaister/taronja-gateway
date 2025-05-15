package db_test

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
	store := db.NewSessionRepositoryDB()

	// Generate a session token
	token, err := store.GenerateToken()
	if err != nil {
		t.Fatalf("Error generating session token: %v", err)
	}

	// Create session object
	sessionData := db.Session{
		UserID:          "test-user-id",
		Username:        "test-user",
		Email:           "test@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(time.Hour),
		Provider:        "test-provider",
	}

	// Test CreateSession
	err = store.CreateSession(token, &sessionData)
	if err != nil {
		t.Fatalf("Error storing session: %v", err)
	}

	// Test GetSessionByToken
	retrievedSession, err := store.GetSessionByToken(token)
	if err != nil {
		t.Fatalf("Error retrieving session: %v", err)
	}
	if retrievedSession == nil {
		t.Fatalf("Retrieved session is nil")
	}

	// Check if retrieved session matches the original
	if retrievedSession.UserID != sessionData.UserID {
		t.Errorf("Expected UserID %s, got %s", sessionData.UserID, retrievedSession.UserID)
	}
	if retrievedSession.Username != sessionData.Username {
		t.Errorf("Expected Username %s, got %s", sessionData.Username, retrievedSession.Username)
	}
	if retrievedSession.Email != sessionData.Email {
		t.Errorf("Expected Email %s, got %s", sessionData.Email, retrievedSession.Email)
	}
	if retrievedSession.IsAuthenticated != sessionData.IsAuthenticated {
		t.Errorf("Expected IsAuthenticated %t, got %t", sessionData.IsAuthenticated, retrievedSession.IsAuthenticated)
	}
	if retrievedSession.Provider != sessionData.Provider {
		t.Errorf("Expected Provider %s, got %s", sessionData.Provider, retrievedSession.Provider)
	}

	// Test DeleteSession
	err = store.DeleteSession(token)
	if err != nil {
		t.Fatalf("Error deleting session: %v", err)
	}

	// Verify session was deleted
	deletedSession, errGet := store.GetSessionByToken(token)
	if errGet == nil && deletedSession != nil { // A closed session might return an error or nil, nil. If it's found and not closed, it's an error.
		// The current GetSessionByToken returns an error for closed sessions.
		// So, if errGet is nil, it means it was found and not closed, which is unexpected.
		// Or, if it's found but marked as closed, GetSessionByToken should return an error.
		t.Error("Expected an error or nil session when getting deleted session, but got a session or no error")
	}
	// More robust check: DeleteSession soft deletes. GetSessionByToken should return an error for closed sessions.
	if errGet == nil {
		t.Error("Expected an error when getting a deleted (closed) session, got nil")
	}

}

func TestSessionStoreDBValidateSession(t *testing.T) {
	// Initialize the test DB
	db.InitForTest()

	// Create a new SessionStoreDB
	repo := db.NewSessionRepositoryDB()

	// Generate a session token for a valid session
	validToken, err := repo.GenerateToken()
	if err != nil {
		t.Fatalf("Error generating session token: %v", err)
	}

	// Create session object with future expiry (valid)
	validSessionData := db.Session{
		UserID:          "valid-user-id",
		Username:        "valid-user",
		Email:           "valid@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(time.Hour),
		Provider:        "test-provider",
	}

	// Store the valid session
	err = repo.CreateSession(validToken, &validSessionData)
	if err != nil {
		t.Fatalf("Error storing valid session: %v", err)
	}

	// Generate a session token for an expired session
	expiredToken, err := repo.GenerateToken()
	if err != nil {
		t.Fatalf("Error generating expired session token: %v", err)
	}

	expiredSessionData := db.Session{
		UserID:          "expired-user-id",
		Username:        "expired-user",
		Email:           "expired@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(-time.Hour), // Set to 1 hour in the past
		Provider:        "test-provider",
	}

	// Store the expired session
	err = repo.CreateSession(expiredToken, &expiredSessionData)
	if err != nil {
		t.Fatalf("Error storing expired session: %v", err)
	}

	// Test 1: Request with valid session cookie
	r1 := httptest.NewRequest("GET", "http://example.com", nil)
	r1.AddCookie(&http.Cookie{
		Name:  db.SessionCookieName, // Using session.SessionCookieName
		Value: validToken,
	})

	retrievedSess1, isValid1 := repo.ValidateSession(r1)
	if !isValid1 {
		t.Error("Expected valid session, got invalid")
	}
	if retrievedSess1 == nil {
		t.Fatalf("Expected session object, got nil for valid session")
	}
	if retrievedSess1.UserID != validSessionData.UserID {
		t.Errorf("Expected UserID %s, got %s", validSessionData.UserID, retrievedSess1.UserID)
	}
	if retrievedSess1.Username != validSessionData.Username {
		t.Errorf("Expected Username %s, got %s", validSessionData.Username, retrievedSess1.Username)
	}

	// Test 2: Request with expired session cookie
	r2 := httptest.NewRequest("GET", "http://example.com", nil)
	r2.AddCookie(&http.Cookie{
		Name:  db.SessionCookieName, // Using session.SessionCookieName
		Value: expiredToken,
	})

	_, isValid2 := repo.ValidateSession(r2)
	if isValid2 {
		t.Error("Expected invalid session for expired token, got valid")
	}

	// Test 3: Request with non-existent session cookie
	r3 := httptest.NewRequest("GET", "http://example.com", nil)
	r3.AddCookie(&http.Cookie{
		Name:  db.SessionCookieName, // Using session.SessionCookieName
		Value: "non-existent-session-token",
	})

	_, isValid3 := repo.ValidateSession(r3)
	if isValid3 {
		t.Error("Expected invalid session for non-existent token, got valid")
	}

	// Test 4: Request with no session cookie
	r4 := httptest.NewRequest("GET", "http://example.com", nil)

	_, isValid4 := repo.ValidateSession(r4)
	if isValid4 {
		t.Error("Expected invalid session for request with no cookie, got valid")
	}
}

func TestSessionStoreDBCloseSession(t *testing.T) {
	// Initialize the test DB
	db.InitForTest()

	// Create a new SessionStoreDB
	repo := db.NewSessionRepositoryDB()

	// Generate a session token
	token, err := repo.GenerateToken()
	if err != nil {
		t.Fatalf("Error generating session token: %v", err)
	}

	// Create session object
	sessionData := db.Session{
		UserID:          "close-user-id",
		Username:        "close-user",
		Email:           "close@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(time.Hour),
		Provider:        "test-provider",
	}

	// Store the session
	err = repo.CreateSession(token, &sessionData)
	if err != nil {
		t.Fatalf("Error storing session: %v", err)
	}

	// Close the session
	err = repo.CloseSession(token)
	if err != nil {
		t.Fatalf("Error closing session: %v", err)
	}

	// Try to get the closed session (should return error and nil)
	closedSession, err := repo.GetSessionByToken(token)
	if err == nil || closedSession != nil {
		t.Error("Expected error and nil session when getting closed session, but got no error or non-nil session")
	}
	if err != nil && err.Error() != "session has been closed" {
		t.Errorf("Expected error 'session has been closed', got: %v", err)
	}

	// Try closing already closed session (should return specific error)
	err = repo.CloseSession(token)
	if err == nil {
		t.Error("Expected error when closing already closed session, got nil")
	} else if err.Error() != "session already closed or not found" {
		t.Errorf("Expected error 'session already closed or not found', got: %v", err)
	}
}
