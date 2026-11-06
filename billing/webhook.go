package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"gorm.io/gorm"
)

// MaxWebhookBody caps a webhook request body.
const MaxWebhookBody = 256 << 10

// ErrRetryLater: the event refers to something the gateway hasn't recorded
// and couldn't get from Stripe; the webhook answers 500 so Stripe sends it
// again later, and the setup page lists it until it is applied.
var ErrRetryLater = errors.New("event arrived before what it refers to; retry later")

// errDuplicateLive is a Stripe subscription for a subject that already has a
// live one in the plan's group.
var errDuplicateLive = errors.New("billing: duplicate live subscription")

func retryLater(reason string) error { return fmt.Errorf("%w: %s", ErrRetryLater, reason) }

// webhookTx is one event's state change: the repository bound to the
// transaction and what to do after it commits.
type webhookTx struct {
	repo       repositories.BillingRepository
	invalidate map[string]bool
	after      []func()
	// fetched is the subscription an invoice event is about, read from
	// Stripe before the transaction because the gateway had no row for it
	// (fetchErr when Stripe didn't answer).
	fetched  *StripeSubscription
	fetchErr error
}

func (tx *webhookTx) touched(subject string) { tx.invalidate[subject] = true }

// HandleWebhook verifies and applies one Stripe webhook request.
func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.stripe == nil {
		return ErrNotConfigured
	}
	ev, err := s.stripe.ParseWebhook(payload, signature)
	if err != nil {
		return err
	}
	return s.HandleEvent(ctx, ev)
}

// HandleEvent applies a verified event. The event id is recorded in the
// same transaction as its effect, so a repeated event changes nothing.
func (s *Service) HandleEvent(ctx context.Context, ev Event) error {
	tx := &webhookTx{invalidate: map[string]bool{}}
	tx.fetched, tx.fetchErr = s.prefetchSubscription(ctx, ev)
	err := s.repo.Transaction(func(repo repositories.BillingRepository) error {
		tx.repo = repo
		fresh, err := repo.RecordStripeEvent(ev.ID, ev.Type)
		if err != nil || !fresh {
			return err
		}
		err = s.apply(tx, ev)
		if err != nil {
			return err
		}
		return repo.ClearStuckEvent(ev.ID)
	})
	if errors.Is(err, ErrRetryLater) {
		subID := ""
		if ev.Invoice != nil {
			subID = ev.Invoice.SubscriptionID
		}
		noteErr := s.repo.RecordStuckEvent(ev.ID, ev.Type, subID, err.Error(), s.now())
		if noteErr != nil {
			log.Printf("billing: noting stuck event %s: %v", ev.ID, noteErr)
		}
	}
	if err != nil {
		return err
	}
	for subject := range tx.invalidate {
		s.Invalidate(subject)
	}
	for _, f := range tx.after {
		f()
	}
	return nil
}

// apply runs the state change of one new event.
func (s *Service) apply(tx *webhookTx, ev Event) error {
	{
		switch {
		case ev.Checkout != nil && (ev.Type == EventCheckoutCompleted || ev.Type == EventCheckoutAsyncPaid):
			return s.onCheckoutCompleted(tx, ev)
		case ev.Checkout != nil && (ev.Type == EventCheckoutExpired || ev.Type == EventCheckoutAsyncFailed):
			return s.onCheckoutExpired(tx, ev.Checkout)
		case ev.Subscription != nil:
			return s.onSubscription(tx, ev)
		case ev.Invoice != nil && ev.Type == EventInvoicePaid:
			return s.onInvoicePaid(tx, ev)
		case ev.Invoice != nil && ev.Type == EventInvoicePaymentFailed:
			return s.onInvoiceFailed(tx, ev)
		case ev.Refund != nil:
			return s.onRefund(tx, ev)
		case ev.Customer != nil:
			return s.onCustomerUpdated(tx, ev)
		}
		return nil
	}
}

