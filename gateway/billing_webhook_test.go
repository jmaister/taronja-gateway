package gateway

import (
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
