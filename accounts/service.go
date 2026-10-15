// Package accounts holds self-service account management: sign-up with email
// verification, password reset, passwordless email login, linking several
// external logins to one user, session management and the security activity
// log. The HTTP pages live in the gateway package.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/session"
	"gorm.io/gorm"
)

const (
	verificationTTL  = 24 * time.Hour
	passwordResetTTL = time.Hour
	emailLoginTTL    = 15 * time.Minute
	// resendInterval is the minimum time between two emails of the same kind
	// to the same address, so the forms cannot be used to flood an inbox.
	resendInterval  = time.Minute
	maxCodeAttempts = 5

	minPasswordLength = 8
	maxPasswordLength = 256
)

var (
	ErrInvalidEmail       = errors.New("enter a valid email address")
	ErrWeakPassword       = fmt.Errorf("passwords need at least %d characters", minPasswordLength)
	ErrLongPassword       = fmt.Errorf("passwords can have at most %d characters", maxPasswordLength)
	ErrEmailNotConfigured = errors.New("email sending is not configured")
	ErrDisabled           = errors.New("this feature is not enabled")
	ErrTooSoon            = errors.New("we just sent you an email; wait a minute before asking for another")
	ErrInvalidToken       = errors.New("this link or code is invalid, expired or already used")
	ErrInvalidCode        = errors.New("wrong code")
	ErrTooManyAttempts    = errors.New("too many wrong codes; ask for a new one")
	ErrAccountExists      = errors.New("an account with this email already exists; log in instead")
	ErrWrongPassword      = errors.New("your current password is wrong")
	ErrLastLoginMethod    = errors.New("this is your only way to log in; set a password or link another account first")
	ErrIdentityTaken      = errors.New("that account is already linked to another user")
	// ErrLinkRequired is returned by ResolveExternalLogin when the external
	// account's email belongs to an existing user it cannot be linked to
	// automatically. The user must log in another way and link it from the
	// account page.
	ErrLinkRequired = errors.New("an account with this email already exists; log in with your usual method and link this one from your account page")
	// ErrEmailNotVerified is returned by ResolveExternalLogin for a new user
	// whose provider does not vouch for their email address.
	ErrEmailNotVerified = errors.New("your email address is not verified with this provider")
)

// Options configures a Service.
type Options struct {
	AppName              string // Shown in emails.
	BaseURL              string // Public URL the emailed links point to (server.url).
	SignupEnabled        bool
	EmailLoginEnabled    bool
	PasswordResetEnabled bool
}

// Client identifies where a request came from, for the activity log.
type Client struct {
	IPAddress string
	UserAgent string
}

// Repositories are the stores a Service works on.
type Repositories struct {
	Users      repositories.UserRepository
	Sessions   repositories.SessionRepository
	Tokens     repositories.AuthTokenRepository
	Identities repositories.UserIdentityRepository
	Events     repositories.AuthEventRepository
}

// Service implements the account flows.
type Service struct {
	repos  Repositories
	mailer Mailer // nil when email is not configured
	opts   Options
	now    func() time.Time
}

// NewService creates a Service. mailer may be nil; flows that send email then
// return ErrEmailNotConfigured.
func NewService(repos Repositories, mailer Mailer, opts Options) *Service {
	if opts.AppName == "" {
		opts.AppName = "Taronja Gateway"
	}
	return &Service{repos: repos, mailer: mailer, opts: opts, now: db.NowUTC}
}

// Options returns the options the Service was built with.
func (s *Service) Options() Options { return s.opts }

// NormalizeEmail trims and lowercases an email address and checks it is a bare
// address ("a@b.c", not "Name <a@b.c>").
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// ValidatePassword checks a new password against the length rules.
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPasswordLength {
		return ErrWeakPassword
	}
	if n > maxPasswordLength {
		return ErrLongPassword
	}
	return nil
}

// --- Activity log ---

// Record adds an entry to userID's activity log. Failures are logged, never
// returned: the log must not break the action it describes.
func (s *Service) Record(userID, eventType, provider string, client Client) {
	if userID == "" {
		return
	}
	err := s.repos.Events.Create(&db.AuthEvent{
		UserID:    userID,
		Type:      eventType,
		Provider:  provider,
		IPAddress: client.IPAddress,
		UserAgent: client.UserAgent,
	})
	if err != nil {
		log.Printf("accounts: recording %s event for user %s failed: %v", eventType, userID, err)
	}
}

// RecentEvents returns userID's newest activity log entries.
func (s *Service) RecentEvents(userID string, limit int) ([]db.AuthEvent, error) {
	return s.repos.Events.ListRecent(userID, limit)
}

// --- Sign-up and email verification ---

