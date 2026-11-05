package billing

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/notification"
)

// Notification types (doc/billing.md, Notifications).
const (
	NotifyPaymentFailed = "billing.payment_failed"
	NotifyTrialEnding   = "billing.trial_ending"
	NotifyCanceled      = "billing.canceled"
	NotifyExpired       = "billing.expired"
)

// TrialEndingNotice is how long before a trial ends its payer is told.
const TrialEndingNotice = 3 * 24 * time.Hour

// SetNotifications sets the notification service billing.* messages go
// through (nil sends none).
func (s *Service) SetNotifications(n *notification.Service) {
	s.notifications = n
}

// recipients are who hears about a subject's subscription: its payer and,
// for an organization, its owners.
func (s *Service) recipients(sub *db.BillingSubscription) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	add(sub.PayerUserID)
	kind, id, err := ParseSubject(sub.Subject)
	if err != nil {
		return out
	}
	if kind == SubjectUser {
		add(id)
	} else if s.orgs != nil {
		members, err := s.orgs.Members(id)
		if err == nil {
			for _, m := range members {
				if m.Role == "owner" {
					add(m.UserID)
				}
			}
		}
	}
	return out
}

// notifySubscription tells sub's payer and owners about it.
func (s *Service) notifySubscription(sub *db.BillingSubscription, kind string) error {
	if s.notifications == nil {
		return nil
	}
	to := s.recipients(sub)
	if len(to) == 0 {
		return nil
	}
	planName := sub.PlanKey
	plan, err := s.repo.GetPlan(sub.PlanKey)
	if err == nil {
		planName = plan.Name
	}
	account := s.serverURL + "/_/account"
	var title, body string
	switch kind {
	case NotifyPaymentFailed:
		title = "No hemos podido cobrar " + planName
		body = "El cobro de la renovación ha fallado. Actualiza tu tarjeta para no perder el acceso"
		if sub.GraceUntil != nil {
			body += " (se mantiene hasta el " + s.day(*sub.GraceUntil) + ")"
		}
		body += "."
	case NotifyTrialEnding:
		title = "Tu prueba de " + planName + " termina pronto"
		body = "La prueba gratuita termina"
		if sub.TrialEnd != nil {
			body += " el " + s.day(*sub.TrialEnd)
		}
		body += "; después se cobrará la suscripción."
	case NotifyCanceled:
		title = "Suscripción a " + planName + " cancelada"
		body = "La suscripción se ha cancelado."
	case NotifyExpired:
		title = "Se ha acabado el acceso a " + planName
		body = "El periodo de gracia ha terminado sin cobro. Puedes suscribirte de nuevo cuando quieras."
	default:
		return nil
	}
	_, _, err = s.notifications.Create(context.Background(), notification.CreateInput{
		UserIDs: to, Type: kind, Title: title, Body: body, URL: &account,
		Metadata: map[string]interface{}{"subject": sub.Subject, "subscription_id": sub.ID, "plan": sub.PlanKey},
	})
	if err != nil {
		log.Printf("billing: notification %s for subscription %s: %v", kind, sub.ID, err)
	}
	return err
}

// noticeCatchUp is how long after its moment a time-based notice is still
// sent: a gateway that was down for a few hours does not skip it.
const noticeCatchUp = 24 * time.Hour

// notifyOnce sends the notice kind for sub about the moment at, unless it
// was sent already (billing_notices); a failed send is forgotten so the
// next run tries again.
func (s *Service) notifyOnce(sub *db.BillingSubscription, kind string, at time.Time, now time.Time) {
	key := fmt.Sprintf("%s:%s:%d", kind, sub.ID, at.Unix())
	isNew, err := s.repo.RecordNotice(key, now)
	if err != nil {
		log.Printf("billing: recording notice %s: %v", key, err)
		return
	}
	if !isNew {
		return
	}
	if s.notifySubscription(sub, kind) != nil {
		_ = s.repo.ClearNotice(key)
	}
}

// NotifyDue sends the time-based notifications due at now: trials ending in
// TrialEndingNotice and grace periods that just ran out. Each one is sent
// once (billing_notices), also if it runs twice or after some downtime, up
// to noticeCatchUp after its moment.
func (s *Service) NotifyDue(now time.Time) error {
	if s.notifications == nil {
		return nil
	}
	trialing, err := s.repo.ListSubscriptions(repositories.SubscriptionFilter{Status: db.SubscriptionTrialing})
	if err != nil {
		return err
	}
	for i := range trialing {
		end := trialing[i].TrialEnd
		if end == nil || trialing[i].CancelAtPeriodEnd || !end.After(now) {
			continue
		}
		moment := end.Add(-TrialEndingNotice)
		if !moment.After(now) && moment.After(now.Add(-noticeCatchUp)) {
			s.notifyOnce(&trialing[i], NotifyTrialEnding, *end, now)
		}
	}
	pastDue, err := s.repo.ListSubscriptions(repositories.SubscriptionFilter{Status: db.SubscriptionPastDue})
	if err != nil {
		return err
	}
	for i := range pastDue {
		g := pastDue[i].GraceUntil
		if g != nil && !g.After(now) && g.After(now.Add(-noticeCatchUp)) {
			s.notifyOnce(&pastDue[i], NotifyExpired, *g, now)
		}
	}
	return nil
}

// RunNotifier calls NotifyDue every hour until ctx is done.
func (s *Service) RunNotifier(ctx context.Context) {
	for {
		now := s.now()
		next := now.Truncate(time.Hour).Add(time.Hour)
		select {
		case <-ctx.Done():
			return
		case <-time.After(next.Sub(now)):
		}
		err := s.NotifyDue(next)
		if err != nil {
			log.Printf("billing: %v", fmt.Errorf("time-based notifications: %w", err))
		}
	}
}
