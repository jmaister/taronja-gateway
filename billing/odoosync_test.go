package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *testEnv) customer(t *testing.T, subject, taxID string) {
	t.Helper()
	require.NoError(t, e.repo.SaveCustomer(&db.BillingCustomer{
		Subject: subject, Name: "Escuela SL", Email: "e@x.es", TaxID: taxID, AddressLine1: "C/ Mayor 1",
		PostalCode: "46001", City: "Valencia", State: "Valencia", Country: "ES",
	}))
}

func (e *testEnv) invoice(t *testing.T, subject, ref string) *db.BillingDocument {
	t.Helper()
	d := &db.BillingDocument{Subject: subject, Kind: db.DocumentInvoice, StripeRef: ref, AmountCents: 2900, PaidAt: e.clock}
	require.NoError(t, EnqueueDocument(e.repo, d, []DocumentLine{{Description: "Academia mensual", PlanKey: "academia-monthly", AmountCents: 2900}}, e.clock))
	return d
}

func (e *testEnv) process(t *testing.T) int {
	t.Helper()
	n, err := e.svc.ProcessDueDocuments(context.Background())
	require.NoError(t, err)
	return n
}

func (e *testEnv) doc(t *testing.T, id string) *db.BillingDocument {
	t.Helper()
	d, err := e.repo.GetDocument(id)
	require.NoError(t, err)
	return d
}

func TestOutbox_IssuesInvoice(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "b12345674")
	d := e.invoice(t, "org:o1", "in_1")
	// The same Stripe reference again (a repeated webhook) is ignored.
	require.NoError(t, EnqueueDocument(e.repo, &db.BillingDocument{Subject: "org:o1", Kind: db.DocumentInvoice, StripeRef: "in_1", PaidAt: e.clock}, nil, e.clock))

	assert.Equal(t, 1, e.process(t))
	got := e.doc(t, d.ID)
	assert.Equal(t, db.OdooStateDone, got.OdooState)
	assert.Equal(t, "F/2026/00001", got.Number)
	require.NotNil(t, got.OdooMoveID)
	assert.Empty(t, got.OdooError)

	partner := e.odoo.Partners["tg:org:o1"]
	assert.Equal(t, "ESB12345674", partner.VAT)
	assert.True(t, partner.IsCompany)
	assert.Equal(t, "Valencia", partner.Address.City)
	move := e.odoo.Moves["in_1"]
	assert.Equal(t, []OdooLine{{ProductID: e.odoo.ProductIDs["TG-academia-monthly"], Description: "Academia mensual", AmountCents: 2900}}, move.Lines)
	c, err := e.repo.GetCustomer("org:o1")
	require.NoError(t, err)
	assert.Equal(t, e.odoo.PartnerIDs["tg:org:o1"], *c.OdooPartnerID)

	assert.Equal(t, 0, e.process(t), "nothing left to do")
	name, pdf, err := e.svc.DocumentPDF(context.Background(), d.ID)
	require.NoError(t, err)
	assert.Equal(t, "F/2026/00001.pdf", name)
	assert.Contains(t, string(pdf), "%PDF")
}

func TestOutbox_RetriesWithBackoffThenFails(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "user:u1", "")
	d := e.invoice(t, "user:u1", "in_2")
	e.odoo.FailIssue = errors.New("odoo is down")

	e.process(t)
	got := e.doc(t, d.ID)
	assert.Equal(t, 1, got.Attempts)
	assert.Equal(t, db.OdooStatePending, got.OdooState)
	assert.Equal(t, "odoo is down", got.OdooError)
	assert.Equal(t, e.clock.Add(time.Minute), got.NextAttemptAt.UTC())
	assert.Equal(t, 0, e.process(t), "not due before the backoff")

	for i := 2; i <= MaxOdooAttempts; i++ {
		e.clock = e.clock.Add(7 * time.Hour)
		assert.Equal(t, 1, e.process(t))
	}
	got = e.doc(t, d.ID)
	assert.Equal(t, db.OdooStateFailed, got.OdooState)
	assert.Equal(t, MaxOdooAttempts, got.Attempts)
	e.clock = e.clock.Add(7 * time.Hour)
	assert.Equal(t, 0, e.process(t), "failed documents wait for Retry")

	e.odoo.FailIssue = nil
	_, err = e.svc.RetryDocument(d.ID)
	require.NoError(t, err)
	e.process(t)
	got = e.doc(t, d.ID)
	assert.Equal(t, db.OdooStateDone, got.OdooState)
	assert.Len(t, e.odoo.Moves, 1)
	_, err = e.svc.RetryDocument(d.ID)
	assert.ErrorIs(t, err, ErrConflict)
	_, err = e.svc.RetryDocument("missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestOutbox_CreditNoteWaitsForItsInvoice(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "")
	inv := e.invoice(t, "org:o1", "in_3")
	// Make the invoice wait so the credit note is processed first.
	inv.NextAttemptAt = e.clock.Add(time.Hour)
	require.NoError(t, e.repo.UpdateDocument(inv))
	credit := &db.BillingDocument{Subject: "org:o1", Kind: db.DocumentCreditNote, StripeRef: "re_1", InvoiceDocumentID: &inv.ID, AmountCents: 1000, PaidAt: e.clock}
	require.NoError(t, EnqueueDocument(e.repo, credit, []DocumentLine{{Description: "Refund", PlanKey: "academia-monthly", AmountCents: 1000}}, e.clock))

	e.process(t)
	got := e.doc(t, credit.ID)
	assert.Equal(t, db.OdooStatePending, got.OdooState)
	assert.Equal(t, 0, got.Attempts, "waiting is not a failed attempt")
	assert.Empty(t, e.odoo.Moves)

	e.clock = e.clock.Add(2 * time.Hour)
	e.process(t) // the credit note (due first) waits again; the invoice is issued
	e.clock = e.clock.Add(2 * time.Minute)
	e.process(t)
	got = e.doc(t, credit.ID)
	assert.Equal(t, db.OdooStateDone, got.OdooState)
	assert.Equal(t, "RF/2026/00001", got.Number)
	assert.Equal(t, *e.doc(t, inv.ID).OdooMoveID, e.odoo.Moves["re_1"].ReversedMoveID)
	assert.True(t, e.odoo.Moves["re_1"].CreditNote)
}

