package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type billingGateway struct {
	server   *httptest.Server
	deps     *deps.Dependencies
	svc      *billing.Service
	stripe   *billing.FakeStripe
	user     *db.User
	token    string
	orgID    string
	received []http.Header
}

func newBillingGateway(t *testing.T) *billingGateway {
	t.Helper()
	bg := &billingGateway{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bg.received = append(bg.received, r.Header.Clone())
	}))
	t.Cleanup(backend.Close)

	d := deps.NewTestWithName(t.Name())
	bg.deps = d
	bg.stripe = billing.NewFakeStripe()
	bg.svc = billing.NewService(billing.Config{Repo: d.BillingRepo, Orgs: d.OrganizationRepo, Stripe: bg.stripe, Odoo: billing.NewFakeOdoo(), ServerURL: "https://gw.test"})
	d.Billing = bg.svc

	bg.user = &db.User{Username: "ana", Email: "ana@example.com", Name: "Ana"}
	require.NoError(t, d.UserRepo.CreateUser(bg.user))
	bg.token = "billing-test-token"
	require.NoError(t, d.SessionRepo.CreateSession(bg.token, &db.Session{
		Token: bg.token, UserID: bg.user.ID, Username: "ana", Email: bg.user.Email, IsAuthenticated: true,
		ValidUntil: time.Now().Add(time.Hour), Provider: "basic",
	}))
	org := &db.Organization{Name: "Escuela"}
	require.NoError(t, d.OrganizationRepo.Create(org))
	require.NoError(t, d.OrganizationRepo.SetMember(org.ID, bg.user.ID, "owner"))
	bg.orgID = org.ID

	gw, err := NewGatewayWithDependencies(&config.GatewayConfig{
		Server:                  config.ServerConfig{Host: "localhost", URL: "https://gw.test"},
		AuthenticationProviders: config.AuthenticationProviders{Basic: config.BasicAuthenticationConfig{Enabled: true}},
		Routes:                  []config.RouteConfig{{Name: "app", From: "/app/*", To: []string{backend.URL}, Authenticated: true}},
	}, nil, d)
	require.NoError(t, err)
	bg.server = httptest.NewServer(gw.Server.Handler)
	t.Cleanup(bg.server.Close)
	return bg
}

func (bg *billingGateway) do(t *testing.T, method, path string, form url.Values, loggedIn bool) *http.Response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, bg.server.URL+path, body)
	require.NoError(t, err)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if loggedIn {
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: bg.token})
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func (bg *billingGateway) plan(t *testing.T, subject string) {
	t.Helper()
	_, err := bg.svc.CreatePlan(context.Background(), billing.PlanInput{
		Key: "academia-monthly", Name: "Academia mensual", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 2900, Features: []string{"academia.access"}, Subject: subject, TrialDays: 14, GraceDays: 7, Group: "academia", Active: true,
	})
	require.NoError(t, err)
}

