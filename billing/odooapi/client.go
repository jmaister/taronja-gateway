// Package odooapi implements billing.Odoo over Odoo's JSON-2 API
// (POST /json/2/<model>/<method>, Odoo 19).
package odooapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/config"
)

// Client talks to one Odoo database as one API user.
type Client struct {
	cfg  config.OdooBillingConfig
	http *http.Client

	mu     sync.Mutex
	fields map[string]map[string]bool // model -> field names, from fields_get
}

// New returns a client for cfg; with no URL every call returns
// billing.ErrNotConfigured.
func New(cfg config.OdooBillingConfig) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second}, fields: map[string]map[string]bool{}}
}

var _ billing.Odoo = (*Client)(nil)

// Error is an error Odoo answered with.
type Error struct {
	Status  int
	Name    string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("odoo: %s (%s, HTTP %d)", e.Message, e.Name, e.Status)
}

// M is a JSON object: named arguments, values, a record.
type M = map[string]any

// call runs model.method with named args; extraCtx is merged into the
// context, which always pins the configured company.
func (c *Client) call(ctx context.Context, model, method string, args M, extraCtx M, out any) error {
	if !c.cfg.IsConfigured() {
		return billing.ErrNotConfigured
	}
	callCtx := M{"allowed_company_ids": []int64{c.cfg.CompanyID}}
	for k, v := range extraCtx {
		callCtx[k] = v
	}
	body := M{"context": callCtx}
	for k, v := range args {
		body[k] = v
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("odoo: encoding %s.%s: %w", model, method, err)
	}
	url := strings.TrimSuffix(c.cfg.URL, "/") + "/json/2/" + model + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+c.cfg.APIKey)
	req.Header.Set("X-Odoo-Database", c.cfg.Database)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("odoo: %s.%s: %w", model, method, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("odoo: reading %s.%s: %w", model, method, err)
	}
	if resp.StatusCode/100 != 2 {
		odooErr := &Error{Status: resp.StatusCode}
		_ = json.Unmarshal(data, odooErr)
		if odooErr.Message == "" {
			odooErr.Message = strings.TrimSpace(string(data))
		}
		return odooErr
	}
	if out == nil {
		return nil
	}
	err = json.Unmarshal(data, out)
	if err != nil {
		return fmt.Errorf("odoo: decoding %s.%s: %w", model, method, err)
	}
	return nil
}

func (c *Client) searchRead(ctx context.Context, model string, domain []any, fields []string, limit int) ([]M, error) {
	args := M{"domain": domain, "fields": fields}
	if limit > 0 {
		args["limit"] = limit
	}
	var records []M
	err := c.call(ctx, model, "search_read", args, nil, &records)
	return records, err
}

func (c *Client) readOne(ctx context.Context, model string, id int64, fields []string) (M, error) {
	records, err := c.searchRead(ctx, model, []any{[]any{"id", "=", id}}, fields, 1)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return records[0], nil
}

func (c *Client) create(ctx context.Context, model string, vals M, extraCtx M) (int64, error) {
	var ids []int64
	err := c.call(ctx, model, "create", M{"vals_list": []M{vals}}, extraCtx, &ids)
	if err != nil {
		return 0, err
	}
	if len(ids) != 1 {
		return 0, fmt.Errorf("odoo: %s.create returned %d ids", model, len(ids))
	}
	return ids[0], nil
}

func (c *Client) write(ctx context.Context, model string, id int64, vals M) error {
	return c.call(ctx, model, "write", M{"ids": []int64{id}, "vals": vals}, nil, nil)
}

// hasField reports whether model has field (cached per model), so optional
// modules (Veri*Factu, l10n_es) are used only when installed.
func (c *Client) hasField(ctx context.Context, model, field string) (bool, error) {
	c.mu.Lock()
	known, ok := c.fields[model]
	c.mu.Unlock()
	if !ok {
		var defs map[string]any
		err := c.call(ctx, model, "fields_get", M{"attributes": []string{"type"}}, nil, &defs)
		if err != nil {
			return false, err
		}
		known = make(map[string]bool, len(defs))
		for name := range defs {
			known[name] = true
		}
		c.mu.Lock()
		c.fields[model] = known
		c.mu.Unlock()
	}
	return known[field], nil
}

