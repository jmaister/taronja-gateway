package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
)

// DocumentLine is one line of a billing document (stored as JSON in
// BillingDocument.Lines).
type DocumentLine struct {
	Description string     `json:"description"`
	PlanKey     string     `json:"planKey"`
	SetupFee    bool       `json:"setupFee,omitempty"`
	AmountCents int64      `json:"amountCents"` // VAT included
	PeriodStart *time.Time `json:"periodStart,omitempty"`
	PeriodEnd   *time.Time `json:"periodEnd,omitempty"`
}

// DocumentLines decodes d's lines.
func DocumentLines(d *db.BillingDocument) ([]DocumentLine, error) {
	var lines []DocumentLine
	err := json.Unmarshal([]byte(d.Lines), &lines)
	return lines, err
}

// MaxOdooAttempts is how many times a document is tried before it waits for
// the admin's Retry.
const MaxOdooAttempts = 10

// OdooBackoff is the wait before attempt n+1 after n failed attempts.
func OdooBackoff(attempts int) time.Duration {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	if attempts >= 1 && attempts <= len(steps) {
		return steps[attempts-1]
	}
	return 6 * time.Hour
}

// EnqueueDocument stores a new document for the outbox. A document with the
// same Stripe reference already queued is not an error (webhooks repeat).
func EnqueueDocument(repo repositories.BillingRepository, d *db.BillingDocument, lines []DocumentLine, now time.Time) error {
	encoded, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	d.Lines = string(encoded)
	d.OdooState = db.OdooStatePending
	d.NextAttemptAt = now
	if d.Currency == "" {
		d.Currency = "eur"
	}
	err = repo.CreateDocument(d)
	if errors.Is(err, repositories.ErrDuplicate) {
		return nil
	}
	return err
}

