package billing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orderScenario is one subscription's life as Stripe sends it, with the
// state Stripe would answer when asked.
type orderScenario struct {
	e         *testEnv
	orgID     string
	subject   string
	plan      *db.BillingPlan
	trialEnd  time.Time
	periodEnd time.Time
	meta      map[string]string
	events    map[string]Event
}

func newOrderScenario(t *testing.T) *orderScenario {
	t.Helper()
	e := newEnv(t)
	plan, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	s := &orderScenario{e: e, orgID: e.org(t, "Escuela", "u1")}
	s.subject = "org:" + s.orgID
	s.plan = &plan.BillingPlan
	s.trialEnd = e.clock.AddDate(0, 0, 14)
	s.periodEnd = s.trialEnd.AddDate(0, 1, 0)
	checkout := e.checkout(t, s.subject, plan.Key)
	s.meta = map[string]string{"tg_subject": s.subject, "tg_plan": plan.Key, "tg_checkout": checkout}
	line := EventLine{PriceID: plan.StripePriceID, AmountCents: 2900, PeriodStart: &s.trialEnd, PeriodEnd: &s.periodEnd}
	s.events = map[string]Event{
		"completed": {ID: "evt_completed", Type: EventCheckoutCompleted, Created: e.clock, Checkout: &CheckoutEvent{
			CheckoutID: checkout, Mode: "subscription", Paid: true, CustomerID: "cus_1", SubscriptionID: "sub_1",
		}},
		"created": {ID: "evt_created", Type: EventSubscriptionCreated, Created: e.clock, Subscription: &StripeSubscription{
			ID: "sub_1", Status: "trialing", CurrentPeriodEnd: s.trialEnd, TrialEnd: &s.trialEnd, PriceID: plan.StripePriceID, Metadata: s.meta,
		}},
		"active": {ID: "evt_active", Type: EventSubscriptionUpdated, Created: s.trialEnd, Subscription: s.active()},
		"paid": {ID: "evt_paid", Type: EventInvoicePaid, Created: s.trialEnd, Invoice: &InvoiceEvent{
			InvoiceID: "in_1", SubscriptionID: "sub_1", AmountCents: 2900, Currency: "eur", PaidAt: s.trialEnd, Lines: []EventLine{line},
		}},
	}
	return s
}

func (s *orderScenario) active() *StripeSubscription {
	return &StripeSubscription{
		ID: "sub_1", Status: "active", CurrentPeriodEnd: s.periodEnd, TrialEnd: &s.trialEnd, PriceID: s.plan.StripePriceID, Metadata: s.meta,
	}
}

// stripeNow is what Stripe answers when the gateway asks for the subscription.
func (s *orderScenario) stripeNow(sub *StripeSubscription) { s.e.stripe.Subscriptions["sub_1"] = sub }

func (s *orderScenario) send(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		s.e.event(t, s.events[n])
	}
}

// assertActive checks the state every delivery order must end in.
func (s *orderScenario) assertActive(t *testing.T, msg string) {
	t.Helper()
	e := s.e
	sub, err := e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err, msg)
	assert.Equal(t, db.SubscriptionActive, sub.Status, msg)
	assert.Equal(t, s.subject, sub.Subject, msg)
	assert.Equal(t, "u1", sub.PayerUserID, msg)
	require.NotNil(t, sub.CurrentPeriodEnd, msg)
	assert.True(t, sub.CurrentPeriodEnd.Equal(s.periodEnd), msg)
	assert.Nil(t, sub.CanceledAt, msg)

	checkout, err := e.repo.GetCheckout(s.meta["tg_checkout"])
	require.NoError(t, err, msg)
	assert.Equal(t, db.CheckoutStatusCompleted, checkout.Status, msg)

	docs, err := e.repo.ListDocuments(repositories.DocumentFilter{})
	require.NoError(t, err, msg)
	require.Len(t, docs, 1, msg)
	assert.Equal(t, "in_1", docs[0].StripeRef, msg)

	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err, msg)
	assert.Equal(t, StatusActive, ent.Orgs[s.orgID]["academia.access"].Status, msg)

	stuck, err := e.repo.StuckEvents()
	require.NoError(t, err, msg)
	assert.Empty(t, stuck, msg)
}

