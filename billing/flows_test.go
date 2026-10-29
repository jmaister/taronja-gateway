package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var payer = Payer{UserID: "u1", Name: "Ana", Email: "ana@example.com"}

func (e *testEnv) withNotifications() *repositories.NotificationRepositoryDB {
	conn := db.GetConnection()
	repo := repositories.NewNotificationRepositoryDB(conn)
	e.svc.SetNotifications(notification.NewService(config.NotificationConfig{}, repo, repositories.NewDBUserRepository(conn), ""))
	return repo
}

func (e *testEnv) checkout(t *testing.T, subject, planKey string) string {
	t.Helper()
	c := &db.BillingCheckout{Subject: subject, PlanKey: planKey, UserID: "u1", Status: db.CheckoutStatusOpen}
	require.NoError(t, e.repo.CreateCheckout(c))
	return c.ID
}

func (e *testEnv) event(t *testing.T, ev Event) {
	t.Helper()
	if ev.Created.IsZero() {
		ev.Created = e.clock
	}
	require.NoError(t, e.svc.HandleEvent(context.Background(), ev))
}

func (e *testEnv) notificationsOf(t *testing.T, userID string) []string {
	t.Helper()
	list, err := e.svc.notifications.List(userID, false, 50, nil)
	require.NoError(t, err)
	var types []string
	for _, n := range list {
		types = append(types, n.Type)
	}
	return types
}

func TestStartCheckout_SubscriptionWithTrial(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := monthlyInput()
	in.SetupFeeCents, in.SetupFeeName = 5000, "Alta"
	plan, err := e.svc.CreatePlan(ctx, in)
	require.NoError(t, err)
	orgID := e.org(t, "Escuela", "u1")

	checkout, url, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer, OrgID: orgID, Redirect: "/settings"})
	require.NoError(t, err)
	assert.Contains(t, url, "https://checkout.stripe.test/")
	require.Len(t, e.stripe.Sessions, 1)
	p := e.stripe.Sessions[0]
	assert.Equal(t, "subscription", p.Mode)
	assert.Equal(t, 14, p.TrialDays)
	assert.Equal(t, []CheckoutLine{{PriceID: plan.StripePriceID, Quantity: 1}, {PriceID: plan.StripeSetupFeePriceID, Quantity: 1}}, p.Lines)
	assert.Equal(t, "https://gw.example.com/_/pay/done?checkout="+checkout.ID, p.SuccessURL)
	assert.Equal(t, "https://gw.example.com/_/pay/academia-monthly?org="+orgID, p.CancelURL)
	assert.Equal(t, map[string]string{"tg_checkout": checkout.ID, "tg_subject": "org:" + orgID, "tg_plan": plan.Key}, p.Metadata)
	assert.Equal(t, "org:"+orgID, e.stripe.Customers[p.CustomerID])
	assert.Equal(t, "/settings", checkout.Redirect)
	assert.Equal(t, db.CheckoutStatusOpen, checkout.Status)

	c, err := e.repo.GetCustomer("org:" + orgID)
	require.NoError(t, err)
	assert.Equal(t, "Escuela", c.Name, "an organization's customer is named after it")
	assert.Equal(t, "ana@example.com", c.Email)
}

