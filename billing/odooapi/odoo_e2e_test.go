//go:build e2e

package odooapi

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// e2eClient connects to the Odoo that e2e/odoo/init.sh prepared
// (`make e2e-up`).
func e2eClient(t *testing.T) *Client {
	t.Helper()
	f, err := os.Open("../../e2e/.odoo/env")
	if err != nil {
		t.Skipf("no e2e Odoo (%v): run make e2e-up", err)
	}
	defer f.Close()
	env := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			env[k] = v
		}
	}
	num := func(k string) int64 {
		n, err := strconv.ParseInt(env[k], 10, 64)
		require.NoError(t, err, k)
		return n
	}
	return New(config.OdooBillingConfig{
		URL: env["ODOO_URL"], Database: env["ODOO_DB"], APIKey: env["ODOO_API_KEY"],
		CompanyID: num("ODOO_COMPANY_ID"), SalesJournalID: num("ODOO_SALES_JOURNAL_ID"),
		PaymentJournalID: num("ODOO_PAYMENT_JOURNAL_ID"), TaxID: num("ODOO_TAX_ID"),
	})
}

func unique(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func issue(t *testing.T, c *Client, partner billing.OdooPartner, cents int64) (int64, int64, billing.OdooIssued) {
	t.Helper()
	ctx := context.Background()
	partnerID, err := c.UpsertPartner(ctx, partner)
	require.NoError(t, err)
	productID, err := c.UpsertProduct(ctx, "TG-e2e-plan", "E2E plan")
	require.NoError(t, err)
	issued, err := c.IssueMove(ctx, billing.OdooMove{
		Ref: unique("in"), PartnerID: partnerID, PaidAt: time.Now(),
		Lines: []billing.OdooLine{{ProductID: productID, Description: "E2E plan, October", AmountCents: cents}},
	})
	require.NoError(t, err)
	return partnerID, productID, issued
}

func company() billing.OdooPartner {
	return billing.OdooPartner{
		Ref: unique("tg:org"), Name: "Escuela E2E SL", Email: "escuela@example.com", VAT: "ESB12345674", IsCompany: true,
		Address: billing.Address{Line1: "C/ Mayor 1", PostalCode: "46001", City: "Valencia", State: "Valencia", Country: "ES"},
	}
}

func TestE2E_UpsertPartnerIsIdempotent(t *testing.T) {
	c := e2eClient(t)
	p := company()
	id1, err := c.UpsertPartner(context.Background(), p)
	require.NoError(t, err)
	p.Name = "Escuela E2E renamed SL"
	id2, err := c.UpsertPartner(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	rec, err := c.readOne(context.Background(), "res.partner", id1, []string{"name", "vat", "state_id", "lang"})
	require.NoError(t, err)
	assert.Equal(t, "Escuela E2E renamed SL", rec["name"])
	assert.Equal(t, "ESB12345674", rec["vat"])
	assert.NotZero(t, id(rec["state_id"]))
	assert.Equal(t, "es_ES", rec["lang"])
}

func TestE2E_InvoiceIsPostedPaidSentAndHasPDF(t *testing.T) {
	c := e2eClient(t)
	ctx := context.Background()
	partnerID, productID, issued := issue(t, c, company(), 2900)
	assert.False(t, issued.Simplified)
	assert.NotEmpty(t, issued.Number)
	assert.NotEqual(t, "/", issued.Number, "a posted move gets its number")

	move, err := c.readOne(ctx, "account.move", issued.MoveID, []string{"state", "payment_state", "is_move_sent", "amount_total", "amount_tax", "ref"})
	require.NoError(t, err)
	assert.Equal(t, "posted", move["state"])
	assert.Contains(t, []string{"paid", "in_payment"}, move["payment_state"])
	assert.Equal(t, true, move["is_move_sent"])
	assert.InDelta(t, 29.0, move["amount_total"], 0.001, "Stripe amounts include VAT")
	assert.InDelta(t, 5.03, move["amount_tax"], 0.001)

	again, err := c.IssueMove(ctx, billing.OdooMove{
		Ref: str(move["ref"]), PartnerID: partnerID,
		Lines: []billing.OdooLine{{ProductID: productID, Description: "x", AmountCents: 2900}},
	})
	require.NoError(t, err)
	assert.Equal(t, issued, again)

	name, data, err := c.PDF(ctx, issued.MoveID)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(name, ".pdf"), name)
	assert.True(t, strings.HasPrefix(string(data), "%PDF"))
}

func TestE2E_CreditNoteHasItsOwnSeries(t *testing.T) {
	c := e2eClient(t)
	partnerID, productID, invoice := issue(t, c, company(), 2900)
	credit, err := c.IssueMove(context.Background(), billing.OdooMove{
		Ref: unique("re"), PartnerID: partnerID, CreditNote: true, ReversedMoveID: invoice.MoveID, PaidAt: time.Now(),
		Lines: []billing.OdooLine{{ProductID: productID, Description: "Refund", AmountCents: 1000}},
	})
	require.NoError(t, err)
	series := func(number string) string { return number[:strings.LastIndex(number, "/")] }
	assert.NotEqual(t, series(invoice.Number), series(credit.Number), "%s vs %s", invoice.Number, credit.Number)

	move, err := c.readOne(context.Background(), "account.move", credit.MoveID, []string{"move_type", "reversed_entry_id", "payment_state", "amount_total"})
	require.NoError(t, err)
	assert.Equal(t, "out_refund", move["move_type"])
	assert.Equal(t, invoice.MoveID, id(move["reversed_entry_id"]))
	assert.Contains(t, []string{"paid", "in_payment"}, move["payment_state"])
	assert.InDelta(t, 10.0, move["amount_total"], 0.001)
}

func TestE2E_SimplifiedInvoiceWithoutTaxID(t *testing.T) {
	c := e2eClient(t)
	_, _, issued := issue(t, c, billing.OdooPartner{
		Ref: unique("tg:user"), Name: "Ana García", Email: "", Address: billing.Address{City: "Valencia", Country: "ES"},
	}, 2900)
	assert.True(t, issued.Simplified, "l10n_es marks it F2")
	_, data, err := c.PDF(context.Background(), issued.MoveID)
	require.NoError(t, err, "a partner without email still gets a PDF")
	assert.True(t, strings.HasPrefix(string(data), "%PDF"))
}

func TestE2E_Checks(t *testing.T) {
	c := e2eClient(t)
	checks := c.Checks(context.Background())
	require.Len(t, checks, 6)
	for _, ch := range checks {
		assert.Equal(t, billing.CheckOK, ch.Status, "%s: %s", ch.ID, ch.Detail)
	}
}

// verifactuCompany skips unless the e2e Odoo was prepared with
// E2E_ODOO_VERIFACTU=1 (the company is subject to Veri*Factu).
func verifactuCompany(t *testing.T, c *Client) {
	t.Helper()
	ctx := context.Background()
	installed, err := c.hasField(ctx, "res.company", "l10n_es_edi_verifactu_required")
	require.NoError(t, err)
	if !installed {
		t.Skip("l10n_es_edi_verifactu is not installed: run E2E_ODOO_VERIFACTU=1 ./e2e/odoo/init.sh")
	}
	var companies []M
	err = c.call(ctx, "res.company", "read", M{"ids": []int64{c.cfg.CompanyID}, "fields": []string{"l10n_es_edi_verifactu_required"}}, nil, &companies)
	require.NoError(t, err)
	if required, _ := companies[0]["l10n_es_edi_verifactu_required"].(bool); !required {
		t.Skip("the e2e company is not subject to Veri*Factu: run E2E_ODOO_VERIFACTU=1 ./e2e/odoo/init.sh")
	}
}

func TestE2E_VerifactuRegistersInvoicesAndCreditNotes(t *testing.T) {
	c := e2eClient(t)
	verifactuCompany(t, c)
	ctx := context.Background()
	partnerID, productID, invoice := issue(t, c, company(), 2900)
	credit, err := c.IssueMove(ctx, billing.OdooMove{
		Ref: unique("re"), PartnerID: partnerID, CreditNote: true, ReversedMoveID: invoice.MoveID, PaidAt: time.Now(),
		Lines: []billing.OdooLine{{ProductID: productID, Description: "Refund", AmountCents: 1000}},
	})
	require.NoError(t, err)

	fields := []string{"state", "l10n_es_edi_verifactu_state", "l10n_es_edi_verifactu_document_ids", "l10n_es_edi_verifactu_refund_reason"}
	for _, issued := range []billing.OdooIssued{invoice, credit} {
		move, err := c.readOne(ctx, "account.move", issued.MoveID, fields)
		require.NoError(t, err)
		assert.Equal(t, "posted", move["state"], issued.Number)
		documents, _ := move["l10n_es_edi_verifactu_document_ids"].([]any)
		assert.Len(t, documents, 1, "%s is queued for the tax agency", issued.Number)
		_, data, err := c.PDF(ctx, issued.MoveID)
		require.NoError(t, err, issued.Number)
		assert.True(t, strings.HasPrefix(string(data), "%PDF"))
	}
	move, err := c.readOne(ctx, "account.move", credit.MoveID, fields)
	require.NoError(t, err)
	assert.Equal(t, "R1", move["l10n_es_edi_verifactu_refund_reason"])
}
