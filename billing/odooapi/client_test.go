package odooapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOdoo stands in for Odoo's JSON-2 endpoint: handlers answer per
// "model/method"; every call is recorded.
type fakeOdoo struct {
	t        *testing.T
	mu       sync.Mutex
	handlers map[string]func(body M) any
	calls    []fakeCall
}

type fakeCall struct {
	Route string
	Body  M
}

func newFake(t *testing.T) (*fakeOdoo, *Client) {
	t.Helper()
	f := &fakeOdoo{t: t, handlers: map[string]func(M) any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bearer key-1", r.Header.Get("Authorization"))
		assert.Equal(t, "taronja", r.Header.Get("X-Odoo-Database"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		route := strings.TrimPrefix(r.URL.Path, "/json/2/")
		var body M
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		f.mu.Lock()
		f.calls = append(f.calls, fakeCall{Route: route, Body: body})
		h, ok := f.handlers[route]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(M{"name": "werkzeug.exceptions.NotFound", "message": "no fake for " + route})
			return
		}
		_ = json.NewEncoder(w).Encode(h(body))
	}))
	t.Cleanup(srv.Close)
	return f, New(config.OdooBillingConfig{
		URL: srv.URL, Database: "taronja", APIKey: "key-1", CompanyID: 1, SalesJournalID: 7, PaymentJournalID: 14, TaxID: 174,
	})
}

func (f *fakeOdoo) on(route string, h func(body M) any) { f.handlers[route] = h }

func (f *fakeOdoo) routes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.Route)
	}
	return out
}

func (f *fakeOdoo) call(t *testing.T, route string) M {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.Route == route {
			return c.Body
		}
	}
	t.Fatalf("no call to %s; calls: %v", route, f.calls)
	return nil
}

func none(M) any { return []any{} }

func TestNotConfigured(t *testing.T) {
	c := New(config.OdooBillingConfig{})
	_, err := c.UpsertPartner(context.Background(), billing.OdooPartner{Ref: "tg:user:1"})
	assert.ErrorIs(t, err, billing.ErrNotConfigured)
	_, err = c.IssueMove(context.Background(), billing.OdooMove{Ref: "in_1"})
	assert.ErrorIs(t, err, billing.ErrNotConfigured)
	_, _, err = c.PDF(context.Background(), 1)
	assert.ErrorIs(t, err, billing.ErrNotConfigured)
	checks := c.Checks(context.Background())
	require.Len(t, checks, 1)
	assert.Equal(t, billing.CheckMissing, checks[0].Status)
}

