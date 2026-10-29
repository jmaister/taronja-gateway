package gateway

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/providers"
)

// Paths of the billing pages (doc/billing.md, Workflows).
const (
	payPath     = config.ManagementPrefix + "/pay/"
	payDonePath = config.ManagementPrefix + "/pay/done"
	webhookPath = config.ManagementPrefix + "/billing/stripe/webhook"
	portalPath  = providers.AccountPath + "/billing/portal"
)

// payView is the pay page of a plan.
type payView struct {
	Plan        string
	Name        string
	Description string
	Price       string // "29,00 € / mes"
	SetupFee    string
	Features    []string
	CSRF        string
	Redirect    string
	Options     []payOption
	// NeedsOrg: the plan is for organizations and the user has none.
	NeedsOrg bool
}

// payOption is one subject the user may pay for.
type payOption struct {
	OrgID      string // "" = the user
	Label      string
	Selected   bool
	Trial      int
	Subscribed bool // live subscription in the plan's group: manage instead
}

// payDoneView is /_/pay/done.
type payDoneView struct {
	Status   string // open, completed, expired
	Redirect string
}

// billingView is the Billing section of the account page.
type billingView struct {
	Subscriptions []accountSubscription
	Documents     []accountDocument
	CSRF          string
}

// HasManage is true when any subscription can be managed in the Stripe portal.
func (b *billingView) HasManage() bool {
	for _, s := range b.Subscriptions {
		if s.Manage {
			return true
		}
	}
	return false
}

type accountSubscription struct {
	Plan, Owner, Status, StatusClass, Next, Subject string
	Manage                                          bool
}

var subscriptionClasses = map[string]string{
	db.SubscriptionTrialing: "warn",
	db.SubscriptionActive:   "ok",
	db.SubscriptionPastDue:  "bad",
}

type accountDocument struct {
	Kind, Number, Date, Amount, Owner, PDF string
}

var subscriptionLabels = map[string]string{
	db.SubscriptionAwaitingActivation: "Pendiente de activar",
	db.SubscriptionTrialing:           "En prueba",
	db.SubscriptionActive:             "Activa",
	db.SubscriptionPastDue:            "Pago fallido",
	db.SubscriptionCanceled:           "Cancelada",
}

// displayFeatures drops the catch-all "all" feature, which says nothing to a buyer.
func displayFeatures(features []string) []string {
	return slices.DeleteFunc(slices.Clone(features), func(f string) bool { return f == "all" })
}

// FormatEUR writes cents as Spanish euros: 2900 -> "29,00 €".
func FormatEUR(cents int64) string { return billing.FormatEUR(cents) }

func formatDay(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Local().Format("02/01/2006")
}

// registerBillingRoutes adds the pay pages, the Stripe webhook and the
// account page's portal button when billing is enabled.
func (g *Gateway) registerBillingRoutes() {
	if g.Dependencies.Billing == nil {
		return
	}
	g.Mux.HandleFunc("GET "+payDonePath, g.handlePayDone)
	g.Mux.HandleFunc("GET "+payPath+"{plan}", g.handlePayPage)
	g.Mux.HandleFunc("POST "+payPath+"{plan}", g.handlePay)
	g.Mux.HandleFunc("POST "+webhookPath, g.handleStripeWebhook)
	g.Mux.HandleFunc("POST "+portalPath, g.withAccount(g.handlePortal))
	log.Printf("Registered billing routes (%s{plan}, %s)", payPath, webhookPath)
}

func (g *Gateway) loginFor(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, config.ManagementPrefix+"/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
}

func (g *Gateway) handlePayPage(w http.ResponseWriter, r *http.Request) {
	acct, ok := g.currentAccount(r)
	if !ok {
		g.loginFor(w, r)
		return
	}
	g.renderPayPage(w, r, http.StatusOK, acct, r.PathValue("plan"), r.URL.Query().Get("org"), r.URL.Query().Get("redirect"), "")
}

