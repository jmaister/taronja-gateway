package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBillingSubscriptionsAdminEndpointNeedsAdmin tests the 401/403 requirements for subscription endpoints.
func TestBillingSubscriptionsAdminEndpointNeedsAdmin(t *testing.T) {
	f := newBillingFixture(t)
	calls := map[string]func(ctx context.Context) (any, error){
		"listSubscriptions": func(ctx context.Context) (any, error) {
			return f.s.ListBillingSubscriptions(ctx, api.ListBillingSubscriptionsRequestObject{})
		},
		"activateSubscription": func(ctx context.Context) (any, error) {
			return f.s.ActivateBillingSubscription(ctx, api.ActivateBillingSubscriptionRequestObject{Id: "x"})
		},
		"cancelSubscription": func(ctx context.Context) (any, error) {
			return f.s.CancelBillingSubscription(ctx, api.CancelBillingSubscriptionRequestObject{Id: "x", Body: &api.BillingCancelRequest{AtPeriodEnd: false}})
		},
	}
	for name, call := range calls {
		resp, err := call(context.Background())
		require.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", resp), "401JSONResponse", name)
		resp, err = call(sessionContext(f.user.ID, false))
		require.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", resp), "403JSONResponse", name)
	}
}

// TestBillingPurchasesAdminEndpointNeedsAdmin tests the 401/403 requirements for purchase endpoints.
func TestBillingPurchasesAdminEndpointNeedsAdmin(t *testing.T) {
	f := newBillingFixture(t)
	calls := map[string]func(ctx context.Context) (any, error){
		"listPurchases": func(ctx context.Context) (any, error) {
			return f.s.ListBillingPurchases(ctx, api.ListBillingPurchasesRequestObject{})
		},
		"refundPurchase": func(ctx context.Context) (any, error) {
			return f.s.RefundBillingPurchase(ctx, api.RefundBillingPurchaseRequestObject{Id: "x", Body: &api.BillingRefundRequest{}})
		},
	}
	for name, call := range calls {
		resp, err := call(context.Background())
		require.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", resp), "401JSONResponse", name)
		resp, err = call(sessionContext(f.user.ID, false))
		require.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", resp), "403JSONResponse", name)
	}
}

// TestBillingSetupAdminEndpointNeedsAdmin tests the 401/403 requirements for setup endpoints.
func TestBillingSetupAdminEndpointNeedsAdmin(t *testing.T) {
	f := newBillingFixture(t)
	calls := map[string]func(ctx context.Context) (any, error){
		"getSetup": func(ctx context.Context) (any, error) {
			return f.s.GetBillingSetup(ctx, api.GetBillingSetupRequestObject{})
		},
		"runSetupAction": func(ctx context.Context) (any, error) {
			return f.s.RunBillingSetupAction(ctx, api.RunBillingSetupActionRequestObject{Body: &api.BillingSetupActionRequest{Action: "sync-portal"}})
		},
	}
	for name, call := range calls {
		resp, err := call(context.Background())
		require.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", resp), "401JSONResponse", name)
		resp, err = call(sessionContext(f.user.ID, false))
		require.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", resp), "403JSONResponse", name)
	}
}