// RunOutbox issues due documents every interval until ctx is done.
func (s *Service) RunOutbox(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_, err := s.ProcessDueDocuments(ctx)
		if err != nil {
			log.Printf("billing: Odoo outbox: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessDueDocuments issues the documents whose next attempt is due and
// returns how many it tried. Without Odoo it does nothing: documents wait.
func (s *Service) ProcessDueDocuments(ctx context.Context) (int, error) {
	if s.odoo == nil {
		return 0, nil
	}
	docs, err := s.repo.DueDocuments(s.now(), 20)
	if err != nil {
		return 0, err
	}
	for i := range docs {
		if ctx.Err() != nil {
			return i, ctx.Err()
		}
		s.processDocument(ctx, &docs[i])
	}
	return len(docs), nil
}

// processDocument runs one attempt and records its outcome on d: issued in
// Odoo, then emailed. A failed email retries without reissuing.
func (s *Service) processDocument(ctx context.Context, d *db.BillingDocument) {
	if d.OdooMoveID == nil {
		issued, err := s.issue(ctx, d)
		if errors.Is(err, errInvoiceNotIssued) {
			// Not a failure: check again shortly without using up attempts.
			d.NextAttemptAt = s.now().Add(time.Minute)
			d.OdooError = err.Error()
			s.saveDocument(d)
			return
		}
		if err != nil {
			s.failAttempt(d, err)
			return
		}
		moveID := issued.MoveID
		issuedAt := issued.IssuedAt
		d.OdooMoveID, d.Number, d.IssuedAt = &moveID, issued.Number, &issuedAt
	}
	if d.EmailedAt == nil {
		err := s.emailDocument(ctx, d)
		if err != nil {
			s.failAttempt(d, fmt.Errorf("emailing: %w", err))
			return
		}
	}
	d.OdooState, d.OdooError = db.OdooStateDone, ""
	s.saveDocument(d)
}

func (s *Service) failAttempt(d *db.BillingDocument, err error) {
	d.Attempts++
	d.OdooError = err.Error()
	if d.Attempts >= MaxOdooAttempts {
		d.OdooState = db.OdooStateFailed
	} else {
		d.NextAttemptAt = s.now().Add(OdooBackoff(d.Attempts))
	}
	log.Printf("billing: Odoo %s %s (attempt %d): %v", d.Kind, d.StripeRef, d.Attempts, err)
	s.saveDocument(d)
}

// BlockedReason says why a credit note cannot be issued yet: its invoice is
// not in Odoo. "" for anything else, including a credit note that is done.
func (s *Service) BlockedReason(d *db.BillingDocument) string {
	if d.Kind != db.DocumentCreditNote || d.OdooState == db.OdooStateDone || d.InvoiceDocumentID == nil {
		return ""
	}
	invoice, err := s.repo.GetDocument(*d.InvoiceDocumentID)
	if err != nil || invoice.OdooMoveID != nil {
		return ""
	}
	if invoice.OdooState == db.OdooStateFailed {
		return "Its invoice failed in Odoo (" + invoice.OdooError + "): retry the invoice first"
	}
	return "Waiting for its invoice, which is not issued in Odoo yet"
}

func (s *Service) saveDocument(d *db.BillingDocument) {
	err := s.repo.UpdateDocument(d)
	if err != nil {
		log.Printf("billing: saving document %s: %v", d.ID, err)
	}
}

var errInvoiceNotIssued = errors.New("waiting for its invoice to be issued in Odoo")

// issue runs the Odoo steps of doc/billing.md for d; each is idempotent.
func (s *Service) issue(ctx context.Context, d *db.BillingDocument) (OdooIssued, error) {
	var reversed int64
	if d.Kind == db.DocumentCreditNote {
		if d.InvoiceDocumentID == nil {
			return OdooIssued{}, errors.New("credit note without its invoice")
		}
		invoice, err := s.repo.GetDocument(*d.InvoiceDocumentID)
		if err != nil {
			return OdooIssued{}, fmt.Errorf("reading its invoice: %w", err)
		}
		if invoice.OdooMoveID == nil {
			return OdooIssued{}, errInvoiceNotIssued
		}
		reversed = *invoice.OdooMoveID
	}

	partnerID, err := s.ensurePartner(ctx, d.Subject)
	if err != nil {
		return OdooIssued{}, err
	}
	lines, err := DocumentLines(d)
	if err != nil {
		return OdooIssued{}, fmt.Errorf("decoding lines: %w", err)
	}
	move := OdooMove{Ref: d.StripeRef, PartnerID: partnerID, CreditNote: d.Kind == db.DocumentCreditNote, ReversedMoveID: reversed, PaidAt: d.PaidAt}
	for _, l := range lines {
		productID, err := s.ensureOdooProduct(ctx, l.PlanKey, l.SetupFee)
		if err != nil {
			return OdooIssued{}, err
		}
		move.Lines = append(move.Lines, OdooLine{ProductID: productID, Description: l.Description, AmountCents: l.AmountCents})
	}
	return s.odoo.IssueMove(ctx, move)
}

// ensurePartner creates or updates the subject's Odoo partner from its
// billing details.
func (s *Service) ensurePartner(ctx context.Context, subject string) (int64, error) {
	c, err := s.repo.GetCustomer(subject)
	if err != nil {
		return 0, fmt.Errorf("billing details of %s: %w", subject, err)
	}
	vat := normalizeVAT(c.TaxID, c.Country)
	id, err := s.odoo.UpsertPartner(ctx, OdooPartner{
		Ref: "tg:" + subject, Name: c.Name, Email: c.Email, VAT: vat, IsCompany: isSpanishCompanyID(vat),
		Address: Address{Line1: c.AddressLine1, Line2: c.AddressLine2, PostalCode: c.PostalCode, City: c.City, State: c.State, Country: c.Country},
	})
	if err != nil {
		return 0, err
	}
	if c.OdooPartnerID == nil || *c.OdooPartnerID != id {
		c.OdooPartnerID = &id
		err = s.repo.SaveCustomer(c)
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

// normalizeVAT turns a tax id as Stripe collected it into Odoo's form:
// Spanish ids get the ES prefix.
func normalizeVAT(taxID, country string) string {
	v := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", ".", "").Replace(taxID))
	if v == "" {
		return ""
	}
	if (country == "" || country == "ES") && !strings.HasPrefix(v, "ES") {
		v = "ES" + v
	}
	return v
}

// isSpanishCompanyID reports whether vat is a Spanish CIF (a company),
// not a person's NIF or NIE.
func isSpanishCompanyID(vat string) bool {
	id, ok := strings.CutPrefix(vat, "ES")
	return ok && id != "" && strings.ContainsRune("ABCDEFGHJNPQRSUVW", rune(id[0]))
}

// ensureOdooProduct returns the Odoo product of a plan (or its setup fee),
// creating it when the plan was saved while Odoo was down.
func (s *Service) ensureOdooProduct(ctx context.Context, planKey string, setupFee bool) (int64, error) {
	plan, err := s.repo.GetPlan(planKey)
	if err != nil {
		return 0, fmt.Errorf("plan %s: %w", planKey, err)
	}
	if !setupFee && plan.OdooProductID != nil {
		return *plan.OdooProductID, nil
	}
	if setupFee && plan.OdooSetupFeeProductID != nil {
		return *plan.OdooSetupFeeProductID, nil
	}
	name := plan.Name
	if setupFee {
		name = plan.SetupFeeName
	}
	id, err := s.odoo.UpsertProduct(ctx, OdooProductCode(planKey, setupFee), name)
	if err != nil {
		return 0, err
	}
	if setupFee {
		plan.OdooSetupFeeProductID = &id
	} else {
		plan.OdooProductID = &id
	}
	return id, s.repo.SavePlan(plan)
}

// RetryDocument puts a document back in the queue, due now, with its
// attempts reset.
func (s *Service) RetryDocument(id string) (*db.BillingDocument, error) {
	d, err := s.repo.GetDocument(id)
	if err != nil {
		return nil, notFound(err)
	}
	if d.OdooState == db.OdooStateDone {
		return nil, fmt.Errorf("%w: document %s is already issued", ErrConflict, id)
	}
	d.OdooState, d.Attempts, d.NextAttemptAt = db.OdooStatePending, 0, s.now()
	err = s.repo.UpdateDocument(d)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// DocumentPDF returns the PDF Odoo stored for an issued document.
func (s *Service) DocumentPDF(ctx context.Context, id string) (string, []byte, error) {
	d, err := s.repo.GetDocument(id)
	if err != nil {
		return "", nil, notFound(err)
	}
	if d.OdooMoveID == nil {
		return "", nil, ErrPDFNotFound
	}
	if s.odoo == nil {
		return "", nil, ErrNotConfigured
	}
	name, data, err := s.odoo.PDF(ctx, *d.OdooMoveID)
	if err != nil {
		return "", nil, err
	}
	if name == "" {
		name = strings.ReplaceAll(d.Number, "/", "_") + ".pdf"
	}
	return name, data, nil
}

// CanViewDocument reports whether userID may see d: the payer of what it
// bills, the user it is issued to, or an owner of the organization.
func (s *Service) CanViewDocument(userID string, d *db.BillingDocument) (bool, error) {
	kind, id, err := ParseSubject(d.Subject)
	if err != nil {
		return false, err
	}
	if kind == SubjectUser && id == userID {
		return true, nil
	}
	payer := ""
	if d.SubscriptionID != nil {
		sub, err := s.repo.GetSubscription(*d.SubscriptionID)
		if err == nil {
			payer = sub.PayerUserID
		}
	}
	if payer == "" && d.PurchaseID != nil {
		p, err := s.repo.GetPurchase(*d.PurchaseID)
		if err == nil {
			payer = p.PayerUserID
		}
	}
	if payer != "" && payer == userID {
		return true, nil
	}
	if kind == SubjectOrg && s.orgs != nil {
		m, err := s.orgs.Member(id, userID)
		if err == nil && m.Role == "owner" {
			return true, nil
		}
	}
	return false, nil
}
