package stripeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

// ParseWebhook verifies the Stripe-Signature header and decodes the event.
func (c *Client) ParseWebhook(payload []byte, signature string) (billing.Event, error) {
	if signature == "" {
		return billing.Event{}, fmt.Errorf("%w: %w", billing.ErrBadSignature, errNoSignature)
	}
	event, err := webhook.ConstructEventWithOptions(payload, signature, c.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return billing.Event{}, fmt.Errorf("%w: %w", billing.ErrBadSignature, err)
	}
	out := billing.Event{ID: event.ID, Type: string(event.Type), Created: time.Unix(event.Created, 0).UTC()}
	if event.Data == nil {
		return out, nil
	}
	raw := event.Data.Raw

	switch out.Type {
	case billing.EventCheckoutCompleted, billing.EventCheckoutExpired:
		var s stripe.CheckoutSession
		err := json.Unmarshal(raw, &s)
		if err != nil {
			return billing.Event{}, fmt.Errorf("stripe: decoding checkout session: %w", err)
		}
		out.Checkout = mapCheckout(&s)
	case billing.EventSubscriptionCreated, billing.EventSubscriptionUpdated, billing.EventSubscriptionDeleted:
		var s stripe.Subscription
		err := json.Unmarshal(raw, &s)
		if err != nil {
			return billing.Event{}, fmt.Errorf("stripe: decoding subscription: %w", err)
		}
		sub := mapSubscription(&s)
		out.Subscription = &sub
	case billing.EventInvoicePaid, billing.EventInvoicePaymentFailed:
		var inv stripe.Invoice
		err := json.Unmarshal(raw, &inv)
		if err != nil {
			return billing.Event{}, fmt.Errorf("stripe: decoding invoice: %w", err)
		}
		out.Invoice = mapInvoice(&inv, out.Type == billing.EventInvoicePaid)
	case billing.EventChargeRefunded:
		var ch stripe.Charge
		err := json.Unmarshal(raw, &ch)
		if err != nil {
			return billing.Event{}, fmt.Errorf("stripe: decoding charge: %w", err)
		}
		refund := &billing.RefundEvent{ChargeID: ch.ID, RefundedCents: ch.AmountRefunded, Currency: string(ch.Currency)}
		if ch.Refunds != nil && len(ch.Refunds.Data) > 0 {
			refund.RefundID = ch.Refunds.Data[0].ID
		}
		if ch.PaymentIntent != nil {
			refund.PaymentIntentID = ch.PaymentIntent.ID
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			invoiceID, err := c.invoiceOfPaymentIntent(ctx, refund.PaymentIntentID)
			if err != nil {
				return billing.Event{}, err
			}
			refund.InvoiceID = invoiceID
		}
		out.Refund = refund
	}
	return out, nil
}

func mapAddress(a *stripe.Address) billing.Address {
	if a == nil {
		return billing.Address{}
	}
	return billing.Address{Line1: a.Line1, Line2: a.Line2, PostalCode: a.PostalCode, City: a.City, State: a.State, Country: a.Country}
}

func mapCheckout(s *stripe.CheckoutSession) *billing.CheckoutEvent {
	out := &billing.CheckoutEvent{
		SessionID:   s.ID,
		CheckoutID:  s.ClientReferenceID,
		Mode:        string(s.Mode),
		Paid:        s.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid || s.PaymentStatus == stripe.CheckoutSessionPaymentStatusNoPaymentRequired,
		AmountCents: s.AmountTotal,
		Currency:    string(s.Currency),
		Metadata:    s.Metadata,
	}
	if s.Customer != nil {
		out.CustomerID = s.Customer.ID
	}
	if s.Subscription != nil {
		out.SubscriptionID = s.Subscription.ID
	}
	if s.PaymentIntent != nil {
		out.PaymentIntentID = s.PaymentIntent.ID
	}
	if d := s.CustomerDetails; d != nil {
		out.Name, out.Email, out.Phone = d.Name, d.Email, d.Phone
		out.BillingAddress = mapAddress(d.Address)
		if len(d.TaxIDs) > 0 {
			out.TaxID = d.TaxIDs[0].Value
		}
	}
	if ci := s.CollectedInformation; ci != nil && ci.ShippingDetails != nil {
		out.ShippingName = ci.ShippingDetails.Name
		addr := mapAddress(ci.ShippingDetails.Address)
		out.ShippingAddress = &addr
	}
	return out
}

func mapInvoice(inv *stripe.Invoice, paid bool) *billing.InvoiceEvent {
	out := &billing.InvoiceEvent{
		InvoiceID:    inv.ID,
		Currency:     string(inv.Currency),
		AttemptCount: inv.AttemptCount,
		PaidAt:       time.Unix(inv.Created, 0).UTC(),
		AmountCents:  inv.AmountDue,
	}
	if paid {
		out.AmountCents = inv.AmountPaid
	}
	if inv.StatusTransitions != nil && inv.StatusTransitions.PaidAt != 0 {
		out.PaidAt = time.Unix(inv.StatusTransitions.PaidAt, 0).UTC()
	}
	if inv.Customer != nil {
		out.CustomerID = inv.Customer.ID
	}
	if p := inv.Parent; p != nil && p.SubscriptionDetails != nil && p.SubscriptionDetails.Subscription != nil {
		out.SubscriptionID = p.SubscriptionDetails.Subscription.ID
	}
	if inv.Lines != nil {
		for _, l := range inv.Lines.Data {
			line := billing.EventLine{Description: l.Description, AmountCents: l.Amount}
			if l.Pricing != nil && l.Pricing.PriceDetails != nil && l.Pricing.PriceDetails.Price != nil {
				line.PriceID = l.Pricing.PriceDetails.Price.ID
			}
			if l.Period != nil {
				start := time.Unix(l.Period.Start, 0).UTC()
				end := time.Unix(l.Period.End, 0).UTC()
				line.PeriodStart, line.PeriodEnd = &start, &end
			}
			out.Lines = append(out.Lines, line)
		}
	}
	return out
}
