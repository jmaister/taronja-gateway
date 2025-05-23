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
var testSessionStore SessionStore // Use the interface

// TestMain sets up the test database and repository.
func TestMain(m *testing.M) {
	db.InitForTest()                                    // Initialize in-memory DB for tests
	testSessionRepo = db.NewSessionRepositoryDB()       // Use the actual DB-backed repository from the db package
	testSessionStore = NewSessionStore(testSessionRepo) // Initialize SessionStore
	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken() // Use package-level GenerateToken
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Len(t, token, 44) // base64 encoded 32 bytes (TokenLength)
}

func TestNewAndValidateSession(t *testing.T) {
	user := &db.User{ID: "test-user-id", Username: "testuser", Email: "test@example.com"}
	req := httptest.NewRequest("GET", "/", nil) // Mock request for NewSession

	createdSession, err := testSessionStore.NewSession(req, user, "test-provider", time.Hour)
	assert.NoError(t, err)
	assert.NotNil(t, createdSession)
	assert.NotEmpty(t, createdSession.Token)
	assert.Equal(t, user.ID, createdSession.UserID)
	assert.Equal(t, user.Username, createdSession.Username)
	assert.True(t, createdSession.IsAuthenticated)
	assert.WithinDuration(t, time.Now().Add(time.Hour), createdSession.ValidUntil, time.Second*5) // Check validity

	// Validate the created session
	validateReq := httptest.NewRequest("GET", "/", nil)
	validateReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: createdSession.Token})

	retrievedSession, isValid := testSessionStore.ValidateSession(validateReq)
	assert.True(t, isValid)
	assert.NotNil(t, retrievedSession)
	assert.Equal(t, createdSession.Token, retrievedSession.Token)
	assert.Equal(t, user.ID, retrievedSession.UserID)

	// Test Get non-existent session via ValidateSession
	invalidReq := httptest.NewRequest("GET", "/", nil)
	invalidReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "non-existent-token"})
	_, isValid = testSessionStore.ValidateSession(invalidReq)
	assert.False(t, isValid)
}

func TestUpdateSessionLastActivityOnValidate(t *testing.T) {
	user := &db.User{ID: "update-user", Username: "original"}
	req := httptest.NewRequest("GET", "/", nil)

	sessionData, err := testSessionStore.NewSession(req, user, "test", time.Hour)
	assert.NoError(t, err)
	assert.NotNil(t, sessionData)

	// Retrieve the session directly from repo to check initial LastActivity
	// This is a bit of a workaround as SessionStore doesn't expose GetSessionByToken
	initialSessionState, err := testSessionRepo.FindSessionByToken(sessionData.Token)
	assert.NoError(t, err)
	assert.NotNil(t, initialSessionState)
	originalLastActivity := initialSessionState.LastActivity

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Validate the session, which should update LastActivity
	validateReq := httptest.NewRequest("GET", "/", nil)
	validateReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionData.Token})
	validatedSession, isValid := testSessionStore.ValidateSession(validateReq)
	assert.True(t, isValid)
	assert.NotNil(t, validatedSession)

	// Check that LastActivity was updated
	assert.True(t, validatedSession.LastActivity.After(originalLastActivity), "LastActivity should be updated after validation")

	// Verify in the DB as well
	updatedSessionFromRepo, err := testSessionRepo.FindSessionByToken(sessionData.Token)
	assert.NoError(t, err)
	assert.NotNil(t, updatedSessionFromRepo)
	assert.True(t, updatedSessionFromRepo.LastActivity.After(originalLastActivity))
	assert.Equal(t, validatedSession.LastActivity.UnixNano()/1e6, updatedSessionFromRepo.LastActivity.UnixNano()/1e6) // Compare milliseconds
}

func TestEndSession(t *testing.T) {
	user := &db.User{ID: "delete-user"}
	req := httptest.NewRequest("GET", "/", nil)
	sessionData, err := testSessionStore.NewSession(req, user, "test", time.Hour)
	assert.NoError(t, err)
	assert.NotNil(t, sessionData)

	err = testSessionStore.EndSession(sessionData.Token)
	assert.NoError(t, err)

	// Try to validate the session - should fail as it's closed
	validateReq := httptest.NewRequest("GET", "/", nil)
	validateReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionData.Token})
	_, isValid := testSessionStore.ValidateSession(validateReq)
	assert.False(t, isValid, "Session should be invalid after EndSession")

	// Verify directly from repo that it's marked as closed (optional, but good for thoroughness)
	closedSession, err := testSessionRepo.FindSessionByToken(sessionData.Token)
	assert.Error(t, err) // Expecting "session has been closed"
	assert.Nil(t, closedSession)
	if err != nil {
		assert.Equal(t, "session has been closed", err.Error())
	}

	// Try ending an already ended session (or non-existent)
	err = testSessionStore.EndSession(sessionData.Token) // token of already closed session
	assert.Error(t, err)                                 // Should error as it's already closed or not found for update
	// The specific error message might depend on the DB repository implementation
	// For SQLite, it might be "session already closed or not found" or similar.
	// For now, just checking for an error is sufficient.

	err = testSessionStore.EndSession("non-existent-token-for-end")
	assert.Error(t, err) // Should also error
}