// Odoo answers false for an empty field and [id, name] for a many2one.

func str(v any) string {
	s, _ := v.(string)
	return s
}

func boolean(v any) bool {
	b, _ := v.(bool)
	return b
}

func id(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case []any:
		if len(x) > 0 {
			return id(x[0])
		}
	}
	return 0
}

func firstID(ctx context.Context, c *Client, model string, domain []any) (int64, error) {
	records, err := c.searchRead(ctx, model, domain, []string{"id"}, 1)
	if err != nil || len(records) == 0 {
		return 0, err
	}
	return id(records[0]["id"]), nil
}

func (c *Client) UpsertPartner(ctx context.Context, p billing.OdooPartner) (int64, error) {
	partnerID, err := firstID(ctx, c, "res.partner", []any{[]any{"ref", "=", p.Ref}})
	if err != nil {
		return 0, err
	}
	country := p.Address.Country
	if country == "" {
		country = "ES"
	}
	vals := M{
		"name": p.Name, "email": p.Email, "is_company": p.IsCompany, "ref": p.Ref,
		"street": p.Address.Line1, "street2": p.Address.Line2, "zip": p.Address.PostalCode, "city": p.Address.City,
	}
	if p.VAT != "" {
		vals["vat"] = p.VAT
	}
	countryID, err := firstID(ctx, c, "res.country", []any{[]any{"code", "=", strings.ToUpper(country)}})
	if err != nil {
		return 0, err
	}
	if countryID != 0 {
		vals["country_id"] = countryID
		if p.Address.State != "" {
			stateID, err := firstID(ctx, c, "res.country.state", []any{
				[]any{"country_id", "=", countryID}, "|",
				[]any{"name", "ilike", p.Address.State}, []any{"code", "=", p.Address.State},
			})
			if err != nil {
				return 0, err
			}
			if stateID != 0 {
				vals["state_id"] = stateID
			}
		}
	}
	langID, err := firstID(ctx, c, "res.lang", []any{[]any{"code", "=", "es_ES"}})
	if err != nil {
		return 0, err
	}
	if langID != 0 {
		vals["lang"] = "es_ES"
	}
	if partnerID == 0 {
		return c.create(ctx, "res.partner", vals, nil)
	}
	return partnerID, c.write(ctx, "res.partner", partnerID, vals)
}

func (c *Client) UpsertProduct(ctx context.Context, code, name string) (int64, error) {
	productID, err := firstID(ctx, c, "product.product", []any{[]any{"default_code", "=", code}})
	if err != nil {
		return 0, err
	}
	if productID != 0 {
		return productID, c.write(ctx, "product.product", productID, M{"name": name})
	}
	return c.create(ctx, "product.product", M{
		"name": name, "default_code": code, "type": "service", "sale_ok": true,
		"taxes_id": []any{[]any{6, 0, []int64{c.cfg.TaxID}}},
	}, nil)
}

var moveFields = []string{"id", "name", "state", "payment_state", "is_move_sent", "invoice_date", "l10n_es_is_simplified"}

