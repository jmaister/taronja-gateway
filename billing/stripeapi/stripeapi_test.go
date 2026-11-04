package stripeapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStripe stands in for api.stripe.com: it records each request and
// answers with the JSON registered for "METHOD /path".
type fakeStripe struct {
	mu        sync.Mutex
	responses map[string]string
	requests  []recordedRequest
}

type recordedRequest struct {
	Method, Path   string
	Form           url.Values
	IdempotencyKey string
	Version        string
}

func newFakeStripe(t *testing.T, responses map[string]string) (*fakeStripe, *Client) {
	t.Helper()
	f := &fakeStripe{responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if r.Method == http.MethodGet {
			form = r.URL.Query()
		}
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Form: form,
			IdempotencyKey: r.Header.Get("Idempotency-Key"), Version: r.Header.Get("Stripe-Version"),
		})
		resp, ok := f.responses[r.Method+" "+r.URL.Path]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"no fake for ` + r.Method + " " + r.URL.Path + `"}}`))
			return
		}
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return f, New("sk_test_123", "whsec_test", WithAPIURL(srv.URL))
}

func (f *fakeStripe) last(t *testing.T) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.requests)
	return f.requests[len(f.requests)-1]
}

func TestEnsureProduct(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{
		"POST /v1/products":          `{"id":"prod_1","object":"product"}`,
		"POST /v1/products/prod_old": `{"id":"prod_old","object":"product"}`,
	})
	id, err := c.EnsureProduct(context.Background(), "", "Academia mensual", "academia-monthly", "ACADEMIA TARONJA")
	require.NoError(t, err)
	assert.Equal(t, "prod_1", id)
	req := f.last(t)
	assert.Equal(t, "Academia mensual", req.Form.Get("name"))
	assert.Equal(t, "academia-monthly", req.Form.Get("metadata[tg_plan]"))
	assert.Equal(t, "2026-09-30.endive", req.Version)
	assert.Equal(t, "ACADEMIA TARONJA", req.Form.Get("statement_descriptor"))

	id, err = c.EnsureProduct(context.Background(), "prod_old", "Renamed", "academia-monthly", "")
	require.NoError(t, err)
	assert.Equal(t, "prod_old", id)
	assert.Equal(t, "Renamed", f.last(t).Form.Get("name"))
	_, has := f.last(t).Form["statement_descriptor"]
	assert.False(t, has, "an empty descriptor leaves the product's as it is")
}

func TestCreatePrice(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/prices": `{"id":"price_1","object":"price"}`})
	id, err := c.CreatePrice(context.Background(), billing.StripePriceParams{
		ProductID: "prod_1", LookupKey: "tg-academia-monthly", AmountCents: 2900, Currency: "eur", Interval: "month", Nickname: "29 €/mes",
	})
	require.NoError(t, err)
	assert.Equal(t, "price_1", id)
	form := f.last(t).Form
	assert.Equal(t, "prod_1", form.Get("product"))
	assert.Equal(t, "2900", form.Get("unit_amount"))
	assert.Equal(t, "eur", form.Get("currency"))
	assert.Equal(t, "inclusive", form.Get("tax_behavior"))
	assert.Equal(t, "tg-academia-monthly", form.Get("lookup_key"))
	assert.Equal(t, "true", form.Get("transfer_lookup_key"))
	assert.Equal(t, "month", form.Get("recurring[interval]"))

	_, err = c.CreatePrice(context.Background(), billing.StripePriceParams{ProductID: "prod_1", AmountCents: 500, Currency: "eur"})
	require.NoError(t, err)
	form = f.last(t).Form
	assert.Empty(t, form.Get("recurring[interval]"))
	assert.Empty(t, form.Get("lookup_key"))
}

func TestArchivePrice(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/prices/price_1": `{"id":"price_1","object":"price"}`})
	require.NoError(t, c.ArchivePrice(context.Background(), "price_1"))
	assert.Equal(t, "false", f.last(t).Form.Get("active"))
}

func TestEnsureCustomer(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{
		"POST /v1/customers":       `{"id":"cus_1","object":"customer"}`,
		"POST /v1/customers/cus_1": `{"id":"cus_1","object":"customer"}`,
	})
	id, err := c.EnsureCustomer(context.Background(), "", "org:abc", "Escuela", "a@example.com")
	require.NoError(t, err)
	assert.Equal(t, "cus_1", id)
	form := f.last(t).Form
	assert.Equal(t, "org:abc", form.Get("metadata[tg_subject]"))
	assert.Equal(t, "a@example.com", form.Get("email"))

	_, err = c.EnsureCustomer(context.Background(), "cus_1", "org:abc", "Escuela 2", "b@example.com")
	require.NoError(t, err)
	req := f.last(t)
	assert.Equal(t, "/v1/customers/cus_1", req.Path)
	assert.Equal(t, "Escuela 2", req.Form.Get("name"))
}

func TestCreateCheckoutSession_Subscription(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/checkout/sessions": `{"id":"cs_1","object":"checkout.session","url":"https://checkout.stripe.com/c/cs_1"}`})
	s, err := c.CreateCheckoutSession(context.Background(), billing.CheckoutParams{
		CheckoutID: "chk_1", CustomerID: "cus_1", Mode: "subscription",
		Lines:     []billing.CheckoutLine{{PriceID: "price_1", Quantity: 1}, {PriceID: "price_setup", Quantity: 1}},
		TrialDays: 14, SuccessURL: "https://gw/ok", CancelURL: "https://gw/ko",
		Metadata: map[string]string{"tg_plan": "academia-monthly"},
	})
	require.NoError(t, err)
	assert.Equal(t, billing.CheckoutSession{ID: "cs_1", URL: "https://checkout.stripe.com/c/cs_1"}, s)
	req := f.last(t)
	form := req.Form
	assert.Equal(t, "tg-checkout-chk_1", req.IdempotencyKey)
	assert.Equal(t, "subscription", form.Get("mode"))
	assert.Equal(t, "cus_1", form.Get("customer"))
	assert.Equal(t, "chk_1", form.Get("client_reference_id"))
	assert.Equal(t, "price_1", form.Get("line_items[0][price]"))
	assert.Equal(t, "price_setup", form.Get("line_items[1][price]"))
	assert.Equal(t, "1", form.Get("line_items[0][quantity]"))
	assert.Equal(t, "14", form.Get("subscription_data[trial_period_days]"))
	assert.Equal(t, "academia-monthly", form.Get("subscription_data[metadata][tg_plan]"))
	assert.Equal(t, "chk_1", form.Get("metadata[tg_checkout]"))
	assert.Equal(t, "required", form.Get("billing_address_collection"))
	assert.Equal(t, "true", form.Get("tax_id_collection[enabled]"))
	assert.Equal(t, "auto", form.Get("customer_update[name]"))
	assert.Equal(t, "auto", form.Get("customer_update[address]"))
	assert.Equal(t, "es", form.Get("locale"))
	assert.Empty(t, form.Get("shipping_address_collection[allowed_countries][0]"))
	assert.Empty(t, form.Get("payment_intent_data[setup_future_usage]"))
}

func TestCreateCheckoutSession_PaymentWithShipping(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/checkout/sessions": `{"id":"cs_2","object":"checkout.session","url":"u"}`})
	_, err := c.CreateCheckoutSession(context.Background(), billing.CheckoutParams{
		CheckoutID: "chk_2", CustomerID: "cus_1", Mode: "payment",
		Lines:    []billing.CheckoutLine{{PriceID: "price_box", Quantity: 1}},
		SaveCard: true, CollectShipping: true, SuccessURL: "s", CancelURL: "c",
	})
	require.NoError(t, err)
	form := f.last(t).Form
	assert.Equal(t, "payment", form.Get("mode"))
	assert.Equal(t, "off_session", form.Get("payment_intent_data[setup_future_usage]"))
	assert.Equal(t, "chk_2", form.Get("payment_intent_data[metadata][tg_checkout]"))
	assert.Equal(t, "ES", form.Get("shipping_address_collection[allowed_countries][0]"))
	assert.Equal(t, "auto", form.Get("customer_update[shipping]"))
	assert.Empty(t, form.Get("subscription_data[trial_period_days]"))
}

const subscriptionJSON = `{"id":"sub_1","object":"subscription","customer":"cus_1","status":"trialing",
	"trial_end":1798761600,"cancel_at_period_end":false,"metadata":{"tg_plan":"p"},
	"items":{"object":"list","data":[{"id":"si_1","object":"subscription_item","current_period_end":1798761600,"price":{"id":"price_1","object":"price"}}]}}`

func TestCreateSubscription_FindsLatestCard(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{
		"GET /v1/customers/cus_1/payment_methods": `{"object":"list","data":[{"id":"pm_9","object":"payment_method"}],"has_more":false}`,
		"POST /v1/subscriptions":                  subscriptionJSON,
	})
	trialEnd := time.Unix(1798761600, 0).UTC()
	sub, err := c.CreateSubscription(context.Background(), billing.SubscriptionParams{
		IdempotencyKey: "tg-activate-1", CustomerID: "cus_1", PriceID: "price_1", TrialEnd: trialEnd,
		Metadata: map[string]string{"tg_plan": "p"},
	})
	require.NoError(t, err)
	req := f.last(t)
	assert.Equal(t, "tg-activate-1", req.IdempotencyKey)
	assert.Equal(t, "pm_9", req.Form.Get("default_payment_method"))
	assert.Equal(t, "price_1", req.Form.Get("items[0][price]"))
	assert.Equal(t, "1798761600", req.Form.Get("trial_end"))
	assert.Equal(t, billing.StripeSubscription{
		ID: "sub_1", CustomerID: "cus_1", Status: "trialing", CurrentPeriodEnd: trialEnd, TrialEnd: &trialEnd,
		PriceID: "price_1", Metadata: map[string]string{"tg_plan": "p"},
	}, sub)
}

func TestCreateSubscription_NoCard(t *testing.T) {
	_, c := newFakeStripe(t, map[string]string{
		"GET /v1/customers/cus_1/payment_methods": `{"object":"list","data":[],"has_more":false}`,
	})
	_, err := c.CreateSubscription(context.Background(), billing.SubscriptionParams{CustomerID: "cus_1", PriceID: "price_1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no saved card")
}

func TestGetSubscription(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"GET /v1/subscriptions/sub_1": subscriptionJSON})
	sub, err := c.GetSubscription(context.Background(), "sub_1")
	require.NoError(t, err)
	assert.Equal(t, "sub_1", sub.ID)
	assert.Equal(t, "GET", f.last(t).Method)
}

func TestCancelSubscription(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{
		"POST /v1/subscriptions/sub_1":   subscriptionJSON,
		"DELETE /v1/subscriptions/sub_1": subscriptionJSON,
	})
	_, err := c.CancelSubscription(context.Background(), "sub_1", true)
	require.NoError(t, err)
	req := f.last(t)
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "true", req.Form.Get("cancel_at_period_end"))

	_, err = c.CancelSubscription(context.Background(), "sub_1", false)
	require.NoError(t, err)
	assert.Equal(t, "DELETE", f.last(t).Method)
}

func TestPortal(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{
		"POST /v1/billing_portal/configurations":       `{"id":"bpc_1","object":"billing_portal.configuration"}`,
		"POST /v1/billing_portal/configurations/bpc_1": `{"id":"bpc_1","object":"billing_portal.configuration"}`,
		"POST /v1/billing_portal/sessions":             `{"id":"bps_1","object":"billing_portal.session","url":"https://billing.stripe.com/p/1"}`,
	})
	id, err := c.ConfigurePortal(context.Background(), billing.PortalParams{
		ReturnURL:      "https://gw/_/account",
		SwitchProducts: map[string][]string{"prod_1": {"price_m", "price_y"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "bpc_1", id)
	form := f.last(t).Form
	assert.Equal(t, "/v1/billing_portal/configurations", f.last(t).Path)
	assert.Equal(t, "true", form.Get("features[payment_method_update][enabled]"))
	assert.Equal(t, "at_period_end", form.Get("features[subscription_cancel][mode]"))
	assert.Equal(t, "false", form.Get("features[invoice_history][enabled]"))
	assert.Equal(t, "true", form.Get("features[subscription_update][enabled]"))
	assert.Equal(t, "price", form.Get("features[subscription_update][default_allowed_updates][0]"))
	assert.Equal(t, "always_invoice", form.Get("features[subscription_update][proration_behavior]"), "upgrades are charged at once")
	assert.Equal(t, "decreasing_item_amount", form.Get("features[subscription_update][schedule_at_period_end][conditions][0][type]"), "downgrades wait for the period end")
	assert.Equal(t, "shortening_interval", form.Get("features[subscription_update][schedule_at_period_end][conditions][1][type]"))
	assert.Equal(t, "prod_1", form.Get("features[subscription_update][products][0][product]"))
	assert.Equal(t, "price_y", form.Get("features[subscription_update][products][0][prices][1]"))
	assert.Equal(t, "https://gw/_/account", form.Get("default_return_url"))
	assert.Equal(t, "true", form.Get("features[customer_update][enabled]"))
	assert.Equal(t, "name", form.Get("features[customer_update][allowed_updates][0]"))
	assert.Equal(t, "address", form.Get("features[customer_update][allowed_updates][1]"))
	assert.Empty(t, form.Get("features[customer_update][allowed_updates][2]"), "neither tax id, email nor phone")

	_, err = c.ConfigurePortal(context.Background(), billing.PortalParams{ReturnURL: "https://gw/_/account"})
	require.NoError(t, err)
	req := f.last(t)
	assert.Equal(t, "/v1/billing_portal/configurations/bpc_1", req.Path)
	assert.Equal(t, "false", req.Form.Get("features[subscription_update][enabled]"))
	assert.Equal(t, "true", req.Form.Get("features[customer_update][enabled]"))
	assert.Equal(t, "address", req.Form.Get("features[customer_update][allowed_updates][1]"))

	_, err = c.ConfigurePortal(context.Background(), billing.PortalParams{
		ReturnURL: "https://gw/_/account", SwitchProducts: map[string][]string{"prod_1": {"price_m"}},
	})
	require.NoError(t, err)
	req = f.last(t)
	assert.Equal(t, "always_invoice", req.Form.Get("features[subscription_update][proration_behavior]"))
	assert.Equal(t, "decreasing_item_amount", req.Form.Get("features[subscription_update][schedule_at_period_end][conditions][0][type]"))
	assert.Equal(t, "shortening_interval", req.Form.Get("features[subscription_update][schedule_at_period_end][conditions][1][type]"))

	u, err := c.PortalURL(context.Background(), "cus_1", "https://gw/_/account")
	require.NoError(t, err)
	assert.Equal(t, "https://billing.stripe.com/p/1", u)
	form = f.last(t).Form
	assert.Equal(t, "bpc_1", form.Get("configuration"))
	assert.Equal(t, "cus_1", form.Get("customer"))
}

func TestRefund(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/refunds": `{"id":"re_1","object":"refund"}`})
	require.NoError(t, c.Refund(context.Background(), "pi_1", 1000, "tg-refund-1"))
	req := f.last(t)
	assert.Equal(t, "pi_1", req.Form.Get("payment_intent"))
	assert.Equal(t, "1000", req.Form.Get("amount"))
	assert.Equal(t, "tg-refund-1", req.IdempotencyKey)

	require.NoError(t, c.Refund(context.Background(), "pi_1", 0, "tg-refund-2"))
	assert.Empty(t, f.last(t).Form.Get("amount"))
}

func TestErrorsAreWrapped(t *testing.T) {
	_, c := newFakeStripe(t, map[string]string{})
	_, err := c.EnsureProduct(context.Background(), "", "x", "y", "")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "stripe: "), err.Error())
}

func TestCreateCheckoutSession_AskCustomerType(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/checkout/sessions": `{"id":"cs_1","object":"checkout.session","url":"https://checkout.stripe.com/c/cs_1"}`})
	p := billing.CheckoutParams{
		CheckoutID: "chk_1", CustomerID: "cus_1", Mode: "subscription",
		Lines: []billing.CheckoutLine{{PriceID: "price_1", Quantity: 1}}, SuccessURL: "https://gw/ok", CancelURL: "https://gw/ko",
	}
	_, err := c.CreateCheckoutSession(context.Background(), p)
	require.NoError(t, err)
	form := f.last(t).Form
	assert.Empty(t, form.Get("name_collection[business][enabled]"))
	assert.Equal(t, "true", form.Get("tax_id_collection[enabled]"), "tax id collection stays")

	p.CheckoutID, p.AskCustomerType = "chk_2", true
	_, err = c.CreateCheckoutSession(context.Background(), p)
	require.NoError(t, err)
	form = f.last(t).Form
	assert.Equal(t, "true", form.Get("name_collection[business][enabled]"))
	assert.Equal(t, "true", form.Get("name_collection[individual][enabled]"))
	assert.Equal(t, "true", form.Get("name_collection[business][optional]"))
	assert.Equal(t, "false", form.Get("name_collection[individual][optional]"))
	assert.Equal(t, "true", form.Get("tax_id_collection[enabled]"))
}

func TestUpdateCustomer(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{
		"POST /v1/customers/cus_1":         `{"id":"cus_1","object":"customer"}`,
		"POST /v1/customers/cus_1/tax_ids": `{"id":"txi_1","object":"tax_id"}`,
	})
	err := c.UpdateCustomer(context.Background(), "cus_1", billing.StripeCustomerUpdate{
		Name: "Escuela SL", Phone: "+34600000000", TaxID: "B12345674",
		Address: billing.Address{Line1: "C/ Mayor 1", Line2: "2B", PostalCode: "46001", City: "Valencia", State: "Valencia", Country: "ES"},
	})
	require.NoError(t, err)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.requests, 2)
	cu := f.requests[0].Form
	assert.Equal(t, "Escuela SL", cu.Get("name"))
	assert.Equal(t, "+34600000000", cu.Get("phone"))
	assert.Equal(t, "C/ Mayor 1", cu.Get("address[line1]"))
	assert.Equal(t, "2B", cu.Get("address[line2]"))
	assert.Equal(t, "46001", cu.Get("address[postal_code]"))
	assert.Equal(t, "ES", cu.Get("address[country]"))
	tax := f.requests[1].Form
	assert.Equal(t, "es_cif", tax.Get("type"))
	assert.Equal(t, "B12345674", tax.Get("value"))
}

func TestUpdateCustomer_WithoutTaxIDOnlyUpdatesCustomer(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/customers/cus_1": `{"id":"cus_1","object":"customer"}`})
	require.NoError(t, c.UpdateCustomer(context.Background(), "cus_1", billing.StripeCustomerUpdate{Name: "Ana"}))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Len(t, f.requests, 1)
}

func TestCreateCheckoutSession_Branding(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/checkout/sessions": `{"id":"cs_1","object":"checkout.session","url":"https://checkout.stripe.com/c/cs_1"}`})
	p := billing.CheckoutParams{
		CheckoutID: "chk_1", CustomerID: "cus_1", Mode: "subscription",
		Lines: []billing.CheckoutLine{{PriceID: "price_1", Quantity: 1}}, SuccessURL: "https://gw/ok", CancelURL: "https://gw/ko",
	}
	_, err := c.CreateCheckoutSession(context.Background(), p)
	require.NoError(t, err)
	for k := range f.last(t).Form {
		assert.False(t, strings.HasPrefix(k, "branding_settings"), k)
	}

	p.CheckoutID = "chk_2"
	p.Brand = billing.Brand{Name: "Academia", LogoURL: "https://a.test/logo.png", ButtonColor: "#112233", BackgroundColor: "#fafafa"}
	_, err = c.CreateCheckoutSession(context.Background(), p)
	require.NoError(t, err)
	form := f.last(t).Form
	assert.Equal(t, "Academia", form.Get("branding_settings[display_name]"))
	assert.Equal(t, "#112233", form.Get("branding_settings[button_color]"))
	assert.Equal(t, "#fafafa", form.Get("branding_settings[background_color]"))
	assert.Equal(t, "url", form.Get("branding_settings[logo][type]"))
	assert.Equal(t, "https://a.test/logo.png", form.Get("branding_settings[logo][url]"))
}

func TestCreateCheckoutSession_StatementDescriptorSuffix(t *testing.T) {
	f, c := newFakeStripe(t, map[string]string{"POST /v1/checkout/sessions": `{"id":"cs_1","object":"checkout.session","url":"https://checkout.stripe.com/c/cs_1"}`})
	p := billing.CheckoutParams{
		CheckoutID: "chk_1", CustomerID: "cus_1", Mode: "payment", StatementDescriptorSuffix: "ACADEMIA",
		Lines: []billing.CheckoutLine{{PriceID: "price_1", Quantity: 1}}, SuccessURL: "https://gw/ok", CancelURL: "https://gw/ko",
	}
	_, err := c.CreateCheckoutSession(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, "ACADEMIA", f.last(t).Form.Get("payment_intent_data[statement_descriptor_suffix]"))
}