// prefetchSubscription reads from Stripe the subscription an invoice event
// is about when the gateway has no row for it yet (invoice.paid can arrive
// before customer.subscription.created). Done outside the transaction: it
// is a network call.
func (s *Service) prefetchSubscription(ctx context.Context, ev Event) (*StripeSubscription, error) {
	inv := ev.Invoice
	if s.stripe == nil || inv == nil || inv.SubscriptionID == "" {
		return nil, nil
	}
	switch ev.Type {
	case EventInvoicePaid:
		if inv.AmountCents <= 0 {
			return nil, nil
		}
	case EventInvoicePaymentFailed:
	default:
		return nil, nil
	}
	_, err := s.repo.GetSubscriptionByStripeID(inv.SubscriptionID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	ss, err := s.stripe.GetSubscription(ctx, inv.SubscriptionID)
	if err != nil {
		return nil, err
	}
	return &ss, nil
}

// invoiceSubscription finds the subscription row an invoice event is about,
// creating it from Stripe's copy when its own event hasn't arrived. Nil when
// the subscription isn't the gateway's.
func (s *Service) invoiceSubscription(tx *webhookTx, ev Event) (*db.BillingSubscription, error) {
	id := ev.Invoice.SubscriptionID
	sub, err := tx.repo.GetSubscriptionByStripeID(id)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return sub, err
	}
	if tx.fetched == nil {
		if tx.fetchErr != nil {
			return nil, retryLater(fmt.Sprintf("subscription %s is not recorded and Stripe didn't return it: %v", id, tx.fetchErr))
		}
		return nil, retryLater(fmt.Sprintf("subscription %s is not recorded", id))
	}
	err = s.onSubscription(tx, Event{ID: ev.ID, Type: EventSubscriptionUpdated, Created: ev.Created, Subscription: tx.fetched})
	if err != nil {
		return nil, err
	}
	sub, err = tx.repo.GetSubscriptionByStripeID(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return sub, err
}

func (s *Service) onCheckoutCompleted(tx *webhookTx, ev Event) error {
	cev := ev.Checkout
	checkout, err := s.findCheckout(tx.repo, cev)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("billing: checkout.session.completed for unknown checkout %q (session %s), ignored", cev.CheckoutID, cev.SessionID)
		return nil
	}
	if err != nil {
		return err
	}
	if checkout.Status == db.CheckoutStatusCompleted {
		return nil
	}
	if !cev.Paid {
		log.Printf("billing: checkout %s completed but not paid (session %s): nothing is activated until it is", checkout.ID, cev.SessionID)
		if checkout.Status == db.CheckoutStatusOpen {
			checkout.Status = db.CheckoutStatusAwaitingPayment
			return tx.repo.UpdateCheckout(checkout)
		}
		return nil
	}
	plan, err := tx.repo.GetPlan(checkout.PlanKey)
	if err != nil {
		return err
	}
	err = s.saveCustomerDetails(tx.repo, checkout.Subject, cev)
	if err != nil {
		return err
	}

	if cev.Mode == "subscription" {
		if plan.TrialDays > 0 {
			err = markTrial(tx.repo, checkout.Subject, planGroup(&plan.BillingPlan))
			if err != nil {
				return err
			}
		}
		if cev.SubscriptionID != "" {
			sub, err := tx.repo.GetSubscriptionByStripeID(cev.SubscriptionID)
			if err == nil && sub.PayerUserID == "" {
				sub.PayerUserID = checkout.UserID
				err = tx.repo.UpdateSubscription(sub)
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			// Otherwise customer.subscription.created creates it from its
			// metadata.
		}
	} else {
		err = s.recordPurchase(tx, checkout, plan, cev, ev.Created)
		if err != nil {
			return err
		}
	}
	checkout.Status = db.CheckoutStatusCompleted
	tx.touched(checkout.Subject)
	return tx.repo.UpdateCheckout(checkout)
}

func (s *Service) findCheckout(repo repositories.BillingRepository, cev *CheckoutEvent) (*db.BillingCheckout, error) {
	if cev.CheckoutID != "" {
		return repo.GetCheckout(cev.CheckoutID)
	}
	return repo.GetCheckoutByStripeSession(cev.SessionID)
}

// saveCustomerDetails keeps what Checkout collected: invoices are issued to it.
func (s *Service) saveCustomerDetails(repo repositories.BillingRepository, subject string, cev *CheckoutEvent) error {
	c, err := repo.GetCustomer(subject)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c, err = &db.BillingCustomer{Subject: subject}, nil
	}
	if err != nil {
		return err
	}
	if cev.CustomerID != "" {
		c.StripeCustomerID = cev.CustomerID
	}
	if cev.Name != "" {
		c.Name = cev.Name
	}
	if cev.Email != "" {
		c.Email = cev.Email
	}
	if cev.BillingAddress != (Address{}) {
		a := cev.BillingAddress
		c.AddressLine1, c.AddressLine2, c.PostalCode, c.City, c.State, c.Country = a.Line1, a.Line2, a.PostalCode, a.City, a.State, a.Country
	}
	if c.TaxID == "" {
		c.TaxID = validTaxID(cev.TaxID, c.Country, subject)
	}
	return repo.SaveCustomer(c)
}