// IssueMove runs each step only if the move still needs it, so a retry
// after a failure halfway (posted but not paid, say) finishes the job.
func (c *Client) IssueMove(ctx context.Context, m billing.OdooMove) (billing.OdooIssued, error) {
	moveType := "out_invoice"
	if m.CreditNote {
		moveType = "out_refund"
	}
	find := []any{[]any{"ref", "=", m.Ref}, []any{"move_type", "=", moveType}, []any{"company_id", "=", c.cfg.CompanyID}}
	records, err := c.searchRead(ctx, "account.move", find, []string{"id"}, 1)
	if err != nil {
		return billing.OdooIssued{}, err
	}
	var moveID int64
	if len(records) > 0 {
		moveID = id(records[0]["id"])
	} else {
		moveID, err = c.createMove(ctx, m, moveType)
		if err != nil {
			return billing.OdooIssued{}, err
		}
	}

	move, err := c.readOne(ctx, "account.move", moveID, moveFields)
	if err != nil {
		return billing.OdooIssued{}, err
	}
	if str(move["state"]) == "draft" {
		err = c.call(ctx, "account.move", "action_post", M{"ids": []int64{moveID}}, nil, nil)
		if err != nil {
			return billing.OdooIssued{}, fmt.Errorf("posting %s: %w", m.Ref, err)
		}
	}
	switch str(move["payment_state"]) {
	case "paid", "in_payment", "reversed":
	default:
		err = c.registerPayment(ctx, moveID, m)
		if err != nil {
			return billing.OdooIssued{}, fmt.Errorf("registering the payment of %s: %w", m.Ref, err)
		}
	}
	if !boolean(move["is_move_sent"]) {
		err = c.renderPDF(ctx, moveID)
		if err != nil {
			return billing.OdooIssued{}, fmt.Errorf("rendering the PDF of %s: %w", m.Ref, err)
		}
	}

	move, err = c.readOne(ctx, "account.move", moveID, moveFields)
	if err != nil {
		return billing.OdooIssued{}, err
	}
	issued := billing.OdooIssued{MoveID: moveID, Number: str(move["name"]), Simplified: boolean(move["l10n_es_is_simplified"])}
	issuedAt, err := time.Parse(time.DateOnly, str(move["invoice_date"]))
	if err == nil {
		issued.IssuedAt = issuedAt
	}
	return issued, nil
}

func (c *Client) createMove(ctx context.Context, m billing.OdooMove, moveType string) (int64, error) {
	if m.CreditNote {
		journal, err := c.readOne(ctx, "account.journal", c.cfg.SalesJournalID, []string{"refund_sequence"})
		if err != nil {
			return 0, err
		}
		if journal == nil {
			return 0, fmt.Errorf("odoo: sales journal %d not found", c.cfg.SalesJournalID)
		}
		if !boolean(journal["refund_sequence"]) {
			return 0, billing.ErrNoCreditNoteSeries
		}
	}
	lines := make([]any, 0, len(m.Lines))
	for _, l := range m.Lines {
		lines = append(lines, []any{0, 0, M{
			"product_id": l.ProductID,
			"name":       l.Description,
			"quantity":   1,
			"price_unit": math.Round(float64(l.AmountCents)) / 100,
			"tax_ids":    []any{[]any{6, 0, []int64{c.cfg.TaxID}}},
		}})
	}
	paidAt := m.PaidAt
	if paidAt.IsZero() {
		paidAt = time.Now()
	}
	vals := M{
		"move_type":        moveType,
		"partner_id":       m.PartnerID,
		"journal_id":       c.cfg.SalesJournalID,
		"company_id":       c.cfg.CompanyID,
		"invoice_date":     paidAt.Format(time.DateOnly),
		"ref":              m.Ref,
		"invoice_line_ids": lines,
	}
	if m.CreditNote && m.ReversedMoveID != 0 {
		vals["reversed_entry_id"] = m.ReversedMoveID
		ok, err := c.hasField(ctx, "account.move", "l10n_es_edi_verifactu_refund_reason")
		if err != nil {
			return 0, err
		}
		if ok {
			vals["l10n_es_edi_verifactu_refund_reason"] = "R1"
		}
	}
	return c.create(ctx, "account.move", vals, nil)
}

