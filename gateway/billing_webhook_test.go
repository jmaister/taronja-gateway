package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (bg *billingGateway) webhook(t *testing.T, signature, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, bg.server.URL+"/_/billing/stripe/webhook", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Stripe-Signature", signature)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestStripeWebhookStatusCodes(t *testing.T) {
	bg := newBillingGateway(t)

	assert.Equal(t, http.StatusBadRequest, bg.webhook(t, "forged", "{}").StatusCode, "bad signature: Stripe must not retry")

	big := `{"id":"evt_big","type":"customer.created","pad":"` + strings.Repeat("x", billing.MaxWebhookBody) + `"}`
	assert.Equal(t, http.StatusRequestEntityTooLarge, bg.webhook(t, billing.FakeWebhookSignature, big).StatusCode)

	bg.stripe.Err = errors.New("stripe down")
	retry, err := json.Marshal(billing.Event{ID: "evt_retry", Type: billing.EventInvoicePaid, Created: time.Now(),
		Invoice: &billing.InvoiceEvent{InvoiceID: "in_1", SubscriptionID: "sub_unknown", AmountCents: 2900}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, bg.webhook(t, billing.FakeWebhookSignature, string(retry)).StatusCode, "Stripe retries what the gateway could not apply yet")
	stuck, err := bg.deps.BillingRepo.StuckEvents()
	require.NoError(t, err)
	assert.Len(t, stuck, 1)

	bg.stripe.Err = nil
	assert.Equal(t, http.StatusOK, bg.webhook(t, billing.FakeWebhookSignature, `{"id":"evt_ok","type":"customer.created"}`).StatusCode)
}

func TestPortalRoute(t *testing.T) {
	bg := newBillingGateway(t)
	repo := bg.deps.BillingRepo
	require.NoError(t, repo.SaveCustomer(&db.BillingCustomer{Subject: "user:" + bg.user.ID, StripeCustomerID: "cus_me"}))
	require.NoError(t, repo.SaveCustomer(&db.BillingCustomer{Subject: "user:someone-else", StripeCustomerID: "cus_other"}))
	require.NoError(t, repo.SaveCustomer(&db.BillingCustomer{Subject: "org:" + bg.orgID, StripeCustomerID: "cus_org"}))
	stranger := &db.Organization{Name: "Otra"}
	require.NoError(t, bg.deps.OrganizationRepo.Create(stranger))
	require.NoError(t, repo.SaveCustomer(&db.BillingCustomer{Subject: "org:" + stranger.ID, StripeCustomerID: "cus_stranger"}))
	csrf := accounts.CSRFToken(bg.token)
	post := func(subject, token string, loggedIn bool) *http.Response {
		return bg.do(t, http.MethodPost, "/_/account/billing/portal", url.Values{"csrf": {token}, "subject": {subject}}, loggedIn)
	}

	for _, subject := range []string{"user:" + bg.user.ID, "org:" + bg.orgID} {
		resp := post(subject, csrf, true)
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode, subject)
		assert.Contains(t, resp.Header.Get("Location"), "https://billing.stripe.test/p/cus_", subject)
	}

	assert.Equal(t, http.StatusForbidden, post("user:"+bg.user.ID, "", true).StatusCode, "no CSRF token")
	assert.Equal(t, http.StatusForbidden, post("user:"+bg.user.ID, "forged", true).StatusCode, "wrong CSRF token")
	assert.Equal(t, http.StatusFound, post("user:"+bg.user.ID, csrf, false).StatusCode, "not logged in: to the login page")

	for _, subject := range []string{"user:someone-else", "org:" + stranger.ID, "nonsense"} {
		resp := post(subject, csrf, true)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, subject)
		assert.NotContains(t, resp.Header.Get("Location"), "stripe.test", subject)
		assert.NotContains(t, readBody(t, resp), "cus_", subject)
	}
	for _, p := range bg.stripe.PortalCustomers {
		assert.Contains(t, []string{"cus_me", "cus_org"}, p, "Stripe was never asked for a portal of somebody else")
	}
}

