//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v87/webhook"
)

const webhookSecret = "whsec_e2e"

// odooEnv reads what e2e/odoo/init.sh wrote (make e2e-up).
func odooEnv(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(".odoo/env")
	require.NoError(t, err, "run make e2e-up: it prepares Odoo and writes e2e/.odoo/env")
	defer f.Close()
	env := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			env[k] = v
		}
	}
	return env
}

func billingConfig(t *testing.T, backendURL string) string {
	odoo := odooEnv(t)
	return baseConfig(`  - name: app
    from: /app/*
    to: ` + backendURL + `
    authenticated: true
authenticationProviders:
  basic:
    enabled: true
billing:
  enabled: true
  stripe:
    secretKey: sk_test_123
    webhookSecret: ` + webhookSecret + `
    apiUrl: ` + envOr("E2E_STRIPE_MOCK_URL", "http://localhost:12111") + `
  odoo:
    url: ` + odoo["ODOO_URL"] + `
    database: ` + odoo["ODOO_DB"] + `
    apiKey: ` + odoo["ODOO_API_KEY"] + `
    companyId: ` + odoo["ODOO_COMPANY_ID"] + `
    salesJournalId: ` + odoo["ODOO_SALES_JOURNAL_ID"] + `
    paymentJournalId: ` + odoo["ODOO_PAYMENT_JOURNAL_ID"] + `
    taxId: ` + odoo["ODOO_TAX_ID"] + `
`)
}

// sendEvent posts a Stripe event, signed like Stripe signs it, to the
// gateway's webhook.
func sendEvent(t *testing.T, gwURL, id, typ, object string) int {
	t.Helper()
	payload := fmt.Appendf(nil, `{"id":%q,"object":"event","api_version":"2026-09-30.endive","created":%d,"type":%q,"data":{"object":%s}}`,
		id, time.Now().Unix(), typ, object)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: webhookSecret, Timestamp: time.Now()})
	req, err := http.NewRequest(http.MethodPost, gwURL+"/_/billing/stripe/webhook", strings.NewReader(string(payload)))
	require.NoError(t, err)
	req.Header.Set("Stripe-Signature", signed.Header)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

var csrfPattern = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// pay opens the pay page as c and presses Pay; it returns the gateway
// checkout id.
func pay(t *testing.T, gw *testGateway, c *client, path string, form url.Values) string {
	t.Helper()
	resp, page := c.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(page))
	m := csrfPattern.FindSubmatch(page)
	require.NotNil(t, m, "the pay page has a form")
	form.Set("csrf", string(m[1]))
	noFollow := *c.http
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u, _ := url.Parse(path)
	resp, err := noFollow.PostForm(c.base+u.Path, form)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, string(body))
	assert.NotEmpty(t, resp.Header.Get("Location"), "redirects to Stripe Checkout")

	var checkout db.BillingCheckout
	require.NoError(t, gw.Deps.DB.Order("created_at DESC").First(&checkout).Error)
	return checkout.ID
}