func permutations(names []string) [][]string {
	if len(names) <= 1 {
		return [][]string{names}
	}
	var out [][]string
	for i, n := range names {
		rest := append(append([]string{}, names[:i]...), names[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]string{n}, p...))
		}
	}
	return out
}

// Whatever the order Stripe's four events of a trial turning into a paid
// subscription arrive in, the end state is the one of the in-order delivery.
func TestWebhookOrder_EveryOrderEndsActive(t *testing.T) {
	for _, order := range permutations([]string{"completed", "created", "active", "paid"}) {
		s := newOrderScenario(t)
		s.stripeNow(s.active())
		s.send(t, order...)
		s.assertActive(t, "order "+joinNames(order))
	}
}

func joinNames(names []string) string {
	out := ""
	for _, n := range names {
		out += n + " "
	}
	return out
}

// invoice.paid first while Stripe is unreachable: retried, and once the rest
// has arrived the retry gives the same end state, with one invoice.
func TestWebhookOrder_InvoiceRetriedAfterTheRest(t *testing.T) {
	s := newOrderScenario(t)
	s.e.stripe.Err = assert.AnError
	err := s.e.svc.HandleEvent(context.Background(), s.events["paid"])
	assert.ErrorIs(t, err, ErrRetryLater)
	s.e.stripe.Err = nil

	s.send(t, "completed", "created", "active")
	s.e.event(t, s.events["paid"]) // Stripe's retry
	s.assertActive(t, "retried invoice")
}

// A stale subscription event never overwrites newer state, and never brings
// a canceled subscription back.
func TestWebhookOrder_StaleSubscriptionEvents(t *testing.T) {
	s := newOrderScenario(t)
	s.send(t, "completed", "created", "active", "paid")

	// An older update (still trialing) arrives late.
	s.e.event(t, Event{ID: "evt_old", Type: EventSubscriptionUpdated, Created: s.e.clock, Subscription: &StripeSubscription{
		ID: "sub_1", Status: "trialing", CurrentPeriodEnd: s.trialEnd, TrialEnd: &s.trialEnd, PriceID: s.plan.StripePriceID,
	}})
	s.assertActive(t, "after a stale update")

	// Canceled, then the update that preceded the cancellation arrives.
	cancelAt := s.periodEnd
	s.e.event(t, Event{ID: "evt_deleted", Type: EventSubscriptionDeleted, Created: cancelAt, Subscription: &StripeSubscription{
		ID: "sub_1", Status: "canceled", CurrentPeriodEnd: s.periodEnd,
	}})
	s.e.event(t, Event{ID: "evt_late", Type: EventSubscriptionUpdated, Created: s.trialEnd.Add(time.Hour), Subscription: s.active()})
	sub, err := s.e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionCanceled, sub.Status, "a stale update doesn't resurrect it")
	require.NotNil(t, sub.CanceledAt)
	ent, err := s.e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Empty(t, ent.Orgs[s.orgID])
}

// customer.subscription.deleted can arrive before the last invoice.paid: the
// charge still gets its invoice and the subscription stays canceled.
func TestWebhookOrder_DeletedBeforeLastInvoice(t *testing.T) {
	s := newOrderScenario(t)
	s.send(t, "completed", "created", "active")
	s.e.event(t, Event{ID: "evt_deleted", Type: EventSubscriptionDeleted, Created: s.periodEnd, Subscription: &StripeSubscription{
		ID: "sub_1", Status: "canceled", CurrentPeriodEnd: s.periodEnd,
	}})
	s.send(t, "paid")

	sub, err := s.e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionCanceled, sub.Status)
	docs, err := s.e.repo.ListDocuments(repositories.DocumentFilter{})
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

