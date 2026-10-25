package billing

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"gorm.io/gorm"
)

// ErrForbidden: the user may not buy for that subject.
var ErrForbidden = errors.New("forbidden")

// ErrAlreadySubscribed: the subject already has a live subscription in the
// plan's group; the pay page offers the portal instead.
var ErrAlreadySubscribed = errors.New("already subscribed")

// Payer is the logged-in user on the pay page.
type Payer struct {
	UserID string
	Name   string
	Email  string
}

// CheckoutRequest is a press of Pay.
type CheckoutRequest struct {
	PlanKey string
	Payer   Payer
	// OrgID buys for that organization; "" buys for the payer.
	OrgID string
	// Redirect is where /_/pay/done links to (a path on this site).
	Redirect string
}

// SafeRedirect keeps redirect only when it is a path on this site.
func SafeRedirect(redirect string) string {
	if !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") || strings.ContainsAny(redirect, "\\\r\n") {
		return ""
	}
	return redirect
}

// PaySubject decides who a purchase of plan is for: the organization orgID
// (the user must belong to it) or the user.
func (s *Service) PaySubject(plan *repositories.BillingPlanWithFeatures, userID, orgID string) (Subject, error) {
	if orgID == "" {
		if plan.Subject == db.PlanSubjectOrg {
			return "", fmt.Errorf("%w: this plan is bought for an organization", ErrInvalid)
		}
		return UserSubject(userID), nil
	}
	if plan.Subject == db.PlanSubjectUser {
		return "", fmt.Errorf("%w: this plan is bought for yourself, not an organization", ErrInvalid)
	}
	if s.orgs == nil {
		return "", ErrForbidden
	}
	_, err := s.orgs.Member(orgID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("%w: you are not a member of that organization", ErrForbidden)
	}
	if err != nil {
		return "", err
	}
	return OrgSubject(orgID), nil
}

// planGroup is the trial and switching group of a plan: its group, or the
// plan alone.
func planGroup(plan *db.BillingPlan) string {
	if plan.GroupKey != "" {
		return plan.GroupKey
	}
	return "plan:" + plan.Key
}

// LiveSubscription returns subject's subscription in plan's group that
// isn't canceled, or nil.
func (s *Service) LiveSubscription(subject string, plan *repositories.BillingPlanWithFeatures) (*db.BillingSubscription, error) {
	if plan.Kind != db.PlanKindSubscription {
		return nil, nil
	}
	subs, err := s.repo.ListSubscriptions(repositories.SubscriptionFilter{Subject: subject})
	if err != nil {
		return nil, err
	}
	group := planGroup(&plan.BillingPlan)
	for i := range subs {
		if subs[i].Status == db.SubscriptionCanceled {
			continue
		}
		if subs[i].PlanKey == plan.Key {
			return &subs[i], nil
		}
		other, err := s.repo.GetPlan(subs[i].PlanKey)
		if err == nil && planGroup(&other.BillingPlan) == group {
			return &subs[i], nil
		}
	}
	return nil, nil
}

func hadTrial(c *db.BillingCustomer, group string) bool {
	return c != nil && slices.Contains(strings.Split(c.TrialGroups, ","), group)
}

// TrialDaysFor is the trial subject would get for plan now: the plan's,
// once per group.
func (s *Service) TrialDaysFor(subject string, plan *repositories.BillingPlanWithFeatures) int {
	if plan.Kind != db.PlanKindSubscription || plan.Starts != db.PlanStartsOnPayment || plan.TrialDays == 0 {
		return 0
	}
	c, err := s.repo.GetCustomer(subject)
	if err == nil && hadTrial(c, planGroup(&plan.BillingPlan)) {
		return 0
	}
	return plan.TrialDays
}