func TestUpsertPartner_CreatesWithNamedArguments(t *testing.T) {
	f, c := newFake(t)
	f.on("res.partner/search_read", none)
	f.on("res.country/search_read", func(M) any { return []M{{"id": 68}} })
	f.on("res.country.state/search_read", func(M) any { return []M{{"id": 420}} })
	f.on("res.lang/search_read", func(M) any { return []M{{"id": 2}} })
	f.on("res.partner/create", func(M) any { return []int64{55} })

	id, err := c.UpsertPartner(context.Background(), billing.OdooPartner{
		Ref: "tg:org:1", Name: "Escuela SL", Email: "e@x.es", VAT: "ESB12345674", IsCompany: true,
		Address: billing.Address{Line1: "C/ Mayor 1", PostalCode: "46001", City: "Valencia", State: "Valencia", Country: "ES"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(55), id)

	search := f.call(t, "res.partner/search_read")
	assert.Equal(t, []any{[]any{"ref", "=", "tg:org:1"}}, search["domain"])
	assert.Equal(t, M{"allowed_company_ids": []any{float64(1)}}, search["context"])
	vals := f.call(t, "res.partner/create")["vals_list"].([]any)[0].(M)
	assert.Equal(t, "ESB12345674", vals["vat"])
	assert.Equal(t, float64(68), vals["country_id"])
	assert.Equal(t, float64(420), vals["state_id"])
	assert.Equal(t, "es_ES", vals["lang"])
	assert.Equal(t, true, vals["is_company"])
}

func TestUpsertPartner_UpdatesExistingWithoutClearingVAT(t *testing.T) {
	f, c := newFake(t)
	f.on("res.partner/search_read", func(M) any { return []M{{"id": 9}} })
	f.on("res.country/search_read", func(M) any { return []M{{"id": 68}} })
	f.on("res.lang/search_read", none)
	f.on("res.partner/write", func(M) any { return true })

	id, err := c.UpsertPartner(context.Background(), billing.OdooPartner{Ref: "tg:user:1", Name: "Ana"})
	require.NoError(t, err)
	assert.Equal(t, int64(9), id)
	write := f.call(t, "res.partner/write")
	assert.Equal(t, []any{float64(9)}, write["ids"])
	vals := write["vals"].(M)
	assert.NotContains(t, vals, "vat")
	assert.NotContains(t, vals, "lang")
	assert.NotContains(t, f.routes(), "res.country.state/search_read")
}

func TestUpsertProduct(t *testing.T) {
	f, c := newFake(t)
	f.on("product.product/search_read", none)
	f.on("product.product/create", func(M) any { return []int64{3} })
	id, err := c.UpsertProduct(context.Background(), "TG-academia-monthly", "Academia mensual")
	require.NoError(t, err)
	assert.Equal(t, int64(3), id)
	vals := f.call(t, "product.product/create")["vals_list"].([]any)[0].(M)
	assert.Equal(t, "service", vals["type"])
	assert.Equal(t, "TG-academia-monthly", vals["default_code"])
	assert.Equal(t, []any{[]any{float64(6), float64(0), []any{float64(174)}}}, vals["taxes_id"])

	f.on("product.product/search_read", func(M) any { return []M{{"id": 3}} })
	f.on("product.product/write", func(M) any { return true })
	_, err = c.UpsertProduct(context.Background(), "TG-academia-monthly", "Renamed")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", f.call(t, "product.product/write")["vals"].(M)["name"])
}

func moveFieldsGet(M) any {
	return M{"name": M{"type": "char"}, "l10n_es_is_simplified": M{"type": "boolean"}}
}

func TestIssueMove_NewInvoiceRunsEveryStep(t *testing.T) {
	f, c := newFake(t)
	f.on("account.move/fields_get", moveFieldsGet)
	searches := 0
	f.on("account.move/search_read", func(body M) any {
		searches++
		if body["fields"].([]any)[0] == "id" && len(body["fields"].([]any)) == 1 {
			return []any{}
		}
		if searches <= 2 {
			return []M{{"id": 100, "state": "draft", "payment_state": "not_paid", "is_move_sent": false, "partner_id": []any{55, "Ana"}}}
		}
		return []M{{"id": 100, "name": "INV/2026/00001", "invoice_date": "2026-10-25", "l10n_es_is_simplified": true}}
	})
	f.on("account.move/create", func(M) any { return []int64{100} })
	f.on("account.move/action_post", func(M) any { return true })
	f.on("account.payment.register/create", func(M) any { return []int64{5} })
	f.on("account.payment.register/action_create_payments", func(M) any { return M{} })
	f.on("res.partner/search_read", func(M) any { return []M{{"id": 55, "email": false}} })
	f.on("account.move.send.wizard/create", func(M) any { return []int64{6} })
	f.on("account.move.send.wizard/action_send_and_print", func(M) any { return M{} })
	f.on("account.move/write", func(M) any { return true })

	paidAt := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	issued, err := c.IssueMove(context.Background(), billing.OdooMove{
		Ref: "in_1", PartnerID: 55, PaidAt: paidAt,
		Lines: []billing.OdooLine{{ProductID: 3, Description: "Academia", AmountCents: 2900}},
	})
	require.NoError(t, err)
	assert.Equal(t, billing.OdooIssued{MoveID: 100, Number: "INV/2026/00001", IssuedAt: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), Simplified: true}, issued)

	vals := f.call(t, "account.move/create")["vals_list"].([]any)[0].(M)
	assert.Equal(t, "out_invoice", vals["move_type"])
	assert.Equal(t, float64(7), vals["journal_id"])
	assert.Equal(t, "2026-10-25", vals["invoice_date"])
	line := vals["invoice_line_ids"].([]any)[0].([]any)[2].(M)
	assert.Equal(t, 29.0, line["price_unit"])
	assert.NotContains(t, vals, "reversed_entry_id")

	pay := f.call(t, "account.payment.register/create")
	assert.Equal(t, M{"allowed_company_ids": []any{float64(1)}, "active_model": "account.move", "active_ids": []any{float64(100)}}, pay["context"])
	assert.Equal(t, float64(14), pay["vals_list"].([]any)[0].(M)["journal_id"])
	send := f.call(t, "account.move.send.wizard/create")["vals_list"].([]any)[0].(M)
	assert.Equal(t, []any{"manual"}, send["sending_methods"])
}

func TestIssueMove_ExistingFinishedMoveMakesNoChanges(t *testing.T) {
	f, c := newFake(t)
	f.on("account.move/fields_get", moveFieldsGet)
	f.on("account.move/search_read", func(M) any {
		return []M{{"id": 100, "name": "INV/2026/00001", "state": "posted", "payment_state": "paid", "is_move_sent": true, "invoice_date": "2026-10-25"}}
	})
	issued, err := c.IssueMove(context.Background(), billing.OdooMove{Ref: "in_1", PartnerID: 55})
	require.NoError(t, err)
	assert.Equal(t, int64(100), issued.MoveID)
	for _, r := range f.routes() {
		assert.True(t, strings.HasSuffix(r, "/search_read") || strings.HasSuffix(r, "/fields_get"), r)
	}
}

func TestIssueMove_ResumesAfterPosting(t *testing.T) {
	f, c := newFake(t)
	f.on("account.move/fields_get", moveFieldsGet)
	f.on("account.move/search_read", func(M) any {
		return []M{{"id": 100, "name": "INV/2026/00001", "state": "posted", "payment_state": "not_paid", "is_move_sent": true, "invoice_date": "2026-10-25"}}
	})
	f.on("account.payment.register/create", func(M) any { return []int64{5} })
	f.on("account.payment.register/action_create_payments", func(M) any { return M{} })
	_, err := c.IssueMove(context.Background(), billing.OdooMove{Ref: "in_1", PartnerID: 55})
	require.NoError(t, err)
	assert.NotContains(t, f.routes(), "account.move/create")
	assert.NotContains(t, f.routes(), "account.move/action_post")
	assert.Contains(t, f.routes(), "account.payment.register/action_create_payments")
	assert.NotContains(t, f.routes(), "account.move.send.wizard/create")
}

func TestIssueMove_CreditNote(t *testing.T) {
	f, c := newFake(t)
	f.on("account.move/fields_get", func(M) any { return M{"l10n_es_edi_verifactu_refund_reason": M{"type": "selection"}} })
	f.on("account.move/search_read", none)
	f.on("account.journal/search_read", func(M) any { return []M{{"id": 7, "refund_sequence": false}} })
	_, err := c.IssueMove(context.Background(), billing.OdooMove{Ref: "re_1", CreditNote: true, ReversedMoveID: 100})
	assert.ErrorIs(t, err, billing.ErrNoCreditNoteSeries)
	assert.NotContains(t, f.routes(), "account.move/create")

	f.on("account.journal/search_read", func(M) any { return []M{{"id": 7, "refund_sequence": true}} })
	f.on("account.move/create", func(M) any { return []int64{101} })
	_, _ = c.IssueMove(context.Background(), billing.OdooMove{Ref: "re_1", CreditNote: true, ReversedMoveID: 100})
	vals := f.call(t, "account.move/create")["vals_list"].([]any)[0].(M)
	assert.Equal(t, "out_refund", vals["move_type"])
	assert.Equal(t, float64(100), vals["reversed_entry_id"])
	assert.Equal(t, "R1", vals["l10n_es_edi_verifactu_refund_reason"])
}

func TestIssueMove_OdooErrorIsReturned(t *testing.T) {
	f, c := newFake(t)
	f.on("account.move/fields_get", moveFieldsGet)
	_, err := c.IssueMove(context.Background(), billing.OdooMove{Ref: "in_1"})
	var odooErr *Error
	require.True(t, errors.As(err, &odooErr), err)
	assert.Equal(t, http.StatusNotFound, odooErr.Status)
	assert.Contains(t, odooErr.Message, "account.move/search_read")
}

func TestPDF(t *testing.T) {
	f, c := newFake(t)
	f.on("ir.attachment/fields_get", func(M) any { return M{"name": M{}, "datas": M{}} })
	f.on("account.move/search_read", func(M) any { return []M{{"id": 100, "invoice_pdf_report_id": []any{77, "INV_2026_00001.pdf"}}} })
	f.on("ir.attachment/read", func(M) any {
		return []M{{"id": 77, "name": "INV_2026_00001.pdf", "datas": base64.StdEncoding.EncodeToString([]byte("%PDF-1.7 x"))}}
	})
	name, data, err := c.PDF(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, "INV_2026_00001.pdf", name)
	assert.Equal(t, "%PDF-1.7 x", string(data))

	f.on("account.move/search_read", func(M) any { return []M{{"id": 100, "invoice_pdf_report_id": false}} })
	_, _, err = c.PDF(context.Background(), 100)
	assert.ErrorIs(t, err, billing.ErrPDFNotFound)
}

func TestChecks(t *testing.T) {
	f, c := newFake(t)
	f.on("res.users/context_get", func(M) any { return M{"lang": "es_ES"} })
	f.on("ir.module.module/search_read", func(M) any { return []M{{"id": 1}} })
	f.on("res.company/search_read", func(M) any { return []M{{"id": 1, "name": "Taronja SL", "vat": "ESB12345674"}} })
	f.on("account.journal/search_read", func(body M) any {
		if body["domain"].([]any)[0].([]any)[2] == float64(7) {
			return []M{{"id": 7, "name": "Facturas", "type": "sale", "refund_sequence": false}}
		}
		return []M{{"id": 14, "name": "Stripe", "type": "bank"}}
	})
	f.on("account.tax/search_read", func(M) any {
		return []M{{"id": 174, "name": "21% IVA incluido", "type_tax_use": "sale", "price_include": true}}
	})
	status := map[string]string{}
	for _, ch := range c.Checks(context.Background()) {
		status[ch.ID] = ch.Status
	}
	assert.Equal(t, map[string]string{
		"odoo-connection": billing.CheckOK, "odoo-l10n-es": billing.CheckOK, "odoo-company": billing.CheckOK,
		"odoo-sales-journal": billing.CheckMissing, "odoo-payment-journal": billing.CheckOK, "odoo-tax": billing.CheckOK,
	}, status)

	f.on("res.users/context_get", func(M) any { return nil })
	delete(f.handlers, "res.users/context_get")
	checks := c.Checks(context.Background())
	require.Len(t, checks, 1)
	assert.Equal(t, billing.CheckMissing, checks[0].Status)
}