func TestPayPage(t *testing.T) {
	bg := newBillingGateway(t)
	bg.plan(t, db.PlanSubjectOrg)

	resp := bg.do(t, http.MethodGet, "/_/pay/academia-monthly?org="+bg.orgID+"&redirect=/settings", nil, false)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/_/login?redirect="+url.QueryEscape("/_/pay/academia-monthly?org="+bg.orgID+"&redirect=/settings"), resp.Header.Get("Location"))

	resp = bg.do(t, http.MethodGet, "/_/pay/academia-monthly?org="+bg.orgID+"&redirect=/settings", nil, true)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := readBody(t, resp)
	assert.Contains(t, page, "Academia mensual")
	assert.Contains(t, page, "29,00 € / mes")
	assert.Contains(t, page, "Escuela")
	assert.Contains(t, page, "Prueba gratuita de 14 días")
	assert.NotContains(t, page, "Myself", "an organization plan")

	other := &db.Organization{Name: "Otra"}
	require.NoError(t, bg.deps.OrganizationRepo.Create(other))
	resp = bg.do(t, http.MethodGet, "/_/pay/academia-monthly?org="+other.ID, nil, true)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp = bg.do(t, http.MethodGet, "/_/pay/missing", nil, true)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPayFlow(t *testing.T) {
	bg := newBillingGateway(t)
	bg.plan(t, db.PlanSubjectOrg)
	csrf := accounts.CSRFToken(bg.token)

	resp := bg.do(t, http.MethodPost, "/_/pay/academia-monthly", url.Values{"org": {bg.orgID}, "redirect": {"/settings"}}, true)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "no CSRF token")
	assert.Empty(t, bg.stripe.Sessions)

	resp = bg.do(t, http.MethodPost, "/_/pay/academia-monthly", url.Values{"csrf": {csrf}, "org": {bg.orgID}, "redirect": {"/settings"}}, true)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, readBody(t, resp))
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "https://checkout.stripe.test/"))
	require.Len(t, bg.stripe.Sessions, 1)
	checkoutID := bg.stripe.Sessions[0].CheckoutID

	done := bg.do(t, http.MethodGet, "/_/pay/done?checkout="+checkoutID, nil, true)
	require.Equal(t, http.StatusOK, done.StatusCode)
	assert.Contains(t, readBody(t, done), `http-equiv="refresh"`, "waits for the webhook")

	// Stripe's webhook (the fake accepts JSON events with its signature).
	trialEnd := time.Now().Add(14 * 24 * time.Hour)
	for _, ev := range []billing.Event{
		{ID: "evt_1", Type: billing.EventSubscriptionCreated, Created: time.Now(), Subscription: &billing.StripeSubscription{
			ID: "sub_1", Status: "trialing", CurrentPeriodEnd: trialEnd, TrialEnd: &trialEnd,
			Metadata: map[string]string{"tg_subject": "org:" + bg.orgID, "tg_plan": "academia-monthly", "tg_checkout": checkoutID},
		}},
		{ID: "evt_2", Type: billing.EventCheckoutCompleted, Created: time.Now(), Checkout: &billing.CheckoutEvent{CheckoutID: checkoutID, Mode: "subscription", Paid: true, SubscriptionID: "sub_1", CustomerID: "cus_1"}},
	} {
		payload, err := json.Marshal(ev)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, bg.server.URL+"/_/billing/stripe/webhook", strings.NewReader(string(payload)))
		require.NoError(t, err)
		req.Header.Set("Stripe-Signature", billing.FakeWebhookSignature)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
	req, err := http.NewRequest(http.MethodPost, bg.server.URL+"/_/billing/stripe/webhook", strings.NewReader("{}"))
	require.NoError(t, err)
	req.Header.Set("Stripe-Signature", "forged")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	done = bg.do(t, http.MethodGet, "/_/pay/done?checkout="+checkoutID, nil, true)
	page := readBody(t, done)
	assert.Contains(t, page, "Tu pago está confirmado")
	assert.Contains(t, page, `href="/settings"`)

	// The app sees the feature on the next proxied request; client copies are dropped.
	req, err = http.NewRequest(http.MethodGet, bg.server.URL+"/app/x", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: bg.token})
	req.Header.Set(billing.HeaderName, `{"user":{"forged.feature":null}}`)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Len(t, bg.received, 1)
	var header billing.Header
	require.NoError(t, json.Unmarshal([]byte(bg.received[0].Get(billing.HeaderName)), &header))
	assert.Empty(t, header.User)
	assert.Contains(t, header.Orgs[bg.orgID], "academia.access")

	// Paying again: the pay page offers the portal.
	page = readBody(t, bg.do(t, http.MethodGet, "/_/pay/academia-monthly?org="+bg.orgID, nil, true))
	assert.Contains(t, page, "Gestionar suscripción")
	resp = bg.do(t, http.MethodPost, "/_/pay/academia-monthly", url.Values{"csrf": {csrf}, "org": {bg.orgID}}, true)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	// The account page lists it, with Manage → the Stripe portal.
	page = readBody(t, bg.do(t, http.MethodGet, "/_/account", nil, true))
	assert.Contains(t, page, "<h2>Suscripciones</h2>")
	assert.Contains(t, page, "<h2>Gestionar suscripción</h2>")
	assert.Contains(t, page, "<h2>Facturas</h2>")
	assert.Contains(t, page, "Academia mensual")
	assert.Contains(t, page, "Prueba hasta el")
	resp = bg.do(t, http.MethodPost, "/_/account/billing/portal", url.Values{"csrf": {csrf}, "subject": {"org:" + bg.orgID}}, true)
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "https://billing.stripe.test/p/cus_1", resp.Header.Get("Location"))
}

func TestBillingRoutesOffWithoutBilling(t *testing.T) {
	d := deps.NewTestWithName(t.Name())
	gw, err := NewGatewayWithDependencies(&config.GatewayConfig{Server: config.ServerConfig{Host: "localhost"}}, nil, d)
	require.NoError(t, err)
	server := httptest.NewServer(gw.Server.Handler)
	defer server.Close()
	resp, err := http.Post(server.URL+"/_/billing/stripe/webhook", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

func TestDisplayFeatures(t *testing.T) {
	assert.Equal(t, []string{"a.b"}, displayFeatures([]string{"all", "a.b"}))
}

func TestFormatEUR(t *testing.T) {
	assert.Equal(t, "29,00 €", FormatEUR(2900))
	assert.Equal(t, "0,05 €", FormatEUR(5))
	assert.Equal(t, "-10,50 €", FormatEUR(-1050))
}