func (g *Gateway) renderPayPage(w http.ResponseWriter, r *http.Request, status int, acct *accountRequest, planKey, orgID, redirect, errorText string) {
	svc := g.Dependencies.Billing
	plan, err := svc.GetPlan(planKey)
	if err != nil || !plan.Active {
		g.renderMessage(w, http.StatusNotFound, "Plan no encontrado", "Este plan no existe o ya no se vende.", "/", "Volver")
		return
	}
	view := &payView{
		Plan: plan.Key, Name: plan.Name, Description: plan.Description, Features: displayFeatures(plan.Features),
		CSRF: accounts.CSRFToken(acct.session.Token), Redirect: billing.SafeRedirect(redirect),
		Price: FormatEUR(plan.AmountCents),
	}
	switch plan.Interval {
	case db.PlanIntervalMonth:
		view.Price += " / mes"
	case db.PlanIntervalYear:
		view.Price += " / año"
	}
	if plan.SetupFeeCents > 0 {
		view.SetupFee = plan.SetupFeeName + ": " + FormatEUR(plan.SetupFeeCents)
	}

	addOption := func(subject billing.Subject, org, label string) {
		opt := payOption{OrgID: org, Label: label, Selected: org == orgID, Trial: svc.TrialDaysFor(string(subject), plan)}
		live, err := svc.LiveSubscription(string(subject), plan)
		if err != nil {
			log.Printf("Pay page: subscriptions of %s: %v", subject, err)
		}
		opt.Subscribed = live != nil
		view.Options = append(view.Options, opt)
	}
	if plan.Subject != db.PlanSubjectOrg {
		addOption(billing.UserSubject(acct.user.ID), "", "Yo ("+acct.user.Email+")")
	}
	if plan.Subject != db.PlanSubjectUser && g.Dependencies.OrganizationRepo != nil {
		memberships, err := g.Dependencies.OrganizationRepo.OfUser(acct.user.ID)
		if err != nil {
			log.Printf("Pay page: organizations of %s: %v", acct.user.ID, err)
		}
		member := false
		owned := 0
		for _, m := range memberships {
			if m.Role != db.OrganizationRoleOwner {
				continue
			}
			owned++
			member = member || m.Organization.ID == orgID
			addOption(billing.OrgSubject(m.Organization.ID), m.Organization.ID, m.Organization.Name)
		}
		if orgID != "" && !member {
			g.renderMessage(w, http.StatusForbidden, "Organización no permitida", "Solo puedes pagar por ti o por una organización de la que eres propietario.", "/", "Volver")
			return
		}
		view.NeedsOrg = plan.Subject == db.PlanSubjectOrg && owned == 0
	}
	if len(view.Options) > 0 {
		selected := false
		for _, o := range view.Options {
			selected = selected || o.Selected
		}
		if !selected {
			view.Options[0].Selected = true
		}
	}
	page := g.newAuthPage(plan.Name)
	page.Error = errorText
	page.Pay = view
	g.renderAuthPage(w, status, "pay.html", page)
}

func (g *Gateway) handlePay(w http.ResponseWriter, r *http.Request) {
	acct, ok := g.currentAccount(r)
	if !ok {
		g.loginFor(w, r)
		return
	}
	if !accounts.ValidCSRF(acct.session.Token, r.FormValue("csrf")) {
		http.Error(w, "Prohibido: el formulario ha caducado, recarga la página e inténtalo de nuevo", http.StatusForbidden)
		return
	}
	planKey, orgID, redirect := r.PathValue("plan"), r.FormValue("org"), r.FormValue("redirect")
	_, checkoutURL, err := g.Dependencies.Billing.StartCheckout(r.Context(), billing.CheckoutRequest{
		PlanKey: planKey, OrgID: orgID, Redirect: redirect,
		Payer: billing.Payer{UserID: acct.user.ID, Name: acct.user.Name, Email: acct.user.Email},
	})
	switch {
	case err == nil:
		http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
	case errors.Is(err, billing.ErrForbidden):
		g.renderMessage(w, http.StatusForbidden, "Organización no permitida", "Solo puedes pagar por ti o por una organización de la que eres propietario.", "/", "Volver")
	case errors.Is(err, billing.ErrAlreadySubscribed):
		g.renderPayPage(w, r, http.StatusConflict, acct, planKey, orgID, redirect, "Ya hay una suscripción a este plan: gestiónala en su lugar.")
	case errors.Is(err, billing.ErrInvalid), errors.Is(err, billing.ErrNotFound):
		g.renderPayPage(w, r, http.StatusBadRequest, acct, planKey, orgID, redirect, strings.TrimPrefix(err.Error(), billing.ErrInvalid.Error()+": "))
	default:
		log.Printf("Pay: checkout for %s by %s failed: %v", planKey, acct.user.ID, err)
		g.renderPayPage(w, r, http.StatusBadGateway, acct, planKey, orgID, redirect, "No se ha podido abrir la página de pago. Inténtalo de nuevo en un momento.")
	}
}

// handlePayDone shows the checkout's state, read from the gateway: it
// reloads itself until Stripe's webhook has completed it.
func (g *Gateway) handlePayDone(w http.ResponseWriter, r *http.Request) {
	acct, ok := g.currentAccount(r)
	if !ok {
		g.loginFor(w, r)
		return
	}
	checkout, err := g.Dependencies.Billing.GetCheckout(r.URL.Query().Get("checkout"), acct.user.ID)
	if err != nil {
		g.renderMessage(w, http.StatusNotFound, "Pago no encontrado", "No hemos encontrado este pago.", "/", "Volver")
		return
	}
	redirect := checkout.Redirect
	if redirect == "" {
		redirect = "/"
	}
	page := g.newAuthPage("Pago")
	page.PayDone = &payDoneView{Status: checkout.Status, Redirect: redirect}
	g.renderAuthPage(w, http.StatusOK, "pay_done.html", page)
}

