package billing

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
)

// HeaderName is the header proxied requests carry with the logged-in user's
// active entitlements; client-sent copies are dropped.
const HeaderName = "X-Entitlements"

// Entitlement statuses and sources (doc/billing.md, Entitlements).
const (
	StatusTrial              = "trial"
	StatusActive             = "active"
	StatusCanceling          = "canceling"
	StatusPastDue            = "past_due"
	StatusAwaitingActivation = "awaiting_activation"
	StatusGranted            = "granted"

	SourceSubscription = "subscription"
	SourcePurchase     = "purchase"
	SourceGrant        = "grant"
)

// Feature is one entitlement: active while now < Until, or forever when
// Until is nil.
type Feature struct {
	Until  *time.Time `json:"until"`
	Status string     `json:"status"`
	Source string     `json:"source"`
}

// Entitlements maps a feature key to the source that grants it longest.
type Entitlements map[string]Feature

// Active returns the features active at now.
func (e Entitlements) Active(now time.Time) Entitlements {
	out := Entitlements{}
	for key, f := range e {
		if f.Until == nil || now.Before(*f.Until) {
			out[key] = f
		}
	}
	return out
}

// UserEntitlements is a user's own entitlements and those of each
// organization they belong to, kept apart.
type UserEntitlements struct {
	User Entitlements
	Orgs map[string]Entitlements
}

// Header is the X-Entitlements value: the until of each active feature
// (null = no expiry); organizations without any are left out.
type Header struct {
	User map[string]*time.Time            `json:"user"`
	Orgs map[string]map[string]*time.Time `json:"orgs"`
}

func untils(e Entitlements) map[string]*time.Time {
	out := make(map[string]*time.Time, len(e))
	for key, f := range e {
		if f.Until != nil {
			u := f.Until.UTC().Truncate(time.Second)
			out[key] = &u
		} else {
			out[key] = nil
		}
	}
	return out
}

// Header returns u's active features at now in header form.
func (u UserEntitlements) Header(now time.Time) Header {
	h := Header{User: untils(u.User.Active(now)), Orgs: map[string]map[string]*time.Time{}}
	for orgID, e := range u.Orgs {
		active := e.Active(now)
		if len(active) > 0 {
			h.Orgs[orgID] = untils(active)
		}
	}
	return h
}

// HeaderValue is Header encoded as JSON.
func (u UserEntitlements) HeaderValue(now time.Time) string {
	b, _ := json.Marshal(u.Header(now))
	return string(b)
}

func addDays(t time.Time, days int) time.Time {
	return t.AddDate(0, 0, days)
}

// Compute applies doc/billing.md's rules to a subject's subscriptions,
// purchases and grants: each feature gets the latest until among its
// sources, nil (no expiry) beating any date. Sources already expired at
// now are left out.
func Compute(now time.Time, plans map[string]repositories.BillingPlanWithFeatures, subs []db.BillingSubscription, purchases []db.BillingPurchase, grants []db.BillingGrant) Entitlements {
	out := Entitlements{}
	grant := func(feature string, f Feature) {
		if f.Until != nil && !now.Before(*f.Until) {
			return
		}
		cur, ok := out[feature]
		if !ok || (cur.Until != nil && (f.Until == nil || f.Until.After(*cur.Until))) {
			out[feature] = f
		}
	}

	for _, s := range subs {
		plan, ok := plans[s.PlanKey]
		if !ok {
			continue
		}
		var f Feature
		switch s.Status {
		case db.SubscriptionTrialing:
			if s.TrialEnd == nil {
				continue
			}
			f = Feature{Until: s.TrialEnd, Status: StatusTrial}
		case db.SubscriptionActive:
			if s.CurrentPeriodEnd == nil {
				continue
			}
			if s.CancelAtPeriodEnd {
				f = Feature{Until: s.CurrentPeriodEnd, Status: StatusCanceling}
			} else {
				until := addDays(*s.CurrentPeriodEnd, plan.GraceDays)
				f = Feature{Until: &until, Status: StatusActive}
			}
		case db.SubscriptionPastDue:
			until := s.GraceUntil
			if until == nil && s.CurrentPeriodEnd != nil {
				u := addDays(*s.CurrentPeriodEnd, plan.GraceDays)
				until = &u
			}
			if until == nil {
				continue
			}
			f = Feature{Until: until, Status: StatusPastDue}
		case db.SubscriptionAwaitingActivation:
			f = Feature{Status: StatusAwaitingActivation}
		default:
			continue
		}
		f.Source = SourceSubscription
		for _, feature := range plan.Features {
			grant(feature, f)
		}
	}

	for _, p := range purchases {
		plan, ok := plans[p.PlanKey]
		// A subscription's first payment grants nothing itself: its
		// subscription does.
		if !ok || p.Status != db.PurchaseStatusPaid || plan.Kind != db.PlanKindOneTime || p.SubscriptionID != nil {
			continue
		}
		f := Feature{Status: StatusActive, Source: SourcePurchase}
		if plan.AccessDays > 0 {
			until := addDays(p.PaidAt, plan.AccessDays)
			f.Until = &until
		}
		for _, feature := range plan.Features {
			grant(feature, f)
		}
	}

	for _, g := range grants {
		grant(g.Feature, Feature{Until: g.Until, Status: StatusGranted, Source: SourceGrant})
	}
	return out
}