func TestStartCheckout_Rules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, monthlyInput()) // subject org
	require.NoError(t, err)
	orgID := e.org(t, "Escuela", "u1")
	other := e.org(t, "Otra")

	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	assert.ErrorIs(t, err, ErrInvalid, "an org plan needs an org")
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer, OrgID: other})
	assert.ErrorIs(t, err, ErrForbidden, "not a member")
	require.NoError(t, e.orgs.SetMember(other, "u1", "member"))
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer, OrgID: other})
	assert.ErrorIs(t, err, ErrForbidden, "a member who isn't an owner")
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: "missing", Payer: payer, OrgID: orgID})
	assert.ErrorIs(t, err, ErrNotFound)

	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer, OrgID: orgID, Redirect: "https://evil.example/x"})
	require.NoError(t, err)
	assert.Empty(t, checkout.Redirect, "only paths on this site")

	// Subscribed in the group: no second checkout, and no second trial.
	e.event(t, Event{ID: "evt_s", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_1", Status: "trialing", Metadata: map[string]string{"tg_subject": "org:" + orgID, "tg_plan": plan.Key, "tg_checkout": checkout.ID},
	}})
	e.event(t, Event{ID: "evt_c", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{CheckoutID: checkout.ID, Mode: "subscription", Paid: true, SubscriptionID: "sub_1"}})
	yearly := monthlyInput()
	yearly.Key, yearly.Interval = "academia-yearly", db.PlanIntervalYear
	y, err := e.svc.CreatePlan(ctx, yearly)
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: y.Key, Payer: payer, OrgID: orgID})
	assert.ErrorIs(t, err, ErrAlreadySubscribed)

	sub, err := e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	e.stripe.Subscriptions["sub_1"] = &StripeSubscription{ID: "sub_1", Status: "trialing"}
	_, err = e.svc.CancelSubscription(ctx, sub.ID, false)
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: y.Key, Payer: payer, OrgID: orgID})
	require.NoError(t, err)
	assert.Equal(t, 0, e.stripe.Sessions[len(e.stripe.Sessions)-1].TrialDays, "one trial per group")

	archived := PlanInputFrom(*y)
	archived.Active = false
	_, err = e.svc.UpdatePlan(ctx, y.Key, archived)
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: y.Key, Payer: payer, OrgID: orgID})
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestSubscriptionLifecycle(t *testing.T) {
	e := newEnv(t)
	e.withNotifications()
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	orgID := e.org(t, "Escuela", "u1")
	require.NoError(t, e.orgs.SetMember(orgID, "owner1", "owner"))
	subject := "org:" + orgID
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer, OrgID: orgID})
	require.NoError(t, err)

	// checkout.session.completed before customer.subscription.created.
	completed := Event{ID: "evt_1", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
		SessionID: checkout.StripeSessionID, CheckoutID: checkout.ID, Mode: "subscription", Paid: true, CustomerID: "cus_x",
		SubscriptionID: "sub_1", Name: "Escuela SL", Email: "admin@escuela.es", TaxID: "B12345674",
		BillingAddress: Address{Line1: "C/ Mayor 1", PostalCode: "46001", City: "Valencia", Country: "ES"},
	}}
	e.event(t, completed)
	e.event(t, completed) // repeated delivery
	got, err := e.repo.GetCheckout(checkout.ID)
	require.NoError(t, err)
	assert.Equal(t, db.CheckoutStatusCompleted, got.Status)
	c, err := e.repo.GetCustomer(subject)
	require.NoError(t, err)
	assert.Equal(t, "B12345674", c.TaxID)
	assert.Equal(t, "Escuela SL", c.Name)
	assert.Equal(t, "academia", c.TrialGroups)

	trialEnd := e.clock.AddDate(0, 0, 14)
	meta := map[string]string{"tg_subject": subject, "tg_plan": plan.Key, "tg_checkout": checkout.ID}
	e.event(t, Event{ID: "evt_2", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_1", Status: "trialing", CurrentPeriodEnd: trialEnd, TrialEnd: &trialEnd, PriceID: plan.StripePriceID, Metadata: meta,
	}})
	sub, err := e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionTrialing, sub.Status)
	assert.Equal(t, "u1", sub.PayerUserID)
	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Equal(t, StatusTrial, ent.Orgs[orgID]["academia.access"].Status)

	// The trial start invoice is zero: nothing to issue.
	e.event(t, Event{ID: "evt_3", Type: EventInvoicePaid, Invoice: &InvoiceEvent{InvoiceID: "in_0", SubscriptionID: "sub_1", AmountCents: 0}})
	docs, _ := e.repo.ListDocuments(repositories.DocumentFilter{})
	assert.Empty(t, docs)

	// First charge.
	periodEnd := trialEnd.AddDate(0, 1, 0)
	e.clock = trialEnd
	e.event(t, Event{ID: "evt_4", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_1", Status: "active", CurrentPeriodEnd: periodEnd, PriceID: plan.StripePriceID}})
	e.event(t, Event{ID: "evt_5", Type: EventInvoicePaid, Invoice: &InvoiceEvent{
		InvoiceID: "in_1", SubscriptionID: "sub_1", AmountCents: 2900, Currency: "eur", PaidAt: e.clock,
		Lines: []EventLine{{Description: "1 × Academia", PriceID: plan.StripePriceID, AmountCents: 2900, PeriodStart: &trialEnd, PeriodEnd: &periodEnd}},
	}})
	inv, err := e.repo.GetDocumentByStripeRef("in_1")
	require.NoError(t, err)
	assert.Equal(t, db.DocumentInvoice, inv.Kind)
	assert.Equal(t, int64(2900), inv.AmountCents)
	lines, err := DocumentLines(inv)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "Academia mensual 08/11/2026–07/12/2026", lines[0].Description)
	ent, _ = e.svc.ForUser("u1")
	assert.Equal(t, StatusActive, ent.Orgs[orgID]["academia.access"].Status)
	until := periodEnd.AddDate(0, 0, 7)
	assert.Equal(t, &until, ent.Orgs[orgID]["academia.access"].Until)

	// The renewal fails: past due with grace, notified once.
	e.clock = periodEnd.Add(time.Hour)
	failed := Event{ID: "evt_6", Type: EventInvoicePaymentFailed, Invoice: &InvoiceEvent{InvoiceID: "in_2", SubscriptionID: "sub_1", AmountCents: 2900, AttemptCount: 1}}
	e.event(t, failed)
	failed.ID, failed.Created = "evt_7", e.clock.AddDate(0, 0, 3)
	e.event(t, failed)
	sub, _ = e.repo.GetSubscriptionByStripeID("sub_1")
	assert.Equal(t, db.SubscriptionPastDue, sub.Status)
	grace := e.clock.AddDate(0, 0, 7)
	assert.Equal(t, grace, sub.GraceUntil.UTC(), "the grace counts from the first failure")
	ent, _ = e.svc.ForUser("u1")
	assert.Equal(t, StatusPastDue, ent.Orgs[orgID]["academia.access"].Status)
	assert.Equal(t, []string{NotifyPaymentFailed}, e.notificationsOf(t, "u1"))
	assert.Equal(t, []string{NotifyPaymentFailed}, e.notificationsOf(t, "owner1"))

	// The retry succeeds.
	e.event(t, Event{ID: "evt_8", Type: EventInvoicePaid, Invoice: &InvoiceEvent{InvoiceID: "in_2", SubscriptionID: "sub_1", AmountCents: 2900, PaidAt: e.clock}})
	sub, _ = e.repo.GetSubscriptionByStripeID("sub_1")
	assert.Equal(t, db.SubscriptionActive, sub.Status)
	assert.Nil(t, sub.GraceUntil)

	// Canceled in the portal at the end of the new period, then deleted.
	nextEnd := periodEnd.AddDate(0, 1, 0)
	e.event(t, Event{ID: "evt_9", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_1", Status: "active", CurrentPeriodEnd: nextEnd, CancelAtPeriodEnd: true}})
	ent, _ = e.svc.ForUser("u1")
	assert.Equal(t, StatusCanceling, ent.Orgs[orgID]["academia.access"].Status)
	e.clock = nextEnd
	e.event(t, Event{ID: "evt_10", Type: EventSubscriptionDeleted, Subscription: &StripeSubscription{ID: "sub_1", Status: "canceled", CurrentPeriodEnd: nextEnd}})
	ent, _ = e.svc.ForUser("u1")
	assert.Empty(t, ent.Orgs[orgID])
	assert.Equal(t, []string{NotifyCanceled, NotifyPaymentFailed}, e.notificationsOf(t, "u1"))
}

func TestWebhook_OutOfOrderAndSwitch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	monthly, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	yearlyIn := monthlyInput()
	yearlyIn.Key, yearlyIn.Name, yearlyIn.Interval = "academia-yearly", "Academia anual", db.PlanIntervalYear
	yearly, err := e.svc.CreatePlan(ctx, yearlyIn)
	require.NoError(t, err)

	// invoice.paid before the subscription exists: retried later, not recorded.
	early := Event{ID: "evt_a", Type: EventInvoicePaid, Created: e.clock, Invoice: &InvoiceEvent{InvoiceID: "in_9", SubscriptionID: "sub_9", AmountCents: 2900}}
	err = e.svc.HandleEvent(ctx, early)
	assert.ErrorIs(t, err, ErrRetryLater)
	stuck, err := e.repo.StuckEvents()
	require.NoError(t, err)
	require.Len(t, stuck, 1)
	assert.Equal(t, "evt_a", stuck[0].EventID)
	assert.Equal(t, "sub_9", stuck[0].SubscriptionID)
	assert.Equal(t, 1, stuck[0].Attempts)
	assert.ErrorIs(t, e.svc.HandleEvent(ctx, early), ErrRetryLater)
	stuck, _ = e.repo.StuckEvents()
	assert.Equal(t, 2, stuck[0].Attempts)

	e.event(t, Event{ID: "evt_b", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_9", Status: "active", CurrentPeriodEnd: e.clock.AddDate(0, 1, 0), PriceID: monthly.StripePriceID,
		Metadata: map[string]string{"tg_subject": "org:o9", "tg_plan": monthly.Key, "tg_checkout": e.checkout(t, "org:o9", monthly.Key)},
	}})
	e.event(t, early) // Stripe's retry now works
	_, err = e.repo.GetDocumentByStripeRef("in_9")
	require.NoError(t, err)
	stuck, err = e.repo.StuckEvents()
	require.NoError(t, err)
	assert.Empty(t, stuck, "applied: no longer waiting")

	// Switched to yearly in the portal.
	e.event(t, Event{ID: "evt_c", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_9", Status: "active", CurrentPeriodEnd: e.clock.AddDate(1, 0, 0), PriceID: yearly.StripePriceID}})
	sub, err := e.repo.GetSubscriptionByStripeID("sub_9")
	require.NoError(t, err)
	assert.Equal(t, yearly.Key, sub.PlanKey)

	// Not the gateway's subscription: ignored.
	e.event(t, Event{ID: "evt_d", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_other", Status: "active"}})
	_, err = e.repo.GetSubscriptionByStripeID("sub_other")
	assert.Error(t, err)
}

// Metadata is a pointer, not a source of truth: it can't attach a
// subscription to a subject or plan its checkout wasn't for.
func TestWebhook_ForgedMetadataIgnored(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	monthly, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	mine := e.checkout(t, "org:mine", monthly.Key)

	for i, meta := range []map[string]string{
		{"tg_subject": "org:victim", "tg_plan": monthly.Key, "tg_checkout": mine},
		{"tg_subject": "org:mine", "tg_plan": "other-plan", "tg_checkout": mine},
		{"tg_subject": "org:mine", "tg_plan": monthly.Key, "tg_checkout": "nope"},
		{"tg_subject": "org:mine", "tg_plan": monthly.Key},
	} {
		id := fmt.Sprintf("sub_forged%d", i)
		e.event(t, Event{ID: "evt_" + id, Type: EventSubscriptionCreated, Subscription: &StripeSubscription{ID: id, Status: "active", Metadata: meta}})
		_, err = e.repo.GetSubscriptionByStripeID(id)
		assert.Error(t, err, "case %d", i)
	}

	row := &db.BillingSubscription{Subject: "org:mine", PlanKey: monthly.Key, Status: db.SubscriptionAwaitingActivation}
	require.NoError(t, e.repo.CreateSubscription(row))
	e.event(t, Event{ID: "evt_rowforged", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_rowforged", Status: "active", Metadata: map[string]string{"tg_subscription": row.ID, "tg_subject": "org:victim"},
	}})
	_, err = e.repo.GetSubscriptionByStripeID("sub_rowforged")
	assert.Error(t, err)
}

// An invoice event that arrives before its subscription's own event reads
// the subscription from Stripe and is applied at once.
func TestWebhook_InvoiceBeforeSubscriptionFetchesIt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	monthly, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	periodEnd := e.clock.AddDate(0, 1, 0)
	e.stripe.Subscriptions["sub_f"] = &StripeSubscription{
		ID: "sub_f", Status: "active", CurrentPeriodEnd: periodEnd, PriceID: monthly.StripePriceID,
		Metadata: map[string]string{"tg_subject": "org:of", "tg_plan": monthly.Key, "tg_checkout": e.checkout(t, "org:of", monthly.Key)},
	}

	e.event(t, Event{ID: "evt_p", Type: EventInvoicePaid, Created: e.clock, Invoice: &InvoiceEvent{InvoiceID: "in_f", SubscriptionID: "sub_f", AmountCents: 2900}})
	sub, err := e.repo.GetSubscriptionByStripeID("sub_f")
	require.NoError(t, err)
	assert.Equal(t, "org:of", sub.Subject)
	assert.Equal(t, db.SubscriptionActive, sub.Status)
	require.NotNil(t, sub.CurrentPeriodEnd)
	assert.True(t, sub.CurrentPeriodEnd.Equal(periodEnd))
	_, err = e.repo.GetDocumentByStripeRef("in_f")
	require.NoError(t, err)
	stuck, err := e.repo.StuckEvents()
	require.NoError(t, err)
	assert.Empty(t, stuck)

	// A failed renewal of a subscription the gateway has not seen starts the grace period.
	e.stripe.Subscriptions["sub_g"] = &StripeSubscription{
		ID: "sub_g", Status: "past_due", CurrentPeriodEnd: periodEnd, PriceID: monthly.StripePriceID,
		Metadata: map[string]string{"tg_subject": "org:og", "tg_plan": monthly.Key, "tg_checkout": e.checkout(t, "org:og", monthly.Key)},
	}
	e.event(t, Event{ID: "evt_q", Type: EventInvoicePaymentFailed, Created: e.clock, Invoice: &InvoiceEvent{InvoiceID: "in_g", SubscriptionID: "sub_g", AmountCents: 2900}})
	sub, err = e.repo.GetSubscriptionByStripeID("sub_g")
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionPastDue, sub.Status)
	require.NotNil(t, sub.GraceUntil)

	// Not the gateway's subscription (no metadata): ignored, not retried.
	e.stripe.Subscriptions["sub_x"] = &StripeSubscription{ID: "sub_x", Status: "active"}
	e.event(t, Event{ID: "evt_r", Type: EventInvoicePaid, Created: e.clock, Invoice: &InvoiceEvent{InvoiceID: "in_x", SubscriptionID: "sub_x", AmountCents: 2900}})
	_, err = e.repo.GetSubscriptionByStripeID("sub_x")
	assert.Error(t, err)
	stuck, _ = e.repo.StuckEvents()
	assert.Empty(t, stuck)

	// Stripe unreachable: retried later and listed.
	e.stripe.Err = errors.New("stripe down")
	err = e.svc.HandleEvent(ctx, Event{ID: "evt_s", Type: EventInvoicePaid, Created: e.clock, Invoice: &InvoiceEvent{InvoiceID: "in_s", SubscriptionID: "sub_s", AmountCents: 2900}})
	assert.ErrorIs(t, err, ErrRetryLater)
	stuck, _ = e.repo.StuckEvents()
	require.Len(t, stuck, 1)
	assert.Contains(t, stuck[0].Reason, "stripe down")
}

func TestWebhook_SignedRequest(t *testing.T) {
	e := newEnv(t)
	payload, err := json.Marshal(Event{ID: "evt_x", Type: "customer.created"})
	require.NoError(t, err)
	assert.ErrorIs(t, e.svc.HandleWebhook(context.Background(), payload, "bad"), ErrBadSignature)
	require.NoError(t, e.svc.HandleWebhook(context.Background(), payload, FakeWebhookSignature))
	fresh, err := e.repo.RecordStripeEvent("evt_x", "customer.created")
	require.NoError(t, err)
	assert.False(t, fresh, "the event id was recorded")
}

// megabox: one payment for the box and the first month, then the
// subscription starts when the box goes online; a refund issues credit notes.
func TestOnActivationPurchaseRefund(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, PlanInput{
		Key: "megabox-familia", Name: "Megabox Familia", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 990, Features: []string{"megabox.protection"}, Subject: db.PlanSubjectUser, Starts: db.PlanStartsOnActivation,
		SetupFeeCents: 4900, SetupFeeName: "Caja Megabox", CollectShipping: true, GraceDays: 7, Active: true,
	})
	require.NoError(t, err)
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)
	p := e.stripe.Sessions[0]
	assert.Equal(t, "payment", p.Mode)
	assert.True(t, p.SaveCard)
	assert.True(t, p.CollectShipping)
	assert.Equal(t, []CheckoutLine{{PriceID: plan.StripeSetupFeePriceID, Quantity: 1}, {PriceID: plan.StripeFirstPeriodPriceID, Quantity: 1}}, p.Lines)

	e.event(t, Event{ID: "evt_1", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
		CheckoutID: checkout.ID, Mode: "payment", Paid: true, CustomerID: "cus_1", PaymentIntentID: "pi_1", AmountCents: 5890,
		Currency: "eur", Name: "Ana", Email: "ana@example.com", Phone: "+34600000000",
		ShippingName: "Ana R", ShippingAddress: &Address{Line1: "C/ Paz 2", PostalCode: "46002", City: "Valencia", Country: "ES"},
	}})
	purchase, err := e.repo.GetPurchaseByPaymentIntent("pi_1")
	require.NoError(t, err)
	assert.Equal(t, db.PurchaseStatusPaid, purchase.Status)
	assert.Equal(t, "C/ Paz 2", purchase.ShippingLine1)
	assert.Equal(t, "+34600000000", purchase.ShippingPhone)
	require.NotNil(t, purchase.SubscriptionID)
	sub, err := e.repo.GetSubscription(*purchase.SubscriptionID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionAwaitingActivation, sub.Status)
	ent, _ := e.svc.ForUser("u1")
	assert.Equal(t, Feature{Status: StatusAwaitingActivation, Source: SourceSubscription}, ent.User["megabox.protection"])

	inv, err := e.repo.GetDocumentByStripeRef("pi_1")
	require.NoError(t, err)
	lines, _ := DocumentLines(inv)
	assert.Equal(t, []DocumentLine{
		{Description: "Caja Megabox", PlanKey: plan.Key, SetupFee: true, AmountCents: 4900},
		{Description: "Megabox Familia", PlanKey: plan.Key, AmountCents: 990},
	}, lines)

	// Activation, twice.
	activated, err := e.svc.ActivateSubscription(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionTrialing, activated.Status)
	require.NotNil(t, activated.StripeSubscriptionID)
	assert.Equal(t, e.clock.AddDate(0, 1, 0), activated.TrialEnd.UTC())
	again, err := e.svc.ActivateSubscription(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, *activated.StripeSubscriptionID, *again.StripeSubscriptionID)
	assert.Len(t, e.stripe.Created, 1)
	assert.Equal(t, "tg-activate-"+sub.ID, e.stripe.Created[0].IdempotencyKey)
	assert.Equal(t, plan.StripePriceID, e.stripe.Created[0].PriceID)

	// Partial refund, then the rest.
	_, err = e.svc.RefundPurchase(ctx, purchase.ID, 1000, false)
	require.NoError(t, err)
	assert.Equal(t, FakeRefund{PaymentIntentID: "pi_1", AmountCents: 1000, IdempotencyKey: "tg-refund-" + purchase.ID + "-1000"}, e.stripe.Refunds[0])
	e.event(t, Event{ID: "evt_r1", Type: EventChargeRefunded, Refund: &RefundEvent{ChargeID: "ch_1", PaymentIntentID: "pi_1", RefundedCents: 1000, RefundID: "re_1"}})
	e.event(t, Event{ID: "evt_r1b", Type: EventChargeRefunded, Refund: &RefundEvent{ChargeID: "ch_1", PaymentIntentID: "pi_1", RefundedCents: 1000, RefundID: "re_1"}})
	purchase, _ = e.repo.GetPurchaseByPaymentIntent("pi_1")
	assert.Equal(t, int64(1000), purchase.RefundedCents)
	assert.Equal(t, db.PurchaseStatusPaid, purchase.Status)
	credited, _ := e.repo.CreditedCents(inv.ID)
	assert.Equal(t, int64(1000), credited)

	_, err = e.svc.RefundPurchase(ctx, purchase.ID, 9999, false)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.RefundPurchase(ctx, purchase.ID, 0, true)
	require.NoError(t, err)
	assert.Equal(t, int64(4890), e.stripe.Refunds[1].AmountCents)
	assert.Contains(t, e.stripe.Canceled, *activated.StripeSubscriptionID)
	e.event(t, Event{ID: "evt_r2", Type: EventChargeRefunded, Refund: &RefundEvent{ChargeID: "ch_1", PaymentIntentID: "pi_1", RefundedCents: 5890, RefundID: "re_2"}})
	purchase, _ = e.repo.GetPurchaseByPaymentIntent("pi_1")
	assert.Equal(t, db.PurchaseStatusRefunded, purchase.Status)
	credit, err := e.repo.GetDocumentByStripeRef("re_2")
	require.NoError(t, err)
	assert.Equal(t, int64(4890), credit.AmountCents)
	assert.Equal(t, db.DocumentCreditNote, credit.Kind)
	assert.Equal(t, inv.ID, *credit.InvoiceDocumentID)
	ent, _ = e.svc.ForUser("u1")
	assert.Empty(t, ent.User)
}