// The same event delivered twice, one after the other and at the same time,
// is applied once.
func TestWebhookOrder_DuplicateEvents(t *testing.T) {
	s := newOrderScenario(t)
	s.send(t, "completed", "created", "active")
	for i := 0; i < 2; i++ {
		s.send(t, "paid")
	}

	s2 := newOrderScenario(t)
	s2.send(t, "completed", "created", "active")
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s2.e.svc.HandleEvent(context.Background(), s2.events["paid"])
		}()
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err)
	}
	s.assertActive(t, "sequential duplicates")
	s2.assertActive(t, "concurrent duplicates")
}

// checkout.session.expired after the payment completed doesn't undo it.
func TestWebhookOrder_ExpiredAfterCompleted(t *testing.T) {
	s := newOrderScenario(t)
	s.send(t, "completed", "created", "active", "paid")
	s.e.event(t, Event{ID: "evt_expired", Type: EventCheckoutExpired, Created: s.trialEnd.Add(time.Hour), Checkout: &CheckoutEvent{CheckoutID: s.meta["tg_checkout"]}})
	s.assertActive(t, "expired after completed")
}

// charge.refunded can reach the gateway before the invoice.paid of the same
// charge: it is retried, not lost, and gives the credit note once the invoice
// exists, with no duplicates when it arrives again.
func TestWebhookOrder_RefundBeforeInvoice(t *testing.T) {
	s := newOrderScenario(t)
	s.stripeNow(s.active())
	s.send(t, "completed", "created", "active")
	refund := Event{ID: "evt_refund", Type: EventChargeRefunded, Created: s.e.clock, Refund: &RefundEvent{
		ChargeID: "ch_1", InvoiceID: "in_1", RefundedCents: 1000, RefundID: "re_1",
	}}

	err := s.e.svc.HandleEvent(context.Background(), refund)
	assert.ErrorIs(t, err, ErrRetryLater)
	stuck, err := s.e.repo.StuckEvents()
	require.NoError(t, err)
	assert.Len(t, stuck, 1)

	s.send(t, "paid")
	s.e.event(t, refund)
	s.e.event(t, refund)
	inv, err := s.e.repo.GetDocumentByStripeRef("in_1")
	require.NoError(t, err)
	credit, err := s.e.repo.GetDocumentByStripeRef("re_1")
	require.NoError(t, err)
	assert.Equal(t, db.DocumentCreditNote, credit.Kind)
	assert.Equal(t, int64(1000), credit.AmountCents)
	assert.Equal(t, inv.ID, *credit.InvoiceDocumentID)
	credited, err := s.e.repo.CreditedCents(inv.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), credited)
	stuck, _ = s.e.repo.StuckEvents()
	assert.Empty(t, stuck)

	// The invoice reaches Odoo before its credit note, whatever the order they were queued in.
	s.e.customer(t, s.subject, "b12345674")
	assert.Equal(t, 2, s.e.process(t))
	assert.Equal(t, db.OdooStateDone, s.e.doc(t, credit.ID).OdooState)
}

// A refund of a charge the gateway never invoiced stops being retried once
// it is old enough that the invoice can't still be on its way.
func TestWebhookOrder_ForeignRefundStopsBeingRetried(t *testing.T) {
	e := newEnv(t)
	refund := Event{ID: "evt_foreign", Type: EventChargeRefunded, Created: e.clock, Refund: &RefundEvent{ChargeID: "ch_x", InvoiceID: "in_x", RefundedCents: 500, RefundID: "re_x"}}
	assert.ErrorIs(t, e.svc.HandleEvent(context.Background(), refund), ErrRetryLater)
	e.clock = e.clock.Add(refundWaitsFor + time.Minute)
	require.NoError(t, e.svc.HandleEvent(context.Background(), refund))
	stuck, err := e.repo.StuckEvents()
	require.NoError(t, err)
	assert.Empty(t, stuck)
}
