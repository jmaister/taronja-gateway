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
// yet (events arrive out of order); the webhook answers 500 so Stripe
// sends it again later.
var ErrRetryLater = errors.New("event arrived before what it refers to; retry later")

// webhookTx is one event's state change: the repository bound to the
// transaction and what to do after it commits.
type webhookTx struct {
	repo       repositories.BillingRepository
	invalidate map[string]bool
	after      []func()
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
	err := s.repo.Transaction(func(repo repositories.BillingRepository) error {
		tx.repo = repo
		fresh, err := repo.RecordStripeEvent(ev.ID, ev.Type)
		if err != nil || !fresh {
			return err
		}
		switch {
		case ev.Checkout != nil && ev.Type == EventCheckoutCompleted:
			return s.onCheckoutCompleted(tx, ev)
		case ev.Checkout != nil && ev.Type == EventCheckoutExpired:
			return s.onCheckoutExpired(tx, ev.Checkout)
		case ev.Subscription != nil:
			return s.onSubscription(tx, ev)
		case ev.Invoice != nil && ev.Type == EventInvoicePaid:
			return s.onInvoicePaid(tx, ev.Invoice)
		case ev.Invoice != nil && ev.Type == EventInvoicePaymentFailed:
			return s.onInvoiceFailed(tx, ev)
		case ev.Refund != nil:
			return s.onRefund(tx, ev)
		}
		return nil
	})
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
	if checkout.Status == db.CheckoutStatusCompleted || !cev.Paid {
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
	if cev.TaxID != "" {
		c.TaxID = cev.TaxID
	}
	if cev.BillingAddress != (Address{}) {
		a := cev.BillingAddress
		c.AddressLine1, c.AddressLine2, c.PostalCode, c.City, c.State, c.Country = a.Line1, a.Line2, a.PostalCode, a.City, a.State, a.Country
	}
	return repo.SaveCustomer(c)
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
			Subject: checkout.Subject, PlanKey: plan.Key, PayerUserID: checkout.UserID,
			Status: db.SubscriptionAwaitingActivation, StripePaymentMethodID: cev.PaymentMethodID,
		}
		err := tx.repo.CreateSubscription(sub)
		if err != nil {
			return err
		}
		p.SubscriptionID = &sub.ID
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
		description = fmt.Sprintf("%s %s–%s", plan.Name, spanishDate(paidAt), spanishDate(paidAt.AddDate(0, 0, plan.AccessDays)))
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

func spanishDate(t time.Time) string {
	return t.UTC().Format("02/01/2006")
}

func (s *Service) onCheckoutExpired(tx *webhookTx, cev *CheckoutEvent) error {
	checkout, err := s.findCheckout(tx.repo, cev)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || checkout.Status != db.CheckoutStatusOpen {
		return err
	}
	checkout.Status = db.CheckoutStatusExpired
	return tx.repo.UpdateCheckout(checkout)
}

// stripeStatus maps Stripe's subscription status to the gateway's. Statuses
// without access (incomplete, unpaid, paused, incomplete_expired) count as
// canceled until Stripe says otherwise.
func stripeStatus(status string) string {
	switch status {
	case "trialing":
		return db.SubscriptionTrialing
	case "active":
		return db.SubscriptionActive
	case "past_due":
		return db.SubscriptionPastDue
	default:
		return db.SubscriptionCanceled
	}
}

// onSubscription copies Stripe's view of a subscription, creating the row
// from the subscription's metadata when it arrives before the checkout.
func (s *Service) onSubscription(tx *webhookTx, ev Event) error {
	ss := ev.Subscription
	sub, err := s.subscriptionRow(tx.repo, ss)
	if err != nil {
		return err
	}
	if sub == nil {
		log.Printf("billing: %s for unknown subscription %s (no tg_ metadata), ignored", ev.Type, ss.ID)
		return nil
	}
	wasCanceled := sub.Status == db.SubscriptionCanceled
	status := stripeStatus(ss.Status)
	if ev.Type == EventSubscriptionDeleted {
		status = db.SubscriptionCanceled
	}
	sub.Status = status
	if !ss.CurrentPeriodEnd.IsZero() {
		end := ss.CurrentPeriodEnd
		sub.CurrentPeriodEnd = &end
	}
	sub.TrialEnd = ss.TrialEnd
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
		sub.StripeSubscriptionID = &stripeID
		return sub, nil
	}
	subject, planKey := ss.Metadata["tg_subject"], ss.Metadata["tg_plan"]
	if subject == "" || planKey == "" {
		return nil, nil
	}
	payer := ""
	if checkoutID := ss.Metadata["tg_checkout"]; checkoutID != "" {
		checkout, err := repo.GetCheckout(checkoutID)
		if err == nil {
			payer = checkout.UserID
		}
	}
	sub = &db.BillingSubscription{Subject: subject, PlanKey: planKey, PayerUserID: payer, Status: stripeStatus(ss.Status), StripeSubscriptionID: &stripeID}
	err = repo.CreateSubscription(sub)
	if errors.Is(err, repositories.ErrDuplicate) {
		return repo.GetSubscriptionByStripeID(ss.ID)
	}
	return sub, err
}

// onInvoicePaid queues the invoice of a subscription charge and ends a
// past-due period.
func (s *Service) onInvoicePaid(tx *webhookTx, inv *InvoiceEvent) error {
	if inv.SubscriptionID == "" || inv.AmountCents <= 0 {
		return nil // a trial start, or not a subscription charge
	}
	sub, err := tx.repo.GetSubscriptionByStripeID(inv.SubscriptionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRetryLater
	}
	if err != nil {
		return err
	}
	if sub.Status == db.SubscriptionPastDue {
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
		line := DocumentLine{PlanKey: sub.PlanKey, AmountCents: l.AmountCents, PeriodStart: l.PeriodStart, PeriodEnd: l.PeriodEnd}
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
		if name == "" {
			name = l.Description
		}
		if !line.SetupFee && l.PeriodStart != nil && l.PeriodEnd != nil && l.PeriodEnd.After(*l.PeriodStart) {
			name = fmt.Sprintf("%s %s–%s", name, spanishDate(*l.PeriodStart), spanishDate(l.PeriodEnd.AddDate(0, 0, -1)))
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
	sub, err := tx.repo.GetSubscriptionByStripeID(inv.SubscriptionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRetryLater
	}
	if err != nil {
		return err
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

// onRefund queues a credit note for what was newly refunded and marks a
// fully refunded purchase.
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
		log.Printf("billing: charge.refunded for %s, which the gateway didn't invoice, ignored", r.ChargeID)
		return nil
	}
	if err != nil {
		return err
	}
	if r.PaymentIntentID != "" {
		p, err := tx.repo.GetPurchaseByPaymentIntent(r.PaymentIntentID)
		if err == nil {
			p.RefundedCents = r.RefundedCents
			if p.RefundedCents >= p.AmountCents {
				p.Status = db.PurchaseStatusRefunded
			}
			err = tx.repo.UpdatePurchase(p)
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
