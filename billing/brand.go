package billing

import (
	"strings"

	"github.com/jmaister/taronja-gateway/db"
)

// Brand is how a plan group looks: billing.brands.<group> over billing.email.
// An empty field means "use the caller's default" (the gateway name and logo
// on pages, the server host in emails, Stripe's own look in Checkout).
type Brand struct {
	Name, LogoURL, ButtonColor, BackgroundColor, EmailSubject string
}

// BrandFor returns the brand of a plan group; "" is a plan without group.
func (s *Service) BrandFor(group string) Brand {
	b := Brand{Name: s.email.BrandName, LogoURL: s.email.LogoURL}
	c, ok := s.brands[group]
	if group == "" || !ok {
		return b
	}
	if c.Name != "" {
		b.Name = c.Name
	}
	if c.LogoURL != "" {
		b.LogoURL = c.LogoURL
	}
	b.ButtonColor, b.BackgroundColor, b.EmailSubject = c.ButtonColor, c.BackgroundColor, c.EmailSubject
	return b
}

// BrandOfPlan is the brand of the group of the plan with that key.
func (s *Service) BrandOfPlan(planKey string) Brand {
	plan, err := s.repo.GetPlan(planKey)
	if err != nil {
		return s.BrandFor("")
	}
	return s.BrandFor(plan.GroupKey)
}

// brandOfDocument is the brand of the plan d was issued for.
func (s *Service) brandOfDocument(d *db.BillingDocument) Brand {
	planKey := ""
	switch {
	case d.SubscriptionID != nil:
		if sub, err := s.repo.GetSubscription(*d.SubscriptionID); err == nil {
			planKey = sub.PlanKey
		}
	case d.PurchaseID != nil:
		if p, err := s.repo.GetPurchase(*d.PurchaseID); err == nil {
			planKey = p.PlanKey
		}
	}
	return s.BrandOfPlan(planKey)
}

func (b Brand) emailSubject(kind, number, brandName string) string {
	if b.EmailSubject == "" {
		return kind + " " + number + " · " + brandName
	}
	if strings.Contains(b.EmailSubject, "{number}") {
		return strings.ReplaceAll(b.EmailSubject, "{number}", number)
	}
	return b.EmailSubject + " · " + kind + " " + number
}
