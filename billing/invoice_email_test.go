package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMailer struct {
	sent []notification.Mail
	fail error
}

func (f *fakeMailer) SendMail(_ context.Context, m notification.Mail) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m)
	return nil
}

func TestInvoiceEmail_SentOnceWithPDFAndSubscriptionDates(t *testing.T) {
	e := newEnv(t)
	mailer := &fakeMailer{}
	e.svc.mailer = mailer
	e.svc.email.Footer = "Escuela SL · C/ Mayor 1"
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "b12345674")

	end := e.clock.AddDate(0, 1, 0)
	trialEnd := e.clock.AddDate(0, 0, 14)
	sub := &db.BillingSubscription{Subject: "org:o1", PlanKey: "academia-monthly", Status: db.SubscriptionActive, CurrentPeriodEnd: &end, TrialEnd: &trialEnd}
	require.NoError(t, e.repo.CreateSubscription(sub))
	d := &db.BillingDocument{Subject: "org:o1", Kind: db.DocumentInvoice, StripeRef: "in_1", AmountCents: 2900, PaidAt: e.clock, SubscriptionID: &sub.ID}
	start := e.clock
	require.NoError(t, EnqueueDocument(e.repo, d, []DocumentLine{{Description: "Academia mensual", PlanKey: "academia-monthly", AmountCents: 2900, PeriodStart: &start, PeriodEnd: &end}}, e.clock))

	e.process(t)
	require.Len(t, mailer.sent, 1)
	m := mailer.sent[0]
	assert.Equal(t, "e@x.es", m.To)
	assert.Equal(t, "Factura F/2026/00001 · gw.example.com", m.Subject)
	require.Len(t, m.Attachments, 1)
	assert.Equal(t, "F/2026/00001.pdf", m.Attachments[0].Name)
	assert.Contains(t, string(m.Attachments[0].Data), "%PDF")
	for _, want := range []string{"Plan: Academia mensual.", "29,00 €", "Periodo: " + spanishDay(start), "Tu prueba gratuita termina el " + spanishDay(trialEnd), "se renueva automáticamente el " + spanishDay(end), "https://gw.example.com/_/account", "Escuela SL · C/ Mayor 1", "Hola Escuela SL,"} {
		assert.Contains(t, m.HTML, want)
		assert.Contains(t, m.Text, want)
	}
	require.NotNil(t, e.doc(t, d.ID).EmailedAt)

	e.process(t)
	e.clock = e.clock.Add(24 * time.Hour)
	e.process(t)
	assert.Len(t, mailer.sent, 1, "sent once")
}

func TestInvoiceEmail_FailureRetriesWithoutReissuing(t *testing.T) {
	e := newEnv(t)
	mailer := &fakeMailer{fail: errors.New("smtp down")}
	e.svc.mailer = mailer
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "")
	d := e.invoice(t, "org:o1", "in_1")

	e.process(t)
	got := e.doc(t, d.ID)
	assert.Equal(t, db.OdooStatePending, got.OdooState)
	assert.Equal(t, 1, got.Attempts)
	assert.Contains(t, got.OdooError, "emailing: smtp down")
	require.NotNil(t, got.OdooMoveID, "issued in Odoo despite the email failing")
	assert.Equal(t, 0, e.process(t), "waits for the backoff")

	mailer.fail = nil
	e.clock = e.clock.Add(OdooBackoff(1) + time.Second)
	assert.Equal(t, 1, e.process(t))
	got = e.doc(t, d.ID)
	assert.Equal(t, db.OdooStateDone, got.OdooState)
	assert.Empty(t, got.OdooError)
	assert.Len(t, mailer.sent, 1)
	assert.Len(t, e.odoo.Moves, 1)
}

func TestInvoiceEmail_FailsAfterMaxAttempts(t *testing.T) {
	e := newEnv(t)
	e.svc.mailer = &fakeMailer{fail: errors.New("smtp down")}
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "")
	d := e.invoice(t, "org:o1", "in_1")
	for i := 0; i < MaxOdooAttempts; i++ {
		e.clock = e.clock.Add(7 * time.Hour)
		e.process(t)
	}
	assert.Equal(t, db.OdooStateFailed, e.doc(t, d.ID).OdooState)
}

func TestInvoiceEmail_EscapesAndCreditNote(t *testing.T) {
	e := newEnv(t)
	mailer := &fakeMailer{}
	e.svc.mailer = mailer
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	require.NoError(t, e.repo.SaveCustomer(&db.BillingCustomer{Subject: "org:o1", Name: "<script>alert(1)</script>", Email: "e@x.es", Country: "ES"}))
	inv := e.invoice(t, "org:o1", "in_1")
	e.process(t)
	credit := &db.BillingDocument{Subject: "org:o1", Kind: db.DocumentCreditNote, StripeRef: "re_1", InvoiceDocumentID: &inv.ID, AmountCents: 1000, PaidAt: e.clock}
	require.NoError(t, EnqueueDocument(e.repo, credit, []DocumentLine{{Description: "Reembolso", PlanKey: "academia-monthly", AmountCents: 1000}}, e.clock))
	e.process(t)

	require.Len(t, mailer.sent, 2)
	assert.NotContains(t, mailer.sent[0].HTML, "<script>")
	assert.Contains(t, mailer.sent[0].HTML, "&lt;script&gt;")
	assert.Contains(t, mailer.sent[1].Subject, "Abono ")
	assert.Contains(t, mailer.sent[1].HTML, "-10,00 €")
	assert.NotContains(t, mailer.sent[1].Text, "renueva")
}

func TestInvoiceEmail_WithoutMailerOrCustomerEmailStillIssues(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "")
	d := e.invoice(t, "org:o1", "in_1")
	e.process(t)
	assert.Equal(t, db.OdooStateDone, e.doc(t, d.ID).OdooState)
	assert.Nil(t, e.doc(t, d.ID).EmailedAt)
}
