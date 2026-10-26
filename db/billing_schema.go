package db

import (
	"crypto/rand"
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// Billing and organization models — see doc/billing.md's "Data model". IDs
// are CUIDs set in BeforeCreate (never BeforeSave: GORM also runs that on
// Model(&X{}).Update, and an ID set there would end up in the WHERE). Times
// are stored in UTC.

func newCUID() (string, error) {
	return cuid.NewCrypto(rand.Reader)
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// Organization roles.
const (
	OrganizationRoleOwner  = "owner"
	OrganizationRoleMember = "member"
)

// Organization groups users; a plan bought for it grants its features to
// every member.
type Organization struct {
	ID        string    `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	Name      string    `gorm:"type:varchar(255);not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`

	// The logo image. The repository never loads Logo with the
	// organization (see OrganizationLogoColumns); LogoUpdatedAt is nil
	// when there is no logo.
	Logo            []byte `gorm:"type:blob"`
	LogoContentType string `gorm:"type:varchar(50)"`
	LogoUpdatedAt   *time.Time
}

// HasLogo reports whether the organization has a logo.
func (o *Organization) HasLogo() bool { return o.LogoUpdatedAt != nil }

func (o *Organization) BeforeCreate(tx *gorm.DB) error {
	if o.ID != "" {
		return nil
	}
	id, err := newCUID()
	o.ID = id
	return err
}

// OrganizationMember is one user in one organization.
type OrganizationMember struct {
	OrganizationID string    `gorm:"primaryKey;column:organization_id;type:varchar(255);not null"`
	UserID         string    `gorm:"primaryKey;column:user_id;type:varchar(255);not null;index"`
	Role           string    `gorm:"type:varchar(20);not null"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

// Plan kinds, intervals, subjects and start modes.
const (
	PlanKindSubscription = "subscription"
	PlanKindOneTime      = "one_time"

	PlanIntervalMonth = "month"
	PlanIntervalYear  = "year"

	PlanSubjectUser   = "user"
	PlanSubjectOrg    = "org"
	PlanSubjectEither = "either"

	PlanStartsOnPayment    = "on_payment"
	PlanStartsOnActivation = "on_activation"
)

// BillingPlan is something sold at /_/pay/<key>.
type BillingPlan struct {
	Key             string `gorm:"primaryKey;column:key;type:varchar(100);not null"`
	Name            string `gorm:"type:varchar(255);not null"`
	Description     string `gorm:"type:text"`
	Kind            string `gorm:"type:varchar(20);not null"`
	Interval        string `gorm:"type:varchar(10)"`
	AmountCents     int64  `gorm:"not null"`
	Currency        string `gorm:"type:varchar(3);not null;default:eur"`
	Subject         string `gorm:"type:varchar(10);not null;default:either"`
	TrialDays       int    `gorm:"not null;default:0"`
	GraceDays       int    `gorm:"not null;default:7"`
	SetupFeeCents   int64  `gorm:"not null;default:0"`
	SetupFeeName    string `gorm:"type:varchar(255)"`
	Starts          string `gorm:"type:varchar(20);not null;default:on_payment"`
	AccessDays      int    `gorm:"not null;default:0"`
	CollectShipping bool   `gorm:"not null;default:false"`
	GroupKey        string `gorm:"type:varchar(100)"`
	Active          bool   `gorm:"not null;default:true"`

	StripeProductID          string `gorm:"type:varchar(255)"`
	StripePriceID            string `gorm:"type:varchar(255)"`
	StripeFirstPeriodPriceID string `gorm:"type:varchar(255)"`
	StripeSetupFeePriceID    string `gorm:"type:varchar(255)"`
	OdooProductID            *int64
	OdooSetupFeeProductID    *int64

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// BillingPlanFeature is one feature a plan grants.
type BillingPlanFeature struct {
	PlanKey string `gorm:"primaryKey;column:plan_key;type:varchar(100);not null"`
	Feature string `gorm:"primaryKey;column:feature;type:varchar(100);not null;index"`
}

// BillingCustomer is a subject's identity at Stripe and Odoo, and the
// details its invoices are issued to.
type BillingCustomer struct {
	Subject          string `gorm:"primaryKey;column:subject;type:varchar(300);not null"`
	StripeCustomerID string `gorm:"type:varchar(255);index"`
	OdooPartnerID    *int64
	Name             string `gorm:"type:varchar(255)"`
	Email            string `gorm:"type:varchar(255)"`
	TaxID            string `gorm:"type:varchar(50)"`
	AddressLine1     string `gorm:"type:varchar(255)"`
	AddressLine2     string `gorm:"type:varchar(255)"`
	PostalCode       string `gorm:"type:varchar(20)"`
	City             string `gorm:"type:varchar(100)"`
	State            string `gorm:"type:varchar(100)"`
	Country          string `gorm:"type:varchar(2)"`
	// TrialGroups lists the plan groups this subject has had a trial in,
	// comma-separated, so a trial is given once per group.
	TrialGroups string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// Checkout statuses.
const (
	CheckoutStatusOpen      = "open"
	CheckoutStatusCompleted = "completed"
	CheckoutStatusExpired   = "expired"
)

// BillingCheckout is one Stripe Checkout Session the gateway started.
type BillingCheckout struct {
	ID              string    `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	Subject         string    `gorm:"type:varchar(300);not null;index"`
	PlanKey         string    `gorm:"type:varchar(100);not null"`
	UserID          string    `gorm:"type:varchar(255);not null"`
	StripeSessionID string    `gorm:"type:varchar(255);index"`
	Status          string    `gorm:"type:varchar(20);not null"`
	Redirect        string    `gorm:"type:varchar(500)"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

func (c *BillingCheckout) BeforeCreate(tx *gorm.DB) error {
	if c.ID != "" {
		return nil
	}
	id, err := newCUID()
	c.ID = id
	return err
}

// Subscription statuses.
const (
	SubscriptionAwaitingActivation = "awaiting_activation"
	SubscriptionTrialing           = "trialing"
	SubscriptionActive             = "active"
	SubscriptionPastDue            = "past_due"
	SubscriptionCanceled           = "canceled"
)

// BillingSubscription is a subject's recurring plan, mirroring Stripe's.
type BillingSubscription struct {
	ID                    string  `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	Subject               string  `gorm:"type:varchar(300);not null;index"`
	PlanKey               string  `gorm:"type:varchar(100);not null"`
	PayerUserID           string  `gorm:"type:varchar(255)"`
	Status                string  `gorm:"type:varchar(30);not null"`
	StripeSubscriptionID  *string `gorm:"type:varchar(255);uniqueIndex"`
	StripePaymentMethodID string  `gorm:"type:varchar(255)"`
	CurrentPeriodEnd      *time.Time
	TrialEnd              *time.Time
	CancelAtPeriodEnd     bool `gorm:"not null;default:false"`
	GraceUntil            *time.Time
	ActivatedAt           *time.Time
	CanceledAt            *time.Time
	CreatedAt             time.Time `gorm:"autoCreateTime"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime"`
}

func (s *BillingSubscription) BeforeCreate(tx *gorm.DB) error {
	if s.ID != "" {
		return nil
	}
	id, err := newCUID()
	s.ID = id
	return err
}

func (s *BillingSubscription) BeforeSave(tx *gorm.DB) error {
	s.CurrentPeriodEnd, s.TrialEnd, s.GraceUntil = utcPtr(s.CurrentPeriodEnd), utcPtr(s.TrialEnd), utcPtr(s.GraceUntil)
	s.ActivatedAt, s.CanceledAt = utcPtr(s.ActivatedAt), utcPtr(s.CanceledAt)
	return nil
}

// Purchase statuses.
const (
	PurchaseStatusPaid     = "paid"
	PurchaseStatusRefunded = "refunded"
)

// BillingPurchase is one paid Checkout in payment mode: a one-time plan, or
// a subscription's first payment (setup fee, or a plan that starts on
// activation). It holds the shipping address for fulfillment.
type BillingPurchase struct {
	ID                    string    `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	Subject               string    `gorm:"type:varchar(300);not null;index"`
	PlanKey               string    `gorm:"type:varchar(100);not null;index"`
	PayerUserID           string    `gorm:"type:varchar(255)"`
	CheckoutID            string    `gorm:"type:varchar(255)"`
	SubscriptionID        *string   `gorm:"type:varchar(255)"`
	Status                string    `gorm:"type:varchar(20);not null"`
	AmountCents           int64     `gorm:"not null"`
	RefundedCents         int64     `gorm:"not null;default:0"`
	Currency              string    `gorm:"type:varchar(3);not null;default:eur"`
	StripePaymentIntentID string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	PaidAt                time.Time `gorm:"not null"`
	ShippingName          string    `gorm:"type:varchar(255)"`
	ShippingPhone         string    `gorm:"type:varchar(50)"`
	ShippingLine1         string    `gorm:"type:varchar(255)"`
	ShippingLine2         string    `gorm:"type:varchar(255)"`
	ShippingPostalCode    string    `gorm:"type:varchar(20)"`
	ShippingCity          string    `gorm:"type:varchar(100)"`
	ShippingState         string    `gorm:"type:varchar(100)"`
	ShippingCountry       string    `gorm:"type:varchar(2)"`
	CreatedAt             time.Time `gorm:"autoCreateTime"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime"`
}

func (p *BillingPurchase) BeforeCreate(tx *gorm.DB) error {
	if p.ID != "" {
		return nil
	}
	id, err := newCUID()
	p.ID = id
	return err
}

func (p *BillingPurchase) BeforeSave(tx *gorm.DB) error {
	p.PaidAt = p.PaidAt.UTC()
	return nil
}

// BillingGrant gives a subject a feature until a date without paying.
type BillingGrant struct {
	ID        string `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	Subject   string `gorm:"type:varchar(300);not null;index"`
	Feature   string `gorm:"type:varchar(100);not null"`
	Until     *time.Time
	GraceDays *int      `gorm:"column:grace_days"`
	Note      string    `gorm:"type:text"`
	CreatedBy string    `gorm:"type:varchar(255)"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (g *BillingGrant) BeforeCreate(tx *gorm.DB) error {
	if g.ID != "" {
		return nil
	}
	id, err := newCUID()
	g.ID = id
	return err
}

func (g *BillingGrant) BeforeSave(tx *gorm.DB) error {
	g.Until = utcPtr(g.Until)
	return nil
}

// Billing document kinds and Odoo states.
const (
	DocumentInvoice    = "invoice"
	DocumentCreditNote = "credit_note"

	OdooStatePending = "pending"
	OdooStateDone    = "done"
	OdooStateFailed  = "failed"
)

// BillingDocument is one invoice or credit note the gateway issues in Odoo.
type BillingDocument struct {
	ID                string  `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	Subject           string  `gorm:"type:varchar(300);not null;index"`
	Kind              string  `gorm:"type:varchar(20);not null"`
	StripeRef         string  `gorm:"type:varchar(255);uniqueIndex;not null"`
	InvoiceDocumentID *string `gorm:"type:varchar(255)"`
	SubscriptionID    *string `gorm:"type:varchar(255)"`
	PurchaseID        *string `gorm:"type:varchar(255)"`
	AmountCents       int64   `gorm:"not null"`
	Currency          string  `gorm:"type:varchar(3);not null;default:eur"`
	// Lines is a JSON array of billing.DocumentLine.
	Lines         string    `gorm:"type:text;not null"`
	PaidAt        time.Time `gorm:"not null"`
	OdooMoveID    *int64
	OdooState     string    `gorm:"type:varchar(20);not null;index"`
	OdooError     string    `gorm:"type:text"`
	Attempts      int       `gorm:"not null;default:0"`
	NextAttemptAt time.Time `gorm:"not null;index"`
	Number        string    `gorm:"type:varchar(100)"`
	IssuedAt      *time.Time
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

func (d *BillingDocument) BeforeCreate(tx *gorm.DB) error {
	if d.ID != "" {
		return nil
	}
	id, err := newCUID()
	d.ID = id
	return err
}

func (d *BillingDocument) BeforeSave(tx *gorm.DB) error {
	d.PaidAt, d.NextAttemptAt, d.IssuedAt = d.PaidAt.UTC(), d.NextAttemptAt.UTC(), utcPtr(d.IssuedAt)
	return nil
}

// StripeEvent records a processed Stripe webhook event id (delivery is at
// least once).
type StripeEvent struct {
	EventID    string    `gorm:"primaryKey;column:event_id;type:varchar(255);not null"`
	Type       string    `gorm:"type:varchar(100);not null"`
	ReceivedAt time.Time `gorm:"autoCreateTime"`
}

// BillingModels are appended to Models.
var BillingModels = []interface{}{
	&Organization{}, &OrganizationMember{}, &BillingPlan{}, &BillingPlanFeature{}, &BillingCustomer{},
	&BillingCheckout{}, &BillingSubscription{}, &BillingPurchase{}, &BillingGrant{}, &BillingDocument{}, &StripeEvent{},
}

func init() {
	Models = append(Models, BillingModels...)
}
