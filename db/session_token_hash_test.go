package db_test

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionToken_NeverStoredInPlaintext is the regression test for
// Finding 7: a session's raw, bearer-usable token used to be the sessions
// table's primary key column, recoverable in full by anyone who could read
// a database dump, a backup, or exploit an unrelated SQL-injection read.
// It must now be persisted only as its SHA-256 hash (Session.TokenHash) —
// this asserts the raw value genuinely never reaches the database, not
// just that a hash also happens to be stored alongside it.
func TestSessionToken_NeverStoredInPlaintext(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewSessionRepositoryDB(db.GetConnection())

	const rawToken = "super-secret-raw-session-token-value"
	session := &db.Session{
		Token:      rawToken,
		UserID:     "test-user",
		ValidUntil: time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.CreateSession(rawToken, session))

	// Read every text-ish column sqlite actually stored, straight off the
	// wire — not off the in-memory struct GORM already populated — so a
	// hook that merely leaves Go's copy of Token blank without touching
	// the write path wouldn't accidentally pass this. Scoped in its own
	// block so `rows` is fully closed (freeing the pooled connection)
	// before any further query runs against the same test DB.
	func() {
		rows, err := db.GetConnection().Raw("SELECT * FROM sessions").Rows()
		require.NoError(t, err)
		defer rows.Close()
		cols, err := rows.Columns()
		require.NoError(t, err)
		require.NotContains(t, cols, "token", "the plaintext \"token\" column must not exist in the current schema")
		require.Contains(t, cols, "token_hash")

		dest := make([]interface{}, len(cols))
		rawVals := make([]interface{}, len(cols))
		for i := range dest {
			dest[i] = &rawVals[i]
		}
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(dest...))
		for i, col := range cols {
			if s, ok := rawVals[i].(string); ok {
				assert.NotContains(t, s, rawToken, "column %q must never contain the raw session token", col)
			} else if b, ok := rawVals[i].([]byte); ok {
				assert.NotContains(t, string(b), rawToken, "column %q must never contain the raw session token", col)
			}
		}
	}()

	// And the hash must actually be usable: FindSessionByToken has to
	// round-trip the raw token back to the same row via its hash.
	found, err := repo.FindSessionByToken(rawToken)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, rawToken, found.Token, "FindSessionByToken should hand back the caller's own raw token")
	assert.Equal(t, "test-user", found.UserID)

	// A wrong token must not resolve to the row via any kind of partial or
	// substring match.
	notFound, err := repo.FindSessionByToken(rawToken[:len(rawToken)-1])
	require.NoError(t, err)
	assert.Nil(t, notFound)
}