func TestOutbox_NoCreditNoteSeriesIsAFailure(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "")
	inv := e.invoice(t, "org:o1", "in_4")
	e.process(t)
	e.odoo.NoCreditNoteSeries = true
	credit := &db.BillingDocument{Subject: "org:o1", Kind: db.DocumentCreditNote, StripeRef: "re_2", InvoiceDocumentID: &inv.ID, AmountCents: 2900, PaidAt: e.clock}
	require.NoError(t, EnqueueDocument(e.repo, credit, []DocumentLine{{Description: "Refund", PlanKey: "academia-monthly", AmountCents: 2900}}, e.clock))
	e.process(t)
	got := e.doc(t, credit.ID)
	assert.Equal(t, 1, got.Attempts)
	assert.Contains(t, got.OdooError, "Dedicated Credit Note Sequence")
}

func TestOutbox_WithoutOdooDocumentsWait(t *testing.T) {
	e := newEnv(t)
	e.svc.odoo = nil
	e.customer(t, "org:o1", "")
	d := e.invoice(t, "org:o1", "in_5")
	assert.Equal(t, 0, e.process(t))
	assert.Equal(t, db.OdooStatePending, e.doc(t, d.ID).OdooState)
	_, _, err := e.svc.DocumentPDF(context.Background(), d.ID)
	assert.ErrorIs(t, err, ErrPDFNotFound)
}

func TestOutbox_CreatesMissingOdooProduct(t *testing.T) {
	e := newEnv(t)
	e.svc.odoo = failingOdoo{e.odoo}
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.svc.odoo = e.odoo
	e.customer(t, "org:o1", "")
	e.invoice(t, "org:o1", "in_6")
	e.process(t)
	plan, err := e.svc.GetPlan("academia-monthly")
	require.NoError(t, err)
	require.NotNil(t, plan.OdooProductID)
	assert.Equal(t, e.odoo.ProductIDs["TG-academia-monthly"], *plan.OdooProductID)
}

func TestOdooBackoff(t *testing.T) {
	assert.Equal(t, []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 6 * time.Hour},
		[]time.Duration{OdooBackoff(1), OdooBackoff(2), OdooBackoff(3), OdooBackoff(4), OdooBackoff(5), OdooBackoff(9)})
}

func TestNormalizeVAT(t *testing.T) {
	assert.Equal(t, "ESB12345674", normalizeVAT("b-1234567.4", "ES"))
	assert.Equal(t, "ES12345678Z", normalizeVAT("ES 12345678Z", ""))
	assert.Equal(t, "FR123", normalizeVAT("FR123", "FR"))
	assert.Equal(t, "", normalizeVAT("", "ES"))
	assert.True(t, isSpanishCompanyID("ESB12345674"))
	assert.False(t, isSpanishCompanyID("ES12345678Z"))
	assert.False(t, isSpanishCompanyID("ESX1234567L"))
}

func TestCanViewDocument(t *testing.T) {
	e := newEnv(t)
	orgID := e.org(t, "Escuela")
	require.NoError(t, e.orgs.SetMember(orgID, "owner1", "owner"))
	require.NoError(t, e.orgs.SetMember(orgID, "member1", "member"))
	sub := &db.BillingSubscription{Subject: string(OrgSubject(orgID)), PlanKey: "p", Status: db.SubscriptionActive, PayerUserID: "payer1"}
	require.NoError(t, e.repo.CreateSubscription(sub))
	d := &db.BillingDocument{Subject: string(OrgSubject(orgID)), SubscriptionID: &sub.ID}

	for user, want := range map[string]bool{"owner1": true, "payer1": true, "member1": false, "stranger": false} {
		got, err := e.svc.CanViewDocument(user, d)
		require.NoError(t, err)
		assert.Equal(t, want, got, user)
	}
	own, err := e.svc.CanViewDocument("u9", &db.BillingDocument{Subject: "user:u9"})
	require.NoError(t, err)
	assert.True(t, own)
}