func TestOneTimePurchase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, PlanInput{
		Key: "curso-verano", Name: "Curso de verano", Kind: db.PlanKindOneTime, AmountCents: 4900,
		Features: []string{"academia.summer"}, AccessDays: 60, Active: true,
	})
	require.NoError(t, err)
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)
	assert.Equal(t, []CheckoutLine{{PriceID: plan.StripePriceID, Quantity: 1}}, e.stripe.Sessions[0].Lines)
	assert.False(t, e.stripe.Sessions[0].SaveCard)

	e.event(t, Event{ID: "evt_1", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{CheckoutID: checkout.ID, Mode: "payment", Paid: true, PaymentIntentID: "pi_7", AmountCents: 4900}})
	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err)
	until := e.clock.AddDate(0, 0, 60)
	assert.Equal(t, Feature{Until: &until, Status: StatusActive, Source: SourcePurchase}, ent.User["academia.summer"])
	inv, err := e.repo.GetDocumentByStripeRef("pi_7")
	require.NoError(t, err)
	lines, _ := DocumentLines(inv)
	assert.Equal(t, "Curso de verano 25/10/2026–24/12/2026", lines[0].Description)

	// Unpaid completion (delayed payment methods) or an expired session.
	other, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)
	e.event(t, Event{ID: "evt_2", Type: EventCheckoutExpired, Checkout: &CheckoutEvent{CheckoutID: other.ID}})
	got, err := e.repo.GetCheckout(other.ID)
	require.NoError(t, err)
	assert.Equal(t, db.CheckoutStatusExpired, got.Status)
}

