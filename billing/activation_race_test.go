package billing

import (
	"context"
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *testEnv) awaitingActivation(t *testing.T) (*db.BillingSubscription, *db.BillingPurchase) {
	t.Helper()
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, PlanInput{
		Key: "megabox-familia", Name: "Megabox Familia", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 990, Features: []string{"megabox.protection"}, Subject: db.PlanSubjectUser, Starts: db.PlanStartsOnActivation,
		SetupFeeCents: 4900, SetupFeeName: "Caja Megabox", GraceDays: 7, Active: true,
	})
	require.NoError(t, err)
	checkout, _, err := e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: plan.Key, Payer: payer})
	require.NoError(t, err)
	e.event(t, Event{ID: "evt_1", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
		CheckoutID: checkout.ID, Mode: "payment", Paid: true, CustomerID: "cus_1", PaymentIntentID: "pi_1", AmountCents: 5890,
		Currency: "eur", Name: "Ana", Email: "ana@example.com", PaymentMethodID: "pm_1",
	}})
	purchase, err := e.repo.GetPurchaseByPaymentIntent("pi_1")
	require.NoError(t, err)
	require.NotNil(t, purchase.SubscriptionID)
	sub, err := e.repo.GetSubscription(*purchase.SubscriptionID)
	require.NoError(t, err)
	return sub, purchase
}

func TestActivateSubscription_CanceledWhileStripeCreatesIt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub, _ := e.awaitingActivation(t)
	e.stripe.OnCreateSubscription = func() {
		s, err := e.repo.GetSubscription(sub.ID)
		require.NoError(t, err)
		now := e.clock
		s.Status, s.CanceledAt = db.SubscriptionCanceled, &now
		require.NoError(t, e.repo.UpdateSubscription(s))
	}

	_, err := e.svc.ActivateSubscription(ctx, sub.ID)
	assert.ErrorIs(t, err, ErrConflict)

	got, err := e.repo.GetSubscription(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, db.SubscriptionCanceled, got.Status)
	assert.Nil(t, got.StripeSubscriptionID, "no live state saved on a canceled row")
	require.Len(t, e.stripe.Created, 1)
	assert.Len(t, e.stripe.Canceled, 1, "the subscription Stripe created is canceled, or it would charge next month")
	for _, s := range e.stripe.Subscriptions {
		assert.Equal(t, "canceled", s.Status)
	}
}

func TestOnRefund_LateEventDoesNotLowerRefundedCents(t *testing.T) {
	e := newEnv(t)
	_, purchase := e.awaitingActivation(t)
	refund := func(id string, cents int64) {
		e.event(t, Event{ID: id, Type: EventChargeRefunded, Refund: &RefundEvent{ChargeID: "ch_1", PaymentIntentID: "pi_1", RefundedCents: cents, RefundID: "re_" + id}})
	}
	refunded := func() *db.BillingPurchase {
		p, err := e.repo.GetPurchase(purchase.ID)
		require.NoError(t, err)
		return p
	}

	refund("a", 1000)
	refund("b", 2500)
	refund("a_late", 1000) // Stripe retries an old event after a newer one
	assert.Equal(t, int64(2500), refunded().RefundedCents)
	assert.Equal(t, db.PurchaseStatusPaid, refunded().Status)

	refund("c", 5890)
	refund("b_late", 2500)
	assert.Equal(t, int64(5890), refunded().RefundedCents)
	assert.Equal(t, db.PurchaseStatusRefunded, refunded().Status)
}
