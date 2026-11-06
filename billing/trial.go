package billing

import (
	"context"
	"fmt"

	"github.com/jmaister/taronja-gateway/db"
)

// StartNow ends a trial at once: Stripe charges the first period with the
// saved card and the subscription becomes active. The local status follows
// Stripe's webhooks, so it changes only when Stripe confirms the charge.
func (s *Service) StartNow(ctx context.Context, id, userID string) error {
	sub, err := s.repo.GetSubscription(id)
	if err != nil {
		return notFound(err)
	}
	err = s.authorizeSubject(sub.Subject, userID)
	if err != nil {
		return err
	}
	if s.stripe == nil {
		return ErrNotConfigured
	}
	plan, err := s.GetPlan(sub.PlanKey)
	if err != nil {
		return err
	}
	if plan.Kind != db.PlanKindSubscription || plan.Starts != db.PlanStartsOnPayment {
		return fmt.Errorf("%w: %s does not start on payment, so it has no trial to end", ErrConflict, plan.Key)
	}
	if sub.Status != db.SubscriptionTrialing || sub.StripeSubscriptionID == nil {
		return fmt.Errorf("%w: %s is not in its trial", ErrConflict, id)
	}
	_, err = s.stripe.EndTrial(ctx, *sub.StripeSubscriptionID, "tg-start-"+sub.ID)
	return err
}