// TestBillingFlows tests a complete billing flow: checkout, activation, refund, cancellation.
func TestBillingFlows(t *testing.T) {
	f := newBillingFixture(t)
	admin := sessionContext("admin", true)
	ctx := context.Background()

	// Create a plan like megabox's with setup fee and on_activation start
	plan, err := f.svc.CreatePlan(ctx, billing.PlanInput{
		Key:           "megabox-familia",
		Name:          "Megabox Familia",
		Kind:          db.PlanKindSubscription,
		Interval:      "month",
		AmountCents:   4990,
		Features:      []string{"megabox.protection"},
		Subject:       "user",
		Starts:        "on_activation",
		SetupFeeCents: 4900,
		SetupFeeName:  "Caja",
		GraceDays:     7,
		Active:        true,
	})
	require.NoError(t, err)

	// Start a checkout
	checkout, _, err := f.svc.StartCheckout(ctx, billing.CheckoutRequest{
		PlanKey: plan.Key,
		Payer:   billing.Payer{UserID: f.user.ID, Email: f.user.Email},
	})
	require.NoError(t, err)

	// Complete the checkout with a webhook event
	evt := billing.Event{
		ID:      "evt_1",
		Type:    billing.EventCheckoutCompleted,
		Created: time.Now(),
		Checkout: &billing.CheckoutEvent{
			CheckoutID:      checkout.ID,
			Mode:            "payment",
			Paid:            true,
			PaymentIntentID: "pi_1",
			AmountCents:     5890,
		},
	}
	err = f.svc.HandleEvent(ctx, evt)
	require.NoError(t, err)

	// Verify the purchase was created
	purchases, err := f.repo.ListPurchases(repositories.PurchaseFilter{})
	require.NoError(t, err)
	require.Len(t, purchases, 1, "purchase created from completed checkout")
	purchase := &purchases[0]
	assert.Equal(t, "pi_1", purchase.StripePaymentIntentID)
	assert.Equal(t, db.PurchaseStatusPaid, purchase.Status)

	// Verify the subscription was created awaiting activation
	subject := billing.UserSubject(f.user.ID)
	subs, err := f.repo.ListSubscriptions(repositories.SubscriptionFilter{Subject: string(subject), PlanKey: plan.Key})
	require.NoError(t, err)
	require.Len(t, subs, 1, "subscription created awaiting activation")
	sub := &subs[0]
	assert.Equal(t, db.SubscriptionAwaitingActivation, sub.Status)

	// List purchases via API
	resp, err := f.s.ListBillingPurchases(admin, api.ListBillingPurchasesRequestObject{})
	require.NoError(t, err)
	purchaseList, ok := resp.(api.ListBillingPurchases200JSONResponse)
	require.True(t, ok, "%T", resp)
	require.Len(t, purchaseList, 1)
	assert.Equal(t, f.user.ID, purchaseList[0].PayerUserId)

	// List subscriptions via API
	resp2, err := f.s.ListBillingSubscriptions(admin, api.ListBillingSubscriptionsRequestObject{
		Params: api.ListBillingSubscriptionsParams{Status: ptr("awaiting_activation")},
	})
	require.NoError(t, err)
	subList, ok := resp2.(api.ListBillingSubscriptions200JSONResponse)
	require.True(t, ok, "%T", resp2)
	require.Len(t, subList, 1)
	assert.Equal(t, db.SubscriptionAwaitingActivation, string(subList[0].Status))

	// Activate the subscription via API twice (second should return the same)
	resp3, err := f.s.ActivateBillingSubscription(admin, api.ActivateBillingSubscriptionRequestObject{Id: sub.ID})
	require.NoError(t, err)
	activated, ok := resp3.(api.ActivateBillingSubscription200JSONResponse)
	require.True(t, ok, "%T", resp3)
	assert.Equal(t, db.SubscriptionTrialing, string(activated.Status))
	firstStripeID := activated.StripeSubscriptionId

	// Activate again, should return same stripe subscription id
	resp3b, err := f.s.ActivateBillingSubscription(admin, api.ActivateBillingSubscriptionRequestObject{Id: sub.ID})
	require.NoError(t, err)
	activated2, ok := resp3b.(api.ActivateBillingSubscription200JSONResponse)
	require.True(t, ok, "%T", resp3b)
	assert.Equal(t, firstStripeID, activated2.StripeSubscriptionId, "activating twice returns same subscription id")

	// Refund via API
	resp4, err := f.s.RefundBillingPurchase(admin, api.RefundBillingPurchaseRequestObject{
		Id:   purchase.ID,
		Body: &api.BillingRefundRequest{AmountCents: ptr(int64(1000))},
	})
	require.NoError(t, err)
	resp4OK, ok := resp4.(api.RefundBillingPurchase200JSONResponse)
	require.True(t, ok, "%T", resp4)
	_ = resp4OK

	// Try to refund more than available → 400
	resp5, err := f.s.RefundBillingPurchase(admin, api.RefundBillingPurchaseRequestObject{
		Id:   purchase.ID,
		Body: &api.BillingRefundRequest{AmountCents: ptr(int64(10000))},
	})
	require.NoError(t, err)
	assert.IsType(t, api.RefundBillingPurchase400JSONResponse{}, resp5, "refund exceeding available amount fails")

	// Cancel the subscription via API
	resp6, err := f.s.CancelBillingSubscription(admin, api.CancelBillingSubscriptionRequestObject{
		Id:   sub.ID,
		Body: &api.BillingCancelRequest{AtPeriodEnd: false},
	})
	require.NoError(t, err)
	canceled, ok := resp6.(api.CancelBillingSubscription200JSONResponse)
	require.True(t, ok, "%T", resp6)
	assert.Equal(t, db.SubscriptionCanceled, string(canceled.Status))
}

