//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paymentStatesSetup starts a gateway with Odoo and creates a payer and a plan
// for them; it returns the admin, the payer and the payer's user id.
func paymentStatesSetup(t *testing.T, plan map[string]any) (*testGateway, *client, *client, string) {
	t.Helper()
	requireMailpit(t)
	gateway.OutboxInterval = time.Second
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(backend.Close)
	gw := startGateway(t, billingConfig(t, backend.URL))
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	user := admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": "payer", "email": uniqueEmail("payer"), "password": "payer-password-1"}, http.StatusCreated)
	admin.sendJSON(http.MethodPost, "/_/api/billing/plans", plan, http.StatusCreated)
	payer := newClient(t, gw.URL)
	payer.login("payer", "payer-password-1")
	return gw, admin, payer, user["id"].(string)
}

func checkoutStatus(t *testing.T, gw *testGateway, id string) string {
	t.Helper()
	var c db.BillingCheckout
	require.NoError(t, gw.Deps.DB.Where("id = ?", id).First(&c).Error)
	return c.Status
}

func purchasesOf(t *testing.T, admin *client, plan string) []map[string]any {
	t.Helper()
	resp, data := admin.do(http.MethodGet, "/_/api/billing/purchases?plan="+plan, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var list []map[string]any
	require.NoError(t, json.Unmarshal(data, &list))
	return list
}

func feature(t *testing.T, c *client, userID, name string) map[string]any {
	t.Helper()
	ent := c.getJSON("/_/api/entitlements?user_id="+userID, http.StatusOK)
	features, _ := ent["features"].(map[string]any)
	f, _ := features[name].(map[string]any)
	return f
}

// A one-time payment that Stripe completes unpaid (a delayed method) is not a
// purchase until its async event says it was paid; a failed one expires.
func TestBilling_AsyncPaymentPending(t *testing.T) {
	gw, admin, payer, userID := paymentStatesSetup(t, map[string]any{
		"key": "curso-sepa", "name": "Curso SEPA", "kind": "one_time", "amountCents": 4900,
		"features": []string{"academia.summer"}, "subject": "user", "accessDays": 60,
	})
	pending := pay(t, gw, payer, "/_/pay/curso-sepa", url.Values{})
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_sepa_1", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_sepa_1","object":"checkout.session",
		"client_reference_id":%q,"mode":"payment","status":"complete","payment_status":"unpaid","customer":"cus_sepa","payment_intent":"pi_sepa","amount_total":4900,"currency":"eur",
		"customer_details":{"name":"Ana Pagadora","email":"ana@example.com","address":{"city":"Valencia","country":"ES"}}}`, pending)))
	assert.Equal(t, db.CheckoutStatusAwaitingPayment, checkoutStatus(t, gw, pending))
	assert.Empty(t, purchasesOf(t, admin, "curso-sepa"), "no purchase until the money arrives")
	assert.Empty(t, feature(t, payer, userID, "academia.summer"))
	page := readPage(t, payer, "/_/account")
	assert.Contains(t, page, "Curso SEPA: tu pago está pendiente de confirmación")

	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_sepa_2", "checkout.session.async_payment_succeeded", fmt.Sprintf(`{"id":"cs_sepa_1","object":"checkout.session",
		"client_reference_id":%q,"mode":"payment","status":"complete","payment_status":"paid","customer":"cus_sepa","payment_intent":"pi_sepa","amount_total":4900,"currency":"eur",
		"customer_details":{"name":"Ana Pagadora","email":"ana@example.com","address":{"city":"Valencia","country":"ES"}}}`, pending)))
	assert.Equal(t, db.CheckoutStatusCompleted, checkoutStatus(t, gw, pending))
	purchases := purchasesOf(t, admin, "curso-sepa")
	require.Len(t, purchases, 1)
	assert.Equal(t, "paid", purchases[0]["status"])
	invoice := waitDocument(t, gw, admin, "pi_sepa")
	assert.NotEmpty(t, invoice["number"])
	assert.NotEmpty(t, feature(t, payer, userID, "academia.summer"))
	assert.NotContains(t, readPage(t, payer, "/_/account"), "pendiente de confirmación")

	failing := pay(t, gw, payer, "/_/pay/curso-sepa", url.Values{})
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_sepa_3", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_sepa_2","object":"checkout.session",
		"client_reference_id":%q,"mode":"payment","status":"complete","payment_status":"unpaid","customer":"cus_sepa","payment_intent":"pi_sepa_fail","amount_total":4900,"currency":"eur",
		"customer_details":{"name":"Ana Pagadora","email":"ana@example.com"}}`, failing)))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_sepa_4", "checkout.session.async_payment_failed", fmt.Sprintf(`{"id":"cs_sepa_2","object":"checkout.session",
		"client_reference_id":%q,"mode":"payment","status":"complete","payment_status":"unpaid","customer":"cus_sepa","payment_intent":"pi_sepa_fail","amount_total":4900,"currency":"eur",
		"customer_details":{"name":"Ana Pagadora","email":"ana@example.com"}}`, failing)))
	assert.Equal(t, db.CheckoutStatusExpired, checkoutStatus(t, gw, failing))
	assert.Len(t, purchasesOf(t, admin, "curso-sepa"), 1, "a failed payment adds no purchase")
	resp, data := admin.do(http.MethodGet, "/_/api/billing/documents", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(data), "pi_sepa_fail", "and no invoice")
}

// Stripe marks a subscription unpaid once its trial is over and the charges
// keep failing: the access ends and the account says why.
func TestBilling_SubscriptionUnpaidAfterTrial(t *testing.T) {
	gw, _, payer, userID := paymentStatesSetup(t, map[string]any{
		"key": "academia-unpaid", "name": "Academia sin pago", "kind": "subscription", "interval": "month",
		"amountCents": 2900, "features": []string{"academia.access"}, "subject": "user", "trialDays": 1, "group": "academia-unpaid",
	})
	checkout := pay(t, gw, payer, "/_/pay/academia-unpaid", url.Values{})
	trialEnd := time.Now().Add(2 * time.Second).Unix()
	sub := func(status string) string {
		return fmt.Sprintf(`{"id":"sub_unpaid","object":"subscription","customer":"cus_unpaid","status":%q,"trial_end":%d,"cancel_at_period_end":false,
			"metadata":{"tg_checkout":%q,"tg_subject":"user:%s","tg_plan":"academia-unpaid"},
			"items":{"object":"list","data":[{"id":"si_u","object":"subscription_item","current_period_end":%d,"price":{"id":"price_unpaid","object":"price"}}]}}`,
			status, trialEnd, checkout, userID, trialEnd)
	}
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_unpaid_1", "customer.subscription.created", sub("trialing")))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_unpaid_2", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_unpaid","object":"checkout.session",
		"client_reference_id":%q,"mode":"subscription","status":"complete","payment_status":"no_payment_required","customer":"cus_unpaid","subscription":"sub_unpaid","amount_total":0,"currency":"eur",
		"customer_details":{"name":"Ana","email":"ana@example.com"}}`, checkout)))
	assert.Equal(t, "trial", feature(t, payer, userID, "academia.access")["status"])

	time.Sleep(3 * time.Second)
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_unpaid_3", "customer.subscription.updated", sub("unpaid")))
	assert.Empty(t, feature(t, payer, userID, "academia.access"), "unpaid after the trial grants nothing")
	page := readPage(t, payer, "/_/account")
	assert.Contains(t, page, "Suscripción no activa (unpaid)")
	assert.Contains(t, page, "Vuelve a suscribirte desde la página de pago del plan.")
}