// validTaxID returns a tax id typed at Stripe in its saved form, or "" when it
// is not valid: Odoo would reject the partner and the invoice would never be
// issued. The customer can then give it again through the billing details.
func validTaxID(raw, country, subject string) string {
	if country == "" {
		country = "ES"
	}
	id := NormalizeTaxID(raw, country)
	if id == "" {
		return ""
	}
	err := ValidateTaxID(id, "", country)
	if err != nil {
		log.Printf("billing: tax id %q of %s is not valid, not saved: %v", raw, subject, err)
		return ""
	}
	return id
}

// onCustomerUpdated copies what the customer edited at Stripe (the Customer
// Portal) into the details invoices are issued to; the next invoice sends
// them to Odoo (ensurePartner). The tax id is kept once saved: changing it
// takes a new account.
func (s *Service) onCustomerUpdated(tx *webhookTx, ev Event) error {
	cu := ev.Customer
	c, err := tx.repo.GetCustomerByStripeID(cu.CustomerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.StripeEventAt != nil && ev.Created.Before(*c.StripeEventAt) {
		log.Printf("billing: %s %s is older than the customer's details, ignored", ev.Type, ev.ID)
		return nil
	}
	c.StripeEventAt = &ev.Created
	if cu.Name != "" {
		c.Name = cu.Name
	}
	if cu.Email != "" {
		c.Email = cu.Email
	}
	if cu.Address != (Address{}) {
		a := cu.Address
		c.AddressLine1, c.AddressLine2, c.PostalCode, c.City, c.State, c.Country = a.Line1, a.Line2, a.PostalCode, a.City, a.State, a.Country
	}
	switch taxID := NormalizeTaxID(cu.TaxID, c.Country); {
	case taxID == "" || taxID == c.TaxID:
	case c.TaxID == "":
		c.TaxID = validTaxID(cu.TaxID, c.Country, c.Subject)
	default:
		log.Printf("billing: customer %s has tax id %q at Stripe, keeping the saved one (event %s)", cu.CustomerID, cu.TaxID, ev.ID)
	}
	return tx.repo.SaveCustomer(c)
}

func markTrial(repo repositories.BillingRepository, subject, group string) error {
	c, err := repo.GetCustomer(subject)
	if err != nil {
		return err
	}
	if hadTrial(c, group) {
		return nil
	}
	groups := strings.Trim(c.TrialGroups+","+group, ",")
	c.TrialGroups = groups
	return repo.SaveCustomer(c)
}

// recordPurchase handles a paid Checkout in payment mode: the purchase, the
// subscription waiting for activation (plans that start on activation) and
// the invoice for the payment.
func (s *Service) recordPurchase(tx *webhookTx, checkout *db.BillingCheckout, plan *repositories.BillingPlanWithFeatures, cev *CheckoutEvent, paidAt time.Time) error {
	if cev.PaymentIntentID == "" {
		return fmt.Errorf("paid checkout %s without a payment intent", checkout.ID)
	}
	p := &db.BillingPurchase{
		Subject: checkout.Subject, PlanKey: plan.Key, PayerUserID: checkout.UserID, CheckoutID: checkout.ID,
		Status: db.PurchaseStatusPaid, AmountCents: cev.AmountCents, Currency: currency(cev.Currency),
		StripePaymentIntentID: cev.PaymentIntentID, PaidAt: paidAt, ShippingName: cev.ShippingName, ShippingPhone: cev.Phone,
	}
	if a := cev.ShippingAddress; a != nil {
		p.ShippingLine1, p.ShippingLine2, p.ShippingPostalCode, p.ShippingCity, p.ShippingState, p.ShippingCountry = a.Line1, a.Line2, a.PostalCode, a.City, a.State, a.Country
	}
	if plan.Kind == db.PlanKindSubscription {
		sub := &db.BillingSubscription{
			Subject: checkout.Subject, PlanKey: plan.Key, GroupKey: planGroup(&plan.BillingPlan), PayerUserID: checkout.UserID,
			Status: db.SubscriptionAwaitingActivation, StripePaymentMethodID: cev.PaymentMethodID,
		}
		err := tx.repo.CreateSubscription(sub)
		switch {
		case errors.Is(err, repositories.ErrDuplicate):
			// Paid twice for the same group: the payment is recorded (and
			// invoiced) without a subscription, for the admin to refund.
			log.Printf("billing: %s already has a live subscription in %s: payment %s recorded without one, refund it", checkout.Subject, sub.GroupKey, cev.PaymentIntentID)
		case err != nil:
			return err
		default:
			p.SubscriptionID = &sub.ID
		}
	}
	err := tx.repo.CreatePurchase(p)
	if errors.Is(err, repositories.ErrDuplicate) {
		return nil
	}
	if err != nil {
		return err
	}

	var lines []DocumentLine
	if plan.Kind == db.PlanKindSubscription && plan.SetupFeeCents > 0 {
		lines = append(lines, DocumentLine{Description: plan.SetupFeeName, PlanKey: plan.Key, SetupFee: true, AmountCents: plan.SetupFeeCents})
	}
	main := plan.AmountCents
	description := plan.Name
	if plan.Kind == db.PlanKindOneTime && plan.AccessDays > 0 {
		description = fmt.Sprintf("%s %s–%s", plan.Name, s.day(paidAt), s.day(paidAt.AddDate(0, 0, plan.AccessDays)))
	}
	lines = append(lines, DocumentLine{Description: description, PlanKey: plan.Key, AmountCents: main})
	// Odoo's total must equal the charge: what Stripe charged wins.
	sum := int64(0)
	for _, l := range lines {
		sum += l.AmountCents
	}
	lines[len(lines)-1].AmountCents += cev.AmountCents - sum

	purchaseID := p.ID
	return EnqueueDocument(tx.repo, &db.BillingDocument{
		Subject: checkout.Subject, Kind: db.DocumentInvoice, StripeRef: cev.PaymentIntentID, PurchaseID: &purchaseID,
		SubscriptionID: p.SubscriptionID, AmountCents: cev.AmountCents, Currency: p.Currency, PaidAt: paidAt,
	}, lines, s.now())
}

func currency(c string) string {
	if c == "" {
		return "eur"
	}
	return strings.ToLower(c)
}

func (s *Service) onCheckoutExpired(tx *webhookTx, cev *CheckoutEvent) error {
	checkout, err := s.findCheckout(tx.repo, cev)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || (checkout.Status != db.CheckoutStatusOpen && checkout.Status != db.CheckoutStatusAwaitingPayment) {
		return err
	}
	checkout.Status = db.CheckoutStatusExpired
	return tx.repo.UpdateCheckout(checkout)
}

// stripeStatus maps Stripe's subscription status to the gateway's. Statuses
// without access (unpaid, paused, incomplete_expired) count as canceled until
// Stripe says otherwise.
func stripeStatus(status string) string {
	switch status {
	case "trialing":
		return db.SubscriptionTrialing
	case "active":
		return db.SubscriptionActive
	case "past_due":
		return db.SubscriptionPastDue
	case "incomplete":
		return db.SubscriptionIncomplete
	default:
		return db.SubscriptionCanceled
	}
}

// onSubscription copies Stripe's view of a subscription, creating the row
// from the subscription's metadata when it arrives before the checkout.
func (s *Service) onSubscription(tx *webhookTx, ev Event) error {
	ss := ev.Subscription
	sub, err := s.subscriptionRow(tx.repo, ss)
	if errors.Is(err, errDuplicateLive) {
		if ss.Status != "canceled" && s.stripe != nil {
			id := ss.ID
			tx.after = append(tx.after, func() {
				_, err := s.stripe.CancelSubscription(context.Background(), id, false)
				if err != nil {
					log.Printf("billing: canceling the duplicate Stripe subscription %s: %v", id, err)
				}
			})
		}
		return nil
	}
	if err != nil {
		return err
	}
	if sub == nil {
		log.Printf("billing: %s for unknown subscription %s (no tg_ metadata), ignored", ev.Type, ss.ID)
		return nil
	}
	if sub.StripeEventAt != nil && ev.Created.Before(*sub.StripeEventAt) {
		log.Printf("billing: %s %s is older than the subscription's state, ignored", ev.Type, ev.ID)
		return nil
	}
	wasCanceled := sub.Status == db.SubscriptionCanceled
	status := stripeStatus(ss.Status)
	if ev.Type == EventSubscriptionDeleted {
		status = db.SubscriptionCanceled
	}
	if wasCanceled && status != db.SubscriptionCanceled && sub.StripeEventAt != nil && !ev.Created.After(*sub.StripeEventAt) {
		log.Printf("billing: %s %s doesn't outrank the cancellation, ignored", ev.Type, ev.ID)
		return nil
	}
	sub.StripeEventAt = &ev.Created
	sub.Status = status
	sub.StripeStatus, sub.StripeCustomerID = ss.Status, ss.CustomerID
	if !ss.CurrentPeriodEnd.IsZero() {
		end := ss.CurrentPeriodEnd
		sub.CurrentPeriodEnd = &end
	}
	sub.TrialEnd = ss.TrialEnd
	if sub.TrialUntil == nil {
		sub.TrialUntil = ss.TrialEnd
	}
	sub.CancelAtPeriodEnd = ss.CancelAtPeriodEnd
	if status != db.SubscriptionPastDue {
		sub.GraceUntil = nil
	}
	if status == db.SubscriptionCanceled && sub.CanceledAt == nil {
		at := ev.Created
		sub.CanceledAt = &at
	}
	if ss.PriceID != "" {
		// A switch in the portal changes the price: follow it to its plan.
		plans, err := tx.repo.ListPlans(false)
		if err != nil {
			return err
		}
		for _, p := range plans {
			if p.StripePriceID == ss.PriceID {
				sub.PlanKey = p.Key
			}
		}
	}
	err = tx.repo.UpdateSubscription(sub)
	if err != nil {
		return err
	}
	tx.touched(sub.Subject)
	if status == db.SubscriptionCanceled && !wasCanceled {
		snapshot := *sub
		tx.after = append(tx.after, func() { s.notifySubscription(&snapshot, NotifyCanceled) })
	}
	return nil
}

// subscriptionRow finds (or creates, from metadata) the row of a Stripe
// subscription; nil when it isn't the gateway's.
func (s *Service) subscriptionRow(repo repositories.BillingRepository, ss *StripeSubscription) (*db.BillingSubscription, error) {
	sub, err := repo.GetSubscriptionByStripeID(ss.ID)
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return sub, err
	}
	stripeID := ss.ID
	// Activation created it for an awaiting row.
	if rowID := ss.Metadata["tg_subscription"]; rowID != "" {
		sub, err := repo.GetSubscription(rowID)
		if err != nil {
			return nil, err
		}
		if sub.Subject != ss.Metadata["tg_subject"] {
			log.Printf("billing: subscription %s metadata names another subject than row %s, ignored", ss.ID, rowID)
			return nil, nil
		}
		sub.StripeSubscriptionID = &stripeID
		return sub, nil
	}
	subject, planKey := ss.Metadata["tg_subject"], ss.Metadata["tg_plan"]
	if subject == "" || planKey == "" {
		return nil, nil
	}
	// The metadata is only a pointer: subject and plan must be what this
	// gateway stored when the checkout started.
	checkout, err := repo.GetCheckout(ss.Metadata["tg_checkout"])
	if err != nil || checkout.Subject != subject || checkout.PlanKey != planKey {
		log.Printf("billing: subscription %s metadata doesn't match a checkout of this gateway, ignored", ss.ID)
		return nil, nil
	}
	plan, err := repo.GetPlan(planKey)
	if err != nil {
		return nil, err
	}
	sub = &db.BillingSubscription{Subject: subject, PlanKey: planKey, GroupKey: planGroup(&plan.BillingPlan), PayerUserID: checkout.UserID, Status: stripeStatus(ss.Status), StripeSubscriptionID: &stripeID}
	err = repo.CreateSubscription(sub)
	if errors.Is(err, repositories.ErrDuplicate) {
		existing, err := repo.GetSubscriptionByStripeID(ss.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("billing: %s already has a live subscription in %s: Stripe subscription %s is not recorded, refund its payment", subject, sub.GroupKey, ss.ID)
			return nil, errDuplicateLive
		}
		return existing, err
	}
	return sub, err
}

