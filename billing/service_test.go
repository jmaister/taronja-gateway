package billing

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEnv is a Service on a real SQLite database with fake Stripe and Odoo
// and a clock tests can move.
type testEnv struct {
	svc    *Service
	repo   *repositories.BillingRepositoryDB
	orgs   *repositories.OrganizationRepositoryDB
	stripe *FakeStripe
	odoo   *FakeOdoo
	clock  time.Time
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	e := &testEnv{
		repo: repositories.NewBillingRepositoryDB(conn), orgs: repositories.NewOrganizationRepositoryDB(conn),
		stripe: NewFakeStripe(), odoo: NewFakeOdoo(), clock: now,
	}
	e.odoo.Now = func() time.Time { return e.clock }
	e.svc = NewService(Config{
		Repo: e.repo, Orgs: e.orgs, Stripe: e.stripe, Odoo: e.odoo,
		ServerURL: "https://gw.example.com/", Now: func() time.Time { return e.clock },
	})
	return e
}

func (e *testEnv) org(t *testing.T, name string, members ...string) string {
	t.Helper()
	o := &db.Organization{Name: name}
	require.NoError(t, e.orgs.Create(o))
	for _, m := range members {
		require.NoError(t, e.orgs.SetMember(o.ID, m, "member"))
	}
	return o.ID
}

func monthlyInput() PlanInput {
	return PlanInput{
		Key: "academia-monthly", Name: "Academia mensual", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 2900, Features: []string{"academia.access"}, Subject: db.PlanSubjectOrg, TrialDays: 14,
		GraceDays: DefaultGraceDays, Group: "academia", Active: true,
	}
}

func TestForUser_CachesAndInvalidates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	orgID := e.org(t, "Escuela", "u1")

	ent, err := e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Empty(t, ent.User)
	assert.Empty(t, ent.Orgs[orgID])

	until := now.AddDate(0, 0, 14)
	_, err = e.svc.CreateGrant(string(OrgSubject(orgID)), "academia.access", &until, "trial", "admin")
	require.NoError(t, err)
	ent, err = e.svc.ForUser("u1")
	require.NoError(t, err)
	assert.Equal(t, Feature{Until: &until, Status: StatusGranted, Source: SourceGrant}, ent.Orgs[orgID]["academia.access"])

	// A subscription written straight to the repository is not seen until
	// the subject is invalidated: the cache is what the request path reads.
	end := now.AddDate(0, 1, 0)
	require.NoError(t, e.repo.CreateSubscription(&db.BillingSubscription{
		Subject: string(UserSubject("u1")), PlanKey: "academia-monthly", Status: db.SubscriptionActive, CurrentPeriodEnd: &end,
	}))
	ent, _ = e.svc.ForUser("u1")
	assert.Empty(t, ent.User)
	e.svc.Invalidate(string(UserSubject("u1")))
	ent, _ = e.svc.ForUser("u1")
	assert.Equal(t, StatusActive, ent.User["academia.access"].Status)

	// New membership: seen after InvalidateUser.
	other := e.org(t, "Otra")
	require.NoError(t, e.orgs.SetMember(other, "u1", "owner"))
	ent, _ = e.svc.ForUser("u1")
	assert.NotContains(t, ent.Orgs, other)
	e.svc.InvalidateUser("u1")
	ent, _ = e.svc.ForUser("u1")
	assert.Contains(t, ent.Orgs, other)

	// Expiry needs no invalidation: Active filters by the clock.
	e.clock = until.Add(time.Second)
	assert.NotContains(t, ent.Orgs[orgID].Active(e.clock), "academia.access")
}

func TestCreateGrant_Validates(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreateGrant("team:1", "academia.access", nil, "", "admin")
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.CreateGrant("user:1", "Academia Access", nil, "", "admin")
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.CreateGrant("org:missing", "academia.access", nil, "", "admin")
	assert.ErrorIs(t, err, ErrNotFound)

	g, err := e.svc.CreateGrant("user:1", "academia.access", nil, " comp ", "admin")
	require.NoError(t, err)
	assert.Equal(t, "comp", g.Note)
	ent, _ := e.svc.SubjectEntitlements("user:1")
	assert.Contains(t, ent, "academia.access")
	require.NoError(t, e.svc.DeleteGrant(g.ID))
	ent, _ = e.svc.SubjectEntitlements("user:1")
	assert.Empty(t, ent)
	assert.ErrorIs(t, e.svc.DeleteGrant(g.ID), ErrNotFound)
}