// TestMyBilling tests GetMyBilling showing user's and org subscriptions/documents.
func TestMyBilling(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()
	userCtx := sessionContext(f.user.ID, false)
	otherCtx := sessionContext(f.other.ID, false)

	// Create a plan
	plan, err := f.svc.CreatePlan(ctx, billing.PlanInput{
		Key:         "test-plan",
		Name:        "Test Plan",
		Kind:        db.PlanKindSubscription,
		Interval:    "month",
		AmountCents: 2900,
		Features:    []string{"feature.test"},
		Subject:     "either",
		Active:      true,
	})
	require.NoError(t, err)

	// Create a subscription for the user
	userSub := &db.BillingSubscription{
		Subject:     string(billing.UserSubject(f.user.ID)),
		PlanKey:     plan.Key,
		PayerUserID: f.user.ID,
		Status:      db.SubscriptionActive,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, f.repo.CreateSubscription(userSub))

	// Create a subscription for the org the user owns
	orgSub := &db.BillingSubscription{
		Subject:     string(billing.OrgSubject(f.orgID)),
		PlanKey:     plan.Key,
		PayerUserID: f.user.ID,
		Status:      db.SubscriptionActive,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, f.repo.CreateSubscription(orgSub))

	// Save customers with Stripe IDs for portal access
	require.NoError(t, f.repo.SaveCustomer(&db.BillingCustomer{
		Subject:          string(billing.UserSubject(f.user.ID)),
		Name:             f.user.Username,
		Country:          "ES",
		StripeCustomerID: "cus_user1",
	}))
	require.NoError(t, f.repo.SaveCustomer(&db.BillingCustomer{
		Subject:          string(billing.OrgSubject(f.orgID)),
		Name:             "Escuela",
		Country:          "ES",
		StripeCustomerID: "cus_org1",
	}))

	// Create an invoice document for the user
	lines := []billing.DocumentLine{{Description: "Test plan", PlanKey: plan.Key, AmountCents: 2900}}
	doc := &db.BillingDocument{
		Subject:     string(billing.UserSubject(f.user.ID)),
		Kind:        db.DocumentInvoice,
		StripeRef:   "in_test",
		AmountCents: 2900,
		PaidAt:      time.Now(),
	}
	require.NoError(t, billing.EnqueueDocument(f.repo, doc, lines, time.Now()))

	// GetMyBilling for the user should show their subscription and org subscription
	resp, err := f.s.GetMyBilling(userCtx, api.GetMyBillingRequestObject{})
	require.NoError(t, err)
	myBilling, ok := resp.(api.GetMyBilling200JSONResponse)
	require.True(t, ok, "%T", resp)
	assert.Len(t, myBilling.Subscriptions, 2, "user sees their own subscription and org subscription")
	assert.Len(t, myBilling.Documents, 1, "user sees their own document")

	// GetMyBilling for the other user should show nothing
	resp2, err := f.s.GetMyBilling(otherCtx, api.GetMyBillingRequestObject{})
	require.NoError(t, err)
	myBilling2, ok := resp2.(api.GetMyBilling200JSONResponse)
	require.True(t, ok, "%T", resp2)
	assert.Empty(t, myBilling2.Subscriptions)
	assert.Empty(t, myBilling2.Documents)

	// CreateMyBillingPortal works for the user's own subscription
	resp3, err := f.s.CreateMyBillingPortal(userCtx, api.CreateMyBillingPortalRequestObject{
		Body: &api.BillingPortalRequest{Subject: string(billing.UserSubject(f.user.ID))},
	})
	require.NoError(t, err)
	_, ok = resp3.(api.CreateMyBillingPortal200JSONResponse)
	require.True(t, ok, "%T", resp3)

	// CreateMyBillingPortal fails for the org when the user is not an owner
	require.NoError(t, f.orgs.SetMember(f.orgID, f.other.ID, "member"))
	resp4, err := f.s.CreateMyBillingPortal(otherCtx, api.CreateMyBillingPortalRequestObject{
		Body: &api.BillingPortalRequest{Subject: string(billing.OrgSubject(f.orgID))},
	})
	require.NoError(t, err)
	assert.IsType(t, api.CreateMyBillingPortal403JSONResponse{}, resp4)

	// CreateMyBillingPortal works when they are an owner
	require.NoError(t, f.orgs.SetMember(f.orgID, f.other.ID, "owner"))
	resp5, err := f.s.CreateMyBillingPortal(otherCtx, api.CreateMyBillingPortalRequestObject{
		Body: &api.BillingPortalRequest{Subject: string(billing.OrgSubject(f.orgID))},
	})
	require.NoError(t, err)
	_, ok = resp5.(api.CreateMyBillingPortal200JSONResponse)
	require.True(t, ok, "%T", resp5)
}

