package db_test

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonUTC is a fixed +05:00 zone, deliberately not UTC and not this host's
// local zone either — every assertion below would still pass by accident if
// it merely happened to match the test machine's TZ, so a value nothing in
// this codebase could confuse for "already UTC" is what actually exercises
// the normalization.
var nonUTC = time.FixedZone("TEST+0500", 5*60*60)

// assertStoredUTC re-reads a single time value straight out of SQLite via a
// fresh query (not off the in-memory struct a hook already normalized in
// place) and checks it round-trips with a zero UTC offset — the same "trust
// the raw DB, not the Go struct" verification this session's real-server
// checks used, just automated. Scanning into time.Time rather than string:
// modernc.org/sqlite's driver recognizes these DATETIME-affinity columns and
// decodes them to a time.Time itself before database/sql ever sees a
// destination type, so a *string destination here would just get that
// time.Time's default RFC3339 rendering — this checks the decoded instant's
// actual offset instead, which is what "stored as UTC" really means.
func assertStoredUTC(t *testing.T, table, column, whereCol, whereVal string) {
	t.Helper()
	var stored time.Time
	err := db.GetConnection().Raw(
		"SELECT "+column+" FROM "+table+" WHERE "+whereCol+" = ?", whereVal,
	).Row().Scan(&stored)
	require.NoError(t, err)
	_, offset := stored.Zone()
	assert.Equal(t, 0, offset, "%s.%s should be stored as UTC, got zone offset %d (value %v)", table, column, offset, stored)
}

// TestSessionBeforeSave_NormalizesToUTC covers Session.BeforeSave: ValidUntil,
// LastActivity, and ClosedOn are all set from plain time.Now() calls
// elsewhere in this codebase (which carry the server's local zone), so this
// hook is what makes the stored value UTC regardless of what zone a caller
// used to build them. See db/schema.go's Session.BeforeSave comment.
func TestSessionBeforeSave_NormalizesToUTC(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewSessionRepositoryDB(db.GetConnection())

	closedOn := time.Date(2026, 9, 3, 10, 0, 0, 0, nonUTC)
	session := &db.Session{
		Token:        "test-token-utc",
		UserID:       "test-user",
		ValidUntil:   time.Date(2026, 12, 25, 10, 0, 0, 0, nonUTC),
		LastActivity: time.Date(2026, 9, 3, 8, 0, 0, 0, nonUTC),
		ClosedOn:     &closedOn,
	}
	require.NoError(t, repo.CreateSession("test-token-utc", session))

	assertStoredUTC(t, "sessions", "valid_until", "token", "test-token-utc")
	assertStoredUTC(t, "sessions", "last_activity", "token", "test-token-utc")
	assertStoredUTC(t, "sessions", "closed_on", "token", "test-token-utc")
	assertStoredUTC(t, "sessions", "created_at", "token", "test-token-utc")
	assertStoredUTC(t, "sessions", "updated_at", "token", "test-token-utc")

	// The in-memory struct GORM handed back should agree too.
	assert.Equal(t, time.UTC, session.ValidUntil.Location())
	assert.Equal(t, time.UTC, session.LastActivity.Location())
	require.NotNil(t, session.ClosedOn)
	assert.Equal(t, time.UTC, session.ClosedOn.Location())
}

// TestSessionCloseSession_NormalizesToUTC covers the one Session update path
// that bypasses BeforeSave entirely: CloseSession's raw single-column
// Update("closed_on", ...) against an empty &Session{} model, which a hook
// can never see — see the comment on that call site in sessionrepository.go.
func TestSessionCloseSession_NormalizesToUTC(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewSessionRepositoryDB(db.GetConnection())

	session := &db.Session{
		Token:      "test-token-close",
		UserID:     "test-user",
		ValidUntil: time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.CreateSession("test-token-close", session))
	require.NoError(t, repo.CloseSession("test-token-close"))

	assertStoredUTC(t, "sessions", "closed_on", "token", "test-token-close")
}