// staleForSubscription reports an invoice event older than the last
// subscription event applied: Stripe's later state already covers it.
func staleForSubscription(sub *db.BillingSubscription, ev Event) bool {
	return sub.StripeEventAt != nil && ev.Created.Before(*sub.StripeEventAt)
}

// onInvoicePaid queues the invoice of a subscription charge and ends a
// past-due period.
func (s *Service) onInvoicePaid(tx *webhookTx, ev Event) error {
	inv := ev.Invoice
	if inv.SubscriptionID == "" || inv.AmountCents <= 0 {
		return nil // a trial start, or not a subscription charge
	}
	sub, err := s.invoiceSubscription(tx, ev)
	if err != nil || sub == nil {
		return err
	}
	if sub.Status == db.SubscriptionPastDue && !staleForSubscription(sub, ev) {
		sub.Status, sub.GraceUntil = db.SubscriptionActive, nil
		err = tx.repo.UpdateSubscription(sub)
		if err != nil {
			return err
		}
		tx.touched(sub.Subject)
	}
	lines, err := s.invoiceLines(tx.repo, sub, inv)
	if err != nil {
		return err
	}
	subID := sub.ID
	return EnqueueDocument(tx.repo, &db.BillingDocument{
		Subject: sub.Subject, Kind: db.DocumentInvoice, StripeRef: inv.InvoiceID, SubscriptionID: &subID,
		AmountCents: inv.AmountCents, Currency: currency(inv.Currency), PaidAt: inv.PaidAt,
	}, lines, s.now())
}

