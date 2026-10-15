package accounts

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sentEmail struct{ to, subject, body string }

// recordingMailer keeps every email instead of sending it.
type recordingMailer struct {
	mu   sync.Mutex
	sent []sentEmail
}

func (m *recordingMailer) SendMessage(ctx context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentEmail{to, subject, body})
	return nil
}

func (m *recordingMailer) last(t *testing.T) sentEmail {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	require.NotEmpty(t, m.sent, "no email was sent")
	return m.sent[len(m.sent)-1]
}

func (m *recordingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

var (
	tokenInLink = regexp.MustCompile(`\?token=([A-Za-z0-9_-]+)`)
	codeInBody  = regexp.MustCompile(`(?m)^\s+(\d{6})$`)
)

func linkToken(t *testing.T, e sentEmail) string {
	t.Helper()
	m := tokenInLink.FindStringSubmatch(e.body)
	require.NotNil(t, m, "email has no link:\n%s", e.body)
	return m[1]
}

type fixture struct {
	svc    *Service
	mailer *recordingMailer
	repos  Repositories
	clock  *time.Time
}

func newFixture(t *testing.T, opts Options) *fixture {
	t.Helper()
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repos := Repositories{
		Users:      repositories.NewDBUserRepository(conn),
		Sessions:   repositories.NewSessionRepositoryDB(conn),
		Tokens:     repositories.NewAuthTokenRepositoryDB(conn),
		Identities: repositories.NewUserIdentityRepositoryDB(conn),
		Events:     repositories.NewAuthEventRepositoryDB(conn),
	}
	opts.BaseURL = "https://app.example.com"
	mailer := &recordingMailer{}
	svc := NewService(repos, mailer, opts)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	return &fixture{svc: svc, mailer: mailer, repos: repos, clock: &now}
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

func TestNormalizeEmail(t *testing.T) {
	email, err := NormalizeEmail("  Bob@Example.COM ")
	require.NoError(t, err)
	assert.Equal(t, "bob@example.com", email)

	for _, bad := range []string{"", "bob", "Bob <bob@example.com>", "bob@example.com, eve@example.com"} {
		_, err := NormalizeEmail(bad)
		assert.ErrorIs(t, err, ErrInvalidEmail, bad)
	}
}

func TestSignUp_AccountExistsOnlyAfterVerification(t *testing.T) {
	f := newFixture(t, Options{SignupEnabled: true, AppName: "Acme"})

	require.NoError(t, f.svc.SignUp(context.Background(), "New@Example.com", "correct horse", "/dashboard"))
	mail := f.mailer.last(t)
	assert.Equal(t, "new@example.com", mail.to)
	assert.Equal(t, "Confirm your email for Acme", mail.subject)
	assert.Contains(t, mail.body, "https://app.example.com/_/verify-email?token=")

	_, err := f.repos.Users.FindUserByEmail("new@example.com")
	assert.Error(t, err, "no account before the link is used")

	user, redirect, err := f.svc.VerifyEmail(linkToken(t, mail), client)
	require.NoError(t, err)
	assert.Equal(t, "/dashboard", redirect)
	assert.Equal(t, "new@example.com", user.Email)
	assert.True(t, user.EmailConfirmed)
	assert.Equal(t, db.SignupProvider, user.Provider)
	matches, err := encryption.ComparePassword("correct horse", user.Password)
	require.NoError(t, err)
	assert.True(t, matches)
	assert.Equal(t, []string{db.AuthEventSignup, db.AuthEventEmailVerified}, f.eventTypes(t, user.ID))

	_, _, err = f.svc.VerifyEmail(linkToken(t, mail), client)
	assert.ErrorIs(t, err, ErrInvalidToken, "a link works once")
}

func TestSignUp_RulesAndExistingAccounts(t *testing.T) {
	f := newFixture(t, Options{SignupEnabled: true, PasswordResetEnabled: true})
	ctx := context.Background()

	assert.ErrorIs(t, f.svc.SignUp(ctx, "not-an-email", "long enough", ""), ErrInvalidEmail)
	assert.ErrorIs(t, f.svc.SignUp(ctx, "a@example.com", "short", ""), ErrWeakPassword)

	f.createUser(t, &db.User{Email: "taken@example.com", Username: "taken", EmailConfirmed: true})
	require.NoError(t, f.svc.SignUp(ctx, "taken@example.com", "long enough", ""), "an existing email gives the same answer")
	mail := f.mailer.last(t)
	assert.Equal(t, "taken@example.com", mail.to)
	assert.Contains(t, mail.subject, "already have")
	assert.Contains(t, mail.body, "/_/forgot-password")
	assert.NotRegexp(t, tokenInLink, mail.body, "no usable link is sent to an existing account")

	assert.ErrorIs(t, f.svc.SignUp(ctx, "taken@example.com", "long enough", ""), ErrTooSoon)
	f.advance(2 * time.Minute)
	assert.NoError(t, f.svc.SignUp(ctx, "taken@example.com", "long enough", ""))

	// Two pending sign-ups for one address: the first link used wins.
	require.NoError(t, f.svc.SignUp(ctx, "race@example.com", "password one", ""))
	first := f.mailer.last(t)
	f.advance(2 * time.Minute)
	require.NoError(t, f.svc.SignUp(ctx, "race@example.com", "password two", ""))
	second := f.mailer.last(t)
	_, _, err := f.svc.VerifyEmail(linkToken(t, second), client)
	require.NoError(t, err)
	_, _, err = f.svc.VerifyEmail(linkToken(t, first), client)
	assert.ErrorIs(t, err, ErrAccountExists)

	f.advance(25 * time.Hour)
	require.NoError(t, f.svc.SignUp(ctx, "late@example.com", "long enough", ""))
	late := f.mailer.last(t)
	f.advance(25 * time.Hour)
	_, _, err = f.svc.VerifyEmail(linkToken(t, late), client)
	assert.ErrorIs(t, err, ErrInvalidToken, "links expire")

	disabled := newFixture(t, Options{})
	assert.ErrorIs(t, disabled.svc.SignUp(ctx, "a@example.com", "long enough", ""), ErrDisabled)
}

func TestSendEmailConfirmation_ConfirmsExistingUser(t *testing.T) {
	f := newFixture(t, Options{})
	user := f.createUser(t, &db.User{Email: "admin-made@example.com", Username: "u1", Password: "secret-pass"})

	require.NoError(t, f.svc.SendEmailConfirmation(context.Background(), user))
	confirmed, redirect, err := f.svc.VerifyEmail(linkToken(t, f.mailer.last(t)), client)
	require.NoError(t, err)
	assert.Equal(t, user.ID, confirmed.ID)
	assert.True(t, confirmed.EmailConfirmed)
	assert.Equal(t, "/_/account", redirect)
}

func TestPasswordReset(t *testing.T) {
	f := newFixture(t, Options{PasswordResetEnabled: true})
	ctx := context.Background()
	user := f.createUser(t, &db.User{Email: "bob@example.com", Username: "bob", Password: "old-password"})
	f.openSession(t, user.ID, "session-a")
	f.openSession(t, user.ID, "session-b")

	require.NoError(t, f.svc.RequestPasswordReset(ctx, "nobody@example.com", client))
	assert.Equal(t, 0, f.mailer.count(), "unknown emails get no email but the same answer")

	require.NoError(t, f.svc.RequestPasswordReset(ctx, "BOB@example.com", client))
	first := linkToken(t, f.mailer.last(t))
	require.NoError(t, f.svc.RequestPasswordReset(ctx, "bob@example.com", client))
	assert.Equal(t, 1, f.mailer.count(), "a second request within a minute sends nothing")
	f.advance(2 * time.Minute)
	require.NoError(t, f.svc.RequestPasswordReset(ctx, "bob@example.com", client))
	second := linkToken(t, f.mailer.last(t))

	require.NoError(t, f.svc.CheckPasswordResetToken(second))
	_, err := f.svc.ResetPassword(second, "short", client)
	assert.ErrorIs(t, err, ErrWeakPassword)
	require.NoError(t, f.svc.CheckPasswordResetToken(second), "a rejected password does not use up the link")

	updated, err := f.svc.ResetPassword(second, "brand-new-password", client)
	require.NoError(t, err)
	assert.True(t, updated.EmailConfirmed, "using the link proves the email")
	matches, err := encryption.ComparePassword("brand-new-password", updated.Password)
	require.NoError(t, err)
	assert.True(t, matches)

	_, err = f.svc.ResetPassword(first, "another-password", client)
	assert.ErrorIs(t, err, ErrInvalidToken, "older links stop working once one is used")
	sessions, err := f.svc.Sessions(user.ID)
	require.NoError(t, err)
	assert.Empty(t, sessions, "a reset signs out everywhere")

	f.advance(2 * time.Minute)
	require.NoError(t, f.svc.RequestPasswordReset(ctx, "bob@example.com", client))
	expired := linkToken(t, f.mailer.last(t))
	f.advance(2 * time.Hour)
	assert.ErrorIs(t, f.svc.CheckPasswordResetToken(expired), ErrInvalidToken)
}

func TestChangePassword(t *testing.T) {
	f := newFixture(t, Options{})
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

func TestEmailLogin_ByLinkAndCode(t *testing.T) {
	f := newFixture(t, Options{EmailLoginEnabled: true})
	ctx := context.Background()
	user := f.createUser(t, &db.User{Email: "dana@example.com", Username: "dana"})

	binding, err := f.svc.StartEmailLogin(ctx, "dana@example.com", "/inbox", client)
	require.NoError(t, err)
	mail := f.mailer.last(t)
	code := codeInBody.FindStringSubmatch(mail.body)
	require.NotNil(t, code, mail.body)
	assert.Contains(t, mail.subject, code[1])

	_, _, err = f.svc.FinishEmailLoginByCode("another-browser", code[1], client)
	assert.ErrorIs(t, err, ErrInvalidToken, "the code only works in the browser that asked for it")

	loggedIn, redirect, err := f.svc.FinishEmailLoginByCode(binding, code[1], client)
	require.NoError(t, err)
	assert.Equal(t, user.ID, loggedIn.ID)
	assert.Equal(t, "/inbox", redirect)
	assert.True(t, loggedIn.EmailConfirmed, "receiving the email proves it")

	_, _, err = f.svc.FinishEmailLoginByLink(linkToken(t, mail), client)
	assert.ErrorIs(t, err, ErrInvalidToken, "link and code are one token, used once")

	f.advance(2 * time.Minute)
	_, err = f.svc.StartEmailLogin(ctx, "dana@example.com", "", client)
	require.NoError(t, err)
	loggedIn, redirect, err = f.svc.FinishEmailLoginByLink(linkToken(t, f.mailer.last(t)), client)
	require.NoError(t, err)
	assert.Equal(t, user.ID, loggedIn.ID)
	assert.Equal(t, "/", redirect)

	_, err = f.svc.StartEmailLogin(ctx, "dana@example.com", "", client)
	assert.ErrorIs(t, err, ErrTooSoon)
}

func TestEmailLogin_WrongCodesLockTheToken(t *testing.T) {
	f := newFixture(t, Options{EmailLoginEnabled: true})
	f.createUser(t, &db.User{Email: "eve@example.com", Username: "eve"})

	binding, err := f.svc.StartEmailLogin(context.Background(), "eve@example.com", "", client)
	require.NoError(t, err)
	code := codeInBody.FindStringSubmatch(f.mailer.last(t).body)[1]
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < maxCodeAttempts-1; i++ {
		_, _, err = f.svc.FinishEmailLoginByCode(binding, wrong, client)
		assert.ErrorIs(t, err, ErrInvalidCode)
	}
	_, _, err = f.svc.FinishEmailLoginByCode(binding, wrong, client)
	assert.ErrorIs(t, err, ErrTooManyAttempts)
	_, _, err = f.svc.FinishEmailLoginByCode(binding, code, client)
	assert.ErrorIs(t, err, ErrTooManyAttempts, "even the right code is refused after too many tries")
}

func TestEmailLogin_UnknownEmail(t *testing.T) {
	ctx := context.Background()

	noSignup := newFixture(t, Options{EmailLoginEnabled: true})
	binding, err := noSignup.svc.StartEmailLogin(ctx, "stranger@example.com", "", client)
	require.NoError(t, err, "same answer as for a known email")
	assert.NotEmpty(t, binding)
	assert.Equal(t, 0, noSignup.mailer.count(), "nothing to log in to, so nothing is sent")

	withSignup := newFixture(t, Options{EmailLoginEnabled: true, SignupEnabled: true})
	_, err = withSignup.svc.StartEmailLogin(ctx, "stranger@example.com", "", client)
	require.NoError(t, err)
	user, _, err := withSignup.svc.FinishEmailLoginByLink(linkToken(t, withSignup.mailer.last(t)), client)
	require.NoError(t, err)
	assert.Equal(t, "stranger@example.com", user.Email)
	assert.Empty(t, user.Password, "a passwordless account")
	assert.True(t, user.EmailConfirmed)
}

func TestResolveExternalLogin(t *testing.T) {
	gh := func(subject, email string, verified bool) ExternalProfile {
		return ExternalProfile{Provider: "github", Subject: subject, Email: email, EmailVerified: verified, Username: "gh-" + subject, Name: "GH " + subject}
	}

	t.Run("new user, then the same identity logs in again", func(t *testing.T) {
		f := newFixture(t, Options{})
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
		f := newFixture(t, Options{})
		legacy := f.createUser(t, &db.User{Email: "legacy@example.com", Username: "legacy", Provider: "github", ProviderId: "7", EmailConfirmed: true})
		user, err := f.svc.ResolveExternalLogin(gh("7", "legacy@example.com", true), client)
		require.NoError(t, err)
		assert.Equal(t, legacy.ID, user.ID)
		identities, err := f.svc.Identities(legacy.ID)
		require.NoError(t, err)
		require.Len(t, identities, 1)
	})

	t.Run("auto-links when both sides proved the email", func(t *testing.T) {
		f := newFixture(t, Options{})
		existing := f.createUser(t, &db.User{Email: "both@example.com", Username: "both", Provider: db.SignupProvider, Password: "pw-123456", EmailConfirmed: true})
		user, err := f.svc.ResolveExternalLogin(gh("3", "Both@Example.com", true), client)
		require.NoError(t, err)
		assert.Equal(t, existing.ID, user.ID)
		assert.Contains(t, f.eventTypes(t, existing.ID), db.AuthEventIdentityLinked)
	})

	t.Run("refuses to auto-link when the account could be squatted", func(t *testing.T) {
		f := newFixture(t, Options{})
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
		f := newFixture(t, Options{})
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
		f := newFixture(t, Options{})
		user, err := f.svc.ResolveExternalLogin(gh("10", "only@example.com", true), client)
		require.NoError(t, err)
		identities, err := f.svc.Identities(user.ID)
		require.NoError(t, err)
		assert.ErrorIs(t, f.svc.UnlinkIdentity(user, identities[0].ID, client), ErrLastLoginMethod)

		withEmailLogin := newFixture(t, Options{EmailLoginEnabled: true})
		user, err = withEmailLogin.svc.ResolveExternalLogin(gh("11", "only@example.com", true), client)
		require.NoError(t, err)
		identities, err = withEmailLogin.svc.Identities(user.ID)
		require.NoError(t, err)
		assert.NoError(t, withEmailLogin.svc.UnlinkIdentity(user, identities[0].ID, client), "email login is still there")
	})
}

func TestSessions(t *testing.T) {
	f := newFixture(t, Options{})
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
