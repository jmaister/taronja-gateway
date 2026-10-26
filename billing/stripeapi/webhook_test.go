package stripeapi

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v87/webhook"
)

// event wraps an object in an event envelope shaped like API 2026-09-30.endive.
func event(id, typ, object string) []byte {
	return fmt.Appendf(nil, `{"id":%q,"object":"event","api_version":"2026-09-30.endive","created":1798761600,"type":%q,"data":{"object":%s}}`, id, typ, object)
}

func sign(payload []byte) string {
	return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: "whsec_test", Timestamp: time.Now()}).Header
}

func parse(t *testing.T, c *Client, payload []byte) billing.Event {
	t.Helper()
	ev, err := c.ParseWebhook(payload, sign(payload))
	require.NoError(t, err)
	return ev
}

func TestParseWebhook_CheckoutCompleted(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	ev := parse(t, c, event("evt_1", "checkout.session.completed", `{
		"id":"cs_1","object":"checkout.session","client_reference_id":"chk_1","mode":"payment","payment_status":"paid",
		"customer":"cus_1","subscription":null,"payment_intent":"pi_1","amount_total":4900,"currency":"eur",
		"customer_details":{"name":"Ana","email":"ana@example.com","phone":"+34600000000",
			"address":{"line1":"C/ Mayor 1","line2":null,"postal_code":"46001","city":"Valencia","state":"V","country":"ES"},
			"tax_ids":[{"type":"es_cif","value":"B12345674"}]},
		"collected_information":{"shipping_details":{"name":"Ana R","address":{"line1":"C/ Paz 2","postal_code":"46002","city":"Valencia","country":"ES"}}},
		"metadata":{"tg_checkout":"chk_1"}}`))

	assert.Equal(t, "evt_1", ev.ID)
	assert.Equal(t, billing.EventCheckoutCompleted, ev.Type)
	assert.Equal(t, time.Unix(1798761600, 0).UTC(), ev.Created)
	require.NotNil(t, ev.Checkout)
	assert.Equal(t, &billing.CheckoutEvent{
		SessionID: "cs_1", CheckoutID: "chk_1", Mode: "payment", Paid: true, CustomerID: "cus_1", PaymentIntentID: "pi_1",
		AmountCents: 4900, Currency: "eur", Name: "Ana", Email: "ana@example.com", Phone: "+34600000000", TaxID: "B12345674",
		BillingAddress:  billing.Address{Line1: "C/ Mayor 1", PostalCode: "46001", City: "Valencia", State: "V", Country: "ES"},
		ShippingName:    "Ana R",
		ShippingAddress: &billing.Address{Line1: "C/ Paz 2", PostalCode: "46002", City: "Valencia", Country: "ES"},
		Metadata:        map[string]string{"tg_checkout": "chk_1"},
	}, ev.Checkout)
	assert.Nil(t, ev.Subscription)
}

func TestParseWebhook_CheckoutExpiredUnpaid(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	ev := parse(t, c, event("evt_2", "checkout.session.expired", `{"id":"cs_2","object":"checkout.session","client_reference_id":"chk_2","mode":"subscription","payment_status":"unpaid","customer":null}`))
	require.NotNil(t, ev.Checkout)
	assert.False(t, ev.Checkout.Paid)
	assert.Equal(t, "chk_2", ev.Checkout.CheckoutID)
	assert.Empty(t, ev.Checkout.CustomerID)
	assert.Nil(t, ev.Checkout.ShippingAddress)
}

func TestParseWebhook_Subscription(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	for _, typ := range []string{"customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted"} {
		ev := parse(t, c, event("evt_s", typ, `{"id":"sub_1","object":"subscription","customer":"cus_1","status":"past_due",
			"trial_end":null,"cancel_at_period_end":true,"metadata":{"tg_plan":"p"},
			"items":{"object":"list","data":[{"id":"si_1","object":"subscription_item","current_period_end":1801440000,"price":{"id":"price_1","object":"price"}}]}}`))
		require.NotNil(t, ev.Subscription, typ)
		assert.Equal(t, billing.StripeSubscription{
			ID: "sub_1", CustomerID: "cus_1", Status: "past_due", CurrentPeriodEnd: time.Unix(1801440000, 0).UTC(),
			CancelAtPeriodEnd: true, PriceID: "price_1", Metadata: map[string]string{"tg_plan": "p"},
		}, *ev.Subscription)
	}
}

