package billing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func at(days int) *time.Time {
	t := now.AddDate(0, 0, days)
	return &t
}

func testPlans() map[string]repositories.BillingPlanWithFeatures {
	return map[string]repositories.BillingPlanWithFeatures{
		"monthly": {BillingPlan: db.BillingPlan{Key: "monthly", Kind: db.PlanKindSubscription, GraceDays: 7}, Features: []string{"app.access", "app.extra"}},
		"nograce": {BillingPlan: db.BillingPlan{Key: "nograce", Kind: db.PlanKindSubscription, GraceDays: 0}, Features: []string{"app.access"}},
		"pass30":  {BillingPlan: db.BillingPlan{Key: "pass30", Kind: db.PlanKindOneTime, AccessDays: 30}, Features: []string{"app.pass"}},
		"forever": {BillingPlan: db.BillingPlan{Key: "forever", Kind: db.PlanKindOneTime}, Features: []string{"app.pass"}},
		"box":     {BillingPlan: db.BillingPlan{Key: "box", Kind: db.PlanKindSubscription, GraceDays: 7, Starts: db.PlanStartsOnActivation}, Features: []string{"box.protection"}},
	}
}

func TestCompute_Subscriptions(t *testing.T) {
	tests := []struct {
		name string
		sub  db.BillingSubscription
		want *Feature // for app.access / box.protection; nil = not granted
	}{
		{"trialing until trial end", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionTrialing, TrialEnd: at(10)},
			&Feature{Until: at(10), Status: StatusTrial}},
		{"trial ended", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionTrialing, TrialEnd: at(-1)}, nil},
		{"active: period end plus grace", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionActive, CurrentPeriodEnd: at(5)},
			&Feature{Until: at(12), Status: StatusActive}},
		{"active renewal pending within grace", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionActive, CurrentPeriodEnd: at(-3)},
			&Feature{Until: at(4), Status: StatusActive}},
		{"active, grace 0, period over", db.BillingSubscription{PlanKey: "nograce", Status: db.SubscriptionActive, CurrentPeriodEnd: at(-1)}, nil},
		{"canceling: period end without grace", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionActive, CurrentPeriodEnd: at(5), CancelAtPeriodEnd: true},
			&Feature{Until: at(5), Status: StatusCanceling}},
		{"past due within grace", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionPastDue, CurrentPeriodEnd: at(-2), GraceUntil: at(5)},
			&Feature{Until: at(5), Status: StatusPastDue}},
		{"past due after grace", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionPastDue, GraceUntil: at(-1)}, nil},
		{"awaiting activation never expires", db.BillingSubscription{PlanKey: "box", Status: db.SubscriptionAwaitingActivation},
			&Feature{Status: StatusAwaitingActivation}},
		{"canceled grants nothing", db.BillingSubscription{PlanKey: "monthly", Status: db.SubscriptionCanceled, CurrentPeriodEnd: at(5)}, nil},
		{"unknown plan grants nothing", db.BillingSubscription{PlanKey: "gone", Status: db.SubscriptionActive, CurrentPeriodEnd: at(5)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compute(now, testPlans(), []db.BillingSubscription{tt.sub}, nil, nil)
			feature := "app.access"
			if tt.sub.PlanKey == "box" {
				feature = "box.protection"
			}
			if tt.want == nil {
				assert.Empty(t, got)
				return
			}
			want := *tt.want
			want.Source = SourceSubscription
			assert.Equal(t, want, got[feature])
			if tt.sub.PlanKey == "monthly" {
				assert.Equal(t, want, got["app.extra"], "every feature of the plan")
			}
		})
	}
}

func TestCompute_Purchases(t *testing.T) {
	subID := "sub_1"
	tests := []struct {
		name     string
		purchase db.BillingPurchase
		want     *Feature
	}{
		{"one-time with access days", db.BillingPurchase{PlanKey: "pass30", Status: db.PurchaseStatusPaid, PaidAt: now.AddDate(0, 0, -10)},
			&Feature{Until: at(20), Status: StatusActive, Source: SourcePurchase}},
		{"one-time expired", db.BillingPurchase{PlanKey: "pass30", Status: db.PurchaseStatusPaid, PaidAt: now.AddDate(0, 0, -31)}, nil},
		{"one-time forever", db.BillingPurchase{PlanKey: "forever", Status: db.PurchaseStatusPaid, PaidAt: now.AddDate(-2, 0, 0)},
			&Feature{Status: StatusActive, Source: SourcePurchase}},
		{"refunded", db.BillingPurchase{PlanKey: "forever", Status: db.PurchaseStatusRefunded, PaidAt: now}, nil},
		{"first payment of a subscription", db.BillingPurchase{PlanKey: "box", Status: db.PurchaseStatusPaid, PaidAt: now, SubscriptionID: &subID}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compute(now, testPlans(), nil, []db.BillingPurchase{tt.purchase}, nil)
			if tt.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, *tt.want, got["app.pass"])
		})
	}
}

func TestCompute_LatestUntilWins(t *testing.T) {
	subs := []db.BillingSubscription{{PlanKey: "monthly", Status: db.SubscriptionTrialing, TrialEnd: at(3)}}
	grants := []db.BillingGrant{
		{Feature: "app.access", Until: at(10)},
		{Feature: "app.access", Until: at(-1)},
		{Feature: "other.thing", Until: nil},
	}
	got := Compute(now, testPlans(), subs, nil, grants)
	assert.Equal(t, Feature{Until: at(10), Status: StatusGranted, Source: SourceGrant}, got["app.access"])
	assert.Equal(t, Feature{Until: at(3), Status: StatusTrial, Source: SourceSubscription}, got["app.extra"])
	assert.Equal(t, Feature{Status: StatusGranted, Source: SourceGrant}, got["other.thing"])

	// No expiry beats any date, in either order.
	got = Compute(now, testPlans(), []db.BillingSubscription{{PlanKey: "box", Status: db.SubscriptionAwaitingActivation}},
		nil, []db.BillingGrant{{Feature: "box.protection", Until: at(100)}})
	assert.Nil(t, got["box.protection"].Until)
	assert.Equal(t, StatusAwaitingActivation, got["box.protection"].Status)
}

func TestHeader(t *testing.T) {
	u := UserEntitlements{
		User: Entitlements{"a.one": {Until: at(1)}, "a.old": {Until: at(-1)}},
		Orgs: map[string]Entitlements{
			"org1": {"b.two": {}},
			"org2": {"b.gone": {Until: at(-2)}},
		},
	}
	var h map[string]any
	require.NoError(t, json.Unmarshal([]byte(u.HeaderValue(now)), &h))
	assert.Equal(t, map[string]any{
		"user": map[string]any{"a.one": "2026-10-26T12:00:00Z"},
		"orgs": map[string]any{"org1": map[string]any{"b.two": nil}},
	}, h)
}
