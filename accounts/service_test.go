package accounts

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	svc   *Service
	repos Repositories
	clock *time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repos := Repositories{
		Users:      repositories.NewDBUserRepository(conn),
		Sessions:   repositories.NewSessionRepositoryDB(conn),
		Identities: repositories.NewUserIdentityRepositoryDB(conn),
		Events:     repositories.NewAuthEventRepositoryDB(conn),
	}
	svc := NewService(repos)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	return &fixture{svc: svc, repos: repos, clock: &now}
}

func (f *fixture) advance(d time.Duration) { *f.clock = f.clock.Add(d) }

func (f *fixture) eventTypes(t *testing.T, userID string) []string {
	t.Helper()
	events, err := f.svc.RecentEvents(userID, 50)
	require.NoError(t, err)
	var types []string
	for i := len(events) - 1; i >= 0; i-- {
		types = append(types, events[i].Type)
	}
	return types
}

func (f *fixture) createUser(t *testing.T, u *db.User) *db.User {
	t.Helper()
	require.NoError(t, f.repos.Users.CreateUser(u))
	return u
}

func (f *fixture) openSession(t *testing.T, userID, token string) {
	t.Helper()
	require.NoError(t, f.repos.Sessions.CreateSession(token, &db.Session{
		UserID: userID, IsAuthenticated: true, ValidUntil: time.Now().Add(time.Hour), LastActivity: time.Now(),
	}))
}

var client = Client{IPAddress: "203.0.113.9", UserAgent: "test"}

func TestChangePassword(t *testing.T) {
	f := newFixture(t)
	user := f.createUser(t, &db.User{Email: "c@example.com", Username: "c", Password: "old-password"})
	f.openSession(t, user.ID, "current")
	f.openSession(t, user.ID, "other")

	assert.ErrorIs(t, f.svc.ChangePassword(user, "wrong", "new-password-1", db.HashSessionToken("current"), client), ErrWrongPassword)
	require.NoError(t, f.svc.ChangePassword(user, "old-password", "new-password-1", db.HashSessionToken("current"), client))

	sessions, err := f.svc.Sessions(user.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "other sessions are signed out")
	assert.Equal(t, db.HashSessionToken("current"), sessions[0].TokenHash)

	// A user without a password (e.g. GitHub only) can set one directly.
	oauthUser := f.createUser(t, &db.User{Email: "o@example.com", Username: "o", Provider: "github"})
	require.NoError(t, f.svc.ChangePassword(oauthUser, "", "first-password", "", client))
}

