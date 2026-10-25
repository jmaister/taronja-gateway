package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"gorm.io/gorm"
)

// PlanInput is a plan as the dashboard edits it.
type PlanInput struct {
	Key             string
	Name            string
	Description     string
	Kind            string
	Interval        string
	AmountCents     int64
	Features        []string
	Subject         string
	TrialDays       int
	GraceDays       int
	SetupFeeCents   int64
	SetupFeeName    string
	Starts          string
	AccessDays      int
	CollectShipping bool
	Group           string
	Active          bool
}

// DefaultGraceDays is a new plan's grace when none is given.
const DefaultGraceDays = 7

var planKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)

// PlanInputFrom returns plan as an input, to apply a partial update to.
func PlanInputFrom(p repositories.BillingPlanWithFeatures) PlanInput {
	return PlanInput{
		Key: p.Key, Name: p.Name, Description: p.Description, Kind: p.Kind, Interval: p.Interval,
		AmountCents: p.AmountCents, Features: slices.Clone(p.Features), Subject: p.Subject,
		TrialDays: p.TrialDays, GraceDays: p.GraceDays, SetupFeeCents: p.SetupFeeCents, SetupFeeName: p.SetupFeeName,
		Starts: p.Starts, AccessDays: p.AccessDays, CollectShipping: p.CollectShipping, Group: p.GroupKey, Active: p.Active,
	}
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// normalize fills defaults and validates in.
func (in *PlanInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.SetupFeeName = strings.TrimSpace(in.SetupFeeName)
	in.Group = strings.TrimSpace(in.Group)
	if in.Subject == "" {
		in.Subject = db.PlanSubjectEither
	}
	if !planKeyPattern.MatchString(in.Key) {
		return invalid("key %q: 1-100 lowercase letters, digits or '-'", in.Key)
	}
	if in.Name == "" || len(in.Name) > 200 {
		return invalid("name: 1-200 characters")
	}
	if in.AmountCents <= 0 {
		return invalid("amount must be positive")
	}
	if !slices.Contains([]string{db.PlanSubjectUser, db.PlanSubjectOrg, db.PlanSubjectEither}, in.Subject) {
		return invalid("subject must be user, org or either")
	}
	if len(in.Features) == 0 {
		return invalid("a plan grants at least one feature")
	}
	seen := map[string]bool{}
	features := in.Features[:0]
	for _, f := range in.Features {
		f = strings.TrimSpace(f)
		err := ValidateFeature(f)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		if !seen[f] {
			seen[f] = true
			features = append(features, f)
		}
	}
	slices.Sort(features)
	in.Features = features
	if in.GraceDays < 0 || in.GraceDays > 60 {
		return invalid("grace days: 0-60")
	}
	if in.SetupFeeCents < 0 {
		return invalid("setup fee can't be negative")
	}
	if in.SetupFeeCents > 0 && in.SetupFeeName == "" {
		return invalid("a setup fee needs a name")
	}

	switch in.Kind {
	case db.PlanKindSubscription:
		if in.Interval != db.PlanIntervalMonth && in.Interval != db.PlanIntervalYear {
			return invalid("interval must be month or year")
		}
		if in.Starts == "" {
			in.Starts = db.PlanStartsOnPayment
		}
		if in.Starts != db.PlanStartsOnPayment && in.Starts != db.PlanStartsOnActivation {
			return invalid("starts must be on_payment or on_activation")
		}
		if in.TrialDays < 0 || in.TrialDays > 730 {
			return invalid("trial days: 0-730")
		}
		if in.Starts == db.PlanStartsOnActivation && in.TrialDays > 0 {
			return invalid("a plan that starts on activation has no trial")
		}
		if in.AccessDays != 0 {
			return invalid("access days are for one-time plans")
		}
	case db.PlanKindOneTime:
		if in.Interval != "" || in.TrialDays != 0 || in.SetupFeeCents != 0 || (in.Starts != "" && in.Starts != db.PlanStartsOnPayment) {
			return invalid("one-time plans have no interval, trial, setup fee or activation")
		}
		in.Starts = db.PlanStartsOnPayment
		if in.AccessDays < 0 {
			return invalid("access days can't be negative")
		}
	default:
		return invalid("kind must be subscription or one_time")
	}
	return nil
}

// ListPlans returns every plan (or only active ones).
func (s *Service) ListPlans(activeOnly bool) ([]repositories.BillingPlanWithFeatures, error) {
	return s.repo.ListPlans(activeOnly)
}

// GetPlan returns a plan, or ErrNotFound.
func (s *Service) GetPlan(key string) (*repositories.BillingPlanWithFeatures, error) {
	p, err := s.repo.GetPlan(key)
	if err != nil {
		return nil, notFound(err)
	}
	return p, nil
}

// CreatePlan validates in, creates its Stripe product and prices and its
// Odoo products, and stores it.
func (s *Service) CreatePlan(ctx context.Context, in PlanInput) (*repositories.BillingPlanWithFeatures, error) {
	err := in.normalize()
	if err != nil {
		return nil, err
	}
	_, err = s.repo.GetPlan(in.Key)
	if err == nil {
		return nil, fmt.Errorf("%w: plan %q already exists", ErrConflict, in.Key)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if s.stripe == nil {
		return nil, ErrNotConfigured
	}
	plan := &repositories.BillingPlanWithFeatures{BillingPlan: db.BillingPlan{Key: in.Key, Currency: "eur"}}
	applyInput(plan, in)
	err = s.syncStripe(ctx, plan, nil)
	if err != nil {
		return nil, err
	}
	s.syncOdooProducts(ctx, plan)
	err = s.repo.SavePlan(plan)
	if err != nil {
		return nil, err
	}
	s.afterPlanChange(ctx)
	return plan, nil
}

// UpdatePlan replaces the plan key with in (build it with PlanInputFrom
// and change what the request changes). A new amount or interval makes new
// Stripe prices; existing subscriptions keep theirs.
func (s *Service) UpdatePlan(ctx context.Context, key string, in PlanInput) (*repositories.BillingPlanWithFeatures, error) {
	current, err := s.GetPlan(key)
	if err != nil {
		return nil, err
	}
	in.Key = key
	err = in.normalize()
	if err != nil {
		return nil, err
	}
	if in.Kind != current.Kind {
		return nil, invalid("a plan's kind can't change; create another plan")
	}
	if s.stripe == nil {
		return nil, ErrNotConfigured
	}
	before := *current
	before.Features = slices.Clone(current.Features)
	plan := current
	applyInput(plan, in)
	err = s.syncStripe(ctx, plan, &before)
	if err != nil {
		return nil, err
	}
	if plan.Name != before.Name || plan.SetupFeeName != before.SetupFeeName || plan.OdooProductID == nil ||
		(plan.SetupFeeCents > 0 && plan.OdooSetupFeeProductID == nil) {
		s.syncOdooProducts(ctx, plan)
	}
	err = s.repo.SavePlan(plan)
	if err != nil {
		return nil, err
	}
	s.afterPlanChange(ctx)
	return plan, nil
}

func applyInput(p *repositories.BillingPlanWithFeatures, in PlanInput) {
	p.Name, p.Description, p.Kind, p.Interval = in.Name, in.Description, in.Kind, in.Interval
	p.AmountCents, p.Subject, p.TrialDays, p.GraceDays = in.AmountCents, in.Subject, in.TrialDays, in.GraceDays
	p.SetupFeeCents, p.SetupFeeName, p.Starts, p.AccessDays = in.SetupFeeCents, in.SetupFeeName, in.Starts, in.AccessDays
	p.CollectShipping, p.GroupKey, p.Active, p.Features = in.CollectShipping, in.Group, in.Active, in.Features
}

// syncStripe makes plan's Stripe product and prices match it; before is
// the stored plan (nil for a new one). Prices can't change in Stripe, so a
// changed amount makes a new price, takes over the lookup key and archives
// the old one.
func (s *Service) syncStripe(ctx context.Context, plan, before *repositories.BillingPlanWithFeatures) error {
	if before == nil || plan.Name != before.Name || plan.StripeProductID == "" {
		id, err := s.stripe.EnsureProduct(ctx, plan.StripeProductID, plan.Name, plan.Key)
		if err != nil {
			return err
		}
		plan.StripeProductID = id
	}
	price := func(current *string, changed bool, p StripePriceParams) error {
		if *current != "" && !changed {
			return nil
		}
		p.ProductID, p.Currency = plan.StripeProductID, plan.Currency
		id, err := s.stripe.CreatePrice(ctx, p)
		if err != nil {
			return err
		}
		if *current != "" {
			err = s.stripe.ArchivePrice(ctx, *current)
			if err != nil {
				log.Printf("billing: archiving Stripe price %s: %v", *current, err)
			}
		}
		*current = id
		return nil
	}
	amountChanged := before != nil && (plan.AmountCents != before.AmountCents || plan.Interval != before.Interval)

	mainInterval := ""
	if plan.Kind == db.PlanKindSubscription {
		mainInterval = plan.Interval
	}
	err := price(&plan.StripePriceID, amountChanged, StripePriceParams{
		LookupKey: "tg_" + plan.Key, AmountCents: plan.AmountCents, Interval: mainInterval, Nickname: plan.Name,
	})
	if err != nil {
		return err
	}
	if plan.Kind == db.PlanKindSubscription && plan.Starts == db.PlanStartsOnActivation {
		err = price(&plan.StripeFirstPeriodPriceID, amountChanged, StripePriceParams{
			LookupKey: "tg_" + plan.Key + "_first", AmountCents: plan.AmountCents, Nickname: plan.Name + " (first period)",
		})
		if err != nil {
			return err
		}
	} else {
		plan.StripeFirstPeriodPriceID = ""
	}
	if plan.SetupFeeCents > 0 {
		feeChanged := before != nil && plan.SetupFeeCents != before.SetupFeeCents
		err = price(&plan.StripeSetupFeePriceID, feeChanged, StripePriceParams{
			LookupKey: "tg_" + plan.Key + "_setup", AmountCents: plan.SetupFeeCents, Nickname: plan.SetupFeeName,
		})
		if err != nil {
			return err
		}
	} else {
		plan.StripeSetupFeePriceID = ""
	}
	return nil
}

// OdooProductCode is the Odoo product code of a plan (and of its setup fee).
func OdooProductCode(planKey string, setupFee bool) string {
	if setupFee {
		return "TG-" + planKey + "-setup"
	}
	return "TG-" + planKey
}

// syncOdooProducts creates or renames the plan's Odoo products. Odoo being
// down doesn't stop the plan: the outbox creates them before invoicing.
func (s *Service) syncOdooProducts(ctx context.Context, plan *repositories.BillingPlanWithFeatures) {
	if s.odoo == nil {
		return
	}
	id, err := s.odoo.UpsertProduct(ctx, OdooProductCode(plan.Key, false), plan.Name)
	if err != nil {
		log.Printf("billing: Odoo product for plan %s: %v (retried when invoicing)", plan.Key, err)
		return
	}
	plan.OdooProductID = &id
	if plan.SetupFeeCents > 0 {
		id, err := s.odoo.UpsertProduct(ctx, OdooProductCode(plan.Key, true), plan.SetupFeeName)
		if err != nil {
			log.Printf("billing: Odoo setup fee product for plan %s: %v (retried when invoicing)", plan.Key, err)
			return
		}
		plan.OdooSetupFeeProductID = &id
	}
}

// afterPlanChange refreshes what depends on the plans: entitlements (a
// plan's features may have changed) and the portal's switchable plans.
func (s *Service) afterPlanChange(ctx context.Context) {
	s.InvalidateAll()
	err := s.SyncPortal(ctx)
	if err != nil {
		log.Printf("billing: updating the Stripe portal configuration: %v", err)
	}
}

// SyncPortal creates or updates the Stripe Customer Portal configuration:
// subscribers may switch between the active subscription plans of their
// plan's group.
func (s *Service) SyncPortal(ctx context.Context) error {
	if s.stripe == nil {
		return ErrNotConfigured
	}
	plans, err := s.repo.ListPlans(true)
	if err != nil {
		return err
	}
	groups := map[string][]repositories.BillingPlanWithFeatures{}
	for _, p := range plans {
		if p.Kind == db.PlanKindSubscription && p.GroupKey != "" && p.Starts == db.PlanStartsOnPayment && p.StripePriceID != "" {
			groups[p.GroupKey] = append(groups[p.GroupKey], p)
		}
	}
	switchable := map[string][]string{}
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		for _, p := range members {
			switchable[p.StripeProductID] = append(switchable[p.StripeProductID], p.StripePriceID)
		}
	}
	_, err = s.stripe.ConfigurePortal(ctx, PortalParams{ReturnURL: s.serverURL + "/_/account", SwitchProducts: switchable})
	return err
}
