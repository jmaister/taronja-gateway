package billing

import (
	"context"
	"fmt"
	"strings"
)

// Setup actions the setup page can run.
const (
	ActionSyncPortal = "sync-portal"
	ActionSyncPlans  = "sync-plans"
)

// WebhookEvents are the events the Stripe webhook endpoint must send.
var WebhookEvents = []string{
	EventCheckoutCompleted, EventCheckoutExpired, EventSubscriptionCreated, EventSubscriptionUpdated,
	EventSubscriptionDeleted, EventInvoicePaid, EventInvoicePaymentFailed, EventChargeRefunded,
}

// SetupChecks lists what the billing setup page shows: Stripe, the plans
// and Odoo.
func (s *Service) SetupChecks(ctx context.Context) []SetupCheck {
	var checks []SetupCheck
	key := SetupCheck{ID: "stripe-key", Title: "Stripe secret key", Status: CheckOK}
	switch s.stripeMode {
	case "live":
		key.Detail = "live mode: real payments"
	case "test":
		key.Status, key.Detail = CheckWarn, "test mode: no real payments"
	default:
		key.Status, key.Detail = CheckMissing, "billing.stripe.secretKey is not set"
	}
	checks = append(checks, key,
		SetupCheck{
			ID: "stripe-webhook", Title: "Stripe webhook endpoint", Status: CheckManual,
			Detail: "Endpoint " + s.serverURL + "/_/billing/stripe/webhook with the events " + strings.Join(WebhookEvents, ", ") +
				"; its signing secret goes in billing.stripe.webhookSecret",
			Link: "https://dashboard.stripe.com/webhooks",
		},
		SetupCheck{
			ID: "stripe-emails", Title: "Stripe customer emails off", Status: CheckManual,
			Detail: "Turn off \"Successful payments\" and \"Finalized invoices\" emails: Odoo sends the legal invoices",
			Link:   "https://dashboard.stripe.com/settings/emails",
		},
		SetupCheck{
			ID: "stripe-portal", Title: "Stripe Customer Portal configuration", Status: CheckWarn,
			Detail: "Created or updated by the gateway at startup and when plans change", Action: ActionSyncPortal,
		},
	)

	plans := SetupCheck{ID: "plans", Title: "Plans synced to Stripe and Odoo", Status: CheckOK}
	list, err := s.repo.ListPlans(true)
	if err != nil {
		plans.Status, plans.Detail = CheckMissing, err.Error()
	} else {
		var missing []string
		for _, p := range list {
			if p.StripePriceID == "" || (s.odoo != nil && p.OdooProductID == nil) {
				missing = append(missing, p.Key)
			}
		}
		switch {
		case len(list) == 0:
			plans.Status, plans.Detail = CheckWarn, "no plans yet"
		case len(missing) > 0:
			plans.Status, plans.Detail, plans.Action = CheckWarn, "not synced: "+strings.Join(missing, ", "), ActionSyncPlans
		default:
			plans.Detail = fmt.Sprintf("%d active plans", len(list))
		}
	}
	checks = append(checks, plans)

	if s.odoo == nil {
		return append(checks, SetupCheck{ID: "odoo-connection", Title: "Odoo", Status: CheckMissing,
			Detail: "billing.odoo is not configured: invoices wait until it is"})
	}
	checks = append(checks, s.odoo.Checks(ctx)...)
	return append(checks, SetupCheck{
		ID: "odoo-api-key", Title: "Odoo API key expiry", Status: CheckManual,
		Detail: "Odoo 19 API keys of non-admin users expire: renew it (scope rpc) before it does",
	})
}

// RunSetupAction applies a fix from the setup page.
func (s *Service) RunSetupAction(ctx context.Context, action string) error {
	switch action {
	case ActionSyncPortal:
		return s.SyncPortal(ctx)
	case ActionSyncPlans:
		list, err := s.repo.ListPlans(true)
		if err != nil {
			return err
		}
		for i := range list {
			p := &list[i]
			if p.StripePriceID == "" && s.stripe != nil {
				err = s.syncStripe(ctx, p, nil)
				if err != nil {
					return fmt.Errorf("plan %s: %w", p.Key, err)
				}
			}
			s.syncOdooProducts(ctx, p)
			err = s.repo.SavePlan(p)
			if err != nil {
				return err
			}
		}
		s.InvalidateAll()
		return nil
	}
	return fmt.Errorf("%w: unknown action %q", ErrInvalid, action)
}
