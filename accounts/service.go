// Package accounts resolves external logins (Google, GitHub, OpenID Connect) to
// one user per person through user_identities, and keeps a security activity
// log.
package accounts

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/session"
	"gorm.io/gorm"
)

var (
	// ErrLinkRequired is returned by ResolveExternalLogin when the external
	// account's email belongs to an existing user it cannot be linked to
	// automatically.
	ErrLinkRequired = errors.New("an account with this email already exists; log in with your usual method")
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
