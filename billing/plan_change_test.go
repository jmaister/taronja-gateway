package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An upgrade is charged at once as one invoice with Stripe's prorated lines
// and an explanatory note; a downgrade waits for the period end, with no
// credit note, no refund and an ordinary invoice at the new price.
func TestPlanChange_UpgradeThenScheduledDowngrade(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	mailer := &fakeMailer{}
	e.svc.mailer = mailer
	basic, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	proIn := monthlyInput()
	proIn.Key, proIn.Name, proIn.AmountCents, proIn.Features = "academia-pro", "Academia Pro", 5900, []string{"academia.access", "academia.pro"}
	pro, err := e.svc.CreatePlan(ctx, proIn)
	require.NoError(t, err)
	e.customer(t, "org:o1", "b12345674")

	start := e.clock
	end := start.AddDate(0, 1, 0)
	e.event(t, Event{ID: "evt_1", Type: EventSubscriptionCreated, Subscription: &StripeSubscription{
		ID: "sub_1", Status: "active", CurrentPeriodEnd: end, PriceID: basic.StripePriceID,
		Metadata: map[string]string{"tg_subject": "org:o1", "tg_plan": basic.Key, "tg_checkout": e.checkout(t, "org:o1", basic.Key)},
	}})
	e.event(t, Event{ID: "evt_2", Type: EventInvoicePaid, Invoice: &InvoiceEvent{
		InvoiceID: "in_1", SubscriptionID: "sub_1", AmountCents: 2900, PaidAt: e.clock,
		Lines: []EventLine{{Description: "1 × Academia mensual", PriceID: basic.StripePriceID, AmountCents: 2900, PeriodStart: &start, PeriodEnd: &end}},
	}})
	ent, err := e.svc.SubjectEntitlements("org:o1")
	require.NoError(t, err)
	assert.Contains(t, ent, "academia.access")
	assert.NotContains(t, ent, "academia.pro")

	// Upgrade ten days in: same period, prorated difference paid at once.
	e.clock = start.AddDate(0, 0, 10)
	change := e.clock
	e.event(t, Event{ID: "evt_3", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_1", Status: "active", CurrentPeriodEnd: end, PriceID: pro.StripePriceID}})
	sub, err := e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	assert.Equal(t, pro.Key, sub.PlanKey)
	require.NotNil(t, sub.CurrentPeriodEnd)
	assert.True(t, sub.CurrentPeriodEnd.Equal(end), "the period keeps its dates")
	ent, err = e.svc.SubjectEntitlements("org:o1")
	require.NoError(t, err)
	assert.Contains(t, ent, "academia.pro", "the new plan's features apply at once")

	e.event(t, Event{ID: "evt_4", Type: EventInvoicePaid, Invoice: &InvoiceEvent{
		InvoiceID: "in_2", SubscriptionID: "sub_1", AmountCents: 2000, PaidAt: e.clock,
		Lines: []EventLine{
			{Description: "Tiempo no utilizado en Academia mensual después del 11 nov 2026", PriceID: basic.StripePriceID, AmountCents: -1933, PeriodStart: &change, PeriodEnd: &end, Proration: true},
			{Description: "Tiempo restante en Academia Pro después del 11 nov 2026", PriceID: pro.StripePriceID, AmountCents: 3933, PeriodStart: &change, PeriodEnd: &end, Proration: true},
		},
	}})
	assert.Equal(t, 2, e.process(t))
	move := e.odoo.Moves["in_2"]
	require.Len(t, move.Lines, 2)
	assert.Equal(t, "Tiempo no utilizado en Academia mensual después del 11 nov 2026", move.Lines[0].Description)
	assert.EqualValues(t, -1933, move.Lines[0].AmountCents)
	assert.Equal(t, "Tiempo restante en Academia Pro después del 11 nov 2026", move.Lines[1].Description)
	assert.EqualValues(t, 3933, move.Lines[1].AmountCents)
	assert.False(t, move.CreditNote, "the credit for the old plan is a line of the invoice, not a credit note")
	assert.Equal(t, "Cambio de plan de Academia mensual a Academia Pro el "+spanishDate(change)+": se descuenta el tiempo no usado de Academia mensual y se cobra el tiempo restante de Academia Pro.", move.Note)
	assert.Empty(t, e.odoo.Moves["in_1"].Note, "an ordinary invoice has no note")

	require.Len(t, mailer.sent, 2)
	up := mailer.sent[1]
	require.Len(t, up.Attachments, 1)
	assert.Contains(t, string(up.Attachments[0].Data), "%PDF")
	for _, want := range []string{"Tiempo no utilizado en Academia mensual", "Tiempo restante en Academia Pro", "20,00 €"} {
		assert.Contains(t, up.Text, want)
	}
	e.process(t)
	assert.Len(t, mailer.sent, 2, "each document is emailed once")

	// Downgrade scheduled for the period end: Stripe changes nothing until then.
	e.clock = start.AddDate(0, 0, 15)
	e.event(t, Event{ID: "evt_5", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_1", Status: "active", CurrentPeriodEnd: end, PriceID: pro.StripePriceID}})
	sub, err = e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	assert.Equal(t, pro.Key, sub.PlanKey, "still on the dearer plan until the period ends")
	assert.Empty(t, e.stripe.Refunds)

	// The period ends: the new price is charged in full, nothing credited.
	e.clock = end
	nextEnd := end.AddDate(0, 1, 0)
	e.event(t, Event{ID: "evt_6", Type: EventSubscriptionUpdated, Subscription: &StripeSubscription{ID: "sub_1", Status: "active", CurrentPeriodEnd: nextEnd, PriceID: basic.StripePriceID}})
	e.event(t, Event{ID: "evt_7", Type: EventInvoicePaid, Invoice: &InvoiceEvent{
		InvoiceID: "in_3", SubscriptionID: "sub_1", AmountCents: 2900, PaidAt: e.clock,
		Lines: []EventLine{{Description: "1 × Academia mensual", PriceID: basic.StripePriceID, AmountCents: 2900, PeriodStart: &end, PeriodEnd: &nextEnd}},
	}})
	e.process(t)
	sub, err = e.repo.GetSubscriptionByStripeID("sub_1")
	require.NoError(t, err)
	assert.Equal(t, basic.Key, sub.PlanKey)
	ent, err = e.svc.SubjectEntitlements("org:o1")
	require.NoError(t, err)
	assert.NotContains(t, ent, "academia.pro")
	renewal := e.odoo.Moves["in_3"]
	require.Len(t, renewal.Lines, 1)
	assert.EqualValues(t, 2900, renewal.Lines[0].AmountCents)
	assert.Equal(t, "Academia mensual "+spanishDate(end)+"–"+spanishDate(nextEnd.AddDate(0, 0, -1)), renewal.Lines[0].Description)
	assert.Empty(t, renewal.Note)

	for ref, m := range e.odoo.Moves {
		assert.False(t, m.CreditNote, ref)
	}
	assert.Empty(t, e.stripe.Refunds)
	assert.Len(t, e.odoo.Moves, 3)
	assert.Len(t, mailer.sent, 3)
}
