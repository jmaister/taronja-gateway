package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// FakeStripe is an in-memory Stripe for tests of the billing service. It
// records every call; its ids are sequential ("cus_1", "price_2", ...).
// ParseWebhook accepts a JSON-encoded Event signed with FakeWebhookSignature.
type FakeStripe struct {
	mu sync.Mutex
	n  int

	Products        map[string]string // id -> name
	Prices          map[string]StripePriceParams
	ArchivedPrice   map[string]bool
	LookupKeys      map[string]string // lookup key -> price id
	Customers       map[string]string // id -> subject
	PortalReturnURL string
	Sessions        []CheckoutParams
	Subscriptions   map[string]*StripeSubscription
	Created         []SubscriptionParams
	Canceled        []string
	Refunds         []FakeRefund
	Portal          *PortalParams

	// Err, when set, is returned by every call.
	Err error
}

// FakeRefund is one recorded refund.
type FakeRefund struct {
	PaymentIntentID string
	AmountCents     int64
	IdempotencyKey  string
}

// FakeWebhookSignature is the signature FakeStripe.ParseWebhook accepts.
const FakeWebhookSignature = "fake-signature"

// NewFakeStripe returns an empty FakeStripe.
func NewFakeStripe() *FakeStripe {
	return &FakeStripe{
		Products: map[string]string{}, Prices: map[string]StripePriceParams{}, ArchivedPrice: map[string]bool{},
		LookupKeys: map[string]string{}, Customers: map[string]string{}, Subscriptions: map[string]*StripeSubscription{},
	}
}

func (f *FakeStripe) id(prefix string) string {
	f.n++
	return fmt.Sprintf("%s_%d", prefix, f.n)
}

func (f *FakeStripe) EnsureProduct(ctx context.Context, existingID, name, planKey string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	if existingID == "" {
		existingID = f.id("prod")
	}
	f.Products[existingID] = name
	return existingID, nil
}

func (f *FakeStripe) CreatePrice(ctx context.Context, p StripePriceParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	id := f.id("price")
	f.Prices[id] = p
	if p.LookupKey != "" {
		f.LookupKeys[p.LookupKey] = id
	}
	return id, nil
}

func (f *FakeStripe) ArchivePrice(ctx context.Context, priceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.ArchivedPrice[priceID] = true
	return nil
}

func (f *FakeStripe) EnsureCustomer(ctx context.Context, existingID, subject, name, email string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	if existingID == "" {
		existingID = f.id("cus")
	}
	f.Customers[existingID] = subject
	return existingID, nil
}

func (f *FakeStripe) CreateCheckoutSession(ctx context.Context, p CheckoutParams) (CheckoutSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return CheckoutSession{}, f.Err
	}
	f.Sessions = append(f.Sessions, p)
	id := f.id("cs")
	return CheckoutSession{ID: id, URL: "https://checkout.stripe.test/" + id}, nil
}

func (f *FakeStripe) CreateSubscription(ctx context.Context, p SubscriptionParams) (StripeSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return StripeSubscription{}, f.Err
	}
	for _, earlier := range f.Created {
		if earlier.IdempotencyKey == p.IdempotencyKey {
			for _, s := range f.Subscriptions {
				if s.Metadata["tg_idempotency"] == p.IdempotencyKey {
					return *s, nil
				}
			}
		}
	}
	f.Created = append(f.Created, p)
	trialEnd := p.TrialEnd
	sub := &StripeSubscription{
		ID: f.id("sub"), CustomerID: p.CustomerID, Status: "trialing", CurrentPeriodEnd: trialEnd, TrialEnd: &trialEnd,
		PriceID: p.PriceID, Metadata: map[string]string{"tg_idempotency": p.IdempotencyKey},
	}
	for k, v := range p.Metadata {
		sub.Metadata[k] = v
	}
	f.Subscriptions[sub.ID] = sub
	return *sub, nil
}

func (f *FakeStripe) GetSubscription(ctx context.Context, subscriptionID string) (StripeSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return StripeSubscription{}, f.Err
	}
	sub, ok := f.Subscriptions[subscriptionID]
	if !ok {
		return StripeSubscription{}, fmt.Errorf("no such subscription: %s", subscriptionID)
	}
	return *sub, nil
}

func (f *FakeStripe) CancelSubscription(ctx context.Context, subscriptionID string, atPeriodEnd bool) (StripeSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return StripeSubscription{}, f.Err
	}
	sub, ok := f.Subscriptions[subscriptionID]
	if !ok {
		return StripeSubscription{}, fmt.Errorf("no such subscription: %s", subscriptionID)
	}
	f.Canceled = append(f.Canceled, subscriptionID)
	if atPeriodEnd {
		sub.CancelAtPeriodEnd = true
	} else {
		sub.Status = "canceled"
	}
	return *sub, nil
}

