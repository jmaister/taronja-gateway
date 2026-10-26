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

	ok, err := s.canAutoLink(user, p)
	if err != nil {
		return nil, err
	}
	if !ok {
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
func (s *Service) canAutoLink(user *db.User, p ExternalProfile) (bool, error) {
	if !p.EmailVerified || !strings.EqualFold(user.Email, p.Email) {
		return false, nil
	}
	// The admin's email comes from the config file and was never proven.
	if user.Provider == db.AdminProvider {
		return false, nil
	}
	// An unconfirmed email is only safe when there is no password someone
	// else could have set for it.
	if !user.EmailConfirmed && user.Password != "" {
		return false, nil
	}
	// The user removed this provider on purpose; only an explicit link adds it back.
	unlinked, err := s.repos.Events.Exists(user.ID, db.AuthEventIdentityUnlinked, p.Provider)
	if err != nil {
		return false, err
	}
	return !unlinked, nil
}

// LinkIdentity links an external login to a signed-in user who asked for it.
// Linking an identity the user already has is a no-op.
func (s *Service) LinkIdentity(userID string, p ExternalProfile, client Client) error {
	if p.Subject == "" {
		return errors.New("external login without a subject")
	}
	identity, err := s.repos.Identities.FindByProviderSubject(p.Provider, p.Subject)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if identity != nil {
		if identity.UserID != userID {
			return ErrIdentityTaken
		}
		return nil
	}
	err = s.createIdentity(userID, p)
	if err != nil {
		return err
	}
	s.Record(userID, db.AuthEventIdentityLinked, p.Provider, client)
	return nil
}

// Identities returns the external logins linked to userID.
func (s *Service) Identities(userID string) ([]db.UserIdentity, error) {
	return s.repos.Identities.ListByUserID(userID)
}

// UnlinkIdentity removes one of the user's external logins, unless it is the
// only way left for them to log in.
func (s *Service) UnlinkIdentity(user *db.User, identityID string, client Client) error {
	identities, err := s.repos.Identities.ListByUserID(user.ID)
	if err != nil {
		return err
	}
	var target *db.UserIdentity
	for i := range identities {
		if identities[i].ID == identityID {
			target = &identities[i]
		}
	}
	if target == nil {
		return gorm.ErrRecordNotFound
	}

	otherWays := len(identities) - 1
	if user.Password != "" {
		otherWays++
	}
	if otherWays == 0 {
		return ErrLastLoginMethod
	}

	err = s.repos.Identities.Delete(user.ID, identityID)
	if err != nil {
		return err
	}
	// Otherwise the "created by this provider" path in ResolveExternalLogin
	// would re-link it on the next login.
	if user.Provider == target.Provider {
		user.Provider = ""
		user.ProviderId = ""
		err = s.repos.Users.UpdateUser(user)
		if err != nil {
			return err
		}
	}
	s.Record(user.ID, db.AuthEventIdentityUnlinked, target.Provider, client)
	return nil
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

// --- Sessions ---

// Sessions returns userID's open sessions, most recently active first.
func (s *Service) Sessions(userID string) ([]db.Session, error) {
	return s.repos.Sessions.ListActiveSessionsByUserID(userID, s.now())
}

// RevokeSession signs out one of userID's sessions.
func (s *Service) RevokeSession(userID, tokenHash string, client Client) error {
	err := s.repos.Sessions.CloseSessionByHash(userID, tokenHash)
	if err != nil {
		return err
	}
	s.Record(userID, db.AuthEventSessionRevoked, "", client)
	return nil
}

// RevokeOtherSessions signs out every session of userID except the current one.
func (s *Service) RevokeOtherSessions(userID, currentTokenHash string, client Client) (int64, error) {
	n, err := s.repos.Sessions.CloseOtherSessions(userID, currentTokenHash)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.Record(userID, db.AuthEventSessionRevoked, "", client)
	}
	return n, nil
}
