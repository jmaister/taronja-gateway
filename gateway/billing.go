package gateway

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/billing/odooapi"
	"github.com/jmaister/taronja-gateway/billing/stripeapi"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
)

// OutboxInterval is how often the Odoo outbox looks for due documents
// (tests shorten it).
var OutboxInterval = 30 * time.Second

// InitBilling builds the billing service when billing.enabled, stores it on
// d and starts its workers: the Odoo outbox and the hourly notifications.
// Run it after InitNotifications. The returned func stops the workers.
func InitBilling(ctx context.Context, cfg config.BillingConfig, serverCfg config.ServerConfig, d *deps.Dependencies) func(context.Context) error {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }
	}
	var stripeOpts []stripeapi.Option
	if cfg.Stripe.APIURL != "" {
		stripeOpts = append(stripeOpts, stripeapi.WithAPIURL(cfg.Stripe.APIURL))
	}
	mode := "test"
	if strings.HasPrefix(cfg.Stripe.SecretKey, "sk_live_") || strings.HasPrefix(cfg.Stripe.SecretKey, "rk_live_") {
		mode = "live"
	}
	svcCfg := billing.Config{
		Repo: d.BillingRepo, Orgs: d.OrganizationRepo, ServerURL: serverCfg.URL, StripeMode: mode,
		Stripe: stripeapi.New(cfg.Stripe.SecretKey, cfg.Stripe.WebhookSecret, stripeOpts...),
	}
	if cfg.Odoo.IsConfigured() {
		svcCfg.Odoo = odooapi.New(cfg.Odoo)
		if cfg.Odoo.APIKeyExpiresAt != "" {
			// validated when the config loads
			svcCfg.OdooKeyExpiresAt, _ = time.Parse(time.DateOnly, cfg.Odoo.APIKeyExpiresAt)
		}
	} else {
		log.Printf("Billing: Odoo is not configured; invoices wait in the outbox until it is")
	}
	svc := billing.NewService(svcCfg)
	svc.SetNotifications(d.NotificationService)
	d.Billing = svc

	workerCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		syncCtx, done := context.WithTimeout(workerCtx, time.Minute)
		defer done()
		err := svc.SyncPortal(syncCtx)
		if err != nil {
			log.Printf("Billing: configuring the Stripe Customer Portal: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		svc.RunOutbox(workerCtx, OutboxInterval)
	}()
	go func() {
		defer wg.Done()
		svc.RunNotifier(workerCtx)
	}()
	log.Printf("Billing enabled (Stripe %s mode, Odoo %t)", mode, cfg.Odoo.IsConfigured())

	return func(shutdownCtx context.Context) error {
		cancel()
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			return nil
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
}
