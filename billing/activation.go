package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// ActivateSubscription starts the Stripe subscription of a plan that starts
// on activation (megabox: when its box first goes online). The first period
// was paid at checkout, so Stripe's first charge comes one interval later.
// Activating twice changes nothing.
func (s *Service) ActivateSubscription(ctx context.Context, id string) (*db.BillingSubscription, error) {
	sub, err := s.repo.GetSubscription(id)
	if err != nil {
		return nil, notFound(err)
	}
	if sub.Status != db.SubscriptionAwaitingActivation {
		return sub, nil
	}
	if s.stripe == nil {
		return nil, ErrNotConfigured
	}
	plan, err := s.GetPlan(sub.PlanKey)
	if err != nil {
		return nil, err
	}
	customer, err := s.repo.GetCustomer(sub.Subject)
	if err != nil || customer.StripeCustomerID == "" {
		return nil, fmt.Errorf("%w: %s has no Stripe customer", ErrConflict, sub.Subject)
	}
	now := s.now()
	trialEnd := now.AddDate(0, 1, 0)
	if plan.Interval == db.PlanIntervalYear {
		trialEnd = now.AddDate(1, 0, 0)
	}
	ss, err := s.stripe.CreateSubscription(ctx, SubscriptionParams{
		IdempotencyKey: "tg-activate-" + sub.ID, CustomerID: customer.StripeCustomerID, PriceID: plan.StripePriceID,
		PaymentMethodID: sub.StripePaymentMethodID, TrialEnd: trialEnd,
		Metadata: map[string]string{"tg_subscription": sub.ID, "tg_subject": sub.Subject, "tg_plan": sub.PlanKey},
	})
	if err != nil {
		return nil, err
	}
	// customer.subscription.created may have been applied meanwhile.
	current, err := s.repo.GetSubscription(id)
	if err != nil {
		return nil, err
	}
	stripeID := ss.ID
	current.StripeSubscriptionID = &stripeID
	if current.Status == db.SubscriptionAwaitingActivation {
		current.Status = db.SubscriptionTrialing
		end := ss.CurrentPeriodEnd
		if end.IsZero() {
			end = trialEnd
		}
		current.TrialEnd, current.CurrentPeriodEnd = &end, &end
	}
	current.ActivatedAt = &now
	err = s.repo.UpdateSubscription(current)
	if err != nil {
		return nil, err
	}
	s.Invalidate(current.Subject)
	return current, nil
}

// CancelSubscription cancels at once or at the end of the period. A
// subscription still awaiting activation has nothing in Stripe and is
// canceled here.
func (s *Service) CancelSubscription(ctx context.Context, id string, atPeriodEnd bool) (*db.BillingSubscription, error) {
	sub, err := s.repo.GetSubscription(id)
	if err != nil {
		return nil, notFound(err)
	}
	if sub.Status == db.SubscriptionCanceled {
		return sub, nil
	}
	now := s.now()
	if sub.StripeSubscriptionID == nil {
		sub.Status, sub.CanceledAt = db.SubscriptionCanceled, &now
	} else {
		if s.stripe == nil {
			return nil, ErrNotConfigured
		}
		ss, err := s.stripe.CancelSubscription(ctx, *sub.StripeSubscriptionID, atPeriodEnd)
		if err != nil {
			return nil, err
		}
		sub.CancelAtPeriodEnd = ss.CancelAtPeriodEnd
		if !atPeriodEnd || ss.Status == "canceled" {
			sub.Status, sub.CanceledAt = db.SubscriptionCanceled, &now
		}
	}
	err = s.repo.UpdateSubscription(sub)
	if err != nil {
		return nil, err
	}
	s.Invalidate(sub.Subject)
	return sub, nil
}

// RefundPurchase refunds amountCents of a purchase (0 = all that is left)
// and, when asked, cancels the subscription it started. Stripe's
// charge.refunded then marks the purchase and queues the credit note.
func (s *Service) RefundPurchase(ctx context.Context, id string, amountCents int64, cancelSubscription bool) (*db.BillingPurchase, error) {
	p, err := s.repo.GetPurchase(id)
	if err != nil {
		return nil, notFound(err)
	}
	left := p.AmountCents - p.RefundedCents
	if amountCents == 0 {
		amountCents = left
	}
	if amountCents <= 0 || amountCents > left {
		return nil, fmt.Errorf("%w: refund between 1 and %d cents", ErrInvalid, left)
	}
	if s.stripe == nil {
		return nil, ErrNotConfigured
	}
	key := fmt.Sprintf("tg-refund-%s-%d", p.ID, p.RefundedCents+amountCents)
	err = s.stripe.Refund(ctx, p.StripePaymentIntentID, amountCents, key)
	if err != nil {
		return nil, err
	}
	if cancelSubscription && p.SubscriptionID != nil {
		_, err = s.CancelSubscription(ctx, *p.SubscriptionID, false)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("refunded, but canceling the subscription failed: %w", err)
		}
	}
	return p, nil
}