// TestTokenBeforeSave_NormalizesToUTC covers Token.BeforeSave. ExpiresAt in
// particular comes from API-caller-supplied JSON (handlers/api_tokens.go),
// not a server-side time.Now() — this proves an arbitrary offset a client
// sends gets normalized on the way in, not just a local-zone time.Now().
func TestTokenBeforeSave_NormalizesToUTC(t *testing.T) {
	db.SetupTestDB(t.Name())

	expiresAt := time.Date(2026, 12, 25, 10, 0, 0, 0, nonUTC)
	lastUsedAt := time.Date(2026, 9, 3, 8, 0, 0, 0, nonUTC)
	revokedAt := time.Date(2026, 9, 3, 9, 0, 0, 0, nonUTC)
	// Token.BeforeCreate always overwrites ID with a fresh CUID, so it's
	// read back off token.ID below rather than a literal set here.
	token := &db.Token{
		UserID:     "test-user",
		TokenHash:  "test-hash-utc",
		Name:       "utc-test-token",
		ExpiresAt:  &expiresAt,
		LastUsedAt: &lastUsedAt,
		RevokedAt:  &revokedAt,
	}
	require.NoError(t, db.GetConnection().Create(token).Error)

	assertStoredUTC(t, "tokens", "expires_at", "id", token.ID)
	assertStoredUTC(t, "tokens", "last_used_at", "id", token.ID)
	assertStoredUTC(t, "tokens", "revoked_at", "id", token.ID)
	assertStoredUTC(t, "tokens", "created_at", "id", token.ID)
	assertStoredUTC(t, "tokens", "updated_at", "id", token.ID)
}

// TestTokenRepository_RawUpdates_NormalizeToUTC covers the two Token update
// paths that bypass BeforeSave: IncrementUsageCount and RevokeToken, both
// raw map-based Updates against an empty &Token{} model — see the comments
// on those two methods in tokenrepository.go.
func TestTokenRepository_RawUpdates_NormalizeToUTC(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewTokenRepositoryDB(db.GetConnection())

	// Token.BeforeCreate always overwrites ID with a fresh CUID, so it's
	// read back off token.ID below rather than a literal set here.
	token := &db.Token{UserID: "test-user", TokenHash: "test-hash-raw", Name: "raw-test-token"}
	require.NoError(t, repo.CreateToken(token))

	require.NoError(t, repo.IncrementUsageCount(token.ID, time.Date(2026, 9, 3, 8, 0, 0, 0, nonUTC)))
	assertStoredUTC(t, "tokens", "last_used_at", "id", token.ID)

	require.NoError(t, repo.RevokeToken(token.ID, "test-admin"))
	assertStoredUTC(t, "tokens", "revoked_at", "id", token.ID)
}

// TestUserBeforeSave_NormalizesToUTC covers User.BeforeSave's normalization
// of PasswordResetExpires and EmailConfirmationExpires.
func TestUserBeforeSave_NormalizesToUTC(t *testing.T) {
	db.SetupTestDB(t.Name())

	resetExpires := time.Date(2026, 12, 25, 10, 0, 0, 0, nonUTC)
	confirmExpires := time.Date(2026, 9, 3, 8, 0, 0, 0, nonUTC)
	user := &db.User{
		Username:                 "utc-test-user",
		Email:                    "utc-test-user@example.com",
		PasswordResetExpires:     &resetExpires,
		EmailConfirmationExpires: &confirmExpires,
	}
	require.NoError(t, db.GetConnection().Create(user).Error)

	assertStoredUTC(t, "users", "password_reset_expires", "id", user.ID)
	assertStoredUTC(t, "users", "email_confirmation_expires", "id", user.ID)
	assertStoredUTC(t, "users", "created_at", "id", user.ID)
	assertStoredUTC(t, "users", "updated_at", "id", user.ID)
}
