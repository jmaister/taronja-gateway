//go:build e2e

package stripeapi

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLive_StripeTestMode runs the client against Stripe's real test mode.
// It is skipped unless STRIPE_TEST_SECRET_KEY (sk_test_…) is set.
func TestLive_StripeTestMode(t *testing.T) {
	key := os.Getenv("STRIPE_TEST_SECRET_KEY")
	if key == "" {
		t.Skip("STRIPE_TEST_SECRET_KEY is not set")
	}
	require.Contains(t, key, "_test_", "refusing to run against a live key")
	client := New(key, "whsec_unused")
	ctx := context.Background()
	run := fmt.Sprintf("e2e-%d", time.Now().UnixNano())

	productID, err := client.EnsureProduct(ctx, "", "Taronja e2e "+run, run)
	require.NoError(t, err)
	productID, err = client.EnsureProduct(ctx, productID, "Taronja e2e renamed "+run, run)
	require.NoError(t, err)

	monthly, err := client.CreatePrice(ctx, billing.StripePriceParams{
		ProductID: productID, AmountCents: 2900, Currency: "eur", Interval: "month", LookupKey: run + "-monthly",
	})
	require.NoError(t, err)
	oneTime, err := client.CreatePrice(ctx, billing.StripePriceParams{
		ProductID: productID, AmountCents: 1500, Currency: "eur", LookupKey: run + "-once",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.ArchivePrice(ctx, monthly)
		_ = client.ArchivePrice(ctx, oneTime)
	})

	customerID, err := client.EnsureCustomer(ctx, "", "user:"+run, "E2E Customer", run+"@example.com")
	require.NoError(t, err)
	again, err := client.EnsureCustomer(ctx, customerID, "user:"+run, "E2E Renamed", run+"@example.com")
	require.NoError(t, err)
	assert.Equal(t, customerID, again)

	urls := billing.CheckoutParams{SuccessURL: "https://example.com/ok", CancelURL: "https://example.com/cancel", CustomerID: customerID}

	pay := urls
	pay.CheckoutID, pay.Mode = run+"-pay", "payment"
	pay.Lines = []billing.CheckoutLine{{PriceID: oneTime, Quantity: 1}}
	pay.SaveCard, pay.CollectShipping = true, true
	session, err := client.CreateCheckoutSession(ctx, pay)
	require.NoError(t, err)
	assert.Contains(t, session.URL, "checkout.stripe.com")

	sub := urls
	sub.CheckoutID, sub.Mode = run+"-sub", "subscription"
	sub.Lines = []billing.CheckoutLine{{PriceID: monthly, Quantity: 1}}
	sub.TrialDays = 14
	sub.Metadata = map[string]string{"tg_plan": run}
	session, err = client.CreateCheckoutSession(ctx, sub)
	require.NoError(t, err)
	assert.Contains(t, session.URL, "checkout.stripe.com")

	_, err = client.ConfigurePortal(ctx, billing.PortalParams{
		ReturnURL: "https://example.com/account", SwitchProducts: map[string][]string{productID: {monthly}},
	})
	require.NoError(t, err)
	portal, err := client.PortalURL(ctx, customerID, "https://example.com/account")
	require.NoError(t, err)
	assert.Contains(t, portal, "billing.stripe.com")
}
