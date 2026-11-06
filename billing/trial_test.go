package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *testEnv) trialing(t *testing.T) *db.BillingSubscription {
	t.Helper()
	ctx := context.Background()
	in := monthlyInput()
	in.Key, in.Group, in.Subject, in.Starts = "personal-monthly", "personal", db.PlanSubjectUser, db.PlanStartsOnPayment
	plan, err := e.svc.CreatePlan(ctx, in)
	require.NoError(t, err)
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)
	end := e.clock.AddDate(0, 0, 14)
	e.stripe.Subscriptions["sub_trial"] = &StripeSubscription{ID: "sub_trial", Status: "trialing", CurrentPeriodEnd: end, TrialEnd: &end}
	e.event(t, Event{ID: "evt_trial", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_trial", Status: "trialing", CurrentPeriodEnd: end, TrialEnd: &end,
		Metadata: map[string]string{"tg_subject": "user:u1", "tg_plan": plan.Key, "tg_checkout": checkout.ID},
	}})
	e.event(t, Event{ID: "evt_checkout", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
		CheckoutID: checkout.ID, Mode: "subscription", Paid: true, SubscriptionID: "sub_trial", CustomerID: "cus_1",
	}})
	sub, err := e.repo.GetSubscriptionByStripeID("sub_trial")
	require.NoError(t, err)
	return sub
}

func (e *testEnv) subscriptionStatus(t *testing.T, status string) {
	t.Helper()
	e.event(t, Event{ID: "evt_" + status, Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{
		ID: "sub_trial", Status: status, Metadata: map[string]string{"tg_subject": "user:u1", "tg_plan": "personal-monthly"},
	}})
}

func TestStartNow_EndsTheTrialAndWaitsForTheWebhook(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.trialing(t)

	require.NoError(t, e.svc.StartNow(ctx, sub.ID, "u1"))
	require.NoError(t, e.svc.StartNow(ctx, sub.ID, "u1"), "a second click before the webhook")
	require.Len(t, e.stripe.Trials, 2)
	assert.Equal(t, "sub_trial", e.stripe.Trials[0].SubscriptionID)
	assert.Equal(t, "tg-start-"+sub.ID, e.stripe.Trials[0].IdempotencyKey)
	assert.Equal(t, e.stripe.Trials[0].IdempotencyKey, e.stripe.Trials[1].IdempotencyKey, "Stripe answers the same charge")
	assert.False(t, e.stripe.Trials[0].TrialEnd.IsZero())

	got, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionTrialing, got.Status, "the status changes with Stripe's webhook, not here")

	e.subscriptionStatus(t, "active")
	got, err = e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TrialUntil)
	assert.True(t, got.TrialUntil.Equal(*sub.TrialUntil), "Empezar ya leaves our trial end alone")
	err = e.svc.StartNow(ctx, sub.ID, "u1")
	assert.ErrorIs(t, err, ErrConflict, "once active, there is no trial left to end")
	assert.Len(t, e.stripe.Trials, 2, "no call to Stripe after the webhook")
}

func TestStartNow_OnlyTheSubjectMayStart(t *testing.T) {
	e := newEnv(t)
	sub := e.trialing(t)

	err := e.svc.StartNow(context.Background(), sub.ID, "u2")
	assert.ErrorIs(t, err, ErrForbidden)
	assert.Empty(t, e.stripe.Trials)
}

func TestStartNow_NotInTrialIsConflict(t *testing.T) {
	e := newEnv(t)
	sub := e.trialing(t)
	e.subscriptionStatus(t, "past_due")

	err := e.svc.StartNow(context.Background(), sub.ID, "u1")
	assert.ErrorIs(t, err, ErrConflict)
	assert.Empty(t, e.stripe.Trials)
}

func TestStartNow_PlanStartingOnActivationIsConflict(t *testing.T) {
	e := newEnv(t)
	sub, _ := e.awaitingActivation(t)

	err := e.svc.StartNow(context.Background(), sub.ID, "u1")
	assert.ErrorIs(t, err, ErrConflict)
	assert.Empty(t, e.stripe.Trials)
}

func TestStartNow_StripeFailureKeepsTheTrial(t *testing.T) {
	e := newEnv(t)
	sub := e.trialing(t)
	e.stripe.Err = errors.New("stripe is down")

	err := e.svc.StartNow(context.Background(), sub.ID, "u1")
	assert.Error(t, err)
	got, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionTrialing, got.Status)
}

func TestStartNow_FailedChargeKeepsAccessUntilOurTrialEnd(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.trialing(t)
	original := *sub.TrialUntil
	require.NoError(t, e.svc.StartNow(ctx, sub.ID, "u1"))

	stripeEnd := e.clock
	e.event(t, Event{ID: "evt_incomplete", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{
		ID: "sub_trial", Status: "incomplete", CurrentPeriodEnd: stripeEnd, TrialEnd: &stripeEnd,
		Metadata: map[string]string{"tg_subject": "user:u1", "tg_plan": "personal-monthly"},
	}})
	got, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionIncomplete, got.Status)
	assert.True(t, got.TrialUntil.Equal(original), "our trial end never moves")

	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err)
	feature := ent.User["academia.access"]
	assert.Equal(t, StatusTrial, feature.Status)
	require.NotNil(t, feature.Until)
	assert.True(t, feature.Until.Equal(original), "access until our trial end")

	past := e.clock.AddDate(0, 0, -1)
	got.TrialUntil = &past
	require.NoError(t, e.repo.UpdateSubscription(got))
	e.svc.Invalidate("user:u1")
	ent, err = e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Empty(t, ent.User, "after our trial end, an incomplete payment gives nothing")
}

func TestActivateSubscription_FixesOurTrialEnd(t *testing.T) {
	e := newEnv(t)
	sub, _ := e.awaitingActivation(t)
	assert.Nil(t, sub.TrialUntil)

	active, err := e.svc.ActivateSubscription(context.Background(), sub.ID)
	require.NoError(t, err)
	require.NotNil(t, active.TrialUntil)
	assert.True(t, active.TrialUntil.Equal(*active.TrialEnd))
}

func TestStripeStatus_IncompleteKeepsItsTrial(t *testing.T) {
	assert.Equal(t, db.SubscriptionIncomplete, stripeStatus("incomplete"))
	assert.Equal(t, db.SubscriptionCanceled, stripeStatus("incomplete_expired"))
	assert.Equal(t, db.SubscriptionCanceled, stripeStatus("unpaid"))
}