// invoiceLines turns a Stripe invoice's lines into document lines: each
// price is matched to its plan (or setup fee), the period added to the
// description.
func (s *Service) invoiceLines(repo repositories.BillingRepository, sub *db.BillingSubscription, inv *InvoiceEvent) ([]DocumentLine, error) {
	plans, err := repo.ListPlans(false)
	if err != nil {
		return nil, err
	}
	var lines []DocumentLine
	sum := int64(0)
	for _, l := range inv.Lines {
		if l.AmountCents == 0 {
			continue
		}
		line := DocumentLine{PlanKey: sub.PlanKey, AmountCents: l.AmountCents, PeriodStart: l.PeriodStart, PeriodEnd: l.PeriodEnd, Proration: l.Proration}
		name := ""
		for _, p := range plans {
			switch l.PriceID {
			case "":
			case p.StripePriceID, p.StripeFirstPeriodPriceID:
				line.PlanKey, name = p.Key, p.Name
			case p.StripeSetupFeePriceID:
				line.PlanKey, line.SetupFee, name = p.Key, true, p.SetupFeeName
			}
		}
		if name == "" || l.Proration {
			name = l.Description
		}
		if !line.SetupFee && !l.Proration && l.PeriodStart != nil && l.PeriodEnd != nil && l.PeriodEnd.After(*l.PeriodStart) {
			name = fmt.Sprintf("%s %s–%s", name, s.day(*l.PeriodStart), s.day(l.PeriodEnd.AddDate(0, 0, -1)))
		}
		line.Description = name
		sum += l.AmountCents
		lines = append(lines, line)
	}
	if len(lines) == 0 || sum != inv.AmountCents {
		// Discounts, prorations or tax lines Stripe adds: one line for the
		// amount charged, so Odoo's total equals the charge.
		plan, err := repo.GetPlan(sub.PlanKey)
		name := sub.PlanKey
		if err == nil {
			name = plan.Name
		}
		lines = []DocumentLine{{Description: name, PlanKey: sub.PlanKey, AmountCents: inv.AmountCents}}
	}
	return lines, nil
}