// registerPayment records the Stripe payment (or, for a credit note, the
// refund) in the payment journal, reconciled with the move.
func (c *Client) registerPayment(ctx context.Context, moveID int64, m billing.OdooMove) error {
	paidAt := m.PaidAt
	if paidAt.IsZero() {
		paidAt = time.Now()
	}
	wizardCtx := M{"active_model": "account.move", "active_ids": []int64{moveID}}
	wizardID, err := c.create(ctx, "account.payment.register", M{
		"journal_id":    c.cfg.PaymentJournalID,
		"payment_date":  paidAt.Format(time.DateOnly),
		"communication": m.Ref,
	}, wizardCtx)
	if err != nil {
		return err
	}
	return c.call(ctx, "account.payment.register", "action_create_payments", M{"ids": []int64{wizardID}}, wizardCtx, nil)
}

// renderPDF has Odoo render the PDF (the gateway emails it) and register the
// move with the required electronic invoicing systems.
func (c *Client) renderPDF(ctx context.Context, moveID int64) error {
	extraEdis, err := c.requiredEdis(ctx, moveID)
	if err != nil {
		return err
	}
	wizardCtx := M{"active_model": "account.move", "active_ids": []int64{moveID}}
	wizardID, err := c.create(ctx, "account.move.send.wizard", M{
		"move_id":         moveID,
		"sending_methods": []string{"manual"},
		"extra_edis":      extraEdis,
	}, wizardCtx)
	if err != nil {
		return err
	}
	err = c.call(ctx, "account.move.send.wizard", "action_send_and_print", M{"ids": []int64{wizardID}, "allow_fallback_pdf": true}, wizardCtx, nil)
	if err != nil {
		return err
	}
	// "manual" doesn't mark the move as sent; mark it so retries skip it.
	return c.write(ctx, "account.move", moveID, M{"is_move_sent": true})
}

// requiredEdis are the electronic invoicing systems the move must be
// registered with when it is sent: Veri*Factu when the company is subject to
// it (module l10n_es_edi_verifactu). Odoo queues the registration; its cron
// sends it to the tax agency.
func (c *Client) requiredEdis(ctx context.Context, moveID int64) ([]string, error) {
	edis := []string{}
	installed, err := c.hasField(ctx, "account.move", "l10n_es_edi_verifactu_required")
	if err != nil || !installed {
		return edis, err
	}
	move, err := c.readOne(ctx, "account.move", moveID, []string{"l10n_es_edi_verifactu_required"})
	if err != nil {
		return nil, err
	}
	required, _ := move["l10n_es_edi_verifactu_required"].(bool)
	if required {
		edis = append(edis, "es_verifactu")
	}
	return edis, nil
}

func (c *Client) PDF(ctx context.Context, moveID int64) (string, []byte, error) {
	move, err := c.readOne(ctx, "account.move", moveID, []string{"invoice_pdf_report_id"})
	if err != nil {
		return "", nil, err
	}
	attachmentID := id(move["invoice_pdf_report_id"])
	if attachmentID == 0 {
		return "", nil, billing.ErrPDFNotFound
	}
	// Read by id: search_read hides attachments that belong to a field.
	var attachments []M
	err = c.call(ctx, "ir.attachment", "read", M{"ids": []int64{attachmentID}, "fields": []string{"name", "datas"}}, nil, &attachments)
	if err != nil {
		return "", nil, err
	}
	if len(attachments) == 0 {
		return "", nil, billing.ErrPDFNotFound
	}
	attachment := attachments[0]
	encoded := str(attachment["datas"])
	if encoded == "" {
		return "", nil, billing.ErrPDFNotFound
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("odoo: decoding the PDF of move %d: %w", moveID, err)
	}
	return str(attachment["name"]), data, nil
}

