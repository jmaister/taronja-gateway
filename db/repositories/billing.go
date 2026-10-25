package repositories

import (
	"errors"
	"time"

	"github.com/jmaister/taronja-gateway/db"
)

// Contracts for doc/billing.md's storage. Implementations live in
// organizationrepository_db.go and billingrepository_db.go; a lookup of a
// missing row returns gorm.ErrRecordNotFound, a second row with a unique
// value returns ErrDuplicate.

// ErrDuplicate is returned when a row with the same unique value exists.
var ErrDuplicate = errors.New("duplicate")

// OrganizationMembership is one of a user's organizations with their role.
type OrganizationMembership struct {
	Organization db.Organization
	Role         string
}

// OrganizationRepository stores organizations and their members.
type OrganizationRepository interface {
	Create(org *db.Organization) error
	Get(id string) (*db.Organization, error)
	List() ([]db.Organization, error)
	Update(org *db.Organization) error
	// Delete removes the organization and its memberships.
	Delete(id string) error

	// SetMember adds userID to the organization or changes their role.
	SetMember(orgID, userID, role string) error
	// RemoveMember returns gorm.ErrRecordNotFound if userID isn't a member.
	RemoveMember(orgID, userID string) error
	Members(orgID string) ([]db.OrganizationMember, error)
	// Member returns userID's membership, or gorm.ErrRecordNotFound.
	Member(orgID, userID string) (*db.OrganizationMember, error)
	// OfUser lists userID's organizations, ordered by name.
	OfUser(userID string) ([]OrganizationMembership, error)
}

// BillingPlanWithFeatures is a plan and the features it grants (sorted).
type BillingPlanWithFeatures struct {
	db.BillingPlan
	Features []string
}

// SubscriptionFilter narrows ListSubscriptions; zero values mean any.
type SubscriptionFilter struct {
	Subject string
	Status  string
	PlanKey string
}

// PurchaseFilter narrows ListPurchases; zero values mean any.
type PurchaseFilter struct {
	Subject string
	PlanKey string
	Status  string
	Since   *time.Time // paid at or after
}

// DocumentFilter narrows ListDocuments; zero values mean any.
type DocumentFilter struct {
	Subject   string
	OdooState string
	Kind      string
}

// BillingRepository stores everything billing.Service keeps. Transaction runs
// fn with a repository bound to one database transaction: the webhook records
// the event id and its state change atomically.
type BillingRepository interface {
	Transaction(fn func(BillingRepository) error) error

	// Plans. SavePlan creates or replaces the plan and its features.
	SavePlan(plan *BillingPlanWithFeatures) error
	GetPlan(key string) (*BillingPlanWithFeatures, error)
	// ListPlans returns plans by key; activeOnly leaves archived ones out.
	ListPlans(activeOnly bool) ([]BillingPlanWithFeatures, error)

	// Customers (one per subject). SaveCustomer creates or updates.
	SaveCustomer(c *db.BillingCustomer) error
	GetCustomer(subject string) (*db.BillingCustomer, error)
	GetCustomerByStripeID(stripeCustomerID string) (*db.BillingCustomer, error)

	// Checkouts.
	CreateCheckout(c *db.BillingCheckout) error
	GetCheckout(id string) (*db.BillingCheckout, error)
	GetCheckoutByStripeSession(sessionID string) (*db.BillingCheckout, error)
	UpdateCheckout(c *db.BillingCheckout) error

	// Subscriptions. CreateSubscription returns ErrDuplicate for a second row
	// with the same Stripe subscription id.
	CreateSubscription(s *db.BillingSubscription) error
	GetSubscription(id string) (*db.BillingSubscription, error)
	GetSubscriptionByStripeID(stripeSubscriptionID string) (*db.BillingSubscription, error)
	UpdateSubscription(s *db.BillingSubscription) error
	// ListSubscriptions returns the newest first.
	ListSubscriptions(filter SubscriptionFilter) ([]db.BillingSubscription, error)
	ListSubscriptionsBySubjects(subjects []string) ([]db.BillingSubscription, error)

	// Purchases. CreatePurchase returns ErrDuplicate for a second row with
	// the same payment intent.
	CreatePurchase(p *db.BillingPurchase) error
	GetPurchase(id string) (*db.BillingPurchase, error)
	GetPurchaseByPaymentIntent(paymentIntentID string) (*db.BillingPurchase, error)
	UpdatePurchase(p *db.BillingPurchase) error
	// ListPurchases returns the newest paid first.
	ListPurchases(filter PurchaseFilter) ([]db.BillingPurchase, error)
	ListPurchasesBySubjects(subjects []string) ([]db.BillingPurchase, error)

	// Grants.
	CreateGrant(g *db.BillingGrant) error
	// DeleteGrant returns gorm.ErrRecordNotFound for an unknown id.
	DeleteGrant(id string) error
	// ListGrants returns every grant when subject is "", newest first.
	ListGrants(subject string) ([]db.BillingGrant, error)
	ListGrantsBySubjects(subjects []string) ([]db.BillingGrant, error)

	// Documents. CreateDocument returns ErrDuplicate for a second row with
	// the same StripeRef.
	CreateDocument(d *db.BillingDocument) error
	GetDocument(id string) (*db.BillingDocument, error)
	GetDocumentByStripeRef(stripeRef string) (*db.BillingDocument, error)
	UpdateDocument(d *db.BillingDocument) error
	// DueDocuments returns pending documents whose NextAttemptAt is not
	// after now, oldest first, at most limit.
	DueDocuments(now time.Time, limit int) ([]db.BillingDocument, error)
	// ListDocuments returns the newest first.
	ListDocuments(filter DocumentFilter) ([]db.BillingDocument, error)
	// CreditedCents sums the credit notes of an invoice document.
	CreditedCents(invoiceDocumentID string) (int64, error)

	// RecordStripeEvent stores eventID and reports whether it was new; false
	// means it was already processed.
	RecordStripeEvent(eventID, eventType string) (bool, error)
}
