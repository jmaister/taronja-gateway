package billing

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/notification"
	"gorm.io/gorm"
)

// Errors the service returns for handlers to map to HTTP statuses.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid request")
	ErrConflict = errors.New("conflict")
)

// Config holds the service's dependencies.
type Config struct {
	Repo repositories.BillingRepository
	Orgs repositories.OrganizationRepository
	// Stripe is required for plans, checkouts and refunds; Odoo may be nil
	// (documents then stay pending).
	Stripe Stripe
	Odoo   Odoo
	// ServerURL is the gateway's public URL (Checkout return URLs).
	ServerURL string
	// StripeMode is "test" or "live" (from the secret key), for the setup page.
	StripeMode string
	// Now defaults to time.Now.
	Now func() time.Time
}

// Service is billing: plans, checkouts, the webhook state machine,
// entitlements and the Odoo outbox. Each area lives in its own file.
type Service struct {
	repo       repositories.BillingRepository
	orgs       repositories.OrganizationRepository
	stripe     Stripe
	odoo       Odoo
	serverURL  string
	stripeMode string
	now        func() time.Time

	notifications *notification.Service

	// entitlements caches computed entitlements per subject.
	mu          sync.RWMutex
	cache       map[string]Entitlements // subject -> entitlements
	memberships map[string][]string     // user id -> organization ids
}

// NewService returns a Service for cfg.
func NewService(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo: cfg.Repo, orgs: cfg.Orgs, stripe: cfg.Stripe, odoo: cfg.Odoo,
		serverURL: strings.TrimSuffix(cfg.ServerURL, "/"), stripeMode: cfg.StripeMode, now: now,
		cache: map[string]Entitlements{}, memberships: map[string][]string{},
	}
}

// Repo is the service's repository (handlers read lists through it).
func (s *Service) Repo() repositories.BillingRepository { return s.repo }

// PayLink is the public pay page of a plan.
func (s *Service) PayLink(planKey string) string {
	return s.serverURL + "/_/pay/" + planKey
}

// Orgs is the organization repository.
func (s *Service) Orgs() repositories.OrganizationRepository { return s.orgs }

// Invalidate drops the cached entitlements of subject; every change to a
// subject's subscriptions, purchases or grants calls it.
func (s *Service) Invalidate(subject string) {
	s.mu.Lock()
	delete(s.cache, subject)
	s.mu.Unlock()
}

// InvalidateAll drops every cached entitlement.
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	s.cache = map[string]Entitlements{}
	s.memberships = map[string][]string{}
	s.mu.Unlock()
}

// notFound turns gorm.ErrRecordNotFound into ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
