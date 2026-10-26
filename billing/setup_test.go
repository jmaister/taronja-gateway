package billing

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupChecks(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	byID := func() map[string]SetupCheck {
		out := map[string]SetupCheck{}
		for _, c := range e.svc.SetupChecks(ctx) {
			out[c.ID] = c
		}
		return out
	}
	checks := byID()
	assert.Equal(t, CheckMissing, checks["stripe-key"].Status, "no mode given")
	assert.Equal(t, CheckWarn, checks["plans"].Status)
	assert.Contains(t, checks["stripe-webhook"].Detail, "https://gw.example.com/_/billing/stripe/webhook")
	assert.Contains(t, checks["stripe-webhook"].Detail, EventChargeRefunded)
	assert.Equal(t, CheckOK, checks["odoo-connection"].Status)

	e.svc.odoo = failingOdoo{e.odoo}
	_, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	e.svc.odoo = e.odoo
	plans := byID()["plans"]
	assert.Equal(t, CheckWarn, plans.Status)
	assert.Equal(t, ActionSyncPlans, plans.Action)
	require.NoError(t, e.svc.RunSetupAction(ctx, ActionSyncPlans))
	assert.Equal(t, CheckOK, byID()["plans"].Status)

	e.stripe.Portal = nil
	require.NoError(t, e.svc.RunSetupAction(ctx, ActionSyncPortal))
	assert.NotNil(t, e.stripe.Portal)
	assert.ErrorIs(t, e.svc.RunSetupAction(ctx, "nope"), ErrInvalid)

	e.svc.odoo = nil
	assert.Equal(t, CheckMissing, byID()["odoo-connection"].Status)
}

func TestSetupChecks_OdooOutbox(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	check := func() SetupCheck { return e.svc.outboxCheck() }
	e.clock = time.Now() // documents are stamped with the real creation time

	c := check()
	assert.Equal(t, CheckOK, c.Status)
	assert.Equal(t, "0 pending, 0 retrying, 0 failed", c.Detail)

	e.customer(t, "org:o1", "")
	inv := e.invoice(t, "org:o1", "in_1")
	c = check()
	assert.Equal(t, CheckOK, c.Status, "a fresh document is just queued")
	assert.Contains(t, c.Detail, "1 pending, 0 retrying, 0 failed, oldest waiting")

	e.clock = e.clock.Add(2 * time.Hour)
	c = check()
	assert.Equal(t, CheckWarn, c.Status, "waiting for over an hour")
	assert.Equal(t, "/billing/invoices?state=pending", c.Link)

	inv.Attempts = 2
	require.NoError(t, e.repo.UpdateDocument(inv))
	assert.Contains(t, check().Detail, "0 pending, 1 retrying, 0 failed")

	inv.OdooState = db.OdooStateFailed
	require.NoError(t, e.repo.UpdateDocument(inv))
	c = check()
	assert.Equal(t, CheckMissing, c.Status)
	assert.Equal(t, OutboxLink, c.Link)
	assert.Contains(t, c.Detail, "1 failed")

	var ids []string
	for _, sc := range e.svc.SetupChecks(ctx) {
		ids = append(ids, sc.ID)
	}
	assert.Contains(t, ids, "odoo-outbox")
}

func TestSetupChecks_StuckStripeEvents(t *testing.T) {
	e := newEnv(t)
	check := func() SetupCheck { return e.svc.stuckEventsCheck() }
	assert.Equal(t, CheckOK, check().Status)

	require.NoError(t, e.repo.RecordStuckEvent("evt_1", EventInvoicePaid, "sub_1", "not recorded", e.clock))
	c := check()
	assert.Equal(t, CheckWarn, c.Status, "just arrived")
	assert.Contains(t, c.Detail, "invoice.paid evt_1 (sub_1, 1 attempts)")

	e.clock = e.clock.Add(3 * time.Hour)
	assert.Equal(t, CheckMissing, check().Status, "waiting for over two hours")

	var ids []string
	for _, sc := range e.svc.SetupChecks(context.Background()) {
		ids = append(ids, sc.ID)
	}
	assert.Contains(t, ids, "stripe-events")

	require.NoError(t, e.repo.ClearStuckEvent("evt_1"))
	assert.Equal(t, CheckOK, check().Status)
}

func TestSetupChecks_OdooAPIKeyExpiry(t *testing.T) {
	e := newEnv(t)
	day := func(d string) time.Time {
		v, err := time.Parse(time.DateOnly, d)
		require.NoError(t, err)
		return v
	}
	e.clock = day("2026-10-26")
	check := func() SetupCheck { return e.svc.apiKeyCheck() }

	assert.Equal(t, CheckManual, check().Status, "unset")
	for exp, want := range map[string]string{
		"2027-03-01": CheckOK,
		"2026-11-25": CheckOK,
		"2026-11-24": CheckWarn,
		"2026-10-26": CheckWarn, // valid through the whole day
		"2026-10-25": CheckMissing,
	} {
		e.svc.odooKeyExp = day(exp)
		assert.Equal(t, want, check().Status, exp)
	}
	e.svc.odooKeyExp = day("2026-10-25")
	assert.Contains(t, check().Detail, "expired on 2026-10-25")
}