// Checks verifies what doc/billing.md's setup page lists for Odoo.
func (c *Client) Checks(ctx context.Context) []billing.SetupCheck {
	check := func(checkID, title string) billing.SetupCheck {
		return billing.SetupCheck{ID: checkID, Title: title, Status: billing.CheckOK}
	}
	missing := func(s billing.SetupCheck, detail string) billing.SetupCheck {
		s.Status, s.Detail = billing.CheckMissing, detail
		return s
	}
	conn := check("odoo-connection", "Odoo connection and API key")
	if !c.cfg.IsConfigured() {
		return []billing.SetupCheck{missing(conn, "billing.odoo.url is not set")}
	}
	err := c.call(ctx, "res.users", "context_get", nil, nil, nil)
	if err != nil {
		return []billing.SetupCheck{missing(conn, err.Error())}
	}
	checks := []billing.SetupCheck{conn}
	link := strings.TrimSuffix(c.cfg.URL, "/") + "/odoo/"

	l10n := check("odoo-l10n-es", "Spanish localization (l10n_es)")
	l10n.Link = link + "apps"
	mods, err := c.searchRead(ctx, "ir.module.module", []any{[]any{"name", "=", "l10n_es"}, []any{"state", "=", "installed"}}, []string{"id"}, 1)
	switch {
	case err != nil:
		l10n = missing(l10n, err.Error())
	case len(mods) == 0:
		l10n = missing(l10n, "install the Spain - Accounting app")
	}
	checks = append(checks, l10n)

	company := check("odoo-company", "Company")
	rec, err := c.readOne(ctx, "res.company", c.cfg.CompanyID, []string{"name", "vat"})
	switch {
	case err != nil:
		company = missing(company, err.Error())
	case rec == nil:
		company = missing(company, fmt.Sprintf("company %d not found (billing.odoo.companyId)", c.cfg.CompanyID))
	case str(rec["vat"]) == "":
		company = missing(company, str(rec["name"])+" has no tax id")
	default:
		company.Detail = str(rec["name"])
	}
	checks = append(checks, company)

	sales := check("odoo-sales-journal", "Sales journal with its own credit note series")
	sales.Link = link + "action-account.action_account_journal_form"
	rec, err = c.readOne(ctx, "account.journal", c.cfg.SalesJournalID, []string{"name", "type", "refund_sequence"})
	switch {
	case err != nil:
		sales = missing(sales, err.Error())
	case rec == nil:
		sales = missing(sales, fmt.Sprintf("journal %d not found (billing.odoo.salesJournalId)", c.cfg.SalesJournalID))
	case str(rec["type"]) != "sale":
		sales = missing(sales, str(rec["name"])+" is not a sales journal")
	case !boolean(rec["refund_sequence"]):
		sales = missing(sales, billing.ErrNoCreditNoteSeries.Error())
	default:
		sales.Detail = str(rec["name"])
	}
	checks = append(checks, sales)

	pay := check("odoo-payment-journal", "Bank journal for Stripe payments")
	pay.Link = sales.Link
	rec, err = c.readOne(ctx, "account.journal", c.cfg.PaymentJournalID, []string{"name", "type"})
	switch {
	case err != nil:
		pay = missing(pay, err.Error())
	case rec == nil:
		pay = missing(pay, fmt.Sprintf("journal %d not found (billing.odoo.paymentJournalId)", c.cfg.PaymentJournalID))
	case str(rec["type"]) != "bank":
		pay = missing(pay, str(rec["name"])+" is not a bank journal")
	default:
		pay.Detail = str(rec["name"])
	}
	checks = append(checks, pay)

	tax := check("odoo-tax", "Sales tax included in price")
	tax.Link = link + "action-account.action_tax_form"
	rec, err = c.readOne(ctx, "account.tax", c.cfg.TaxID, []string{"name", "type_tax_use", "price_include"})
	switch {
	case err != nil:
		tax = missing(tax, err.Error())
	case rec == nil:
		tax = missing(tax, fmt.Sprintf("tax %d not found (billing.odoo.taxId)", c.cfg.TaxID))
	case str(rec["type_tax_use"]) != "sale":
		tax = missing(tax, str(rec["name"])+" is not a sales tax")
	case !boolean(rec["price_include"]):
		tax = missing(tax, str(rec["name"])+" is not included in price; Stripe prices include VAT")
	default:
		tax.Detail = str(rec["name"])
	}
	checks = append(checks, tax)
	return checks
}