func TestResolveExternalLogin(t *testing.T) {
	gh := func(subject, email string, verified bool) ExternalProfile {
		return ExternalProfile{Provider: "github", Subject: subject, Email: email, EmailVerified: verified, Username: "gh-" + subject, Name: "GH " + subject}
	}

	t.Run("new user, then the same identity logs in again", func(t *testing.T) {
		f := newFixture(t)
		user, err := f.svc.ResolveExternalLogin(gh("1", "new@example.com", true), client)
		require.NoError(t, err)
		assert.Equal(t, "github", user.Provider)
		again, err := f.svc.ResolveExternalLogin(gh("1", "changed-email@example.com", true), client)
		require.NoError(t, err)
		assert.Equal(t, user.ID, again.ID, "identities are matched by subject, not email")

		_, err = f.svc.ResolveExternalLogin(gh("2", "unverified@example.com", false), client)
		assert.ErrorIs(t, err, ErrEmailNotVerified)
	})

	t.Run("users from before identities keep logging in", func(t *testing.T) {
		f := newFixture(t)
		legacy := f.createUser(t, &db.User{Email: "legacy@example.com", Username: "legacy", Provider: "github", ProviderId: "7", EmailConfirmed: true})
		user, err := f.svc.ResolveExternalLogin(gh("7", "legacy@example.com", true), client)
		require.NoError(t, err)
		assert.Equal(t, legacy.ID, user.ID)
		identities, err := f.svc.Identities(legacy.ID)
		require.NoError(t, err)
		require.Len(t, identities, 1)
	})

	t.Run("auto-links when both sides proved the email", func(t *testing.T) {
		f := newFixture(t)
		existing := f.createUser(t, &db.User{Email: "both@example.com", Username: "both", Password: "pw-123456", EmailConfirmed: true})
		user, err := f.svc.ResolveExternalLogin(gh("3", "Both@Example.com", true), client)
		require.NoError(t, err)
		assert.Equal(t, existing.ID, user.ID)
		assert.Contains(t, f.eventTypes(t, existing.ID), db.AuthEventIdentityLinked)
	})

	t.Run("refuses to auto-link when the account could be squatted", func(t *testing.T) {
		f := newFixture(t)
		f.createUser(t, &db.User{Email: "admin@example.com", Username: "admin", Provider: db.AdminProvider, Password: "pw-123456", EmailConfirmed: true})
		f.createUser(t, &db.User{Email: "unconfirmed@example.com", Username: "unconfirmed", Password: "pw-123456"})
		f.createUser(t, &db.User{Email: "ok@example.com", Username: "ok", Password: "pw-123456", EmailConfirmed: true})

		_, err := f.svc.ResolveExternalLogin(gh("4", "admin@example.com", true), client)
		assert.ErrorIs(t, err, ErrLinkRequired, "the admin email comes from config, never proven")
		_, err = f.svc.ResolveExternalLogin(gh("5", "unconfirmed@example.com", true), client)
		assert.ErrorIs(t, err, ErrLinkRequired, "someone else may have set that password")
		_, err = f.svc.ResolveExternalLogin(gh("6", "ok@example.com", false), client)
		assert.ErrorIs(t, err, ErrLinkRequired, "the provider did not verify the email")
	})

	t.Run("explicit link, unlink, and no silent re-link", func(t *testing.T) {
		f := newFixture(t)
		user := f.createUser(t, &db.User{Email: "me@example.com", Username: "me", Password: "pw-123456", EmailConfirmed: true})
		other := f.createUser(t, &db.User{Email: "other@example.com", Username: "other", EmailConfirmed: true})

		profile := gh("8", "work-me@corp.example", true)
		require.NoError(t, f.svc.LinkIdentity(user.ID, profile, client))
		require.NoError(t, f.svc.LinkIdentity(user.ID, profile, client), "linking twice is a no-op")
		assert.ErrorIs(t, f.svc.LinkIdentity(other.ID, profile, client), ErrIdentityTaken)

		loggedIn, err := f.svc.ResolveExternalLogin(profile, client)
		require.NoError(t, err)
		assert.Equal(t, user.ID, loggedIn.ID, "a linked identity logs in even with a different email")

		identities, err := f.svc.Identities(user.ID)
		require.NoError(t, err)
		require.Len(t, identities, 1)
		require.NoError(t, f.svc.UnlinkIdentity(user, identities[0].ID, client))

		_, err = f.svc.ResolveExternalLogin(gh("9", "me@example.com", true), client)
		assert.ErrorIs(t, err, ErrLinkRequired, "after an unlink, the provider is only added back explicitly")
	})

	t.Run("cannot unlink the last way to log in", func(t *testing.T) {
		f := newFixture(t)
		user, err := f.svc.ResolveExternalLogin(gh("10", "only@example.com", true), client)
		require.NoError(t, err)
		identities, err := f.svc.Identities(user.ID)
		require.NoError(t, err)
		assert.ErrorIs(t, f.svc.UnlinkIdentity(user, identities[0].ID, client), ErrLastLoginMethod)

	})
}

func TestSessions(t *testing.T) {
	f := newFixture(t)
	user := f.createUser(t, &db.User{Email: "s@example.com", Username: "s"})
	stranger := f.createUser(t, &db.User{Email: "x@example.com", Username: "x"})
	f.openSession(t, user.ID, "one")
	f.openSession(t, user.ID, "two")
	f.openSession(t, user.ID, "three")
	f.openSession(t, stranger.ID, "theirs")

	assert.Error(t, f.svc.RevokeSession(user.ID, db.HashSessionToken("theirs"), client), "only your own sessions")
	require.NoError(t, f.svc.RevokeSession(user.ID, db.HashSessionToken("two"), client))
	n, err := f.svc.RevokeOtherSessions(user.ID, db.HashSessionToken("one"), client)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	sessions, err := f.svc.Sessions(user.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, db.HashSessionToken("one"), sessions[0].TokenHash)

	theirs, err := f.svc.Sessions(stranger.ID)
	require.NoError(t, err)
	assert.Len(t, theirs, 1)
}

func TestCSRFToken(t *testing.T) {
	token := CSRFToken("session-token")
	assert.True(t, ValidCSRF("session-token", token))
	assert.False(t, ValidCSRF("other-session", token))
	assert.False(t, ValidCSRF("session-token", ""))
	assert.False(t, ValidCSRF("", CSRFToken("")))
}