// SubjectEntitlements returns subject's entitlements (cached; expired
// features still have to be filtered with Active).
func (s *Service) SubjectEntitlements(subject string) (Entitlements, error) {
	s.mu.RLock()
	e, ok := s.cache[subject]
	s.mu.RUnlock()
	if ok {
		return e, nil
	}
	computed, err := s.computeSubjects([]string{subject})
	if err != nil {
		return nil, err
	}
	return computed[subject], nil
}

// computeSubjects computes and caches the entitlements of subjects.
func (s *Service) computeSubjects(subjects []string) (map[string]Entitlements, error) {
	plans, err := s.repo.ListPlans(false)
	if err != nil {
		return nil, fmt.Errorf("billing: listing plans: %w", err)
	}
	byKey := make(map[string]repositories.BillingPlanWithFeatures, len(plans))
	for _, p := range plans {
		byKey[p.Key] = p
	}
	subs, err := s.repo.ListSubscriptionsBySubjects(subjects)
	if err != nil {
		return nil, fmt.Errorf("billing: listing subscriptions: %w", err)
	}
	purchases, err := s.repo.ListPurchasesBySubjects(subjects)
	if err != nil {
		return nil, fmt.Errorf("billing: listing purchases: %w", err)
	}
	grants, err := s.repo.ListGrantsBySubjects(subjects)
	if err != nil {
		return nil, fmt.Errorf("billing: listing grants: %w", err)
	}
	now := s.now()
	out := make(map[string]Entitlements, len(subjects))
	for _, subject := range subjects {
		out[subject] = Compute(now, byKey,
			filter(subs, func(x db.BillingSubscription) bool { return x.Subject == subject }),
			filter(purchases, func(x db.BillingPurchase) bool { return x.Subject == subject }),
			filter(grants, func(x db.BillingGrant) bool { return x.Subject == subject }))
	}
	s.mu.Lock()
	for subject, e := range out {
		s.cache[subject] = e
	}
	s.mu.Unlock()
	return out, nil
}

func filter[T any](items []T, keep func(T) bool) []T {
	var out []T
	for _, it := range items {
		if keep(it) {
			out = append(out, it)
		}
	}
	return out
}

// UserOrgIDs returns the ids of the organizations userID belongs to (cached
// until InvalidateUser).
func (s *Service) UserOrgIDs(userID string) ([]string, error) {
	s.mu.RLock()
	ids, ok := s.memberships[userID]
	s.mu.RUnlock()
	if ok {
		return ids, nil
	}
	if s.orgs == nil {
		return nil, nil
	}
	memberships, err := s.orgs.OfUser(userID)
	if err != nil {
		return nil, fmt.Errorf("billing: organizations of %s: %w", userID, err)
	}
	ids = make([]string, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.Organization.ID)
	}
	s.mu.Lock()
	s.memberships[userID] = ids
	s.mu.Unlock()
	return ids, nil
}

// ForUser returns userID's entitlements and those of their organizations.
func (s *Service) ForUser(userID string) (UserEntitlements, error) {
	orgIDs, err := s.UserOrgIDs(userID)
	if err != nil {
		return UserEntitlements{}, err
	}
	subjects := []string{string(UserSubject(userID))}
	for _, id := range orgIDs {
		subjects = append(subjects, string(OrgSubject(id)))
	}
	byS := make(map[string]Entitlements, len(subjects))
	var missing []string
	s.mu.RLock()
	for _, subject := range subjects {
		if e, ok := s.cache[subject]; ok {
			byS[subject] = e
		} else {
			missing = append(missing, subject)
		}
	}
	s.mu.RUnlock()
	if len(missing) > 0 {
		computed, err := s.computeSubjects(missing)
		if err != nil {
			return UserEntitlements{}, err
		}
		for k, v := range computed {
			byS[k] = v
		}
	}
	out := UserEntitlements{User: byS[subjects[0]], Orgs: map[string]Entitlements{}}
	for _, id := range orgIDs {
		out.Orgs[id] = byS[string(OrgSubject(id))]
	}
	return out, nil
}

// InvalidateUser drops userID's cached memberships; organization member
// changes call it.
func (s *Service) InvalidateUser(userID string) {
	s.mu.Lock()
	delete(s.memberships, userID)
	s.mu.Unlock()
}

// CreateGrant gives subject feature until (nil = forever).
func (s *Service) CreateGrant(subject, feature string, until *time.Time, note, createdBy string) (*db.BillingGrant, error) {
	_, _, err := ParseSubject(subject)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	err = ValidateFeature(feature)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	err = s.checkSubjectExists(subject)
	if err != nil {
		return nil, err
	}
	g := &db.BillingGrant{Subject: subject, Feature: feature, Until: until, Note: strings.TrimSpace(note), CreatedBy: createdBy}
	err = s.repo.CreateGrant(g)
	if err != nil {
		return nil, fmt.Errorf("billing: creating grant: %w", err)
	}
	s.Invalidate(subject)
	return g, nil
}

// DeleteGrant removes a grant.
func (s *Service) DeleteGrant(id string) error {
	err := s.repo.DeleteGrant(id)
	if err != nil {
		return notFound(err)
	}
	// The repository doesn't say whose grant it was.
	s.InvalidateAll()
	return nil
}

// checkSubjectExists refuses organizations that don't exist (users are
// checked by the handlers, which have the user repository).
func (s *Service) checkSubjectExists(subject string) error {
	kind, id, err := ParseSubject(subject)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if kind != SubjectOrg || s.orgs == nil {
		return nil
	}
	_, err = s.orgs.Get(id)
	if err != nil {
		return notFound(err)
	}
	return nil
}
