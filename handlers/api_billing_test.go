package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type billingFixture struct {
	s      *StrictApiServer
	svc    *billing.Service
	repo   *repositories.BillingRepositoryDB
	orgs   *repositories.OrganizationRepositoryDB
	stripe *billing.FakeStripe
	user   *db.User
	other  *db.User
	orgID  string
}

func newBillingFixture(t *testing.T) *billingFixture {
	t.Helper()
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	f := &billingFixture{
		repo: repositories.NewBillingRepositoryDB(conn), orgs: repositories.NewOrganizationRepositoryDB(conn), stripe: billing.NewFakeStripe(),
	}
	userRepo := repositories.NewDBUserRepository(conn)
	f.svc = billing.NewService(billing.Config{Repo: f.repo, Orgs: f.orgs, Stripe: f.stripe, Odoo: billing.NewFakeOdoo(), ServerURL: "https://gw.test"})
	f.s = &StrictApiServer{userRepo: userRepo}
	f.s.SetOrganizations(f.orgs)
	f.s.SetBilling(f.svc)
	f.user = &db.User{Username: "ana", Email: "ana@example.com"}
	require.NoError(t, userRepo.CreateUser(f.user))
	f.other = &db.User{Username: "bob", Email: "bob@example.com"}
	require.NoError(t, userRepo.CreateUser(f.other))
	org := &db.Organization{Name: "Escuela"}
	require.NoError(t, f.orgs.Create(org))
	require.NoError(t, f.orgs.SetMember(org.ID, f.user.ID, "owner"))
	f.orgID = org.ID
	return f
}

func ptr[T any](v T) *T { return &v }

func planBody() *api.BillingPlanInput {
	return &api.BillingPlanInput{
		Key: "academia-monthly", Name: "Academia mensual", Kind: "subscription", Interval: ptr("month"),
		AmountCents: 2900, Features: []string{"academia.access"}, Subject: ptr(api.BillingPlanInputSubject("org")),
		TrialDays: ptr(14), Group: ptr("academia"),
	}
}