func (f *FakeStripe) ConfigurePortal(ctx context.Context, p PortalParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	f.Portal = &p
	return "bpc_1", nil
}

func (f *FakeStripe) PortalURL(ctx context.Context, customerID, returnURL string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	f.mu.Lock()
	f.PortalReturnURL = returnURL
	f.mu.Unlock()
	return "https://billing.stripe.test/p/" + customerID, nil
}

func (f *FakeStripe) Refund(ctx context.Context, paymentIntentID string, amountCents int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.Refunds = append(f.Refunds, FakeRefund{PaymentIntentID: paymentIntentID, AmountCents: amountCents, IdempotencyKey: idempotencyKey})
	return nil
}

func (f *FakeStripe) ParseWebhook(payload []byte, signature string) (Event, error) {
	if signature != FakeWebhookSignature {
		return Event{}, ErrBadSignature
	}
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return Event{}, fmt.Errorf("decoding fake event: %w", err)
	}
	return event, nil
}

// FakeOdoo is an in-memory Odoo for tests of the billing service. Moves are
// numbered "F/<year>/<n>" and credit notes "RF/<year>/<n>".
type FakeOdoo struct {
	mu sync.Mutex
	n  int64

	Partners   map[string]OdooPartner // ref -> partner
	PartnerIDs map[string]int64
	Products   map[string]string // code -> name
	ProductIDs map[string]int64
	Moves      map[string]FakeMove // ref -> move
	// NoCreditNoteSeries makes credit notes fail with ErrNoCreditNoteSeries.
	NoCreditNoteSeries bool
	// FailIssue, when set, is returned by IssueMove (then cleared if FailOnce).
	FailIssue error
	FailOnce  bool
	Now       func() time.Time
}

// FakeMove is an issued move.
type FakeMove struct {
	OdooMove
	Issued OdooIssued
}

// NewFakeOdoo returns an empty FakeOdoo.
func NewFakeOdoo() *FakeOdoo {
	return &FakeOdoo{
		Partners: map[string]OdooPartner{}, PartnerIDs: map[string]int64{}, Products: map[string]string{},
		ProductIDs: map[string]int64{}, Moves: map[string]FakeMove{}, Now: time.Now,
	}
}

func (f *FakeOdoo) UpsertPartner(ctx context.Context, p OdooPartner) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.PartnerIDs[p.Ref]
	if !ok {
		f.n++
		id = f.n
		f.PartnerIDs[p.Ref] = id
	}
	f.Partners[p.Ref] = p
	return id, nil
}

func (f *FakeOdoo) UpsertProduct(ctx context.Context, code, name string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.ProductIDs[code]
	if !ok {
		f.n++
		id = f.n
		f.ProductIDs[code] = id
	}
	f.Products[code] = name
	return id, nil
}

func (f *FakeOdoo) IssueMove(ctx context.Context, m OdooMove) (OdooIssued, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailIssue != nil {
		err := f.FailIssue
		if f.FailOnce {
			f.FailIssue = nil
		}
		return OdooIssued{}, err
	}
	if existing, ok := f.Moves[m.Ref]; ok {
		return existing.Issued, nil
	}
	if m.CreditNote && f.NoCreditNoteSeries {
		return OdooIssued{}, ErrNoCreditNoteSeries
	}
	prefix := "F"
	if m.CreditNote {
		prefix = "RF"
	}
	count := 1
	for _, mv := range f.Moves {
		if mv.CreditNote == m.CreditNote {
			count++
		}
	}
	f.n++
	now := f.Now()
	issued := OdooIssued{MoveID: f.n, Number: fmt.Sprintf("%s/%d/%05d", prefix, now.Year(), count), IssuedAt: now}
	f.Moves[m.Ref] = FakeMove{OdooMove: m, Issued: issued}
	return issued, nil
}

func (f *FakeOdoo) PDF(ctx context.Context, moveID int64) (string, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, mv := range f.Moves {
		if mv.Issued.MoveID == moveID {
			name := mv.Issued.Number + ".pdf"
			return name, []byte("%PDF-1.4 fake " + mv.Issued.Number), nil
		}
	}
	return "", nil, ErrPDFNotFound
}

func (f *FakeOdoo) Checks(ctx context.Context) []SetupCheck {
	return []SetupCheck{{ID: "odoo-connection", Title: "Odoo connection", Status: CheckOK}}
}

var (
	_ Stripe = (*FakeStripe)(nil)
	_ Odoo   = (*FakeOdoo)(nil)
)