const invoiceJSON = `{"id":"in_1","object":"invoice","customer":"cus_1","currency":"eur","amount_paid":2900,"amount_due":2900,
	"attempt_count":2,"created":1798700000,"status_transitions":{"paid_at":1798761600},
	"parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_1"}},
	"lines":{"object":"list","data":[{"id":"il_1","object":"line_item","description":"1 × Academia (at €29.00 / month)","amount":2900,
		"pricing":{"type":"price_details","price_details":{"price":"price_1","product":"prod_1"}},
		"period":{"start":1798761600,"end":1801440000}}]}}`

func TestParseWebhook_InvoicePaid(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	ev := parse(t, c, event("evt_i", "invoice.paid", invoiceJSON))
	require.NotNil(t, ev.Invoice)
	start, end := time.Unix(1798761600, 0).UTC(), time.Unix(1801440000, 0).UTC()
	assert.Equal(t, &billing.InvoiceEvent{
		InvoiceID: "in_1", CustomerID: "cus_1", SubscriptionID: "sub_1", AmountCents: 2900, Currency: "eur",
		PaidAt: start, AttemptCount: 2,
		Lines: []billing.EventLine{{Description: "1 × Academia (at €29.00 / month)", PriceID: "price_1", AmountCents: 2900, PeriodStart: &start, PeriodEnd: &end}},
	}, ev.Invoice)
}

func TestParseWebhook_InvoicePaymentFailed(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	ev := parse(t, c, event("evt_f", "invoice.payment_failed", `{"id":"in_2","object":"invoice","customer":"cus_1","currency":"eur",
		"amount_paid":0,"amount_due":2900,"attempt_count":1,"created":1798700000,"status_transitions":{"paid_at":null}}`))
	require.NotNil(t, ev.Invoice)
	assert.Equal(t, int64(2900), ev.Invoice.AmountCents)
	assert.Equal(t, int64(1), ev.Invoice.AttemptCount)
	assert.Equal(t, time.Unix(1798700000, 0).UTC(), ev.Invoice.PaidAt)
	assert.Empty(t, ev.Invoice.SubscriptionID)
}

func TestParseWebhook_ChargeRefunded(t *testing.T) {
	charge := `{"id":"ch_1","object":"charge","payment_intent":"pi_1","amount_refunded":1000,"currency":"eur",
		"refunds":{"object":"list","data":[{"id":"re_2","object":"refund"}]}}`

	f, c := newFakeStripe(t, map[string]string{
		"GET /v1/invoice_payments": `{"object":"list","data":[{"id":"inpay_1","object":"invoice_payment","invoice":"in_1"}],"has_more":false}`,
	})
	ev := parse(t, c, event("evt_r", "charge.refunded", charge))
	require.NotNil(t, ev.Refund)
	assert.Equal(t, &billing.RefundEvent{ChargeID: "ch_1", PaymentIntentID: "pi_1", InvoiceID: "in_1", RefundedCents: 1000, Currency: "eur", RefundID: "re_2"}, ev.Refund)
	req := f.last(t)
	assert.Equal(t, "pi_1", req.Form.Get("payment[payment_intent]"))
	assert.Equal(t, "payment_intent", req.Form.Get("payment[type]"))

	_, c = newFakeStripe(t, map[string]string{"GET /v1/invoice_payments": `{"object":"list","data":[],"has_more":false}`})
	ev = parse(t, c, event("evt_r2", "charge.refunded", charge))
	assert.Empty(t, ev.Refund.InvoiceID)
}

func TestParseWebhook_UnknownType(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	ev := parse(t, c, event("evt_u", "customer.created", `{"id":"cus_1","object":"customer"}`))
	assert.Equal(t, billing.Event{ID: "evt_u", Type: "customer.created", Created: time.Unix(1798761600, 0).UTC()}, ev)
}

func TestParseWebhook_BadSignature(t *testing.T) {
	c := New("sk_test_123", "whsec_test")
	payload := event("evt_1", "customer.created", `{}`)
	_, err := c.ParseWebhook(payload, "t=1,v1=deadbeef")
	assert.True(t, errors.Is(err, billing.ErrBadSignature), err)
	_, err = c.ParseWebhook(payload, "")
	assert.True(t, errors.Is(err, billing.ErrBadSignature), err)

	other := New("sk_test_123", "whsec_other")
	_, err = other.ParseWebhook(payload, sign(payload))
	assert.True(t, errors.Is(err, billing.ErrBadSignature), err)
}