// handleStripeWebhook applies a Stripe event. 400 for a bad signature
// (Stripe won't retry), 500 for anything to retry.
func (g *Gateway) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, billing.MaxWebhookBody))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	err = g.Dependencies.Billing.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature"))
	if errors.Is(err, billing.ErrBadSignature) {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}
	if err != nil {
		if !errors.Is(err, billing.ErrRetryLater) {
			log.Printf("Stripe webhook: %v", err)
		}
		http.Error(w, "try again later", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (g *Gateway) handlePortal(w http.ResponseWriter, r *http.Request, acct *accountRequest) {
	portalURL, err := g.Dependencies.Billing.PortalURL(r.Context(), r.FormValue("subject"), acct.user.ID)
	if err != nil {
		log.Printf("Account: portal for %s by %s: %v", r.FormValue("subject"), acct.user.ID, err)
		g.renderAccountPage(w, http.StatusBadRequest, acct, "", "La suscripción no se puede gestionar ahora mismo.")
		return
	}
	http.Redirect(w, r, portalURL, http.StatusSeeOther)
}

// accountBilling builds the account page's Billing section: the user's own
// subscriptions and documents and those of organizations they own.
func (g *Gateway) accountBilling(acct *accountRequest) *billingView {
	svc := g.Dependencies.Billing
	if svc == nil {
		return nil
	}
	type owned struct{ subject, label string }
	subjects := []owned{{string(billing.UserSubject(acct.user.ID)), "Personal"}}
	if g.Dependencies.OrganizationRepo != nil {
		memberships, err := g.Dependencies.OrganizationRepo.OfUser(acct.user.ID)
		if err != nil {
			log.Printf("Account page: organizations of %s: %v", acct.user.ID, err)
		}
		for _, m := range memberships {
			if m.Role == "owner" {
				subjects = append(subjects, owned{string(billing.OrgSubject(m.Organization.ID)), m.Organization.Name})
			}
		}
	}
	view := &billingView{CSRF: accounts.CSRFToken(acct.session.Token)}
	for _, o := range subjects {
		subs, err := svc.Repo().ListSubscriptions(repositories.SubscriptionFilter{Subject: o.subject})
		if err != nil {
			log.Printf("Account page: subscriptions of %s: %v", o.subject, err)
		}
		for _, s := range subs {
			name := s.PlanKey
			plan, err := svc.GetPlan(s.PlanKey)
			if err == nil {
				name = plan.Name
			}
			item := accountSubscription{Plan: name, Owner: o.label, Status: labelOr(subscriptionLabels, s.Status), StatusClass: subscriptionClasses[s.Status], Subject: o.subject,
				Manage: s.StripeSubscriptionID != nil && s.Status != db.SubscriptionCanceled}
			switch {
			case s.Status == db.SubscriptionCanceled:
			case s.Status == db.SubscriptionTrialing:
				item.Next = "Prueba hasta el " + formatDay(s.TrialEnd)
			case s.CancelAtPeriodEnd:
				item.Next = "Termina el " + formatDay(s.CurrentPeriodEnd)
			case s.Status == db.SubscriptionPastDue:
				item.Next = "Acceso hasta el " + formatDay(s.GraceUntil)
			case s.CurrentPeriodEnd != nil:
				item.Next = "Se renueva el " + formatDay(s.CurrentPeriodEnd)
			}
			view.Subscriptions = append(view.Subscriptions, item)
		}
		docs, err := svc.Repo().ListDocuments(repositories.DocumentFilter{Subject: o.subject})
		if err != nil {
			log.Printf("Account page: documents of %s: %v", o.subject, err)
		}
		for _, d := range docs {
			item := accountDocument{Kind: "Factura", Number: d.Number, Date: formatDay(&d.PaidAt), Amount: FormatEUR(d.AmountCents), Owner: o.label}
			if d.Kind == db.DocumentCreditNote {
				item.Kind, item.Amount = "Abono", FormatEUR(-d.AmountCents)
			}
			if d.OdooState == db.OdooStateDone {
				item.PDF = config.ManagementPrefix + "/api/billing/documents/" + url.PathEscape(d.ID) + "/pdf"
			} else {
				item.Number = "being issued"
			}
			view.Documents = append(view.Documents, item)
		}
	}
	return view
}
