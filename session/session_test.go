package session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
)

var testSessionRepo db.SessionRepository

// TestMain sets up the test database and repository.
func TestMain(m *testing.M) {
	db.InitForTest()                              // Initialize in-memory DB for tests
	testSessionRepo = db.NewSessionRepositoryDB() // Use the actual DB-backed repository from the db package
	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestGenerateToken(t *testing.T) {
	token, err := testSessionRepo.GenerateToken()
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Len(t, token, 44) // base64 encoded 32 bytes (TokenLength)
}

func TestCreateAndGetSession(t *testing.T) {
	token, _ := testSessionRepo.GenerateToken()
	userID := "test-user-id"
	sessionData := db.NewSession(nil, &db.User{ID: userID, Username: "testuser", Email: "test@example.com"}, "test", time.Hour)
	// sessionData.Token is set by CreateSession or should be set before if repo expects it.
	// Our CreateSession now takes token and sets it.

	err := testSessionRepo.CreateSession(token, sessionData)
	assert.NoError(t, err)

	retrievedSession, err := testSessionRepo.GetSessionByToken(token)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSession)
	assert.Equal(t, token, retrievedSession.Token)
	assert.Equal(t, userID, retrievedSession.UserID)
	assert.Equal(t, "testuser", retrievedSession.Username)

	// Test Get non-existent session
	_, err = testSessionRepo.GetSessionByToken("non-existent-token")
	// Not found should return (nil, nil)
	assert.Nil(t, err)
}

func TestUpdateSession(t *testing.T) {
	token, _ := testSessionRepo.GenerateToken()
	sessionData := db.NewSession(nil, &db.User{ID: "update-user", Username: "original"}, "test", time.Hour)

	err := testSessionRepo.CreateSession(token, sessionData)
	assert.NoError(t, err)

	retrievedSession, _ := testSessionRepo.GetSessionByToken(token)
	assert.NotNil(t, retrievedSession)
	retrievedSession.Username = "updated_username"
	retrievedSession.LastActivity = time.Now().Add(time.Minute) // Simulate activity

	err = testSessionRepo.UpdateSession(retrievedSession)
	assert.NoError(t, err)

	updatedSession, _ := testSessionRepo.GetSessionByToken(token)
	assert.NotNil(t, updatedSession)
	assert.Equal(t, "updated_username", updatedSession.Username)
	assert.True(t, updatedSession.LastActivity.After(sessionData.LastActivity))
}

func TestDeleteSession(t *testing.T) { // DeleteSession now means CloseSession
	token, _ := testSessionRepo.GenerateToken()
	sessionData := db.NewSession(nil, &db.User{ID: "delete-user"}, "test", time.Hour)
	_ = testSessionRepo.CreateSession(token, sessionData)

	err := testSessionRepo.DeleteSession(token)
	assert.NoError(t, err)

	// Try to get the session - should fail as it's closed
	closedSession, err := testSessionRepo.GetSessionByToken(token)
	assert.Error(t, err) // Expecting "session has been closed"
	assert.Nil(t, closedSession)
	if err != nil {
		assert.Equal(t, "session has been closed", err.Error())
	}
}

func TestCloseSession(t *testing.T) {
	token, _ := testSessionRepo.GenerateToken()
	sessionData := db.NewSession(nil, &db.User{ID: "close-user"}, "test", time.Hour)
	_ = testSessionRepo.CreateSession(token, sessionData)

	err := testSessionRepo.CloseSession(token)
	assert.NoError(t, err)

	closedSession, err := testSessionRepo.GetSessionByToken(token)
	assert.Error(t, err)
	assert.Nil(t, closedSession)
	if err != nil {
		assert.Equal(t, "session has been closed", err.Error())
	}

	// Try closing already closed session
	err = testSessionRepo.CloseSession(token)
	assert.Error(t, err) // Should error as it's already closed or not found for update
	if err != nil {
		assert.Equal(t, "session already closed or not found", err.Error())
	}
}

