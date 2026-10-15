package dbmigrations_test

import (
	"io/fs"
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Users who logged in with Google/GitHub before identities existed get one
// identity each, so their next login matches by provider subject.
func TestAccountsMigration_BackfillsOAuthIdentities(t *testing.T) {
	db.SetupTestDB(t.Name())
	gdb := db.GetConnection()
	for _, table := range []string{"auth_tokens", "user_identities", "auth_events"} {
		require.NoError(t, gdb.Exec("DROP TABLE "+table).Error)
	}
	users := []*db.User{
		{Email: "g@example.com", Username: "g", Provider: "google", ProviderId: "g-1"},
		{Email: "h@example.com", Username: "h", Provider: "github", ProviderId: "42"},
		{Email: "b@example.com", Username: "b", Provider: ""},
		{Email: "n@example.com", Username: "n", Provider: "github"},
	}
	for _, u := range users {
		require.NoError(t, gdb.Create(u).Error)
	}

	sqlText, err := fs.ReadFile(db.MigrationSchema().Migrations, "0004_accounts.up.sql")
	require.NoError(t, err)
	require.NoError(t, gdb.Exec(string(sqlText)).Error)

	var identities []db.UserIdentity
	require.NoError(t, gdb.Order("provider").Find(&identities).Error)
	require.Len(t, identities, 2, "only OAuth users with a provider ID")
	assert.Equal(t, "github", identities[0].Provider)
	assert.Equal(t, "42", identities[0].Subject)
	assert.Equal(t, users[1].ID, identities[0].UserID)
	assert.Equal(t, "google", identities[1].Provider)
	assert.Equal(t, users[0].ID, identities[1].UserID)

	require.NoError(t, gdb.Create(&db.AuthToken{Purpose: db.AuthTokenEmailLogin, Email: "x@example.com", TokenHash: "h"}).Error)
	require.NoError(t, gdb.Create(&db.AuthEvent{UserID: users[0].ID, Type: db.AuthEventLogin}).Error)
}
