//go:build e2e

package stripeapi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_AllMethods tests all methods against stripe-mock
func TestE2E_AllMethods(t *testing.T) {
	mockURL := os.Getenv("E2E_STRIPE_MOCK_URL")
	if mockURL == "" {
		mockURL = "http://localhost:12111"
	}

	client := New("sk_test_123", "whsec_test", WithAPIURL(mockURL))
	ctx := context.Background()

	// Test EnsureProduct
	productID, err := client.EnsureProduct(ctx, "", "Test Product", "test-plan")
	require.NoError(t, err)
	assert.NotEmpty(t, productID)

	// Test CreatePrice
	priceID, err := client.CreatePrice(ctx, billing.StripePriceParams{
		ProductID:   productID,
		AmountCents: 9999,
		Currency:    "eur",
		LookupKey:   "test-lookup",
		Interval:    "month",
		Nickname:    "Test Price",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, priceID)

	// Test ArchivePrice
	err = client.ArchivePrice(ctx, priceID)
	require.NoError(t, err)

	// Test EnsureCustomer
	customerID, err := client.EnsureCustomer(ctx, "", "user:123", "John Doe", "john@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, customerID)

	// Test CreateCheckoutSession
	session, err := client.CreateCheckoutSession(ctx, billing.CheckoutParams{
		CheckoutID: "checkout_e2e",
		CustomerID: customerID,
		Mode:       "payment",
		Lines: []billing.CheckoutLine{
			{PriceID: priceID, Quantity: 1},
		},
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, session.ID)
	assert.NotEmpty(t, session.URL)

	// Every option the pay page uses: trial, extra setup line, shipping.
	_, err = client.CreateCheckoutSession(ctx, billing.CheckoutParams{
		CheckoutID: "checkout_e2e_sub", CustomerID: customerID, Mode: "subscription",
		Lines:     []billing.CheckoutLine{{PriceID: priceID, Quantity: 1}, {PriceID: priceID, Quantity: 1}},
		TrialDays: 14, CollectShipping: true, Metadata: map[string]string{"tg_plan": "test-plan"},
		SuccessURL: "https://example.com/success", CancelURL: "https://example.com/cancel",
	})
	require.NoError(t, err)
	_, err = client.CreateCheckoutSession(ctx, billing.CheckoutParams{
		CheckoutID: "checkout_e2e_card", CustomerID: customerID, Mode: "payment", SaveCard: true,
		Lines:      []billing.CheckoutLine{{PriceID: priceID, Quantity: 1}},
		SuccessURL: "https://example.com/success", CancelURL: "https://example.com/cancel",
	})
	require.NoError(t, err)

	// Test ConfigurePortal: create with switchable prices, then update.
	portalConfigID, err := client.ConfigurePortal(ctx, billing.PortalParams{
		ReturnURL:      "https://example.com/account",
		SwitchProducts: map[string][]string{productID: {priceID}},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, portalConfigID)
	_, err = client.ConfigurePortal(ctx, billing.PortalParams{ReturnURL: "https://example.com/account"})
	require.NoError(t, err)

	// Test PortalURL
	portalURL, err := client.PortalURL(ctx, customerID, "https://example.com/account")
	require.NoError(t, err)
	assert.NotEmpty(t, portalURL)

	require.NoError(t, client.Refund(ctx, "pi_test", 500, "refund-e2e-key"))
	require.NoError(t, client.Refund(ctx, "pi_test", 0, "refund-e2e-key-2"))

	// charge.refunded looks up the invoice through the invoice payments API.
	payload := event("evt_e2e", "charge.refunded", `{"id":"ch_1","object":"charge","payment_intent":"pi_test","amount_refunded":500,"currency":"eur"}`)
	ev, err := client.ParseWebhook(payload, sign(payload))
	require.NoError(t, err)
	require.NotNil(t, ev.Refund)
	assert.Equal(t, int64(500), ev.Refund.RefundedCents)
}

// TestE2E_Subscription tests subscription creation
func TestE2E_Subscription(t *testing.T) {
	mockURL := os.Getenv("E2E_STRIPE_MOCK_URL")
	if mockURL == "" {
		mockURL = "http://localhost:12111"
	}

	client := New("sk_test_123", "whsec_test", WithAPIURL(mockURL))
	ctx := context.Background()

	// Create product and price
	productID, err := client.EnsureProduct(ctx, "", "Test Product", "test-plan")
	require.NoError(t, err)

	priceID, err := client.CreatePrice(ctx, billing.StripePriceParams{
		ProductID:   productID,
		AmountCents: 9999,
		Currency:    "eur",
		Interval:    "month",
	})
	require.NoError(t, err)

	// Create customer
	customerID, err := client.EnsureCustomer(ctx, "", "user:456", "Jane Doe", "jane@example.com")
	require.NoError(t, err)

	// Test CreateSubscription with a payment method
	// Note: stripe-mock may not have payment methods set up, so this might error
	sub, err := client.CreateSubscription(ctx, billing.SubscriptionParams{
		IdempotencyKey:  "sub-e2e-123",
		TrialEnd:        time.Now().Add(30 * 24 * time.Hour),
		CustomerID:      customerID,
		PriceID:         priceID,
		PaymentMethodID: "pm_test", // stripe-mock test ID
		Metadata: map[string]string{
			"test": "true",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sub.ID)

	// Test CancelSubscription
	_, err = client.CancelSubscription(ctx, sub.ID, true)
	require.NoError(t, err)
	_, err = client.CancelSubscription(ctx, sub.ID, false)
	require.NoError(t, err)

	// Without a payment method the latest saved card is used.
	_, err = client.CreateSubscription(ctx, billing.SubscriptionParams{CustomerID: customerID, PriceID: priceID})
	require.NoError(t, err)
}
