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
	var events []db.AuthEvent
	require.NoError(t, db.GetConnection().Where("user_id = ?", userID).Order("id").Find(&events).Error)
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	return types
}

func (f *fixture) createUser(t *testing.T, u *db.User) *db.User {
	t.Helper()
	require.NoError(t, f.repos.Users.CreateUser(u))
	return u
}

var client = Client{IPAddress: "203.0.113.9", UserAgent: "test"}

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
		_, err = f.repos.Identities.FindByProviderSubject("github", "7")
		require.NoError(t, err, "the identity was created on this login")
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
}
