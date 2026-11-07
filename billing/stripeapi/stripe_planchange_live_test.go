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
	"github.com/stripe/stripe-go/v87"
)

// TestLive_PlanChange runs a monthly subscription through an upgrade and a
// downgrade on a Stripe test clock, with the proration behavior the portal
// config uses: upgrades are invoiced at once for the prorated difference,
// downgrades wait for the period end with no credit or refund.
func TestLive_PlanChange(t *testing.T) {
	key := os.Getenv("STRIPE_TEST_SECRET_KEY")
	if key == "" {
		t.Skip("STRIPE_TEST_SECRET_KEY is not set")
	}
	require.Contains(t, key, "_test_", "refusing to run against a live key")
	client := New(key, "whsec_unused")
	sc := client.sc
	ctx := context.Background()
	run := fmt.Sprintf("e2e-pc-%d", time.Now().UnixNano())
	const cheap, dear = 2000, 5000

	productID, err := client.EnsureProduct(ctx, "", "Taronja plan change "+run, run, "")
	require.NoError(t, err)
	priceCheap, err := client.CreatePrice(ctx, billing.StripePriceParams{ProductID: productID, AmountCents: cheap, Currency: "eur", Interval: "month", LookupKey: run + "-cheap"})
	require.NoError(t, err)
	priceDear, err := client.CreatePrice(ctx, billing.StripePriceParams{ProductID: productID, AmountCents: dear, Currency: "eur", Interval: "month", LookupKey: run + "-dear"})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.ArchivePrice(ctx, priceCheap)
		_ = client.ArchivePrice(ctx, priceDear)
	})

	start := time.Now().UTC().Truncate(time.Hour)
	clock, err := sc.V1TestHelpersTestClocks.Create(ctx, &stripe.TestHelpersTestClockCreateParams{FrozenTime: stripe.Int64(start.Unix()), Name: stripe.String(run)})
	require.NoError(t, err)
	advance := func(to time.Time) {
		_, err := sc.V1TestHelpersTestClocks.Advance(ctx, clock.ID, &stripe.TestHelpersTestClockAdvanceParams{FrozenTime: stripe.Int64(to.Unix())})
		require.NoError(t, err)
		for i := 0; i < 60; i++ {
			c, err := sc.V1TestHelpersTestClocks.Retrieve(ctx, clock.ID, nil)
			require.NoError(t, err)
			if c.Status == stripe.TestHelpersTestClockStatusReady {
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Fatal("the test clock did not become ready")
	}

	customer, err := sc.V1Customers.Create(ctx, &stripe.CustomerCreateParams{Name: stripe.String("Plan change " + run), Email: stripe.String(run + "@example.com"), TestClock: stripe.String(clock.ID)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = sc.V1TestHelpersTestClocks.Delete(ctx, clock.ID, nil)
	})
	pm, err := sc.V1PaymentMethods.Attach(ctx, "pm_card_visa", &stripe.PaymentMethodAttachParams{Customer: stripe.String(customer.ID)})
	require.NoError(t, err)

	sub, err := sc.V1Subscriptions.Create(ctx, &stripe.SubscriptionCreateParams{
		Customer:             stripe.String(customer.ID),
		DefaultPaymentMethod: stripe.String(pm.ID),
		Items:                []*stripe.SubscriptionCreateItemParams{{Price: stripe.String(priceCheap)}},
	})
	require.NoError(t, err)
	item := sub.Items.Data[0]
	periodStart, periodEnd := item.CurrentPeriodStart, item.CurrentPeriodEnd
	invoices := func() []*stripe.Invoice {
		var out []*stripe.Invoice
		for inv, err := range sc.V1Invoices.List(ctx, &stripe.InvoiceListParams{Customer: stripe.String(customer.ID)}).All(ctx) {
			require.NoError(t, err)
			out = append(out, inv)
		}
		return out
	}
	first := invoices()
	require.Len(t, first, 1)
	assert.EqualValues(t, cheap, first[0].AmountPaid, "the first period is charged in full")

	// Upgrade ten days in: the portal does this with always_invoice.
	changeAt := start.Add(10 * 24 * time.Hour)
	advance(changeAt)
	sub, err = sc.V1Subscriptions.Update(ctx, sub.ID, &stripe.SubscriptionUpdateParams{
		Items:             []*stripe.SubscriptionUpdateItemParams{{ID: stripe.String(item.ID), Price: stripe.String(priceDear)}},
		ProrationBehavior: stripe.String(portalProration),
	})
	require.NoError(t, err)
	assert.Equal(t, periodStart, sub.Items.Data[0].CurrentPeriodStart, "the billing period keeps its dates")
	assert.Equal(t, periodEnd, sub.Items.Data[0].CurrentPeriodEnd, "the billing period keeps its dates")

	all := invoices()
	require.Len(t, all, 2, "the upgrade is invoiced at once")
	up := all[0]
	remaining := float64(periodEnd-changeAt.Unix()) / float64(periodEnd-periodStart)
	want := int64(float64(dear-cheap)*remaining + 0.5)
	assert.InDelta(t, want, up.AmountPaid, 2, "prorated difference, remaining %.4f of the period", remaining)
	assert.EqualValues(t, up.Total, up.AmountPaid, "paid, not left as customer credit")
	var credit, charge int64
	for _, l := range up.Lines.Data {
		t.Logf("upgrade line: %q %d", l.Description, l.Amount)
		if l.Amount < 0 {
			credit += l.Amount
		} else {
			charge += l.Amount
		}
	}
	assert.Negative(t, credit, "unused time of the old plan is a line of the same invoice")
	assert.Positive(t, charge)
	t.Logf("upgrade invoice %s: %d cents (credit %d, charge %d, remaining %.4f)", up.ID, up.AmountPaid, credit, charge, remaining)

	// Downgrade: scheduled for the period end, like the portal does for a
	// decreasing amount.
	schedule, err := sc.V1SubscriptionSchedules.Create(ctx, &stripe.SubscriptionScheduleCreateParams{FromSubscription: stripe.String(sub.ID)})
	require.NoError(t, err)
	phase := schedule.Phases[0]
	_, err = sc.V1SubscriptionSchedules.Update(ctx, schedule.ID, &stripe.SubscriptionScheduleUpdateParams{
		EndBehavior: stripe.String("release"),
		Phases: []*stripe.SubscriptionScheduleUpdatePhaseParams{
			{
				StartDate:         stripe.Int64(phase.StartDate),
				EndDate:           stripe.Int64(phase.EndDate),
				Items:             []*stripe.SubscriptionScheduleUpdatePhaseItemParams{{Price: stripe.String(priceDear)}},
				ProrationBehavior: stripe.String("none"),
			},
			{Items: []*stripe.SubscriptionScheduleUpdatePhaseItemParams{{Price: stripe.String(priceCheap)}}, Duration: &stripe.SubscriptionScheduleUpdatePhaseDurationParams{Interval: stripe.String("month"), IntervalCount: stripe.Int64(1)}},
		},
	})
	require.NoError(t, err)
	assert.Len(t, invoices(), 2, "a scheduled downgrade invoices nothing now")
	cust, err := sc.V1Customers.Retrieve(ctx, customer.ID, nil)
	require.NoError(t, err)
	assert.Zero(t, cust.Balance, "no credit is left for the cheaper plan")
	sub, err = sc.V1Subscriptions.Retrieve(ctx, sub.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, priceDear, sub.Items.Data[0].Price.ID, "still on the dearer plan until the period ends")
	assert.Equal(t, periodEnd, sub.Items.Data[0].CurrentPeriodEnd)

	// The period ends: the cheaper plan is charged in full, nothing credited.
	advance(time.Unix(periodEnd, 0).Add(2 * time.Hour))
	all = invoices()
	require.Len(t, all, 3)
	assert.EqualValues(t, cheap, all[0].AmountPaid, "the next cycle charges the new price")
	assert.Len(t, all[0].Lines.Data, 1)
	sub, err = sc.V1Subscriptions.Retrieve(ctx, sub.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, priceCheap, sub.Items.Data[0].Price.ID)
	assert.Greater(t, sub.Items.Data[0].CurrentPeriodEnd, periodEnd)
	cust, err = sc.V1Customers.Retrieve(ctx, customer.ID, nil)
	require.NoError(t, err)
	assert.Zero(t, cust.Balance)
	t.Logf("renewal invoice %s: %d cents, new period ends %s", all[0].ID, all[0].AmountPaid, time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0).UTC().Format(time.DateOnly))
}