// onInvoiceFailed starts (or keeps) the grace period of a failed renewal.
func (s *Service) onInvoiceFailed(tx *webhookTx, ev Event) error {
	inv := ev.Invoice
	if inv.SubscriptionID == "" {
		return nil
	}
	sub, err := s.invoiceSubscription(tx, ev)
	if err != nil || sub == nil {
		return err
	}
	if sub.Status == db.SubscriptionCanceled || staleForSubscription(sub, ev) {
		log.Printf("billing: %s %s arrived after the subscription's state, ignored", ev.Type, ev.ID)
		return nil
	}
	plan, err := tx.repo.GetPlan(sub.PlanKey)
	if err != nil {
		return err
	}
	first := sub.GraceUntil == nil
	sub.Status = db.SubscriptionPastDue
	if first {
		until := ev.Created.AddDate(0, 0, plan.GraceDays)
		sub.GraceUntil = &until
	}
	err = tx.repo.UpdateSubscription(sub)
	if err != nil {
		return err
	}
	tx.touched(sub.Subject)
	if first {
		snapshot := *sub
		tx.after = append(tx.after, func() { s.notifySubscription(&snapshot, NotifyPaymentFailed) })
	}
	return nil
}

// cancelAwaiting cancels a subscription still awaiting activation whose
// purchase was refunded, so it can no longer be activated or grant access.
func cancelAwaiting(repo repositories.BillingRepository, id string, at time.Time) error {
	sub, err := repo.GetSubscription(id)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && sub.Status != db.SubscriptionAwaitingActivation) {
		return nil
	}
	if err != nil {
		return err
	}
	sub.Status, sub.CanceledAt = db.SubscriptionCanceled, &at
	return repo.UpdateSubscription(sub)
}