// TestBillingSetup tests GetBillingSetup and RunBillingSetupAction.
func TestBillingSetup(t *testing.T) {
	f := newBillingFixture(t)
	admin := sessionContext("admin", true)

	// GetBillingSetup should return the checks
	resp, err := f.s.GetBillingSetup(admin, api.GetBillingSetupRequestObject{})
	require.NoError(t, err)
	setup, ok := resp.(api.GetBillingSetup200JSONResponse)
	require.True(t, ok, "%T", resp)
	assert.True(t, len(setup.Checks) >= 6, "should have at least stripe-key, stripe-webhook, stripe-emails, stripe-portal, plans, and odoo checks")
	require.NotNil(t, setup.PaymentMethods)
	assert.Empty(t, setup.PaymentMethods.Immediate)
	assert.Empty(t, setup.PaymentMethods.Deferred)
	f.stripe.PaymentMethods = []string{"card", "sepa_debit"}
	resp, err = f.s.GetBillingSetup(admin, api.GetBillingSetupRequestObject{})
	require.NoError(t, err)
	setup, ok = resp.(api.GetBillingSetup200JSONResponse)
	require.True(t, ok, "%T", resp)
	require.NotNil(t, setup.PaymentMethods)
	assert.Equal(t, []string{"card"}, setup.PaymentMethods.Immediate)
	assert.Equal(t, []string{"sepa_debit"}, setup.PaymentMethods.Deferred)

	// Check that "stripe-key" is in the checks
	var stripeKeyCheck *api.SetupCheck
	for i := range setup.Checks {
		if setup.Checks[i].Id == "stripe-key" {
			stripeKeyCheck = &setup.Checks[i]
			break
		}
	}
	require.NotNil(t, stripeKeyCheck, "should have stripe-key check")

	// RunBillingSetupAction with sync-portal should work
	resp2, err := f.s.RunBillingSetupAction(admin, api.RunBillingSetupActionRequestObject{
		Body: &api.BillingSetupActionRequest{Action: "sync-portal"},
	})
	require.NoError(t, err)
	setup2, ok := resp2.(api.RunBillingSetupAction200JSONResponse)
	require.True(t, ok, "%T", resp2)
	assert.True(t, len(setup2.Checks) >= 6, "should return updated checks after action")

	// RunBillingSetupAction with invalid action should fail
	resp3, err := f.s.RunBillingSetupAction(admin, api.RunBillingSetupActionRequestObject{
		Body: &api.BillingSetupActionRequest{Action: "nope"},
	})
	require.NoError(t, err)
	assert.IsType(t, api.RunBillingSetupAction400JSONResponse{}, resp3)
}