func TestNotifyDue(t *testing.T) {
	e := newEnv(t)
	e.withNotifications()
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	trialEnd := e.clock.Add(TrialEndingNotice).Add(-30 * time.Minute)
	stripeID := "sub_t"
	require.NoError(t, e.repo.CreateSubscription(&db.BillingSubscription{Subject: "user:u1", PlanKey: "academia-monthly", PayerUserID: "u1", Status: db.SubscriptionTrialing, TrialEnd: &trialEnd, StripeSubscriptionID: &stripeID}))
	grace := e.clock.Add(-10 * time.Minute)
	require.NoError(t, e.repo.CreateSubscription(&db.BillingSubscription{Subject: "user:u2", PlanKey: "academia-monthly", PayerUserID: "u2", Status: db.SubscriptionPastDue, GraceUntil: &grace}))

	require.NoError(t, e.svc.NotifyDue(e.clock))
	require.NoError(t, e.svc.NotifyDue(e.clock.Add(time.Hour)), "the next hour sends nothing again")
	assert.Equal(t, []string{NotifyTrialEnding}, e.notificationsOf(t, "u1"))
	assert.Equal(t, []string{NotifyExpired}, e.notificationsOf(t, "u2"))
}

func TestPortalURL(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	orgID := e.org(t, "Escuela")
	require.NoError(t, e.orgs.SetMember(orgID, "member1", "member"))
	require.NoError(t, e.orgs.SetMember(orgID, "owner1", "owner"))
	require.NoError(t, e.repo.SaveCustomer(&db.BillingCustomer{Subject: "org:" + orgID, StripeCustomerID: "cus_9"}))

	u, err := e.svc.PortalURL(ctx, "org:"+orgID, "owner1")
	require.NoError(t, err)
	assert.Equal(t, "https://billing.stripe.test/p/cus_9", u)
	_, err = e.svc.PortalURL(ctx, "org:"+orgID, "member1")
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.PortalURL(ctx, "user:u5", "u6")
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.PortalURL(ctx, "user:u5", "u5")
	assert.True(t, errors.Is(err, ErrNotFound), err)
}