// A subscription whose first charge fails inside its trial is incomplete: the
// access stays with status trial until trial_until, and the account says so.
func TestBilling_SubscriptionIncompleteInsideTrial(t *testing.T) {
	gw, _, payer, userID := paymentStatesSetup(t, map[string]any{
		"key": "academia-trial", "name": "Academia prueba", "kind": "subscription", "interval": "month",
		"amountCents": 2900, "features": []string{"academia.access"}, "subject": "user", "trialDays": 7, "graceDays": 7, "group": "academia-trial",
	})
	checkout := pay(t, gw, payer, "/_/pay/academia-trial", url.Values{})
	trialEnd := time.Now().Add(7 * 24 * time.Hour).Unix()
	sub := func(status string) string {
		return fmt.Sprintf(`{"id":"sub_inc","object":"subscription","customer":"cus_inc","status":%q,"trial_end":%d,"cancel_at_period_end":false,
			"metadata":{"tg_checkout":%q,"tg_subject":"user:%s","tg_plan":"academia-trial"},
			"items":{"object":"list","data":[{"id":"si_i","object":"subscription_item","current_period_end":%d,"price":{"id":"price_inc","object":"price"}}]}}`,
			status, trialEnd, checkout, userID, trialEnd)
	}
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_inc_1", "customer.subscription.created", sub("trialing")))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_inc_2", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_inc","object":"checkout.session",
		"client_reference_id":%q,"mode":"subscription","status":"complete","payment_status":"no_payment_required","customer":"cus_inc","subscription":"sub_inc","amount_total":0,"currency":"eur",
		"customer_details":{"name":"Ana","email":"ana@example.com"}}`, checkout)))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_inc_3", "customer.subscription.updated", sub("incomplete")))

	f := feature(t, payer, userID, "academia.access")
	assert.Equal(t, "trial", f["status"])
	assert.Equal(t, time.Unix(trialEnd, 0).UTC(), parseTime(t, f["until"]), "access until trial_until")
	assert.Equal(t, false, f["grace"], "no extra grace")
	page := readPage(t, payer, "/_/account")
	assert.Contains(t, page, "pero tu prueba sigue hasta el")
	assert.Contains(t, page, "Revisa tu tarjeta con el botón Gestionar suscripción.")
	assert.Contains(t, page, "En prueba")
}

func parseTime(t *testing.T, v any) time.Time {
	t.Helper()
	s, ok := v.(string)
	require.True(t, ok, "a time, got %v", v)
	parsed, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return parsed.UTC()
}

func readPage(t *testing.T, c *client, path string) string {
	t.Helper()
	resp, page := c.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return string(page)
}