// StartCheckout creates the Stripe Checkout Session for a press of Pay and
// returns the gateway checkout (its StripeSessionID set) and the Stripe URL.
// Amounts come from the plan, never from the browser.
func (s *Service) StartCheckout(ctx context.Context, req CheckoutRequest) (*db.BillingCheckout, string, error) {
	if s.stripe == nil {
		return nil, "", ErrNotConfigured
	}
	plan, err := s.GetPlan(req.PlanKey)
	if err != nil {
		return nil, "", err
	}
	if !plan.Active {
		return nil, "", fmt.Errorf("%w: plan %s is no longer sold", ErrInvalid, plan.Key)
	}
	subject, err := s.PaySubject(plan, req.Payer.UserID, req.OrgID)
	if err != nil {
		return nil, "", err
	}
	live, err := s.LiveSubscription(string(subject), plan)
	if err != nil {
		return nil, "", err
	}
	if live != nil {
		return nil, "", ErrAlreadySubscribed
	}

	customer, err := s.ensureStripeCustomer(ctx, subject, req.Payer)
	if err != nil {
		return nil, "", err
	}
	checkout := &db.BillingCheckout{
		Subject: string(subject), PlanKey: plan.Key, UserID: req.Payer.UserID,
		Status: db.CheckoutStatusOpen, Redirect: SafeRedirect(req.Redirect),
	}
	err = s.repo.CreateCheckout(checkout)
	if err != nil {
		return nil, "", err
	}

	cancel := s.serverURL + "/_/pay/" + plan.Key
	if req.OrgID != "" {
		cancel += "?org=" + url.QueryEscape(req.OrgID)
	}
	params := CheckoutParams{
		CheckoutID: checkout.ID, CustomerID: customer.StripeCustomerID, CollectShipping: plan.CollectShipping,
		SuccessURL: s.serverURL + "/_/pay/done?checkout=" + url.QueryEscape(checkout.ID), CancelURL: cancel,
		Metadata: map[string]string{"tg_checkout": checkout.ID, "tg_subject": string(subject), "tg_plan": plan.Key},
	}
	switch {
	case plan.Kind == db.PlanKindSubscription && plan.Starts == db.PlanStartsOnPayment:
		params.Mode = "subscription"
		params.Lines = []CheckoutLine{{PriceID: plan.StripePriceID, Quantity: 1}}
		params.TrialDays = s.TrialDaysFor(string(subject), plan)
		if plan.StripeSetupFeePriceID != "" {
			params.Lines = append(params.Lines, CheckoutLine{PriceID: plan.StripeSetupFeePriceID, Quantity: 1})
		}
	case plan.Kind == db.PlanKindSubscription:
		params.Mode = "payment"
		params.SaveCard = true
		if plan.StripeSetupFeePriceID != "" {
			params.Lines = append(params.Lines, CheckoutLine{PriceID: plan.StripeSetupFeePriceID, Quantity: 1})
		}
		params.Lines = append(params.Lines, CheckoutLine{PriceID: plan.StripeFirstPeriodPriceID, Quantity: 1})
	default:
		params.Mode = "payment"
		params.Lines = []CheckoutLine{{PriceID: plan.StripePriceID, Quantity: 1}}
	}
	session, err := s.stripe.CreateCheckoutSession(ctx, params)
	if err != nil {
		return nil, "", err
	}
	checkout.StripeSessionID = session.ID
	err = s.repo.UpdateCheckout(checkout)
	if err != nil {
		return nil, "", err
	}
	return checkout, session.URL, nil
}

// ensureStripeCustomer returns subject's billing customer with its Stripe
// customer, creating both when missing. An organization's customer is named
// after it; the payer's email receives Stripe's emails.
func (s *Service) ensureStripeCustomer(ctx context.Context, subject Subject, payer Payer) (*db.BillingCustomer, error) {
	c, err := s.repo.GetCustomer(string(subject))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c, err = &db.BillingCustomer{Subject: string(subject)}, nil
	}
	if err != nil {
		return nil, err
	}
	name := payer.Name
	kind, id, _ := ParseSubject(string(subject))
	if kind == SubjectOrg && s.orgs != nil {
		org, err := s.orgs.Get(id)
		if err == nil {
			name = org.Name
		}
	}
	if c.Name != "" {
		name = c.Name // as Checkout last collected it
	}
	stripeID, err := s.stripe.EnsureCustomer(ctx, c.StripeCustomerID, string(subject), name, payer.Email)
	if err != nil {
		return nil, err
	}
	if c.StripeCustomerID != stripeID || c.Email == "" {
		c.StripeCustomerID = stripeID
		if c.Email == "" {
			c.Email = payer.Email
		}
		if c.Name == "" {
			c.Name = name
		}
		err = s.repo.SaveCustomer(c)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

// GetCheckout returns a checkout of userID (ErrNotFound for anyone else's).
func (s *Service) GetCheckout(id, userID string) (*db.BillingCheckout, error) {
	c, err := s.repo.GetCheckout(id)
	if err != nil {
		return nil, notFound(err)
	}
	if c.UserID != userID {
		return nil, ErrNotFound
	}
	return c, nil
}

// PortalURL returns a Stripe Customer Portal URL for subject, for a user
// allowed to manage it (the user themselves, or an owner of the
// organization).
func (s *Service) PortalURL(ctx context.Context, subject, userID string) (string, error) {
	if s.stripe == nil {
		return "", ErrNotConfigured
	}
	kind, id, err := ParseSubject(subject)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	switch {
	case kind == SubjectUser && id != userID:
		return "", ErrForbidden
	case kind == SubjectOrg:
		if s.orgs == nil {
			return "", ErrForbidden
		}
		m, err := s.orgs.Member(id, userID)
		if err != nil || m.Role != "owner" {
			return "", ErrForbidden
		}
	}
	c, err := s.repo.GetCustomer(subject)
	if err != nil || c.StripeCustomerID == "" {
		return "", fmt.Errorf("%w: no Stripe customer for %s", ErrNotFound, subject)
	}
	return s.stripe.PortalURL(ctx, c.StripeCustomerID, s.serverURL+"/_/account")
}