func customerBody(subject string) *api.BillingCustomerRequest {
	return &api.BillingCustomerRequest{
		Subject: subject, Kind: api.BillingCustomerRequestKindCompany, Name: "Escuela SL", TaxId: ptr("B12345674"),
		AddressLine1: "C/ Mayor 1", PostalCode: "46001", City: "Valencia", Country: "ES", Phone: ptr("+34600000000"),
	}
}

func TestMyBillingCustomer(t *testing.T) {
	f := newBillingFixture(t)
	subject := string(billing.OrgSubject(f.orgID))
	ctx := sessionContext(f.user.ID, false)
	get := func(ctx context.Context, subject string) (api.GetMyBillingCustomerResponseObject, error) {
		return f.s.GetMyBillingCustomer(ctx, api.GetMyBillingCustomerRequestObject{Params: api.GetMyBillingCustomerParams{Subject: subject}})
	}

	t.Run("anonymous", func(t *testing.T) {
		r, err := get(context.Background(), subject)
		require.NoError(t, err)
		assert.IsType(t, api.GetMyBillingCustomer401JSONResponse{}, r)
		r2, err := f.s.SaveMyBillingCustomer(context.Background(), api.SaveMyBillingCustomerRequestObject{Body: customerBody(subject)})
		require.NoError(t, err)
		assert.IsType(t, api.SaveMyBillingCustomer401JSONResponse{}, r2)
	})
	t.Run("not a member", func(t *testing.T) {
		r, err := get(sessionContext(f.other.ID, false), subject)
		require.NoError(t, err)
		assert.IsType(t, api.GetMyBillingCustomer403JSONResponse{}, r)
		r2, err := f.s.SaveMyBillingCustomer(sessionContext(f.other.ID, false), api.SaveMyBillingCustomerRequestObject{Body: customerBody(subject)})
		require.NoError(t, err)
		assert.IsType(t, api.SaveMyBillingCustomer403JSONResponse{}, r2)
	})
	t.Run("empty before saving", func(t *testing.T) {
		r, err := get(ctx, subject)
		require.NoError(t, err)
		d, ok := r.(api.GetMyBillingCustomer200JSONResponse)
		require.True(t, ok, "%T", r)
		assert.Nil(t, d.Kind)
		assert.Empty(t, d.Name)
		assert.Equal(t, subject, d.Subject)
	})
	t.Run("invalid", func(t *testing.T) {
		b := customerBody(subject)
		b.TaxId = ptr("B12345675")
		r, err := f.s.SaveMyBillingCustomer(ctx, api.SaveMyBillingCustomerRequestObject{Body: b})
		require.NoError(t, err)
		assert.IsType(t, api.SaveMyBillingCustomer400JSONResponse{}, r)
		r, err = f.s.SaveMyBillingCustomer(ctx, api.SaveMyBillingCustomerRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, api.SaveMyBillingCustomer400JSONResponse{}, r)
	})
	t.Run("save, read, and the tax id cannot change", func(t *testing.T) {
		r, err := f.s.SaveMyBillingCustomer(ctx, api.SaveMyBillingCustomerRequestObject{Body: customerBody(subject)})
		require.NoError(t, err)
		saved, ok := r.(api.SaveMyBillingCustomer200JSONResponse)
		require.True(t, ok, "%T", r)
		require.NotNil(t, saved.Kind)
		assert.Equal(t, api.BillingCustomerDetailsKind("company"), *saved.Kind)
		assert.Equal(t, "B12345674", saved.TaxId)
		assert.Len(t, f.stripe.CustomerUpdates, 1)

		r2, err := get(ctx, subject)
		require.NoError(t, err)
		d, ok := r2.(api.GetMyBillingCustomer200JSONResponse)
		require.True(t, ok, "%T", r2)
		assert.Equal(t, "Escuela SL", d.Name)
		assert.Equal(t, "+34600000000", d.Phone)
		assert.Equal(t, "Valencia", d.City)

		b := customerBody(subject)
		b.TaxId = ptr("A58818501")
		r3, err := f.s.SaveMyBillingCustomer(ctx, api.SaveMyBillingCustomerRequestObject{Body: b})
		require.NoError(t, err)
		assert.IsType(t, api.SaveMyBillingCustomer409JSONResponse{}, r3)
	})
}