// onRefund queues a credit note for what was newly refunded and marks a
// fully refunded purchase.
// refundWaitsFor is how long a refund of an unknown charge is retried: its
// invoice may still be on its way; after that it is not the gateway's.
const refundWaitsFor = time.Hour

func (s *Service) onRefund(tx *webhookTx, ev Event) error {
	r := ev.Refund
	// The invoice the charge paid (a subscription charge) or the payment
	// intent (a Checkout payment, invoiced by the gateway itself).
	var invoice *db.BillingDocument
	err := gorm.ErrRecordNotFound
	for _, ref := range []string{r.InvoiceID, r.PaymentIntentID} {
		if ref != "" && errors.Is(err, gorm.ErrRecordNotFound) {
			invoice, err = tx.repo.GetDocumentByStripeRef(ref)
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if s.now().Sub(ev.Created) < refundWaitsFor {
			return retryLater("charge.refunded for " + r.ChargeID + " before its invoice")
		}
		log.Printf("billing: charge.refunded for %s, which the gateway didn't invoice, ignored", r.ChargeID)
		return nil
	}
	if err != nil {
		return err
	}
	if r.PaymentIntentID != "" {
		p, err := tx.repo.GetPurchaseByPaymentIntent(r.PaymentIntentID)
		if err == nil {
			p.RefundedCents = max(p.RefundedCents, r.RefundedCents)
			if p.RefundedCents >= p.AmountCents {
				p.Status = db.PurchaseStatusRefunded
			}
			err = tx.repo.UpdatePurchase(p)
			if err == nil && p.Status == db.PurchaseStatusRefunded && p.SubscriptionID != nil {
				err = cancelAwaiting(tx.repo, *p.SubscriptionID, ev.Created)
			}
			tx.touched(p.Subject)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	credited, err := tx.repo.CreditedCents(invoice.ID)
	if err != nil {
		return err
	}
	amount := min(r.RefundedCents, invoice.AmountCents) - credited
	if amount <= 0 {
		return nil
	}
	lines, err := DocumentLines(invoice)
	if err != nil || len(lines) == 0 {
		return fmt.Errorf("lines of invoice document %s: %w", invoice.ID, err)
	}
	description := "Rectificación de la factura"
	if invoice.Number != "" {
		description += " " + invoice.Number
	}
	creditRef := r.RefundID
	if creditRef == "" {
		creditRef = fmt.Sprintf("%s-refund-%d", r.ChargeID, r.RefundedCents)
	}
	invoiceID := invoice.ID
	return EnqueueDocument(tx.repo, &db.BillingDocument{
		Subject: invoice.Subject, Kind: db.DocumentCreditNote, StripeRef: creditRef, InvoiceDocumentID: &invoiceID,
		SubscriptionID: invoice.SubscriptionID, PurchaseID: invoice.PurchaseID, AmountCents: amount,
		Currency: invoice.Currency, PaidAt: ev.Created,
	}, []DocumentLine{{Description: description, PlanKey: lines[0].PlanKey, AmountCents: amount}}, s.now())
}
