package accounts

import (
	"errors"
	"log"
	"strings"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// ExternalProfile is what an external login provider says about the user.
type ExternalProfile struct {
	Provider      string // e.g. "google"
	Subject       string // The provider's stable user ID.
	Email         string
	EmailVerified bool
	Username      string
	Name          string
	GivenName     string
	FamilyName    string
	Picture       string
	Locale        string
}

// ResolveExternalLogin finds or creates the user for an external login:
//   - an identity already linked to (provider, subject) logs in its user;
//   - otherwise, a user with the same email gets the identity linked when it
//     is safe (see canAutoLink), or ErrLinkRequired;
//   - otherwise a new user is created, if the provider verified the email.
func (s *Service) ResolveExternalLogin(p ExternalProfile, client Client) (*db.User, error) {
	if p.Subject == "" {
		return nil, errors.New("external login without a subject")
	}

	identity, err := s.repos.Identities.FindByProviderSubject(p.Provider, p.Subject)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if identity != nil {
		user, err := s.repos.Users.FindUserByIdOrUsername(identity.UserID, "", "")
		if err != nil {
			return nil, err
		}
		if user.Provider == p.Provider {
			s.refreshProfile(user, p)
		}
		s.touchIdentity(identity.ID)
		s.Record(user.ID, db.AuthEventLogin, p.Provider, client)
		return user, nil
	}

	var user *db.User
	if p.Email != "" {
		user, err = s.findUserByEmail(p.Email)
		if err != nil {
			return nil, err
		}
	}

	if user == nil {
		if !p.EmailVerified || p.Email == "" {
			return nil, ErrEmailNotVerified
		}
		user = &db.User{
			Email:          p.Email,
			Username:       p.Username,
			Name:           p.Name,
			GivenName:      p.GivenName,
			FamilyName:     p.FamilyName,
			Picture:        p.Picture,
			Locale:         p.Locale,
			Provider:       p.Provider,
			ProviderId:     p.Subject,
			EmailConfirmed: true,
		}
		if user.Username == "" {
			user.Username = p.Email
		}
		err = s.repos.Users.CreateUser(user)
		if err != nil {
			return nil, err
		}
		err = s.createIdentity(user.ID, p)
		if err != nil {
			return nil, err
		}
		s.Record(user.ID, db.AuthEventSignup, p.Provider, client)
		s.Record(user.ID, db.AuthEventLogin, p.Provider, client)
		return user, nil
	}

	// A user from before identities existed, created by this same provider.
	if user.Provider == p.Provider {
		if !p.EmailVerified {
			return nil, ErrEmailNotVerified
		}
		user.ProviderId = p.Subject
		user.EmailConfirmed = true
		s.refreshProfile(user, p)
		err = s.createIdentity(user.ID, p)
		if err != nil {
			return nil, err
		}
		s.Record(user.ID, db.AuthEventLogin, p.Provider, client)
		return user, nil
	}

	if !canAutoLink(user, p) {
		return nil, ErrLinkRequired
	}
	err = s.createIdentity(user.ID, p)
	if err != nil {
		return nil, err
	}
	if !user.EmailConfirmed {
		user.EmailConfirmed = true
		err = s.repos.Users.UpdateUser(user)
		if err != nil {
			return nil, err
		}
	}
	s.Record(user.ID, db.AuthEventIdentityLinked, p.Provider, client)
	s.Record(user.ID, db.AuthEventLogin, p.Provider, client)
	return user, nil
}

// canAutoLink decides whether an external login whose email matches user may
// be linked to user without the user asking. Both sides must have proven the
// email: otherwise whoever registered the address first (without owning it)
// would get access to the real owner's external account, or the reverse.
func canAutoLink(user *db.User, p ExternalProfile) bool {
	if !p.EmailVerified || !strings.EqualFold(user.Email, p.Email) {
		return false
	}
	// The admin's email comes from the config file and was never proven.
	if user.Provider == db.AdminProvider {
		return false
	}
	// An unconfirmed email is only safe when there is no password someone
	// else could have set for it.
	if !user.EmailConfirmed && user.Password != "" {
		return false
	}
	return true
}

func (s *Service) createIdentity(userID string, p ExternalProfile) error {
	now := s.now()
	return s.repos.Identities.Create(&db.UserIdentity{
		UserID:      userID,
		Provider:    p.Provider,
		Subject:     p.Subject,
		Email:       p.Email,
		LastLoginAt: &now,
	})
}

func (s *Service) touchIdentity(id string) {
	err := s.repos.Identities.TouchLastLogin(id, s.now())
	if err != nil {
		log.Printf("accounts: updating identity %s last login failed: %v", id, err)
	}
}

// refreshProfile copies the provider's current profile onto the user it
// created, as logins always did before identities existed.
func (s *Service) refreshProfile(user *db.User, p ExternalProfile) {
	user.Name = p.Name
	user.GivenName = p.GivenName
	user.FamilyName = p.FamilyName
	user.Picture = p.Picture
	user.Locale = p.Locale
	err := s.repos.Users.UpdateUser(user)
	if err != nil {
		log.Printf("accounts: updating profile of %s failed: %v", user.ID, err)
	}
}