func waitDocument(t *testing.T, gw *testGateway, admin *client, stripeRef string) map[string]any {
	t.Helper()
	var doc map[string]any
	eventually(t, 2*time.Minute, "Odoo issuing "+stripeRef, func() bool {
		resp, data := admin.do(http.MethodGet, "/_/api/billing/documents", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var docs []map[string]any
		require.NoError(t, json.Unmarshal(data, &docs))
		for _, d := range docs {
			if d["stripeRef"] == stripeRef {
				doc = d
				require.NotEqual(t, "failed", d["odooState"], "Odoo error: %v", d["odooError"])
				return d["odooState"] == "done"
			}
		}
		return false
	})
	return doc
}

// The whole flow of doc/billing.md: plan → pay link → Checkout → webhooks →
// entitlement in the header → Odoo invoice with PDF; a one-time purchase →
// refund → credit note in its own series; a failed renewal; activation.
func TestBilling_WholeFlow(t *testing.T) {
	gateway.OutboxInterval = time.Second
	backendSaw := make(chan string, 10)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendSaw <- r.Header.Get("X-Entitlements")
	}))
	defer backend.Close()
	gw := startGateway(t, billingConfig(t, backend.URL))

	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	email := uniqueEmail("payer")
	user := admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": "payer", "email": email, "password": "payer-password-1"}, http.StatusCreated)
	userID := user["id"].(string)
	org := admin.sendJSON(http.MethodPost, "/_/api/organizations", map[string]any{"name": "Escuela E2E", "ownerUserId": userID}, http.StatusCreated)
	orgID := org["id"].(string)

	plan := admin.sendJSON(http.MethodPost, "/_/api/billing/plans", map[string]any{
		"key": "academia-monthly", "name": "Academia mensual", "kind": "subscription", "interval": "month",
		"amountCents": 2900, "features": []string{"academia.access"}, "subject": "org", "trialDays": 14, "group": "academia",
	}, http.StatusCreated)
	assert.Equal(t, gw.URL+"/_/pay/academia-monthly", plan["payLink"])
	priceID := plan["stripePriceId"].(string)

	payer := newClient(t, gw.URL)
	payer.login("payer", "payer-password-1")
	checkoutID := pay(t, gw, payer, "/_/pay/academia-monthly?org="+orgID+"&redirect=/app/settings", url.Values{"org": {orgID}, "redirect": {"/app/settings"}})

	// Stripe: the subscription (trialing), the completed session, then the
	// first charge after the trial.
	now := time.Now()
	trialEnd := now.Add(14 * 24 * time.Hour).Unix()
	periodEnd := now.Add(44 * 24 * time.Hour).Unix()
	subJSON := func(status string, end int64) string {
		return fmt.Sprintf(`{"id":"sub_e2e","object":"subscription","customer":"cus_e2e","status":%q,"trial_end":%d,"cancel_at_period_end":false,
			"metadata":{"tg_checkout":%q,"tg_subject":"org:%s","tg_plan":"academia-monthly"},
			"items":{"object":"list","data":[{"id":"si_1","object":"subscription_item","current_period_end":%d,"price":{"id":%q,"object":"price"}}]}}`,
			status, trialEnd, checkoutID, orgID, end, priceID)
	}
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_1", "customer.subscription.created", subJSON("trialing", trialEnd)))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_2", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_e2e","object":"checkout.session",
		"client_reference_id":%q,"mode":"subscription","payment_status":"no_payment_required","customer":"cus_e2e","subscription":"sub_e2e","amount_total":0,"currency":"eur",
		"customer_details":{"name":"Escuela E2E SL","email":%q,"address":{"line1":"C/ Mayor 1","postal_code":"46001","city":"Valencia","state":"Valencia","country":"ES"},
		"tax_ids":[{"type":"es_cif","value":"B12345674"}]}}`, checkoutID, email)))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_2", "checkout.session.completed", `{}`), "a repeated event is accepted and ignored")
	require.Equal(t, http.StatusBadRequest, func() int {
		req, _ := http.NewRequest(http.MethodPost, gw.URL+"/_/billing/stripe/webhook", strings.NewReader(`{}`))
		req.Header.Set("Stripe-Signature", "t=1,v1=forged")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return resp.StatusCode
	}())

	// The done page, the entitlement in /_/me and in the header.
	resp, page := payer.do(http.MethodGet, "/_/pay/done?checkout="+checkoutID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(page), "/app/settings")
	me := payer.getJSON("/_/me", http.StatusOK)
	orgs := me["entitlements"].(map[string]any)["orgs"].(map[string]any)
	assert.Contains(t, orgs[orgID], "academia.access")
	req, err := http.NewRequest(http.MethodGet, gw.URL+"/app/settings", nil)
	require.NoError(t, err)
	req.Header.Set("X-Entitlements", `{"user":{"forged":null}}`)
	resp, err = payer.http.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	header := <-backendSaw
	assert.Contains(t, header, `"academia.access"`)
	assert.NotContains(t, header, "forged")
	ent := payer.getJSON("/_/api/entitlements?org_id="+orgID, http.StatusOK)
	assert.Equal(t, "trial", ent["features"].(map[string]any)["academia.access"].(map[string]any)["status"])

	// The first charge: invoice in Odoo, PDF for the payer.
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_3", "customer.subscription.updated", subJSON("active", periodEnd)))
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_4", "invoice.paid", fmt.Sprintf(`{"id":"in_e2e_1","object":"invoice","customer":"cus_e2e",
		"currency":"eur","amount_paid":2900,"amount_due":2900,"attempt_count":1,"created":%d,"status_transitions":{"paid_at":%d},
		"parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_e2e"}},
		"lines":{"object":"list","data":[{"id":"il_1","object":"line_item","description":"1 × Academia","amount":2900,
		"pricing":{"type":"price_details","price_details":{"price":%q,"product":"prod"}},"period":{"start":%d,"end":%d}}]}}`,
		trialEnd, trialEnd, priceID, trialEnd, periodEnd)))
	invoice := waitDocument(t, gw, admin, "in_e2e_1")
	assert.NotEmpty(t, invoice["number"])
	resp, pdf := payer.do(http.MethodGet, "/_/api/billing/documents/"+invoice["id"].(string)+"/pdf", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(pdf))
	assert.True(t, strings.HasPrefix(string(pdf), "%PDF"))
	other := admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": "other", "email": uniqueEmail("other"), "password": "other-password-1"}, http.StatusCreated)
	require.NotEmpty(t, other["id"])
	stranger := newClient(t, gw.URL)
	stranger.login("other", "other-password-1")
	resp, _ = stranger.do(http.MethodGet, "/_/api/billing/documents/"+invoice["id"].(string)+"/pdf", nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// A failed renewal: past due, still active during the grace.
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_5", "invoice.payment_failed", `{"id":"in_e2e_2","object":"invoice","customer":"cus_e2e",
		"currency":"eur","amount_paid":0,"amount_due":2900,"attempt_count":1,"created":1,"parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_e2e"}}}`))
	ent = payer.getJSON("/_/api/entitlements?org_id="+orgID, http.StatusOK)
	assert.Equal(t, "past_due", ent["features"].(map[string]any)["academia.access"].(map[string]any)["status"])

	// One-time purchase, refunded in two steps: two credit notes.
	admin.sendJSON(http.MethodPost, "/_/api/billing/plans", map[string]any{
		"key": "curso-verano", "name": "Curso de verano", "kind": "one_time", "amountCents": 4900,
		"features": []string{"academia.summer"}, "subject": "user", "accessDays": 60,
	}, http.StatusCreated)
	checkoutID = pay(t, gw, payer, "/_/pay/curso-verano", url.Values{})
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_6", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_e2e_2","object":"checkout.session",
		"client_reference_id":%q,"mode":"payment","payment_status":"paid","customer":"cus_e2e_u","payment_intent":"pi_e2e","amount_total":4900,"currency":"eur",
		"customer_details":{"name":"Ana Pagadora","email":%q,"address":{"city":"Valencia","country":"ES"}}}`, checkoutID, email)))
	purchaseInvoice := waitDocument(t, gw, admin, "pi_e2e")
	resp, data := admin.do(http.MethodGet, "/_/api/billing/purchases?plan=curso-verano", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var list []map[string]any
	require.NoError(t, json.Unmarshal(data, &list))
	require.Len(t, list, 1)
	purchaseID := list[0]["id"].(string)

	admin.sendJSON(http.MethodPost, "/_/api/billing/purchases/"+purchaseID+"/refund", map[string]any{"amountCents": 1000}, http.StatusOK)
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_7", "charge.refunded",
		`{"id":"ch_e2e","object":"charge","payment_intent":"pi_e2e","amount_refunded":1000,"currency":"eur","refunds":{"object":"list","data":[{"id":"re_e2e_1","object":"refund"}]}}`))
	credit1 := waitDocument(t, gw, admin, "re_e2e_1")
	admin.sendJSON(http.MethodPost, "/_/api/billing/purchases/"+purchaseID+"/refund", map[string]any{}, http.StatusOK)
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_8", "charge.refunded",
		`{"id":"ch_e2e","object":"charge","payment_intent":"pi_e2e","amount_refunded":4900,"currency":"eur","refunds":{"object":"list","data":[{"id":"re_e2e_2","object":"refund"}]}}`))
	credit2 := waitDocument(t, gw, admin, "re_e2e_2")
	assert.Equal(t, float64(1000), credit1["amountCents"])
	assert.Equal(t, float64(3900), credit2["amountCents"])
	series := func(number any) string { s := number.(string); return s[:strings.LastIndex(s, "/")] }
	assert.NotEqual(t, series(purchaseInvoice["number"]), series(credit1["number"]), "credit notes have their own series")
	ent = payer.getJSON("/_/api/entitlements?user_id="+userID, http.StatusOK)
	assert.NotContains(t, ent["features"], "academia.summer", "a refunded purchase grants nothing")

	// Activation of a plan that starts on activation (megabox).
	admin.sendJSON(http.MethodPost, "/_/api/billing/plans", map[string]any{
		"key": "megabox-familia", "name": "Megabox Familia", "kind": "subscription", "interval": "month", "amountCents": 990,
		"features": []string{"megabox.protection"}, "subject": "user", "starts": "on_activation", "setupFeeCents": 4900,
		"setupFeeName": "Caja Megabox", "collectShipping": true,
	}, http.StatusCreated)
	checkoutID = pay(t, gw, payer, "/_/pay/megabox-familia", url.Values{})
	require.Equal(t, http.StatusOK, sendEvent(t, gw.URL, "evt_e2e_9", "checkout.session.completed", fmt.Sprintf(`{"id":"cs_e2e_3","object":"checkout.session",
		"client_reference_id":%q,"mode":"payment","payment_status":"paid","customer":"cus_e2e_u","payment_intent":"pi_e2e_box","amount_total":5890,"currency":"eur",
		"customer_details":{"name":"Ana Pagadora","email":%q,"phone":"+34600000000","address":{"city":"Valencia","country":"ES"}},
		"collected_information":{"shipping_details":{"name":"Ana","address":{"line1":"C/ Paz 2","postal_code":"46002","city":"Valencia","country":"ES"}}}}`, checkoutID, email)))
	resp, data = admin.do(http.MethodGet, "/_/api/billing/subscriptions?status=awaiting_activation", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var subs []map[string]any
	require.NoError(t, json.Unmarshal(data, &subs))
	require.Len(t, subs, 1)
	activated := admin.sendJSON(http.MethodPost, "/_/api/billing/subscriptions/"+subs[0]["id"].(string)+"/activate", nil, http.StatusOK)
	assert.Equal(t, "trialing", activated["status"])
	again := admin.sendJSON(http.MethodPost, "/_/api/billing/subscriptions/"+subs[0]["id"].(string)+"/activate", nil, http.StatusOK)
	assert.Equal(t, activated["stripeSubscriptionId"], again["stripeSubscriptionId"])
	boxInvoice := waitDocument(t, gw, admin, "pi_e2e_box")
	assert.Len(t, boxInvoice["lines"], 2, "the box and the first month")
}

func uniqueEmail(name string) string {
	return fmt.Sprintf("%s-%d@example.com", name, time.Now().UnixNano())
}