// SignUp starts a sign-up: it emails a confirmation link and creates the
// account only when that link is used. It succeeds the same way whether or not
// the email is already registered, so the form does not reveal who has an
// account; an existing user gets a "you already have an account" email instead.
func (s *Service) SignUp(ctx context.Context, rawEmail, password, redirect string) error {
	if !s.opts.SignupEnabled {
		return ErrDisabled
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	err = ValidatePassword(password)
	if err != nil {
		return err
	}
	err = s.checkResendInterval(db.AuthTokenEmailVerification, email)
	if err != nil {
		return err
	}

	existing, err := s.findUserByEmail(email)
	if err != nil {
		return err
	}
	if existing != nil {
		// Recorded as a token so the resend interval applies to this email too.
		_, err = s.issueToken(&db.AuthToken{Purpose: db.AuthTokenEmailVerification, Email: email, UserID: existing.ID, ExpiresAt: s.now()})
		if err != nil {
			return err
		}
		return s.sendAlreadyRegistered(ctx, email)
	}

	passwordHash, err := encryption.GeneratePasswordHash(password)
	if err != nil {
		return err
	}
	token, err := s.issueToken(&db.AuthToken{
		Purpose:      db.AuthTokenEmailVerification,
		Email:        email,
		PasswordHash: passwordHash,
		Redirect:     session.SanitizeRedirectPath(redirect),
		ExpiresAt:    s.now().Add(verificationTTL),
	})
	if err != nil {
		return err
	}
	return s.sendSignupConfirmation(ctx, email, token)
}

// SendEmailConfirmation emails an existing user a link that marks their email
// confirmed.
func (s *Service) SendEmailConfirmation(ctx context.Context, user *db.User) error {
	if user.EmailConfirmed {
		return nil
	}
	email, err := NormalizeEmail(user.Email)
	if err != nil {
		return err
	}
	err = s.checkResendInterval(db.AuthTokenEmailVerification, email)
	if err != nil {
		return err
	}
	token, err := s.issueToken(&db.AuthToken{
		Purpose:   db.AuthTokenEmailVerification,
		Email:     email,
		UserID:    user.ID,
		Redirect:  "/_/account",
		ExpiresAt: s.now().Add(verificationTTL),
	})
	if err != nil {
		return err
	}
	return s.sendEmailConfirmation(ctx, email, token)
}

// VerifyEmail uses a confirmation link. For a pending sign-up it creates the
// account; for an existing user it marks the email confirmed. It returns the
// user, now proven to own the email, and where to send them.
func (s *Service) VerifyEmail(rawToken string, client Client) (*db.User, string, error) {
	token, err := s.useToken(db.AuthTokenEmailVerification, rawToken)
	if err != nil {
		return nil, "", err
	}

	if token.UserID != "" {
		user, err := s.repos.Users.FindUserByIdOrUsername(token.UserID, "", "")
		if err != nil {
			return nil, "", ErrInvalidToken
		}
		if !strings.EqualFold(user.Email, token.Email) {
			// The user changed their email after the link was sent.
			return nil, "", ErrInvalidToken
		}
		if !user.EmailConfirmed {
			user.EmailConfirmed = true
			err = s.repos.Users.UpdateUser(user)
			if err != nil {
				return nil, "", err
			}
			s.Record(user.ID, db.AuthEventEmailVerified, "", client)
		}
		return user, token.Redirect, nil
	}

	existing, err := s.findUserByEmail(token.Email)
	if err != nil {
		return nil, "", err
	}
	if existing != nil {
		return nil, "", ErrAccountExists
	}
	user := &db.User{
		Email:          token.Email,
		Username:       token.Email,
		Password:       token.PasswordHash,
		Provider:       db.SignupProvider,
		EmailConfirmed: true,
	}
	err = s.repos.Users.CreateUser(user)
	if err != nil {
		return nil, "", err
	}
	s.Record(user.ID, db.AuthEventSignup, "", client)
	s.Record(user.ID, db.AuthEventEmailVerified, "", client)
	return user, token.Redirect, nil
}

// --- Password reset and change ---

// RequestPasswordReset emails a reset link if an account uses this email. It
// returns nil whether or not one does, so the form does not reveal who has an
// account.
func (s *Service) RequestPasswordReset(ctx context.Context, rawEmail string, client Client) error {
	if !s.opts.PasswordResetEnabled {
		return ErrDisabled
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	user, err := s.findUserByEmail(email)
	if err != nil || user == nil {
		return err
	}
	if s.checkResendInterval(db.AuthTokenPasswordReset, email) != nil {
		return nil
	}
	token, err := s.issueToken(&db.AuthToken{
		Purpose:   db.AuthTokenPasswordReset,
		Email:     email,
		UserID:    user.ID,
		ExpiresAt: s.now().Add(passwordResetTTL),
	})
	if err != nil {
		return err
	}
	s.Record(user.ID, db.AuthEventPasswordResetRequested, "", client)
	return s.sendPasswordReset(ctx, email, token)
}

// CheckPasswordResetToken reports whether a reset link is still usable,
// without using it, so the page can show the form or an error up front.
func (s *Service) CheckPasswordResetToken(rawToken string) error {
	_, err := s.activeToken(db.AuthTokenPasswordReset, rawToken)
	return err
}

// ResetPassword sets a new password through a reset link. Using the link also
// proves the user owns the email, and every existing session is closed.
func (s *Service) ResetPassword(rawToken, newPassword string, client Client) (*db.User, error) {
	err := ValidatePassword(newPassword)
	if err != nil {
		return nil, err
	}
	token, err := s.useToken(db.AuthTokenPasswordReset, rawToken)
	if err != nil {
		return nil, err
	}
	user, err := s.repos.Users.FindUserByIdOrUsername(token.UserID, "", "")
	if err != nil || !strings.EqualFold(user.Email, token.Email) {
		return nil, ErrInvalidToken
	}

	user.Password = newPassword // hashed by User.BeforeSave
	user.EmailConfirmed = true
	err = s.repos.Users.UpdateUser(user)
	if err != nil {
		return nil, err
	}
	err = s.repos.Tokens.InvalidateForUser(user.ID, db.AuthTokenPasswordReset, s.now())
	if err != nil {
		log.Printf("accounts: invalidating older reset links for %s failed: %v", user.ID, err)
	}
	_, err = s.repos.Sessions.CloseOtherSessions(user.ID, "")
	if err != nil {
		log.Printf("accounts: closing sessions after password reset for %s failed: %v", user.ID, err)
	}
	s.Record(user.ID, db.AuthEventPasswordReset, "", client)
	return user, nil
}

// ChangePassword sets a new password for a signed-in user. A user who already
// has one must give it. Every other session is closed.
func (s *Service) ChangePassword(user *db.User, currentPassword, newPassword, currentSessionHash string, client Client) error {
	if user.Password != "" {
		matches, err := encryption.ComparePassword(currentPassword, user.Password)
		if err != nil || !matches {
			return ErrWrongPassword
		}
	}
	err := ValidatePassword(newPassword)
	if err != nil {
		return err
	}
	user.Password = newPassword // hashed by User.BeforeSave
	err = s.repos.Users.UpdateUser(user)
	if err != nil {
		return err
	}
	_, err = s.repos.Sessions.CloseOtherSessions(user.ID, currentSessionHash)
	if err != nil {
		log.Printf("accounts: closing sessions after password change for %s failed: %v", user.ID, err)
	}
	s.Record(user.ID, db.AuthEventPasswordChanged, "", client)
	return nil
}

// --- Email login ---

// StartEmailLogin emails a login link and code. It returns a binding secret
// for the browser to keep in a cookie: the code is only accepted together with
// it. The result is the same whether or not the email has an account.
func (s *Service) StartEmailLogin(ctx context.Context, rawEmail, redirect string, client Client) (string, error) {
	if !s.opts.EmailLoginEnabled {
		return "", ErrDisabled
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return "", err
	}
	err = s.checkResendInterval(db.AuthTokenEmailLogin, email)
	if err != nil {
		return "", err
	}
	user, err := s.findUserByEmail(email)
	if err != nil {
		return "", err
	}

	binding, err := randomSecret()
	if err != nil {
		return "", err
	}
	code, err := randomCode()
	if err != nil {
		return "", err
	}
	token := &db.AuthToken{
		Purpose:     db.AuthTokenEmailLogin,
		Email:       email,
		BindingHash: hashSecret(binding),
		Redirect:    session.SanitizeRedirectPath(redirect),
		ExpiresAt:   s.now().Add(emailLoginTTL),
	}
	if user != nil {
		token.UserID = user.ID
	}
	rawToken, err := s.issueToken(token)
	if err != nil {
		return "", err
	}
	// The ID only exists once the token is saved, and the code hash uses it.
	token.CodeHash = hashCode(token.ID, code)
	err = s.repos.Tokens.SetCodeHash(token.ID, token.CodeHash)
	if err != nil {
		return "", err
	}

	// Without an account and without sign-up there is nothing to log in to;
	// the token still exists so the resend interval looks the same.
	if user == nil && !s.opts.SignupEnabled {
		return binding, nil
	}
	if user != nil {
		s.Record(user.ID, db.AuthEventEmailLoginRequested, "", client)
	}
	err = s.sendEmailLogin(ctx, email, rawToken, code)
	if err != nil {
		return "", err
	}
	return binding, nil
}

// FinishEmailLoginByLink logs in with the emailed link. It returns the user
// and where to send them.
func (s *Service) FinishEmailLoginByLink(rawToken string, client Client) (*db.User, string, error) {
	token, err := s.useToken(db.AuthTokenEmailLogin, rawToken)
	if err != nil {
		return nil, "", err
	}
	user, err := s.completeEmailLogin(token, client)
	if err != nil {
		return nil, "", err
	}
	return user, token.Redirect, nil
}

// FinishEmailLoginByCode logs in with the emailed code, typed into the
// browser holding binding.
func (s *Service) FinishEmailLoginByCode(binding, code string, client Client) (*db.User, string, error) {
	if binding == "" {
		return nil, "", ErrInvalidToken
	}
	token, err := s.repos.Tokens.FindActiveByBindingHash(hashSecret(binding), s.now())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", ErrInvalidToken
	}
	if err != nil {
		return nil, "", err
	}
	if token.Attempts >= maxCodeAttempts {
		return nil, "", ErrTooManyAttempts
	}
	if token.CodeHash == "" || !equalHashes(token.CodeHash, hashCode(token.ID, strings.TrimSpace(code))) {
		err = s.repos.Tokens.IncrementAttempts(token.ID)
		if err != nil {
			return nil, "", err
		}
		if token.Attempts+1 >= maxCodeAttempts {
			return nil, "", ErrTooManyAttempts
		}
		return nil, "", ErrInvalidCode
	}
	used, err := s.repos.Tokens.MarkUsed(token.ID, s.now())
	if err != nil {
		return nil, "", err
	}
	if !used {
		return nil, "", ErrInvalidToken
	}
	user, err := s.completeEmailLogin(token, client)
	if err != nil {
		return nil, "", err
	}
	return user, token.Redirect, nil
}

func (s *Service) completeEmailLogin(token *db.AuthToken, client Client) (*db.User, error) {
	user, err := s.findUserByEmail(token.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		if !s.opts.SignupEnabled {
			return nil, ErrInvalidToken
		}
		user = &db.User{
			Email:          token.Email,
			Username:       token.Email,
			Provider:       db.SignupProvider,
			EmailConfirmed: true,
		}
		err = s.repos.Users.CreateUser(user)
		if err != nil {
			return nil, err
		}
		s.Record(user.ID, db.AuthEventSignup, "email", client)
	} else if !user.EmailConfirmed {
		user.EmailConfirmed = true
		err = s.repos.Users.UpdateUser(user)
		if err != nil {
			return nil, err
		}
		s.Record(user.ID, db.AuthEventEmailVerified, "", client)
	}
	s.Record(user.ID, db.AuthEventLogin, "email", client)
	return user, nil
}

// --- Tokens ---

// issueToken stores token with a fresh secret and returns the raw secret.
func (s *Service) issueToken(token *db.AuthToken) (string, error) {
	raw, err := randomSecret()
	if err != nil {
		return "", err
	}
	token.TokenHash = hashSecret(raw)
	// The resend interval compares CreatedAt with s.now, so both use one clock.
	token.CreatedAt = s.now()
	err = s.repos.Tokens.Create(token)
	if err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Service) activeToken(purpose, raw string) (*db.AuthToken, error) {
	if raw == "" {
		return nil, ErrInvalidToken
	}
	token, err := s.repos.Tokens.FindByTokenHash(hashSecret(raw))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if token.Purpose != purpose || token.UsedAt != nil || !s.now().Before(token.ExpiresAt) {
		return nil, ErrInvalidToken
	}
	return token, nil
}

// useToken checks a link token and marks it used; only one caller can win.
func (s *Service) useToken(purpose, raw string) (*db.AuthToken, error) {
	token, err := s.activeToken(purpose, raw)
	if err != nil {
		return nil, err
	}
	used, err := s.repos.Tokens.MarkUsed(token.ID, s.now())
	if err != nil {
		return nil, err
	}
	if !used {
		return nil, ErrInvalidToken
	}
	return token, nil
}

func (s *Service) checkResendInterval(purpose, email string) error {
	last, err := s.repos.Tokens.LatestCreatedAt(purpose, email)
	if err != nil {
		return err
	}
	if !last.IsZero() && s.now().Sub(last) < resendInterval {
		return ErrTooSoon
	}
	return nil
}

func (s *Service) findUserByEmail(email string) (*db.User, error) {
	user, err := s.repos.Users.FindUserByEmail(email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return user, err
}

// ClientFromRequest reads the client address and user agent for the activity log.
func ClientFromRequest(r *http.Request) Client {
	return Client{IPAddress: session.GetClientIP(r), UserAgent: r.UserAgent()}
}