func TestValidateSessionComprehensive(t *testing.T) {
	// Valid session
	userValid := &db.User{ID: "valid-user", Username: "validator"}
	reqValidSetup := httptest.NewRequest("GET", "/", nil)
	validSession, err := testSessionStore.NewSession(reqValidSetup, userValid, "test", time.Hour)
	assert.NoError(t, err)
	assert.NotNil(t, validSession)

	reqValid := httptest.NewRequest("GET", "/", nil)
	reqValid.AddCookie(&http.Cookie{Name: SessionCookieName, Value: validSession.Token})

	s, isValid := testSessionStore.ValidateSession(reqValid)
	assert.True(t, isValid)
	assert.NotNil(t, s)
	if s != nil {
		assert.Equal(t, validSession.Token, s.Token)
		// LastActivity should be updated to now, so it should be >= CreatedAt
		// We fetch the original session from repo to get its CreatedAt for comparison
		originalSessionFromRepo, _ := testSessionRepo.FindSessionByToken(validSession.Token)
		assert.True(t, !s.LastActivity.Before(originalSessionFromRepo.CreatedAt), "LastActivity should be updated")
	}

	// Expired session
	userExpired := &db.User{ID: "expired-user"}
	reqExpiredSetup := httptest.NewRequest("GET", "/", nil)
	// Create a session that is already expired
	expiredSession, err := testSessionStore.NewSession(reqExpiredSetup, userExpired, "test", -time.Hour)
	assert.NoError(t, err)
	assert.NotNil(t, expiredSession)

	// Wait for a moment to ensure it's definitely expired if validity was very short
	time.Sleep(10 * time.Millisecond)

	reqExpired := httptest.NewRequest("GET", "/", nil)
	reqExpired.AddCookie(&http.Cookie{Name: SessionCookieName, Value: expiredSession.Token})
	_, isExpiredValid := testSessionStore.ValidateSession(reqExpired)
	assert.False(t, isExpiredValid, "Expired session should not be valid")

	// Ensure expired session is marked closed by ValidateSession
	closedExpiredSession, err := testSessionRepo.FindSessionByToken(expiredSession.Token)
	assert.Error(t, err) // Should be "session has been closed"
	assert.Nil(t, closedExpiredSession)
	if err != nil {
		assert.Equal(t, "session has been closed", err.Error())
	}

	// No cookie
	reqNoCookie := httptest.NewRequest("GET", "/", nil)
	_, isNoCookieValid := testSessionStore.ValidateSession(reqNoCookie)
	assert.False(t, isNoCookieValid)

	// Invalid token
	reqInvalidToken := httptest.NewRequest("GET", "/", nil)
	reqInvalidToken.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "invalid-token-value"})
	_, isInvalidTokenValid := testSessionStore.ValidateSession(reqInvalidToken)
	assert.False(t, isInvalidTokenValid)
}

func TestFindSessionsByUserID(t *testing.T) {
	userID := "user-with-multiple-sessions-store"
	req := httptest.NewRequest("GET", "/", nil) // Mock request

	// Clean up any old sessions for this user ID to ensure a clean test slate.
	// This is tricky without a direct "delete all by user" in SessionStore.
	// For testing, we rely on unique user IDs or manual cleanup if tests interfere.
	// Or, accept that previous test runs might leave data if not using a fresh DB per test.

	s1, err := testSessionStore.NewSession(req, &db.User{ID: userID, Username: "multi1"}, "test", time.Hour)
	assert.NoError(t, err)
	s2, err := testSessionStore.NewSession(req, &db.User{ID: userID, Username: "multi2"}, "test", 2*time.Hour)
	assert.NoError(t, err)

	// Expired session for same user (created as already expired)
	sExp, err := testSessionStore.NewSession(req, &db.User{ID: userID, Username: "multiExp"}, "test", -time.Hour)
	assert.NoError(t, err)
	// ValidateSession will mark it as closed in the DB
	reqExpValidate := httptest.NewRequest("GET", "/", nil)
	reqExpValidate.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sExp.Token})
	testSessionStore.ValidateSession(reqExpValidate) // This should close it

	// Closed session for same user
	sClosed, err := testSessionStore.NewSession(req, &db.User{ID: userID, Username: "multiClose"}, "test", time.Hour)
	assert.NoError(t, err)
	err = testSessionStore.EndSession(sClosed.Token)
	assert.NoError(t, err)

	// Session for a different user
	_, err = testSessionStore.NewSession(req, &db.User{ID: "other-user-id-store", Username: "other"}, "test", time.Hour)
	assert.NoError(t, err)

	userSessions, err := testSessionStore.FindSessionsByUserID(userID)
	assert.NoError(t, err)
	// FindSessionsByUserID from SessionStore is expected to return all sessions (active, expired, closed)
	// as per the SessionStore interface and SessionStoreDB implementation.
	assert.Len(t, userSessions, 4, "Should retrieve all sessions (active, expired, closed) for the user")

	tokensFound := map[string]bool{
		s1.Token:      false,
		s2.Token:      false,
		sExp.Token:    false,
		sClosed.Token: false,
	}

	for _, s := range userSessions {
		assert.Equal(t, userID, s.UserID)
		tokensFound[s.Token] = true
		// Optionally, check session state based on what FindSessionsByUserID is expected to return
		if s.Token == sClosed.Token {
			assert.NotNil(t, s.ClosedOn, "Closed session should have ClosedOn set")
		} else if s.Token == sExp.Token {
			// After ValidateSession on an expired token, it should be marked as closed.
			assert.NotNil(t, s.ClosedOn, "Expired session (after validation attempt) should have ClosedOn set")
			assert.True(t, s.ValidUntil.Before(time.Now()), "Expired session should have ValidUntil in the past")
		} else { // s1 and s2
			assert.Nil(t, s.ClosedOn, "Active session should not have ClosedOn set")
			assert.True(t, s.ValidUntil.After(time.Now()), "Active session should have ValidUntil in the future")
		}
	}
	for token, found := range tokensFound {
		assert.True(t, found, "Session with token %s not found", token)
	}
}
