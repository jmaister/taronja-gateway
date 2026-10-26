// Package accounts holds account management: linking several external logins
// to one user, password change, session management and the security activity
// log. The HTTP pages live in the gateway package.
package accounts

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/session"
	"gorm.io/gorm"
)

const (
	minPasswordLength = 8
	maxPasswordLength = 256
)

var (
	ErrWeakPassword    = fmt.Errorf("passwords need at least %d characters", minPasswordLength)
	ErrLongPassword    = fmt.Errorf("passwords can have at most %d characters", maxPasswordLength)
	ErrWrongPassword   = errors.New("your current password is wrong")
	ErrLastLoginMethod = errors.New("this is your only way to log in; set a password or link another account first")
	ErrIdentityTaken   = errors.New("that account is already linked to another user")
	// ErrLinkRequired is returned by ResolveExternalLogin when the external
	// account's email belongs to an existing user it cannot be linked to
	// automatically. The user must log in another way and link it from the
	// account page.
	ErrLinkRequired = errors.New("an account with this email already exists; log in with your usual method and link this one from your account page")
	// ErrEmailNotVerified is returned by ResolveExternalLogin for a new user
	// whose provider does not vouch for their email address.
	ErrEmailNotVerified = errors.New("your email address is not verified with this provider")
)

// Client identifies where a request came from, for the activity log.
type Client struct {
	IPAddress string
	UserAgent string
}

// Repositories are the stores a Service works on.
type Repositories struct {
	Users      repositories.UserRepository
	Sessions   repositories.SessionRepository
	Identities repositories.UserIdentityRepository
	Events     repositories.AuthEventRepository
}

// Service implements the account flows.
type Service struct {
	repos Repositories
	now   func() time.Time
}

// NewService creates a Service.
func NewService(repos Repositories) *Service {
	return &Service{repos: repos, now: db.NowUTC}
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

// --- Password change ---

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