func TestValidateSession(t *testing.T) {
	// Valid session
	validToken, _ := testSessionRepo.GenerateToken()
	validSessionData := db.NewSession(nil, &db.User{ID: "valid-user", Username: "validator"}, "test", time.Hour)
	_ = testSessionRepo.CreateSession(validToken, validSessionData)

	reqValid := httptest.NewRequest("GET", "/", nil)
	reqValid.AddCookie(&http.Cookie{Name: db.SessionCookieName, Value: validToken})

	s, isValid := testSessionRepo.ValidateSession(reqValid)
	assert.True(t, isValid)
	assert.NotNil(t, s)
	if s != nil {
		assert.Equal(t, validToken, s.Token)
		// LastActivity should be updated to now, so it should be >= CreatedAt
		assert.True(t, !s.LastActivity.Before(validSessionData.CreatedAt), "LastActivity should be updated")
	}

	// Expired session
	expiredToken, _ := testSessionRepo.GenerateToken()
	expiredSessionData := db.NewSession(reqValid, &db.User{ID: "expired-user"}, "test", -time.Hour) // Expired
	_ = testSessionRepo.CreateSession(expiredToken, expiredSessionData)

	reqExpired := httptest.NewRequest("GET", "/", nil)
	reqExpired.AddCookie(&http.Cookie{Name: db.SessionCookieName, Value: expiredToken})
	_, isExpiredValid := testSessionRepo.ValidateSession(reqExpired)
	assert.False(t, isExpiredValid)

	// Ensure expired session is marked closed
	closedExpiredSession, err := testSessionRepo.GetSessionByToken(expiredToken)
	assert.Error(t, err) // Should be "session has been closed"
	assert.Nil(t, closedExpiredSession)
	if err != nil {
		assert.Equal(t, "session has been closed", err.Error())
	}

	// No cookie
	reqNoCookie := httptest.NewRequest("GET", "/", nil)
	_, isNoCookieValid := testSessionRepo.ValidateSession(reqNoCookie)
	assert.False(t, isNoCookieValid)

	// Invalid token
	reqInvalidToken := httptest.NewRequest("GET", "/", nil)
	reqInvalidToken.AddCookie(&http.Cookie{Name: db.SessionCookieName, Value: "invalid-token-value"})
	_, isInvalidTokenValid := testSessionRepo.ValidateSession(reqInvalidToken)
	assert.False(t, isInvalidTokenValid)
}

func TestGetSessionsByUserID(t *testing.T) {
	userID := "user-with-multiple-sessions"
	// Clean up previous sessions for this user for a clean test run
	// This might require a direct DB op or a helper if not careful with test data

	s1Token, _ := testSessionRepo.GenerateToken()
	s1Data := db.NewSession(nil, &db.User{ID: userID}, "test", time.Hour)
	_ = testSessionRepo.CreateSession(s1Token, s1Data)

	s2Token, _ := testSessionRepo.GenerateToken()
	s2Data := db.NewSession(nil, &db.User{ID: userID}, "test", 2*time.Hour)
	_ = testSessionRepo.CreateSession(s2Token, s2Data)

	// Expired session for same user
	expToken, _ := testSessionRepo.GenerateToken()
	expData := db.NewSession(nil, &db.User{ID: userID}, "test", -time.Hour)
	_ = testSessionRepo.CreateSession(expToken, expData)

	// Closed session for same user
	closedToken, _ := testSessionRepo.GenerateToken()
	closedData := db.NewSession(nil, &db.User{ID: userID}, "test", time.Hour)
	_ = testSessionRepo.CreateSession(closedToken, closedData)
	_ = testSessionRepo.CloseSession(closedToken)

	// Session for a different user
	otherUserToken, _ := testSessionRepo.GenerateToken()
	otherUserData := db.NewSession(nil, &db.User{ID: "other-user-id"}, "test", time.Hour)
	_ = testSessionRepo.CreateSession(otherUserToken, otherUserData)

	userSessions, err := testSessionRepo.GetSessionsByUserID(userID)
	assert.NoError(t, err)
	assert.Len(t, userSessions, 4, "Should retrieve all sessions (active, expired, closed) for the user")

	tokensFound := map[string]bool{
		s1Token:     false,
		s2Token:     false,
		expToken:    false,
		closedToken: false,
	}

	for _, s := range userSessions {
		assert.Equal(t, userID, s.UserID)
		tokensFound[s.Token] = true
		// Optionally, check session state
		if s.Token == closedToken {
			assert.NotNil(t, s.ClosedOn, "Closed session should have ClosedOn set")
		} else if s.Token == expToken {
			assert.True(t, s.ValidUntil.Before(time.Now()), "Expired session should have ValidUntil in the past")
		} else {
			assert.Nil(t, s.ClosedOn, "Active session should not have ClosedOn set")
			assert.True(t, s.ValidUntil.After(time.Now()), "Active session should have ValidUntil in the future")
		}
	}
	for token, found := range tokensFound {
		assert.True(t, found, "Session with token %s not found", token)
	}
}
