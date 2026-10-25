package billing

import (
	"context"
	"testing"

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