func TestBillingAdminEndpointsNeedAdmin(t *testing.T) {
	f := newBillingFixture(t)
	calls := map[string]func(ctx context.Context) (any, error){
		"listPlans": func(ctx context.Context) (any, error) {
			return f.s.ListBillingPlans(ctx, api.ListBillingPlansRequestObject{})
		},
		"createPlan": func(ctx context.Context) (any, error) {
			return f.s.CreateBillingPlan(ctx, api.CreateBillingPlanRequestObject{Body: planBody()})
		},
		"getPlan": func(ctx context.Context) (any, error) {
			return f.s.GetBillingPlan(ctx, api.GetBillingPlanRequestObject{Key: "x"})
		},
		"updatePlan": func(ctx context.Context) (any, error) {
			return f.s.UpdateBillingPlan(ctx, api.UpdateBillingPlanRequestObject{Key: "x", Body: &api.BillingPlanUpdate{}})
		},
		"listGrants": func(ctx context.Context) (any, error) {
			return f.s.ListBillingGrants(ctx, api.ListBillingGrantsRequestObject{})
		},
		"createGrant": func(ctx context.Context) (any, error) {
			return f.s.CreateBillingGrant(ctx, api.CreateBillingGrantRequestObject{Body: &api.BillingGrantRequest{Subject: "user:x", Features: []string{"a.b"}}})
		},
		"deleteGrant": func(ctx context.Context) (any, error) {
			return f.s.DeleteBillingGrant(ctx, api.DeleteBillingGrantRequestObject{Id: "x"})
		},
		"listDocs": func(ctx context.Context) (any, error) {
			return f.s.ListBillingDocuments(ctx, api.ListBillingDocumentsRequestObject{})
		},
		"retryDoc": func(ctx context.Context) (any, error) {
			return f.s.RetryBillingDocument(ctx, api.RetryBillingDocumentRequestObject{Id: "x"})
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

func TestBillingPlansAPI(t *testing.T) {
	f := newBillingFixture(t)
	admin := sessionContext("admin", true)

	resp, err := f.s.CreateBillingPlan(admin, api.CreateBillingPlanRequestObject{Body: planBody()})
	require.NoError(t, err)
	created, ok := resp.(api.CreateBillingPlan201JSONResponse)
	require.True(t, ok, "%T %+v", resp, resp)
	assert.Equal(t, "https://gw.test/_/pay/academia-monthly", created.PayLink)
	assert.Equal(t, billing.DefaultGraceDays, created.GraceDays)
	assert.True(t, created.Active)
	oldPrice := *created.StripePriceId

	resp, _ = f.s.CreateBillingPlan(admin, api.CreateBillingPlanRequestObject{Body: planBody()})
	assert.IsType(t, api.CreateBillingPlan409JSONResponse{}, resp)
	bad := planBody()
	bad.Key = "Bad Key"
	resp, _ = f.s.CreateBillingPlan(admin, api.CreateBillingPlanRequestObject{Body: bad})
	assert.IsType(t, api.CreateBillingPlan400JSONResponse{}, resp)

	usd := planBody()
	usd.Key, usd.Currency = "usd-plan", ptr("USD")
	resp, _ = f.s.CreateBillingPlan(admin, api.CreateBillingPlanRequestObject{Body: usd})
	assert.IsType(t, api.CreateBillingPlan400JSONResponse{}, resp, "EUR only")
	eur := planBody()
	eur.Key, eur.Currency = "eur-plan", ptr("EUR")
	resp, _ = f.s.CreateBillingPlan(admin, api.CreateBillingPlanRequestObject{Body: eur})
	require.IsType(t, api.CreateBillingPlan201JSONResponse{}, resp)
	assert.Equal(t, "eur", resp.(api.CreateBillingPlan201JSONResponse).Currency)
	r0, _ := f.s.UpdateBillingPlan(admin, api.UpdateBillingPlanRequestObject{Key: "academia-monthly", Body: &api.BillingPlanUpdate{Currency: ptr("gbp")}})
	assert.IsType(t, api.UpdateBillingPlan400JSONResponse{}, r0, "EUR only")

	var r any
	r, _ = f.s.UpdateBillingPlan(admin, api.UpdateBillingPlanRequestObject{Key: "academia-monthly", Body: &api.BillingPlanUpdate{AmountCents: ptr(int64(3500))}})
	updated, ok := r.(api.UpdateBillingPlan200JSONResponse)
	require.True(t, ok, "%T %+v", resp, r)
	assert.NotEqual(t, oldPrice, *updated.StripePriceId)
	assert.Equal(t, int64(3500), updated.AmountCents)
	assert.Equal(t, 14, updated.TrialDays, "fields not in the patch are kept")

	r, _ = f.s.UpdateBillingPlan(admin, api.UpdateBillingPlanRequestObject{Key: "academia-monthly", Body: &api.BillingPlanUpdate{Active: ptr(false)}})
	require.IsType(t, api.UpdateBillingPlan200JSONResponse{}, r)
	r, _ = f.s.ListBillingPlans(admin, api.ListBillingPlansRequestObject{})
	assert.Len(t, r, 1, "archived plans are hidden by default")
	r, _ = f.s.ListBillingPlans(admin, api.ListBillingPlansRequestObject{Params: api.ListBillingPlansParams{IncludeArchived: ptr(true)}})
	assert.Len(t, r, 2)

	r, _ = f.s.GetBillingPlan(admin, api.GetBillingPlanRequestObject{Key: "missing"})
	assert.IsType(t, api.GetBillingPlan404JSONResponse{}, r)
	r, _ = f.s.UpdateBillingPlan(admin, api.UpdateBillingPlanRequestObject{Key: "missing", Body: &api.BillingPlanUpdate{}})
	assert.IsType(t, api.UpdateBillingPlan404JSONResponse{}, r)
}

func TestBillingGrantsAndEntitlementsAPI(t *testing.T) {
	f := newBillingFixture(t)
	admin := sessionContext("admin", true)
	until := time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Second)

	resp, err := f.s.CreateBillingGrant(admin, api.CreateBillingGrantRequestObject{Body: &api.BillingGrantRequest{
		Subject: "org:" + f.orgID, Features: []string{"academia.access", "academia.reports"}, Until: &until, Note: ptr("trial"),
	}})
	require.NoError(t, err)
	grants, ok := resp.(api.CreateBillingGrant201JSONResponse)
	require.True(t, ok, "%T %+v", resp, resp)
	require.Len(t, grants, 2)
	assert.Equal(t, "admin", grants[0].CreatedBy)

	for name, body := range map[string]api.BillingGrantRequest{
		"unknown org":              {Subject: "org:nope", Features: []string{"a.b"}},
		"unknown user":             {Subject: "user:nope", Features: []string{"a.b"}},
		"bad feature":              {Subject: "org:" + f.orgID, Features: []string{"a.b", "Bad"}},
		"no features":              {Subject: "org:" + f.orgID},
		"bad subject":              {Subject: "team:1", Features: []string{"a.b"}},
		"grace days without until": {Subject: "org:" + f.orgID, Features: []string{"a.b"}, GraceDays: ptr(3)},
		"too many grace days":      {Subject: "org:" + f.orgID, Features: []string{"a.b"}, Until: &until, GraceDays: ptr(61)},
	} {
		resp, err := f.s.CreateBillingGrant(admin, api.CreateBillingGrantRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, api.CreateBillingGrant400JSONResponse{}, resp, name)
	}
	list, _ := f.s.ListBillingGrants(admin, api.ListBillingGrantsRequestObject{Params: api.ListBillingGrantsParams{Subject: ptr("org:" + f.orgID)}})
	assert.Len(t, list, 2, "a failed request created nothing")

	ent := func(ctx context.Context, params api.GetEntitlementsParams) api.GetEntitlementsResponseObject {
		resp, err := f.s.GetEntitlements(ctx, api.GetEntitlementsRequestObject{Params: params})
		require.NoError(t, err)
		return resp
	}
	byOrg := api.GetEntitlementsParams{OrgId: &f.orgID}
	got, ok := ent(admin, byOrg).(api.GetEntitlements200JSONResponse)
	require.True(t, ok)
	assert.Equal(t, "org:"+f.orgID, got.Subject)
	assert.Equal(t, api.EntitlementStatus("granted"), got.Features["academia.access"].Status)
	assert.Equal(t, until, got.Features["academia.access"].Until.UTC())
	assert.False(t, got.Features["academia.access"].Grace)
	assert.Nil(t, got.Features["academia.access"].GraceUntil)
	assert.Nil(t, meGrace(t, f, f.user.ID), "no grace member in /_/me")
	assert.IsType(t, api.GetEntitlements200JSONResponse{}, ent(sessionContext(f.user.ID, false), byOrg), "a member")
	assert.IsType(t, api.GetEntitlements403JSONResponse{}, ent(sessionContext(f.other.ID, false), byOrg), "not a member")
	assert.IsType(t, api.GetEntitlements200JSONResponse{}, ent(sessionContext(f.other.ID, false), api.GetEntitlementsParams{UserId: &f.other.ID}), "themselves")
	assert.IsType(t, api.GetEntitlements403JSONResponse{}, ent(sessionContext(f.other.ID, false), api.GetEntitlementsParams{UserId: &f.user.ID}))
	assert.IsType(t, api.GetEntitlements400JSONResponse{}, ent(admin, api.GetEntitlementsParams{}))
	assert.IsType(t, api.GetEntitlements400JSONResponse{}, ent(admin, api.GetEntitlementsParams{UserId: &f.user.ID, OrgId: &f.orgID}))
	assert.IsType(t, api.GetEntitlements401JSONResponse{}, ent(context.Background(), byOrg))

	me, err := f.s.GetCurrentUser(sessionContext(f.user.ID, false), api.GetCurrentUserRequestObject{})
	require.NoError(t, err)
	meResp, ok := me.(api.GetCurrentUser200JSONResponse)
	require.True(t, ok, "%T", me)
	require.NotNil(t, meResp.Entitlements)
	assert.Contains(t, meResp.Entitlements.Orgs[f.orgID], "academia.access")
	require.NotNil(t, meResp.Organizations)
	assert.Equal(t, f.orgID, (*meResp.Organizations)[0].Id)

	resp2, _ := f.s.DeleteBillingGrant(admin, api.DeleteBillingGrantRequestObject{Id: grants[0].Id})
	assert.IsType(t, api.DeleteBillingGrant204Response{}, resp2)
	resp2, _ = f.s.DeleteBillingGrant(admin, api.DeleteBillingGrantRequestObject{Id: grants[0].Id})
	assert.IsType(t, api.DeleteBillingGrant404JSONResponse{}, resp2)
}

func TestBillingDocumentsAPI(t *testing.T) {
	f := newBillingFixture(t)
	admin := sessionContext("admin", true)
	ctx := context.Background()
	_, err := f.svc.CreatePlan(ctx, billing.PlanInput{
		Key: "academia-monthly", Name: "Academia mensual", Kind: db.PlanKindSubscription, Interval: "month",
		AmountCents: 2900, Features: []string{"academia.access"}, GraceDays: 7, Active: true,
	})
	require.NoError(t, err)
	subject := "org:" + f.orgID
	require.NoError(t, f.repo.SaveCustomer(&db.BillingCustomer{Subject: subject, Name: "Escuela", Country: "ES"}))
	lines := []billing.DocumentLine{{Description: "Academia mensual", PlanKey: "academia-monthly", AmountCents: 2900}}
	issued := &db.BillingDocument{Subject: subject, Kind: db.DocumentInvoice, StripeRef: "in_1", AmountCents: 2900, PaidAt: time.Now()}
	require.NoError(t, billing.EnqueueDocument(f.repo, issued, lines, time.Now()))
	_, err = f.svc.ProcessDueDocuments(ctx)
	require.NoError(t, err)
	failed := &db.BillingDocument{Subject: "user:" + f.other.ID, Kind: db.DocumentInvoice, StripeRef: "in_2", AmountCents: 2900, PaidAt: time.Now()}
	require.NoError(t, billing.EnqueueDocument(f.repo, failed, lines, time.Now().Add(time.Hour)))
	failed.OdooState = db.OdooStateFailed
	require.NoError(t, f.repo.UpdateDocument(failed))

	resp, _ := f.s.ListBillingDocuments(admin, api.ListBillingDocumentsRequestObject{Params: api.ListBillingDocumentsParams{State: ptr("failed")}})
	docs, ok := resp.(api.ListBillingDocuments200JSONResponse)
	require.True(t, ok)
	require.Len(t, docs, 1)
	assert.Equal(t, "in_2", docs[0].StripeRef)
	assert.Equal(t, "Academia mensual", docs[0].Lines[0].Description)

	credit := &db.BillingDocument{Subject: failed.Subject, Kind: db.DocumentCreditNote, StripeRef: "re_2", InvoiceDocumentID: &failed.ID, AmountCents: 500, PaidAt: time.Now()}
	require.NoError(t, billing.EnqueueDocument(f.repo, credit, lines[:1], time.Now().Add(time.Hour)))
	resp, _ = f.s.ListBillingDocuments(admin, api.ListBillingDocumentsRequestObject{Params: api.ListBillingDocumentsParams{Kind: ptr("credit_note")}})
	creditDocs, ok := resp.(api.ListBillingDocuments200JSONResponse)
	require.True(t, ok)
	require.Len(t, creditDocs, 1)
	require.NotNil(t, creditDocs[0].BlockedReason)
	assert.Contains(t, *creditDocs[0].BlockedReason, "invoice failed in Odoo")
	assert.Nil(t, docs[0].BlockedReason)

	resp2, _ := f.s.RetryBillingDocument(admin, api.RetryBillingDocumentRequestObject{Id: failed.ID})
	retried, ok := resp2.(api.RetryBillingDocument200JSONResponse)
	require.True(t, ok, "%T", resp2)
	assert.Equal(t, api.BillingDocumentOdooState("pending"), retried.OdooState)
	resp2, _ = f.s.RetryBillingDocument(admin, api.RetryBillingDocumentRequestObject{Id: "missing"})
	assert.IsType(t, api.RetryBillingDocument404JSONResponse{}, resp2)

	pdf := func(ctx context.Context, id string) api.GetBillingDocumentPdfResponseObject {
		resp, err := f.s.GetBillingDocumentPdf(ctx, api.GetBillingDocumentPdfRequestObject{Id: id})
		require.NoError(t, err)
		return resp
	}
	for who, ctx := range map[string]context.Context{"admin": admin, "owner": sessionContext(f.user.ID, false)} {
		resp := pdf(ctx, issued.ID)
		w := httptest.NewRecorder()
		require.NoError(t, resp.VisitGetBillingDocumentPdfResponse(w), who)
		assert.Equal(t, 200, w.Code, who)
		assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "F/2026/00001.pdf")
		assert.Contains(t, w.Body.String(), "%PDF")
	}
	assert.IsType(t, api.GetBillingDocumentPdf403JSONResponse{}, pdf(sessionContext(f.other.ID, false), issued.ID))
	assert.IsType(t, api.GetBillingDocumentPdf401JSONResponse{}, pdf(context.Background(), issued.ID))
	assert.IsType(t, api.GetBillingDocumentPdf404JSONResponse{}, pdf(admin, "missing"))
	assert.IsType(t, api.GetBillingDocumentPdf404JSONResponse{}, pdf(sessionContext(f.other.ID, false), failed.ID), "not issued yet")
}

func meGrace(t *testing.T, f *billingFixture, userID string) *api.EntitlementHeaderGrace {
	t.Helper()
	me, err := f.s.GetCurrentUser(sessionContext(userID, false), api.GetCurrentUserRequestObject{})
	require.NoError(t, err)
	meResp, ok := me.(api.GetCurrentUser200JSONResponse)
	require.True(t, ok, "%T", me)
	require.NotNil(t, meResp.Entitlements)
	return meResp.Entitlements.Grace
}

// A grant that ended but has grace days is still returned, flagged.
func TestBillingGrantGraceAPI(t *testing.T) {
	f := newBillingFixture(t)
	admin := sessionContext("admin", true)
	ended := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)

	resp, err := f.s.CreateBillingGrant(admin, api.CreateBillingGrantRequestObject{Body: &api.BillingGrantRequest{
		Subject: "org:" + f.orgID, Features: []string{"academia.access"}, Until: &ended, GraceDays: ptr(2),
	}})
	require.NoError(t, err)
	grants, ok := resp.(api.CreateBillingGrant201JSONResponse)
	require.True(t, ok, "%T %+v", resp, resp)
	require.NotNil(t, grants[0].GraceDays)
	assert.Equal(t, 2, *grants[0].GraceDays)

	list, _ := f.s.ListBillingGrants(admin, api.ListBillingGrantsRequestObject{})
	assert.Equal(t, 2, *list.(api.ListBillingGrants200JSONResponse)[0].GraceDays)

	r, err := f.s.GetEntitlements(admin, api.GetEntitlementsRequestObject{Params: api.GetEntitlementsParams{OrgId: &f.orgID}})
	require.NoError(t, err)
	got := r.(api.GetEntitlements200JSONResponse).Features["academia.access"]
	assert.True(t, got.Grace)
	require.NotNil(t, got.GraceUntil)
	assert.Equal(t, ended.AddDate(0, 0, 2), got.GraceUntil.UTC())
	assert.Equal(t, got.Until.UTC(), got.GraceUntil.UTC(), "until includes the grace")
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"grace":true`)

	grace := meGrace(t, f, f.user.ID)
	require.NotNil(t, grace)
	assert.Equal(t, ended.AddDate(0, 0, 2), (*grace.Orgs)[f.orgID]["academia.access"].UTC())
}
