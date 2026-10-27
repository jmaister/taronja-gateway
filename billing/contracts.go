// Package billing takes payments through Stripe, issues invoices and credit
// notes in Odoo and turns paid plans into entitlements — see doc/billing.md.
// Stripe and Odoo are reached only through the Stripe and Odoo interfaces in
// this file (billing/stripeapi and billing/odooapi implement them; fakes.go
// has in-memory versions for tests).
package billing

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/notification"
)

// Subject is who a purchase is for: "user:<id>" or "org:<id>".
type Subject string

// Subject kinds.
const (
	SubjectUser = "user"
	SubjectOrg  = "org"
)

// UserSubject and OrgSubject build subjects.
func UserSubject(userID string) Subject { return Subject(SubjectUser + ":" + userID) }
func OrgSubject(orgID string) Subject   { return Subject(SubjectOrg + ":" + orgID) }

// ParseSubject splits a subject into its kind and id.
func ParseSubject(s string) (kind, id string, err error) {
	kind, id, ok := strings.Cut(s, ":")
	if !ok || id == "" || (kind != SubjectUser && kind != SubjectOrg) {
		return "", "", fmt.Errorf("invalid subject %q: want user:<id> or org:<id>", s)
	}
	return kind, id, nil
}

var featurePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,99}$`)

// ValidateFeature checks a feature key ("academia.access").
func ValidateFeature(feature string) error {
	if !featurePattern.MatchString(feature) {
		return fmt.Errorf("invalid feature %q: 1-100 lowercase letters, digits, '.', '_' or '-', starting with a letter", feature)
	}
	return nil
}

var (
	// ErrNotConfigured: billing (or Odoo, for Odoo calls) isn't configured.
	ErrNotConfigured = errors.New("billing is not configured")
	// ErrBadSignature: a webhook request isn't from Stripe.
	ErrBadSignature = errors.New("invalid webhook signature")
)

// Address is a postal address as Stripe collects it.
type Address struct {
	Line1, Line2, PostalCode, City, State, Country string
}

// --- Stripe ---------------------------------------------------------------

// StripePriceParams describes one Stripe Price of a plan.
type StripePriceParams struct {
	ProductID   string
	LookupKey   string // moved to this price ("transfer_lookup_key")
	AmountCents int64
	Currency    string
	// Interval is "month" or "year" for a recurring price, "" for one-time.
	Interval string
	Nickname string
}

// CheckoutLine is one line of a Checkout Session.
type CheckoutLine struct {
	PriceID  string
	Quantity int64
}

// CheckoutParams creates a Stripe Checkout Session.
type CheckoutParams struct {
	CheckoutID string // client_reference_id and metadata tg_checkout; idempotency key
	CustomerID string
	// Mode is "subscription" or "payment".
	Mode  string
	Lines []CheckoutLine
	// TrialDays, for Mode "subscription": days before the first charge (0 = none).
	TrialDays int
	// SaveCard, for Mode "payment": keep the card for a subscription created later.
	SaveCard        bool
	CollectShipping bool
	SuccessURL      string
	CancelURL       string
	// Metadata goes on the session and, in subscription mode, the subscription.
	Metadata map[string]string
}

// CheckoutSession is a created session.
type CheckoutSession struct {
	ID  string
	URL string
}

// SubscriptionParams creates a subscription for a saved card (a plan that
// starts on activation).
type SubscriptionParams struct {
	IdempotencyKey  string
	CustomerID      string
	PriceID         string
	PaymentMethodID string
	TrialEnd        time.Time
	Metadata        map[string]string
}

// StripeSubscription is Stripe's view of a subscription.
type StripeSubscription struct {
	ID                string
	CustomerID        string
	Status            string // trialing, active, past_due, canceled, incomplete, incomplete_expired, unpaid, paused
	CurrentPeriodEnd  time.Time
	TrialEnd          *time.Time
	CancelAtPeriodEnd bool
	PriceID           string
	Metadata          map[string]string
}

// PortalParams configures the Customer Portal.
type PortalParams struct {
	ReturnURL string
	// SwitchProducts lists, per Stripe product, the prices a customer may
	// switch between (plans of one group).
	SwitchProducts map[string][]string
}

// Stripe is everything the gateway asks Stripe. Implementations pin the API
// version of the SDK they use (stripe-go/v87: 2026-09-30.endive).
type Stripe interface {
	// EnsureProduct creates the product (existingID "") or renames it, and
	// returns its id.
	EnsureProduct(ctx context.Context, existingID, name, planKey string) (string, error)
	// CreatePrice creates a price and moves its lookup key to it.
	CreatePrice(ctx context.Context, p StripePriceParams) (string, error)
	// ArchivePrice deactivates a price (existing subscriptions keep it).
	ArchivePrice(ctx context.Context, priceID string) error
	// EnsureCustomer creates the customer (existingID "") or updates name
	// and email, and returns its id.
	EnsureCustomer(ctx context.Context, existingID, subject, name, email string) (string, error)
	CreateCheckoutSession(ctx context.Context, p CheckoutParams) (CheckoutSession, error)
	CreateSubscription(ctx context.Context, p SubscriptionParams) (StripeSubscription, error)
	// GetSubscription reads a subscription (a webhook that arrives before the
	// subscription's own event).
	GetSubscription(ctx context.Context, subscriptionID string) (StripeSubscription, error)
	// CancelSubscription cancels at once (atPeriodEnd false) or at the end
	// of the current period.
	CancelSubscription(ctx context.Context, subscriptionID string, atPeriodEnd bool) (StripeSubscription, error)
	// ConfigurePortal creates or updates the portal configuration and
	// returns its id.
	ConfigurePortal(ctx context.Context, p PortalParams) (string, error)
	PortalURL(ctx context.Context, customerID, returnURL string) (string, error)
	// Refund refunds amountCents (0 = everything left) of a payment intent.
	Refund(ctx context.Context, paymentIntentID string, amountCents int64, idempotencyKey string) error
	// ParseWebhook verifies a webhook request (ErrBadSignature) and decodes it.
	ParseWebhook(payload []byte, signature string) (Event, error)
}

// Event types the gateway acts on; any other comes back with Type set and
// nothing else.
const (
	EventCheckoutCompleted    = "checkout.session.completed"
	EventCheckoutExpired      = "checkout.session.expired"
	EventSubscriptionCreated  = "customer.subscription.created"
	EventSubscriptionUpdated  = "customer.subscription.updated"
	EventSubscriptionDeleted  = "customer.subscription.deleted"
	EventInvoicePaid          = "invoice.paid"
	EventInvoicePaymentFailed = "invoice.payment_failed"
	EventChargeRefunded       = "charge.refunded"
)

// Event is a verified Stripe event; exactly one payload field is set for the
// types above.
type Event struct {
	ID           string
	Type         string
	Created      time.Time
	Checkout     *CheckoutEvent
	Subscription *StripeSubscription
	Invoice      *InvoiceEvent
	Refund       *RefundEvent
}

// CheckoutEvent is a completed or expired Checkout Session.
type CheckoutEvent struct {
	SessionID       string
	CheckoutID      string // client_reference_id
	Mode            string
	Paid            bool
	CustomerID      string
	SubscriptionID  string // mode subscription
	PaymentIntentID string // mode payment
	PaymentMethodID string // the saved card, when SaveCard
	AmountCents     int64
	Currency        string
	Lines           []EventLine
	Name, Email     string
	Phone           string
	TaxID           string // the customer's NIF, "" when not given
	BillingAddress  Address
	ShippingName    string
	ShippingAddress *Address
	Metadata        map[string]string
}

// EventLine is one charged line.
type EventLine struct {
	Description string
	PriceID     string
	AmountCents int64 // VAT included
	PeriodStart *time.Time
	PeriodEnd   *time.Time
}

// InvoiceEvent is a paid or failed Stripe invoice.
type InvoiceEvent struct {
	InvoiceID       string
	CustomerID      string
	SubscriptionID  string
	PaymentIntentID string
	AmountCents     int64 // paid (invoice.paid) or due (payment_failed)
	Currency        string
	Lines           []EventLine
	PaidAt          time.Time
	// AttemptCount is the payment attempt number (payment_failed).
	AttemptCount int64
}

// RefundEvent is a refunded charge.
type RefundEvent struct {
	ChargeID        string
	PaymentIntentID string
	// InvoiceID is the Stripe invoice the charge paid, "" for a Checkout
	// payment in payment mode.
	InvoiceID string
	// RefundedCents is everything refunded so far on the charge.
	RefundedCents int64
	Currency      string
	RefundID      string // the latest refund
}

// --- Odoo -----------------------------------------------------------------

// OdooPartner is the customer an invoice is issued to.
type OdooPartner struct {
	Ref       string // "tg:<subject>", the lookup key
	Name      string
	Email     string
	VAT       string // "ES<NIF>", or "" for a simplified invoice
	IsCompany bool
	Address   Address
}

// OdooLine is one invoice line.
type OdooLine struct {
	ProductID   int64
	Description string
	AmountCents int64 // VAT included
}

// OdooMove is an invoice (out_invoice) or credit note (out_refund) to issue.
type OdooMove struct {
	Ref        string // the Stripe reference: idempotency key and payment memo
	PartnerID  int64
	CreditNote bool
	// ReversedMoveID is the invoice a credit note corrects.
	ReversedMoveID int64
	Lines          []OdooLine
	PaidAt         time.Time
}

// OdooIssued is what Odoo numbered.
type OdooIssued struct {
	MoveID     int64
	Number     string
	IssuedAt   time.Time
	Simplified bool
}

// Odoo is everything the gateway asks Odoo, over JSON-2. Every method is
// idempotent: it checks what already exists before creating anything.
type Odoo interface {
	// UpsertPartner finds the partner by Ref, creates or updates it, and
	// returns its id.
	UpsertPartner(ctx context.Context, p OdooPartner) (int64, error)
	// UpsertProduct finds the service product by code ("TG-<plan>"),
	// creates or renames it, and returns its id.
	UpsertProduct(ctx context.Context, code, name string) (int64, error)
	// IssueMove creates (or finds, by Ref), posts and pays the move, renders its PDF,
	// and returns its number. A credit note fails with ErrNoCreditNoteSeries
	// while the sales journal has no dedicated credit note sequence.
	IssueMove(ctx context.Context, m OdooMove) (OdooIssued, error)
	// PDF returns the PDF Odoo rendered when it issued the move, or ErrPDFNotFound.
	PDF(ctx context.Context, moveID int64) (filename string, data []byte, err error)
	// Checks returns the setup checks of doc/billing.md's setup page.
	Checks(ctx context.Context) []SetupCheck
}

// Mailer sends the invoice emails (notification.EmailProvider).
type Mailer interface {
	SendMail(ctx context.Context, m notification.Mail) error
}

var (
	// ErrNoCreditNoteSeries: Spanish credit notes need their own series.
	ErrNoCreditNoteSeries = errors.New(`the Odoo sales journal needs "Dedicated Credit Note Sequence" on, so credit notes get their own series`)
	// ErrPDFNotFound: Odoo has no PDF for the move.
	ErrPDFNotFound = errors.New("invoice PDF not found")
)

// Setup check statuses.
const (
	CheckOK      = "ok"
	CheckWarn    = "check"
	CheckMissing = "missing"
	CheckManual  = "manual"
)

// SetupCheck is one line of the billing setup page.
type SetupCheck struct {
	ID     string
	Title  string
	Status string
	Detail string
	// Action is the id of a fix the gateway can apply, "" when none.
	Action string
	// Link is where to fix it by hand, "" when none.
	Link string
}
