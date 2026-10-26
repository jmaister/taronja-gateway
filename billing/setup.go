package billing

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	return append(checks, s.outboxCheck(), s.apiKeyCheck())
}

// OutboxLink is an in-app path: the documents page filtered to the ones Odoo gave up on.
const OutboxLink = "/billing/invoices?state=failed"

func (s *Service) outboxCheck() SetupCheck {
	c := SetupCheck{ID: "odoo-outbox", Title: "Odoo invoice queue", Status: CheckOK, Link: OutboxLink}
	st, err := s.repo.OutboxStats()
	if err != nil {
		c.Status, c.Detail = CheckMissing, err.Error()
		return c
	}
	c.Detail = fmt.Sprintf("%d pending, %d retrying, %d failed", st.Pending, st.Retrying, st.Failed)
	if st.OldestPending != nil {
		c.Detail += ", oldest waiting " + s.now().Sub(*st.OldestPending).Round(time.Minute).String()
	}
	switch {
	case st.Failed > 0:
		c.Status = CheckMissing
	case st.Retrying > 0 || (st.OldestPending != nil && s.now().Sub(*st.OldestPending) > time.Hour):
		c.Status, c.Link = CheckWarn, "/billing/invoices?state=pending"
	}
	return c
}

// apiKeyWarnDays is how long before billing.odoo.apiKeyExpiresAt the setup
// page starts warning.
const apiKeyWarnDays = 30

func (s *Service) apiKeyCheck() SetupCheck {
	c := SetupCheck{ID: "odoo-api-key", Title: "Odoo API key expiry", Status: CheckManual}
	if s.odooKeyExp.IsZero() {
		c.Detail = "Odoo 19 API keys of non-admin users expire: renew it (scope rpc) before it does, and set billing.odoo.apiKeyExpiresAt (YYYY-MM-DD) to be warned"
		return c
	}
	days := int(s.odooKeyExp.Sub(s.now()).Hours() / 24)
	date := s.odooKeyExp.Format(time.DateOnly)
	switch {
	case !s.now().Before(s.odooKeyExp.AddDate(0, 0, 1)):
		c.Status, c.Detail = CheckMissing, "the Odoo API key expired on "+date+": create a new one and update billing.odoo.apiKey"
	case days < apiKeyWarnDays:
		c.Status, c.Detail = CheckWarn, fmt.Sprintf("the Odoo API key expires on %s (%d days): renew it", date, days)
	default:
		c.Status, c.Detail = CheckOK, fmt.Sprintf("the Odoo API key expires on %s (%d days)", date, days)
	}
	return c
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