func (bg *billingGateway) trialSubscription(t *testing.T) *db.BillingSubscription {
	t.Helper()
	ctx := context.Background()
	_, err := bg.svc.CreatePlan(ctx, billing.PlanInput{
		Key: "academia-personal", Name: "Academia personal", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 1000, Features: []string{"academia.access"}, Subject: db.PlanSubjectUser, TrialDays: 7, GraceDays: 7,
		Group: "academia-personal", Starts: db.PlanStartsOnPayment, Active: true,
	})
	require.NoError(t, err)
	require.NoError(t, bg.deps.BillingRepo.SaveCustomer(&db.BillingCustomer{Subject: "user:" + bg.user.ID, StripeCustomerID: "cus_me"}))
	checkout, _, err := bg.svc.StartCheckout(ctx, billing.CheckoutRequest{PlanKey: "academia-personal", Payer: billing.Payer{UserID: bg.user.ID, Name: "Ana", Email: "ana@example.com"}})
	require.NoError(t, err)
	end := time.Now().AddDate(0, 0, 7)
	bg.stripe.Subscriptions["sub_trial"] = &billing.StripeSubscription{ID: "sub_trial", Status: "trialing", CurrentPeriodEnd: end, TrialEnd: &end}
	require.NoError(t, bg.svc.HandleEvent(ctx, billing.Event{ID: "evt_trial", Type: billing.EventSubscriptionCreated, Created: time.Now(), Subscription: &billing.StripeSubscription{
		ID: "sub_trial", Status: "trialing", CurrentPeriodEnd: end, TrialEnd: &end,
		Metadata: map[string]string{"tg_subject": "user:" + bg.user.ID, "tg_plan": "academia-personal", "tg_checkout": checkout.ID},
	}}))
	require.NoError(t, bg.svc.HandleEvent(ctx, billing.Event{ID: "evt_checkout", Type: billing.EventCheckoutCompleted, Created: time.Now(), Checkout: &billing.CheckoutEvent{
		CheckoutID: checkout.ID, Mode: "subscription", Paid: true, SubscriptionID: "sub_trial", CustomerID: "cus_me",
	}}))
	sub, err := bg.deps.BillingRepo.GetSubscriptionByStripeID("sub_trial")
	require.NoError(t, err)
	return sub
}

func TestStartNowRoute(t *testing.T) {
	bg := newBillingGateway(t)
	ctx := context.Background()
	sub := bg.trialSubscription(t)
	csrf := accounts.CSRFToken(bg.token)
	start := func(token string) *http.Response {
		return bg.do(t, http.MethodPost, "/_/account/billing/start", url.Values{"csrf": {token}, "subscription": {sub.ID}}, true)
	}

	page := readBody(t, bg.do(t, http.MethodGet, "/_/account", nil, true))
	assert.Contains(t, page, "Empezar ya")
	assert.Contains(t, page, "Se cobra hoy 10,00 € con la tarjeta guardada y termina la prueba.")

	assert.Equal(t, http.StatusForbidden, start("").StatusCode, "no CSRF token")
	assert.Empty(t, bg.stripe.Trials)

	resp := start(csrf)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, readBody(t, resp), "Cobramos la primera factura")
	require.Len(t, bg.stripe.Trials, 1)
	assert.Equal(t, "tg-start-"+sub.ID, bg.stripe.Trials[0].IdempotencyKey)

	require.NoError(t, bg.svc.HandleEvent(ctx, billing.Event{ID: "evt_active", Type: billing.EventSubscriptionUpdated, Created: time.Now(), Subscription: &billing.StripeSubscription{
		ID: "sub_trial", Status: "active", CurrentPeriodEnd: end(),
		Metadata: map[string]string{"tg_subject": "user:" + bg.user.ID, "tg_plan": "academia-personal"},
	}}))
	assert.Equal(t, http.StatusBadRequest, start(csrf).StatusCode, "already active")
	assert.Len(t, bg.stripe.Trials, 1)
	assert.NotContains(t, readBody(t, bg.do(t, http.MethodGet, "/_/account", nil, true)), "Empezar ya")
}

