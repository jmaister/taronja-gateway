package billing

import (
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/notification"
)

//go:embed invoice_email.html
var defaultInvoiceEmail string

var defaultInvoiceTemplate = template.Must(template.New("invoice").Parse(defaultInvoiceEmail))

// ParseEmailTemplate reads billing.email.templateFile.
func ParseEmailTemplate(path string) (*template.Template, error) {
	return template.ParseFiles(path)
}

// FormatEUR writes cents as Spanish euros: 2900 -> "29,00 €".
func FormatEUR(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d,%02d €", sign, cents/100, cents%100)
}

// InvoiceEmail is what the invoice email template receives.
type InvoiceEmail struct {
	Brand, LogoURL, Greeting, Thanks, Footer string
	CreditNote                               bool
	Number, Date, Total, AccountURL          string
	Lines                                    []InvoiceEmailLine
	// Notes say how long the subscription lasts: trial end, renewal or end.
	Notes []string
}

// InvoiceEmailLine is one line of the invoice email.
type InvoiceEmailLine struct {
	Description, Amount, Period string
}

func (s *Service) invoiceEmail(d *db.BillingDocument, c *db.BillingCustomer) (InvoiceEmail, error) {
	lines, err := DocumentLines(d)
	if err != nil {
		return InvoiceEmail{}, err
	}
	credit := d.Kind == db.DocumentCreditNote
	sign := int64(1)
	if credit {
		sign = -1
	}
	e := InvoiceEmail{
		Brand: s.email.BrandName, LogoURL: s.email.LogoURL, Thanks: s.email.Thanks, Footer: s.email.Footer,
		CreditNote: credit, Number: d.Number, Total: FormatEUR(sign * d.AmountCents), AccountURL: s.serverURL + "/_/account",
		Greeting: "Hola,",
	}
	if e.Brand == "" {
		e.Brand = "Taronja"
		if u, err := url.Parse(s.serverURL); err == nil && u.Host != "" {
			e.Brand = u.Host
		}
	}
	if e.Thanks == "" {
		e.Thanks = "Gracias por tu compra."
		if credit {
			e.Thanks = "Hemos registrado tu reembolso."
		}
	}
	if c.Name != "" {
		e.Greeting = "Hola " + c.Name + ","
	}
	issued := d.PaidAt
	if d.IssuedAt != nil {
		issued = *d.IssuedAt
	}
	e.Date = spanishDay(issued)
	for _, l := range lines {
		line := InvoiceEmailLine{Description: l.Description, Amount: FormatEUR(sign * l.AmountCents)}
		if l.PeriodStart != nil && l.PeriodEnd != nil {
			line.Period = "Periodo: " + spanishDay(*l.PeriodStart) + " – " + spanishDay(*l.PeriodEnd)
		}
		e.Lines = append(e.Lines, line)
	}
	if !credit && d.SubscriptionID != nil {
		sub, err := s.repo.GetSubscription(*d.SubscriptionID)
		if err == nil {
			e.Notes = s.subscriptionNotes(sub)
		}
	}
	return e, nil
}

func (s *Service) subscriptionNotes(sub *db.BillingSubscription) []string {
	var notes []string
	if plan, err := s.repo.GetPlan(sub.PlanKey); err == nil {
		notes = append(notes, "Plan: "+plan.Name+".")
	}
	if sub.TrialEnd != nil && sub.TrialEnd.After(s.now()) {
		notes = append(notes, "Tu prueba gratuita termina el "+spanishDay(*sub.TrialEnd)+".")
	}
	switch {
	case sub.CurrentPeriodEnd == nil:
	case sub.CancelAtPeriodEnd:
		notes = append(notes, "La suscripción termina el "+spanishDay(*sub.CurrentPeriodEnd)+" y no se renovará.")
	default:
		notes = append(notes, "La suscripción se renueva automáticamente el "+spanishDay(*sub.CurrentPeriodEnd)+".")
	}
	return notes
}

func (e InvoiceEmail) text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s\n\n", e.Greeting, e.Thanks)
	kind := "la factura"
	if e.CreditNote {
		kind = "el abono"
	}
	fmt.Fprintf(&b, "Adjuntamos %s %s del %s.\n\n", kind, e.Number, e.Date)
	for _, l := range e.Lines {
		fmt.Fprintf(&b, "- %s: %s", l.Description, l.Amount)
		if l.Period != "" {
			fmt.Fprintf(&b, " (%s)", l.Period)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Total (IVA incluido): %s\n\n", e.Total)
	for _, n := range e.Notes {
		b.WriteString(n + "\n")
	}
	if e.AccountURL != "" {
		fmt.Fprintf(&b, "\nGestiona tu suscripción y consulta tus facturas: %s\n", e.AccountURL)
	}
	if e.Footer != "" {
		b.WriteString("\n" + e.Footer + "\n")
	}
	return b.String()
}

// emailDocument sends d's PDF to its customer. It does nothing without an
// email provider or a customer email; the caller records EmailedAt.
func (s *Service) emailDocument(ctx context.Context, d *db.BillingDocument) error {
	if s.mailer == nil {
		return nil
	}
	c, err := s.repo.GetCustomer(d.Subject)
	if err != nil {
		return fmt.Errorf("billing details of %s: %w", d.Subject, err)
	}
	if c.Email == "" {
		return nil
	}
	e, err := s.invoiceEmail(d, c)
	if err != nil {
		return err
	}
	var html strings.Builder
	tmpl := s.emailTemplate
	if tmpl == nil {
		tmpl = defaultInvoiceTemplate
	}
	err = tmpl.Execute(&html, e)
	if err != nil {
		return fmt.Errorf("rendering the email: %w", err)
	}
	name, pdf, err := s.odoo.PDF(ctx, *d.OdooMoveID)
	if err != nil {
		return err
	}
	if name == "" {
		name = strings.ReplaceAll(d.Number, "/", "_") + ".pdf"
	}
	kind := "Factura"
	if e.CreditNote {
		kind = "Abono"
	}
	err = s.mailer.SendMail(ctx, notification.Mail{
		To: c.Email, Subject: kind + " " + e.Number + " · " + e.Brand, Text: e.text(), HTML: html.String(),
		Attachments: []notification.Attachment{{Name: name, Data: pdf}},
	})
	if err != nil {
		return err
	}
	now := s.now()
	d.EmailedAt = &now
	return nil
}
