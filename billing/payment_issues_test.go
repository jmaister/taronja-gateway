package billing

import (
	"context"
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paidSubscription is an active subscription of plan personal-monthly for user u1.
func (e *testEnv) paidSubscription(t *testing.T) *db.BillingSubscription {
	t.Helper()
	ctx := context.Background()
	in := monthlyInput()
	in.Key, in.Group, in.Subject, in.Starts, in.TrialDays = "personal-monthly", "personal", db.PlanSubjectUser, db.PlanStartsOnPayment, 0
	plan, err := e.svc.CreatePlan(ctx, in)
	require.NoError(t, err)
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)
	end := e.clock.AddDate(0, 1, 0)
	e.stripe.Subscriptions["sub_paid"] = &StripeSubscription{ID: "sub_paid", Status: "active", CurrentPeriodEnd: end, PriceID: plan.StripePriceID}
	e.event(t, Event{ID: "evt_paid", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_paid", CustomerID: "cus_1", Status: "active", CurrentPeriodEnd: end, PriceID: plan.StripePriceID,
		Metadata: map[string]string{"tg_subject": "user:u1", "tg_plan": plan.Key, "tg_checkout": checkout.ID},
	}})
	e.event(t, Event{ID: "evt_checkout", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
		CheckoutID: checkout.ID, Mode: "subscription", Paid: true, SubscriptionID: "sub_paid", CustomerID: "cus_1",
	}})
	sub, err := e.repo.GetSubscriptionByStripeID("sub_paid")
	require.NoError(t, err)
	return sub
}

func TestPlanChange_FailedChargeKeepsAccessInGrace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.paidSubscription(t)
	bigger := monthlyInput()
	bigger.Key, bigger.Group, bigger.Subject, bigger.Starts, bigger.TrialDays, bigger.AmountCents = "personal-pro", "personal", db.PlanSubjectUser, db.PlanStartsOnPayment, 0, 3000
	pro, err := e.svc.CreatePlan(ctx, bigger)
	require.NoError(t, err)

	e.clock = e.clock.Add(1)
	e.event(t, Event{ID: "evt_switch", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{
		ID: "sub_paid", CustomerID: "cus_1", Status: "past_due", CurrentPeriodEnd: e.clock.AddDate(0, 1, 0), PriceID: pro.StripePriceID,
		Metadata: map[string]string{"tg_subject": "user:u1", "tg_plan": sub.PlanKey},
	}})
	e.event(t, Event{ID: "evt_fail", Type: EventInvoicePaymentFailed, Invoice: &InvoiceEvent{
		InvoiceID: "in_switch", SubscriptionID: "sub_paid", AmountCents: 3000, AttemptCount: 1,
	}})

	got, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionPastDue, got.Status)
	assert.Equal(t, "past_due", got.StripeStatus)
	assert.Equal(t, "personal-pro", got.PlanKey, "the new plan is followed")
	require.NotNil(t, got.GraceUntil)
	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Equal(t, StatusPastDue, ent.User["academia.access"].Status)
}

func TestPortalCardChange_PaidInvoiceRestoresActive(t *testing.T) {
	e := newEnv(t)
	sub := e.paidSubscription(t)
	e.event(t, Event{ID: "evt_fail", Type: EventInvoicePaymentFailed, Invoice: &InvoiceEvent{InvoiceID: "in_1", SubscriptionID: "sub_paid", AmountCents: 1000, AttemptCount: 1}})
	failed, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	require.Equal(t, db.SubscriptionPastDue, failed.Status)

	e.clock = e.clock.Add(1)
	e.event(t, Event{ID: "evt_invoice_paid", Type: EventInvoicePaid, Invoice: &InvoiceEvent{InvoiceID: "in_1", SubscriptionID: "sub_paid", AmountCents: 1000, PaidAt: e.clock}})
	got, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionActive, got.Status, "a paid invoice ends the past due period")
	assert.Nil(t, got.GraceUntil)

	e.clock = e.clock.Add(1)
	e.event(t, Event{ID: "evt_active", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{
		ID: "sub_paid", CustomerID: "cus_1", Status: "active", CurrentPeriodEnd: e.clock.AddDate(0, 1, 0),
		Metadata: map[string]string{"tg_subject": "user:u1", "tg_plan": sub.PlanKey},
	}})
	got, err = e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionActive, got.Status)
	assert.Equal(t, "active", got.StripeStatus)
	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Equal(t, StatusActive, ent.User["academia.access"].Status)
}

func TestActivateSubscription_DuplicateCallCreatesOneSubscription(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub, _ := e.awaitingActivation(t)

	first, err := e.svc.ActivateSubscription(ctx, sub.ID)
	require.NoError(t, err)
	second, err := e.svc.ActivateSubscription(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, first.StripeSubscriptionID, second.StripeSubscriptionID)
	assert.Len(t, e.stripe.Created, 1, "a second activation doesn't create another Stripe subscription")
}

func TestCheckoutCompletedUnpaid_IsAwaitingPaymentUntilAsyncPaid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := monthlyInput()
	in.Key, in.Subject, in.Starts, in.TrialDays = "personal-monthly", db.PlanSubjectUser, db.PlanStartsOnPayment, 0
	plan, err := e.svc.CreatePlan(ctx, in)
	require.NoError(t, err)
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)

	e.event(t, Event{ID: "evt_c", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{CheckoutID: checkout.ID, Mode: "subscription", Paid: false, SubscriptionID: "sub_async"}})
	waiting, err := e.repo.ListCheckoutsByStatus("user:u1", db.CheckoutStatusAwaitingPayment)
	require.NoError(t, err)
	require.Len(t, waiting, 1)

	e.event(t, Event{ID: "evt_a", Type: EventCheckoutAsyncPaid, Checkout: &CheckoutEvent{CheckoutID: checkout.ID, Mode: "subscription", Paid: true, SubscriptionID: "sub_async"}})
	done, err := e.repo.GetCheckout(checkout.ID)
	require.NoError(t, err)
	assert.Equal(t, db.CheckoutStatusCompleted, done.Status)
	waiting, err = e.repo.ListCheckoutsByStatus("user:u1", db.CheckoutStatusAwaitingPayment)
	require.NoError(t, err)
	assert.Empty(t, waiting)
}

func TestCustomerChangedCheck_WarnsWhenASubscriptionIsOnAnotherCustomer(t *testing.T) {
	e := newEnv(t)
	sub := e.paidSubscription(t)
	require.NoError(t, e.repo.SaveCustomer(&db.BillingCustomer{Subject: "user:u1", StripeCustomerID: "cus_new"}))
	check := e.svc.customerChangedCheck()
	assert.Equal(t, CheckWarn, check.Status)
	assert.Contains(t, check.Detail, sub.ID)

	require.NoError(t, e.repo.SaveCustomer(&db.BillingCustomer{Subject: "user:u1", StripeCustomerID: "cus_1"}))
	assert.Equal(t, CheckOK, e.svc.customerChangedCheck().Status)
}
