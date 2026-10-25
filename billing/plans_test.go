package billing

import (
	"context"
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePlan_SyncsStripeAndOdoo(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := monthlyInput()
	in.Features = []string{"academia.reports", "academia.access", "academia.access"}
	plan, err := e.svc.CreatePlan(ctx, in)
	require.NoError(t, err)

	assert.Equal(t, []string{"academia.access", "academia.reports"}, plan.Features)
	assert.Equal(t, "Academia mensual", e.stripe.Products[plan.StripeProductID])
	price := e.stripe.Prices[plan.StripePriceID]
	assert.Equal(t, StripePriceParams{
		ProductID: plan.StripeProductID, LookupKey: "tg_academia-monthly", AmountCents: 2900, Currency: "eur", Interval: "month", Nickname: "Academia mensual",
	}, price)
	assert.Empty(t, plan.StripeFirstPeriodPriceID)
	assert.Empty(t, plan.StripeSetupFeePriceID)
	require.NotNil(t, plan.OdooProductID)
	assert.Equal(t, "Academia mensual", e.odoo.Products["TG-academia-monthly"])

	stored, err := e.svc.GetPlan("academia-monthly")
	require.NoError(t, err)
	assert.Equal(t, plan.StripePriceID, stored.StripePriceID)
	assert.Equal(t, 14, stored.TrialDays)

	_, err = e.svc.CreatePlan(ctx, monthlyInput())
	assert.ErrorIs(t, err, ErrConflict)
}

func TestCreatePlan_OnActivationWithSetupFee(t *testing.T) {
	e := newEnv(t)
	plan, err := e.svc.CreatePlan(context.Background(), PlanInput{
		Key: "megabox-familia", Name: "Megabox Familia", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 990, Features: []string{"megabox.protection"}, Subject: db.PlanSubjectUser, Starts: db.PlanStartsOnActivation,
		SetupFeeCents: 4900, SetupFeeName: "Caja Megabox", CollectShipping: true, GraceDays: 7, Active: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "month", e.stripe.Prices[plan.StripePriceID].Interval)
	first := e.stripe.Prices[plan.StripeFirstPeriodPriceID]
	assert.Equal(t, int64(990), first.AmountCents)
	assert.Empty(t, first.Interval, "the first period is a one-time price")
	fee := e.stripe.Prices[plan.StripeSetupFeePriceID]
	assert.Equal(t, int64(4900), fee.AmountCents)
	assert.Equal(t, "Caja Megabox", fee.Nickname)
	assert.Equal(t, "Caja Megabox", e.odoo.Products["TG-megabox-familia-setup"])
	require.NotNil(t, plan.OdooSetupFeeProductID)
}

func TestUpdatePlan_PriceChangeMakesNewPrice(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	oldPrice := plan.StripePriceID
	productCalls := len(e.stripe.Products)

	in := PlanInputFrom(*plan)
	in.Description = "Todo incluido"
	updated, err := e.svc.UpdatePlan(ctx, plan.Key, in)
	require.NoError(t, err)
	assert.Equal(t, oldPrice, updated.StripePriceID, "no new price without an amount change")

	in = PlanInputFrom(*updated)
	in.AmountCents = 3500
	updated, err = e.svc.UpdatePlan(ctx, plan.Key, in)
	require.NoError(t, err)
	assert.NotEqual(t, oldPrice, updated.StripePriceID)
	assert.True(t, e.stripe.ArchivedPrice[oldPrice])
	assert.Equal(t, updated.StripePriceID, e.stripe.LookupKeys["tg_academia-monthly"], "the lookup key moves to the new price")
	assert.Equal(t, productCalls, len(e.stripe.Products))

	in = PlanInputFrom(*updated)
	in.Name = "Academia (mes)"
	updated, err = e.svc.UpdatePlan(ctx, plan.Key, in)
	require.NoError(t, err)
	assert.Equal(t, "Academia (mes)", e.stripe.Products[updated.StripeProductID])
	assert.Equal(t, "Academia (mes)", e.odoo.Products["TG-academia-monthly"])

	in = PlanInputFrom(*updated)
	in.Active = false
	updated, err = e.svc.UpdatePlan(ctx, plan.Key, in)
	require.NoError(t, err)
	assert.False(t, updated.Active)
	active, err := e.svc.ListPlans(true)
	require.NoError(t, err)
	assert.Empty(t, active)

	in.Kind = db.PlanKindOneTime
	in.Interval = ""
	in.TrialDays = 0
	_, err = e.svc.UpdatePlan(ctx, plan.Key, in)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.UpdatePlan(ctx, "missing", in)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPlanValidation(t *testing.T) {
	e := newEnv(t)
	cases := map[string]func(*PlanInput){
		"bad key":                func(p *PlanInput) { p.Key = "Academia Monthly" },
		"no name":                func(p *PlanInput) { p.Name = " " },
		"zero amount":            func(p *PlanInput) { p.AmountCents = 0 },
		"bad interval":           func(p *PlanInput) { p.Interval = "week" },
		"bad subject":            func(p *PlanInput) { p.Subject = "team" },
		"no features":            func(p *PlanInput) { p.Features = nil },
		"bad feature":            func(p *PlanInput) { p.Features = []string{"Academia"} },
		"fee without name":       func(p *PlanInput) { p.SetupFeeCents = 100 },
		"trial on activation":    func(p *PlanInput) { p.Starts = db.PlanStartsOnActivation },
		"access days on sub":     func(p *PlanInput) { p.AccessDays = 30 },
		"one-time with interval": func(p *PlanInput) { p.Kind = db.PlanKindOneTime; p.TrialDays = 0 },
		"one-time with trial":    func(p *PlanInput) { p.Kind = db.PlanKindOneTime; p.Interval = "" },
		"grace too long":         func(p *PlanInput) { p.GraceDays = 90 },
		"unknown kind":           func(p *PlanInput) { p.Kind = "lifetime" },
		"negative access days": func(p *PlanInput) {
			*p = PlanInput{Key: "x", Name: "x", Kind: db.PlanKindOneTime, AmountCents: 1, Features: []string{"a.b"}, AccessDays: -1}
		},
		"bad starts":              func(p *PlanInput) { p.Starts = "later" },
		"negative setup fee":      func(p *PlanInput) { p.SetupFeeCents = -1 },
		"trial days out of range": func(p *PlanInput) { p.TrialDays = 1000 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := monthlyInput()
			mutate(&in)
			_, err := e.svc.CreatePlan(context.Background(), in)
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
	assert.Empty(t, e.stripe.Products, "nothing reaches Stripe for an invalid plan")
}

func TestSyncPortal_SwitchesWithinGroups(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	monthly, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	require.NotNil(t, e.stripe.Portal)
	assert.Empty(t, e.stripe.Portal.SwitchProducts, "a group of one has nothing to switch to")
	assert.Equal(t, "https://gw.example.com/_/account", e.stripe.Portal.ReturnURL)

	yearly := monthlyInput()
	yearly.Key, yearly.Name, yearly.Interval, yearly.AmountCents = "academia-yearly", "Academia anual", db.PlanIntervalYear, 29000
	y, err := e.svc.CreatePlan(ctx, yearly)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{
		monthly.StripeProductID: {monthly.StripePriceID},
		y.StripeProductID:       {y.StripePriceID},
	}, e.stripe.Portal.SwitchProducts)
}

func TestCreatePlan_OdooDownStillCreates(t *testing.T) {
	e := newEnv(t)
	e.svc.odoo = failingOdoo{e.odoo}
	plan, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	assert.Nil(t, plan.OdooProductID)
}

type failingOdoo struct{ *FakeOdoo }

func (failingOdoo) UpsertProduct(ctx context.Context, code, name string) (int64, error) {
	return 0, assert.AnError
}