func end() time.Time { return time.Now().AddDate(0, 0, 7) }

func (bg *billingGateway) subscriptionEvent(t *testing.T, status string, trialEnd *time.Time) {
	t.Helper()
	ev := billing.Event{ID: "evt_" + status, Type: billing.EventSubscriptionUpdated, Created: time.Now().Add(time.Minute), Subscription: &billing.StripeSubscription{
		ID: "sub_trial", CustomerID: "cus_me", Status: status, CurrentPeriodEnd: end(), TrialEnd: trialEnd,
		Metadata: map[string]string{"tg_subject": "user:" + bg.user.ID, "tg_plan": "academia-personal"},
	}}
	require.NoError(t, bg.svc.HandleEvent(context.Background(), ev))
}

func TestAccountPaymentProblems(t *testing.T) {
	const pastDue = "el cobro no se pudo completar. Revisa tu tarjeta con el botón Gestionar suscripción."
	cases := []struct {
		name      string
		status    string
		trialOver bool
		want      []string
		notWant   []string
	}{
		{name: "past due", status: "past_due", want: []string{"Academia personal: " + pastDue, "Pago fallido"}},
		{name: "incomplete during the trial", status: "incomplete", want: []string{"pero tu prueba sigue hasta el", "Revisa tu tarjeta", "En prueba"}},
		{name: "incomplete after the trial", status: "incomplete", trialOver: true, want: []string{"Suscripción no activa (incomplete)", "Stripe no ha podido confirmar el pago"}},
		{name: "unpaid", status: "unpaid", want: []string{"Suscripción no activa (unpaid)", "Vuelve a suscribirte desde la página de pago del plan."}},
		{name: "incomplete expired", status: "incomplete_expired", want: []string{"Suscripción no activa (incomplete_expired)"}},
		{name: "paused", status: "paused", want: []string{"Suscripción no activa (paused)"}},
		{name: "active", status: "active", want: []string{"Activa"}, notWant: []string{"Revisa tu tarjeta", "Suscripción no activa"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bg := newBillingGateway(t)
			sub := bg.trialSubscription(t)
			if tc.trialOver {
				past := time.Now().AddDate(0, 0, -1)
				sub.TrialUntil = &past
				require.NoError(t, bg.deps.BillingRepo.UpdateSubscription(sub))
			}
			bg.subscriptionEvent(t, tc.status, nil)
			page := readBody(t, bg.do(t, http.MethodGet, "/_/account", nil, true))
			for _, w := range tc.want {
				assert.True(t, strings.Contains(page, w), "account page lacks %q", w)
			}
			for _, w := range tc.notWant {
				assert.False(t, strings.Contains(page, w), "account page has %q", w)
			}
		})
	}
}

func TestAccountShowsCheckoutAwaitingPayment(t *testing.T) {
	bg := newBillingGateway(t)
	ctx := context.Background()
	_, err := bg.svc.CreatePlan(ctx, billing.PlanInput{
		Key: "academia-sepa", Name: "Academia SEPA", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth,
		AmountCents: 1000, Features: []string{"academia.access"}, Subject: db.PlanSubjectUser, GraceDays: 7,
		Group: "academia-sepa", Starts: db.PlanStartsOnPayment, Active: true,
	})
	require.NoError(t, err)
	checkout, _, err := bg.svc.StartCheckout(ctx, billing.CheckoutRequest{PlanKey: "academia-sepa", Payer: billing.Payer{UserID: bg.user.ID, Name: "Ana", Email: "ana@example.com"}})
	require.NoError(t, err)
	require.NoError(t, bg.svc.HandleEvent(ctx, billing.Event{ID: "evt_unpaid", Type: billing.EventCheckoutCompleted, Created: time.Now(), Checkout: &billing.CheckoutEvent{
		CheckoutID: checkout.ID, Mode: "subscription", Paid: false, SubscriptionID: "sub_sepa",
	}}))
	page := readBody(t, bg.do(t, http.MethodGet, "/_/account", nil, true))
	assert.Contains(t, page, "Academia SEPA: tu pago está pendiente de confirmación")
}
