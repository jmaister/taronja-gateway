# Billing and entitlements

Plan: [billing-plan.md](billing-plan.md).

The gateway takes payments (Stripe), issues the legal invoices (Odoo) and
turns paid plans into **entitlements**: features that are active for a user
or an organization until a date. Applications never touch money: they read
which features are active, from a header on every proxied request or from one
API call. No payment webhooks, no Stripe or Odoo code, no invoice tables in
any application.

```
        user ──login──▶ Gateway /_/pay/<plan> ──▶ Stripe Checkout
                              │      ▲
            Stripe webhooks ──┘      └── Stripe Customer Portal
                              │
                              ├──▶ Odoo: partner, invoice, credit note, PDF
                              │
  app ◀── X-Entitlements ─────┘   (or GET /_/api/entitlements)
```

## Decisions

| Topic | Decision |
|---|---|
| Who pays | A **subject**: a gateway user, or an **organization** of the gateway. Any member who pays activates the plan's features for the whole organization. |
| Organizations | Optional gateway feature: the gateway admin (or an app, with the admin API token) creates organizations and assigns users to them. |
| Payment link | A gateway page, `/_/pay/<plan>`. It requires login, then redirects to Stripe Checkout. The purchase is tied to the logged-in user or their organization, never matched by email. |
| Plans and prices | Created in the gateway dashboard. The gateway creates the Stripe Product and Price and the Odoo product. A price change creates a new Stripe Price; existing subscribers keep theirs. |
| Plan kinds | Subscription (monthly / yearly, optional trial, optional setup fee, optional start on activation) and one-time payment (optional access period, optional shipping address). |
| Apps | Read entitlements: `X-Entitlements` on proxied requests, `GET /_/api/entitlements`, or `/_/me`. Megabox also calls one endpoint, *activate*, when its box first goes online. |
| Money | **EUR only (Spain)**: plans in any other currency are refused. Prices **VAT included**, IVA 21 %, customers in Spain. |
| Stripe | `stripe-go/v87` (API version `2026-09-30.endive`), hosted Checkout and Customer Portal; card data never reaches the gateway. Stripe sends no invoices or receipts. |
| Odoo | One instance, one company, for every app. **JSON-2 API** (`/json/2/<model>/<method>`, Odoo 19); `/jsonrpc` and `/xmlrpc` are deprecated in Odoo 19 and removed in Odoo 22. Odoo 20 is not supported yet (it has no OIDC login). No custom Odoo module. |
| Invoice series | One sales journal for every app: one invoice series plus its own credit note series (*Dedicated Credit Note Sequence*). Products tell apps apart. See [Invoice series](#invoice-series). |
| Customer NIF | Optional, collected by Stripe Checkout. With NIF: full invoice (Veri\*Factu F1). Without: simplified invoice (F2). |
| Grace | 7 days after a failed renewal by default, per plan. Features stay active during grace. |
| Emails | The gateway emails each invoice and credit note with its PDF attached (through `notification.email`; the look is `billing.email`). Odoo sends nothing. The gateway's notifications send the rest (payment failed, trial ending, cancelled). |
| Data | No production data yet: migrations may assume empty billing tables. |

## Concepts

- **Organization** — `id`, `name`, members (`user_id`, `role`: `owner` or
  `member`). A user may belong to several. Owners can manage the
  organization's subscriptions from their account page; the gateway admin can
  do everything.
- **Subject** — who a purchase is for: `user:<id>` or `org:<id>`. A subject
  has at most one Stripe customer and one Odoo partner.
- **Feature** — a key `<app>.<name>` (`academia.access`,
  `megabox.protection`): 1-100 characters of lowercase letters, digits, `.`,
  `_` and `-`, starting with a letter. Plans list them; apps check them.
- **Plan** — what is sold. Fields:

  | Field | Meaning |
  |---|---|
  | `key` | URL slug: `/_/pay/<key>` (`academia-monthly`) |
  | `name`, `description` | Shown on the pay page and the invoice line |
  | `kind` | `subscription` or `one_time` |
  | `interval` | `month` or `year` (subscription) |
  | `amount_cents` | VAT included |
  | `features` | Feature keys it grants |
  | `subject` | `user`, `org` or `either`: who may buy it |
  | `trial_days` | Subscription: free days before the first charge, once per subject per plan group |
  | `grace_days` | Subscription: access kept after a failed renewal (default 7) |
  | `setup_fee_cents`, `setup_fee_name` | Subscription: a one-time line on the first payment (megabox's box) |
  | `starts` | Subscription: `on_payment` (default) or `on_activation`: the first period is paid at checkout and the recurring subscription starts when the app calls *activate* |
  | `access_days` | One-time: how long its features last; 0 = forever |
  | `collect_shipping` | Ask Stripe Checkout for a shipping address (Spain) |
  | `group` | Plans of a group can be switched in the portal (monthly ↔ yearly) and share the trial |
  | `active` | Archived plans can't be bought; their subscriptions go on |
- **Subscription** — a subject's recurring plan, mirroring Stripe's: status
  `awaiting_activation`, `trialing`, `active`, `past_due`, `canceled`.
- **Purchase** — one paid Checkout: the one-time plan, or a subscription's
  first payment when it has a setup fee or starts on activation. Holds the
  shipping address for fulfillment.
- **Grant** — features given by hand (or by an app) to a subject until a
  date, without paying: trials not tied to a plan, comps, corrections.
- **Billing document** — one invoice or credit note the gateway issues in
  Odoo, with its sync state.

## Entitlements

A subject's entitlements are computed from its subscriptions, purchases and
grants: each feature maps to the **latest** `until` among the sources that
grant it, and is active while `now < until` (or `until` is empty).

| Source | `until` | `status` | Grace starts |
|---|---|---|---|
| Subscription `trialing` | `trial_end + grace_days` | `trial` | `trial_end` |
| Subscription `active` | `current_period_end + grace_days` (renewal pending) | `active` | `current_period_end` |
| Subscription `active` with `cancel_at_period_end` | `current_period_end` | `canceling` | never |
| Subscription `past_due` | `grace_until` (first failed charge + `grace_days`) | `past_due` | the failed charge |
| Subscription `awaiting_activation` | empty (no expiry until activated) | `awaiting_activation` | never |
| Subscription `canceled` | — (not granted) | — | — |
| Purchase, one-time, `paid` | `paid_at + access_days`, or empty when 0 | `active` | never |
| Purchase `refunded` | — | — | — |
| Grant | its `until`, plus its `graceDays` when given (empty = forever) | `granted` | `until` |

### Grace period

Access is kept for the plan's `grace_days` after a subscription's period or
trial ends without payment, and the user must be told. A feature is **in
grace** when `now` is past its grace start and before its `until`. Then its
`grace` is `true` and `graceUntil` is when access ends (equal to `until`);
otherwise `grace` is `false` and `graceUntil` is `null`. When several
sources grant the same feature, one that is not in grace wins, so a paid
subscription is not reported as grace because of an older grant. A grant
has grace only when created with `graceDays` (0–60, needs an `until`);
that is the grace of an app-managed trial (Academia).

`GET /_/api/billing/subscriptions` (and `/me/billing`) report the same
`grace` and `graceUntil` for each subscription.

Entitlements are cached per subject in memory for **24 hours** (grace
periods are days long, so an old answer is never far off), and the cache is
cleared on **any** billing change: a subscription, purchase, grant,
membership or plan change, a Stripe webhook, activation, cancel or refund.
The cache lives in the gateway process: one instance only. Run several and
a change on one is not seen by the others until the 24 hours pass.

### X-Entitlements header

On every proxied request with a logged-in user, the gateway drops any
client-sent `X-Entitlements` and sets the active ones as JSON:

```json
{"user":{"megabox.protection":"2026-11-27T10:00:00Z"},
 "orgs":{"org_7Qm2":{"academia.access":"2026-12-01T00:00:00Z"}},
 "grace":{"orgs":{"org_7Qm2":{"academia.access":"2026-12-01T00:00:00Z"}}}}
```

Values are the `until` in RFC 3339 UTC, or `null` for no expiry. Only active
features appear; an organization with none is left out. `grace` is present
only when some feature is in grace, and lists those features with when
access ends (`user` and `orgs`, left out when empty). Requests without a
session get no header. Apps that need more (status, source) call the API.

### API for apps

Authenticated with a session or the admin API token, like every gateway API.

| Operation | Who | What |
|---|---|---|
| `GET /_/api/entitlements?user_id=` or `?org_id=` | admin token; or the user themselves / a member of the org | `{"subject":"org:…","features":{"academia.access":{"until":…,"status":"trial","source":"grant","grace":false,"graceUntil":null}}}` |
| `GET /_/me` | the user | adds `organizations` (id, name, role) and `entitlements` (same shape as the header, with `grace` when a feature is in it) |

## Workflows

### Subscribe (pay link)

1. The app links to `/_/pay/<plan>` (optionally `?org=<id>` and
   `?redirect=<path>`). Academia's "Subscribe" button is such a link.
2. Not logged in → the login page, back here after.
3. The page shows the plan (name, price VAT included, interval, trial,
   features in words) and the subject: the user, or one of their
   organizations (`?org=` preselects it; a user who isn't a member is
   refused). A subject that already has an active subscription in the
   plan's group gets "Manage subscription" (the portal) instead of "Pay".
4. **Pay** (POST, CSRF) → the gateway ensures the subject's Stripe customer
   and creates a Checkout Session, then 303 to Stripe:
   - subscription, `starts: on_payment`: `mode: subscription`, the plan's
     price, `subscription_data.trial_period_days` when the subject hasn't
     had a trial in this group, the setup fee as an extra one-time line;
   - subscription, `starts: on_activation`, or one-time: `mode: payment`,
     lines = setup fee (if any) + first period (or the one-time price),
     `payment_intent_data.setup_future_usage: off_session` when a
     subscription will follow;
   - always: `billing_address_collection: required`,
     `tax_id_collection.enabled: true`, `customer_update` name and
     address, `locale: es`, `shipping_address_collection` (ES) when
     `collect_shipping`, `client_reference_id` = the gateway checkout id,
     metadata `tg_checkout`, `tg_subject`, `tg_plan`; success URL
     `/_/pay/done?checkout=<id>`, cancel URL back to the pay page.
5. `/_/pay/done` shows the state read from the gateway (it never decides
   anything), polling until the webhook has arrived, then links to
   `redirect`.

### Webhook (Stripe → gateway)

`POST /_/billing/stripe/webhook`, raw body, verified with the signing
secret; each event id is stored in `stripe_events` and the state change is
made in the same transaction, so a repeated event changes nothing.
`invoice.paid` and `invoice.payment_failed` can arrive before their
subscription's own event: the gateway then reads the subscription from Stripe
and applies it first (a subscription without `tg_` metadata is not ours and
the event is ignored). If Stripe doesn't answer, the webhook answers 500 and
Stripe retries for up to 3 days; until it succeeds the event is listed in
`stuck_stripe_events` and on **Billing → Setup** (yellow, red after 2 hours).
Odoo work is never done inside the webhook: documents go to the
outbox.

| Event | Effect |
|---|---|
| `checkout.session.completed` (paid) | Customer details (name, address, NIF) saved on the subject's billing customer. `mode: subscription`: the subscription row is created from the session's subscription. `mode: payment`: a purchase (`paid`, shipping address, payment intent, saved payment method) and, for `starts: on_activation`, a subscription `awaiting_activation`. An invoice document for the payment. |
| `checkout.session.expired` | The checkout is closed. |
| `customer.subscription.created` / `updated` | Status, `current_period_end`, `trial_end`, `cancel_at_period_end` copied. |
| `customer.subscription.deleted` | `canceled`. |
| `invoice.paid` (amount > 0) | Invoice document for it; `past_due` → `active`, `grace_until` cleared. A zero invoice (trial start) creates nothing. |
| `invoice.payment_failed` | `past_due`, `grace_until` = first failure + `grace_days` (kept on later failures of the same period); notification to the payer. |
| `charge.refunded` | Credit note document for the newly refunded amount (refunded so far minus earlier credit notes); a fully refunded purchase becomes `refunded`; a fully refunded subscription period stops counting (the subscription is cancelled at the same time by the admin action that refunded it). |

### Activation (megabox)

`POST /_/api/billing/subscriptions/{id}/activate` (admin token): for an
`awaiting_activation` subscription, the gateway creates the Stripe
Subscription with the saved card and `trial_end` = now + one interval (the
first period was paid at checkout), and the subscription becomes
`trialing`. Idempotent: activating twice changes nothing. The app finds the
subscription id with `GET /_/api/billing/subscriptions?subject=…`.

### Renewals, failures, cancellation

Stripe charges and retries the card. `invoice.paid` issues the invoice;
`invoice.payment_failed` starts the grace; after the grace the feature is no
longer active, and the app sees it (megabox stops filtering, Academia
answers 402). Customers cancel in the Stripe portal (at period end only);
features last until the period end.

### Refund

Gateway admin, **Billing → Purchases → Refund** (whole or partial amount):
Stripe refund of a *purchase* (a one-time plan, or a subscription's first
payment with a setup fee or `on_activation`); the subscription it started
can be cancelled in the same action. Renewal charges are refunded in the
Stripe dashboard. Either way `charge.refunded` creates the credit note.

### Trials outside Stripe (Academia)

Academia's 14-day trial starts when a school is created, before anyone pays:
the app calls `POST /_/api/billing/grants` with the organization, the
features and `until` = now + 14 days. A grant can also comp a subject or fix
a mistake through the API (there is no grants form in the dashboard yet).
With `graceDays` the grant also gets a grace period after `until`.

## Odoo

JSON-2: `POST {url}/json/2/{model}/{method}`, headers
`Authorization: bearer <api key>`, `X-Odoo-Database: <db>`, body with named
arguments (`ids`, `domain`, `fields`, `vals_list`, `context`, a method's own
keyword arguments); the result is the method's return value; errors are
HTTP 4xx/5xx with `{"name", "message", "arguments", "context", "debug"}`.
The user is a dedicated one (`taronja-billing`, groups *Billing*,
*Contact creation* and *Products / Create*) with an API key of scope `rpc`
(Odoo 19 requires an expiry date on the keys of non-admin users, so the
key must be renewed before it expires). Every call carries
`context.allowed_company_ids = [company_id]`.

Each document goes through these steps; every step first checks whether it
already happened, so a retry never duplicates anything:

1. **Partner** — searched by `ref = "tg:<subject>"`, created if missing,
   then written with the latest details: name, `is_company` (a CIF), street,
   zip, city, state (by name in Spain), country ES, email, `lang` es_ES when
   installed, `vat` = `ES<NIF>` when there is one.
2. **Move** — searched by `ref` (the Stripe invoice id, payment intent id,
   or refund id) and `move_type`; else created in the sales journal, dated
   the day it is issued (Odoo numbers in date order): one line per Stripe
   line, the plan's product, the line's description ("Academia mensual
   20/10/2026–19/11/2026"), quantity 1, `price_unit` = the amount charged
   with the VAT-included 21 % tax, so Odoo's total always equals the charge.
   A credit note is `out_refund`, `reversed_entry_id` = its invoice's move,
   `l10n_es_edi_verifactu_refund_reason = "R1"` when that field exists,
   lines for the refunded amount.
3. **Post** (`action_post`) — numbers it and queues Veri\*Factu.
4. **Payment** — `account.payment.register` in the "Stripe" journal
   (inbound for an invoice, outbound for a credit note), memo = the Stripe
   reference.
5. **PDF** — `account.move.send.wizard` → `action_send_and_print` with the
   `manual` method: Odoo renders the PDF and submits the move to Veri\*Factu,
   but sends no email.
6. **Email** — the gateway sends the PDF to the customer (see *Invoice
   emails*).
7. **Done** — the move id, number (`F/2026/00012`), invoice date and
   Veri\*Factu state are stored.

Failures store the error and retry with backoff (1 min, 5 min, 30 min, 2 h,
then every 6 h); a failed email retries the same way without reissuing in Odoo; after 10 attempts the document is `failed` until the admin
presses **Retry**. With Odoo not configured, documents stay pending. A
credit note waits until its invoice is done.

**PDF** — `GET /_/api/billing/documents/{id}/pdf` returns the attachment
Odoo rendered for the document (`invoice_pdf_report_id`), so Odoo
never needs to be reachable from the internet. Allowed to the gateway admin,
the paying user, and owners of the paying organization. 404 until issued; 503
when Odoo is down or not configured.

**Products** — the gateway creates an Odoo service product per plan
(`default_code = TG-<plan key>`, the VAT-included tax) and one per setup
fee, and updates their names when the plan changes.

**Simplified invoices** — without a NIF the invoice must be F2. Odoo's
`l10n_es` computes `l10n_es_is_simplified` itself: no VAT on the partner,
an amount within the company's simplified-invoice limit (400 € by
default) and a European country. The gateway only reads it.

### Invoice series

Spanish invoicing rules (RD 1619/2012, art. 6.1.a) require correlative
numbers within each series, allow several series when justified, and require
credit notes (*facturas rectificativas*) in a series of their own. So one
company selling several products can use **one series for all of them**:
one sales journal (e.g. short code `F`, giving `F/2026/00001`) with
*Dedicated Credit Note Sequence* on (credit notes `RF/2026/00001`). Products
tell the apps apart in reports. The gateway checks the journal's
`refund_sequence` before creating a credit note and refuses with a clear
error (visible in the dashboard) while it is off. Choose the journal code
before the first real invoice: numbers sent to Veri\*Factu can't change.

## Stripe

- Prices: one Stripe Product per plan (metadata `tg_plan`), one Price per
  plan version (lookup key `tg_<plan>`, moved to the newest Price; recurring
  for subscriptions, one-time for one-time plans, the first period of an
  `on_activation` plan and setup fees). Amounts in cents, `tax_behavior:
  inclusive`.
- Customer per subject: name, payer's email, metadata `tg_subject`; Checkout
  updates its name and address.
- Customer Portal: a configuration the gateway creates or updates at startup
  and from the setup page: update payment method, cancel at period end,
  switch between plans of the same group, no invoice history (invoices come
  from Odoo), return URL `/_/account`.
- Stripe settings done by hand (the setup page reminds): *Email customers
  about successful payments* and *Email finalized invoices* off.
- Idempotency keys on every create (`tg-<checkout id>`, `tg-activate-<id>`,
  `tg-refund-<payment>-<amount>`).

## Data model

Migration `0005_billing` (and the same models in `db.Models`):

| Table | Key columns |
|---|---|
| `organizations` | `id`, `name`, `logo` (blob), `logo_content_type`, `logo_updated_at`, timestamps |
| `organization_members` | `organization_id`, `user_id` (unique pair), `role` |
| `billing_plans` | `key` (PK), `name`, `description`, `kind`, `interval`, `amount_cents`, `currency`, `subject`, `trial_days`, `grace_days`, `setup_fee_cents`, `setup_fee_name`, `starts`, `access_days`, `collect_shipping`, `group_key`, `active`, `stripe_product_id`, `stripe_price_id`, `stripe_first_period_price_id`, `stripe_setup_fee_price_id`, `odoo_product_id`, `odoo_setup_fee_product_id`, timestamps |
| `billing_plan_features` | `plan_key`, `feature` |
| `billing_customers` | `subject` (PK), `stripe_customer_id`, `odoo_partner_id`, `name`, `email`, `tax_id`, `address_*`, `trial_groups` (groups already trialed) |
| `billing_checkouts` | `id`, `subject`, `plan_key`, `user_id` (payer), `stripe_session_id`, `status` (`open`, `completed`, `expired`), `redirect` |
| `billing_subscriptions` | `id`, `subject`, `plan_key`, `payer_user_id`, `status`, `stripe_subscription_id` (unique, null while awaiting activation), `stripe_payment_method_id`, `current_period_end`, `trial_end`, `cancel_at_period_end`, `grace_until`, `activated_at`, `canceled_at` |
| `billing_purchases` | `id`, `subject`, `plan_key`, `payer_user_id`, `checkout_id`, `subscription_id` (for a first payment), `status` (`paid`, `refunded`), `amount_cents`, `refunded_cents`, `stripe_payment_intent_id` (unique), `paid_at`, `shipping_*` (name, phone, address) |
| `billing_grants` | `id`, `subject`, `feature`, `until`, `note`, `created_by` |
| `billing_documents` | `id`, `subject`, `kind` (`invoice`, `credit_note`), `stripe_ref` (unique), `invoice_document_id`, `subscription_id`, `purchase_id`, `amount_cents`, `currency`, `lines` (JSON: description, plan, amount, period), `paid_at`, `odoo_move_id`, `odoo_state` (`pending`, `done`, `failed`), `odoo_error`, `attempts`, `next_attempt_at`, `number`, `issued_at`, `emailed_at` |
| `stripe_events` | `event_id` (PK), `type`, `received_at` |
| `stuck_stripe_events` | `event_id` (PK), `type`, `subscription_id`, `reason`, `attempts`, `first_seen_at`, `last_seen_at` (removed when the event is applied) |

## Gateway API

Under `/_/api`, in `api/taronja-gateway-api.yaml`; admin = gateway admin
session or admin API token.

| Operation | Who |
|---|---|
| `GET/POST /organizations`, `GET/PATCH/DELETE /organizations/{id}`, `PUT/DELETE /organizations/{id}/members/{userId}` | admin |
| `GET/PUT/DELETE /organizations/{id}/logo` | `GET`: admin or a member; `PUT`/`DELETE`: admin or an owner. The logo is stored in the database (PNG, JPEG or WebP up to 512 KiB, the type read from the bytes; SVG is refused) and served with an ETag; organizations carry a `logoUrl` (empty without a logo) |
| `GET /me/organizations` | the user (also in `/_/me`) |
| `GET/POST /billing/plans`, `GET/PATCH /billing/plans/{key}` (PATCH `active: false` archives) | admin |
| `GET /entitlements` | see above |
| `GET /billing/subscriptions?subject=&status=`, `POST /billing/subscriptions/{id}/activate`, `POST /billing/subscriptions/{id}/cancel` | admin |
| `GET /billing/purchases?plan=&status=&since=` | admin (fulfillment: megabox reads paid purchases with shipping addresses) |
| `POST /billing/purchases/{id}/refund` `{amountCents?, cancelSubscription?}` | admin |
| `GET/POST /billing/grants`, `DELETE /billing/grants/{id}` | admin |
| `GET /billing/documents?subject=&state=`, `POST /billing/documents/{id}/retry` | admin |
| `GET /billing/documents/{id}/pdf` | admin, payer, organization owners |
| `GET /me/billing` | the user: subscriptions and documents of themselves and of organizations they own |
| `POST /me/billing/portal` `{subject}` | the payer or an owner: a Stripe portal URL |
| `GET /billing/setup` , `POST /billing/setup/actions` | admin: configuration checks and fixes |

Pages (server-rendered like the account pages): `/_/pay/{plan}` (GET page,
POST pay), `/_/pay/done`. Raw: `POST /_/billing/stripe/webhook`.

## Account page and dashboard

- **Account page** (`/_/account`), which only holds the *Billing* section (and the user's email): the user's and
  their owned organizations' subscriptions (plan, status, next charge or
  end), **Manage** (portal), and issued invoices and credit notes with PDF.
- **Dashboard** pages: *Organizations* (create, rename, members),
  *Plans* (create, edit, archive, copy pay link), *Subscriptions*
  (filters, activate, cancel), *Purchases* (shipping addresses, refund),
  *Invoices* (Odoo state, error, retry, PDF), *Billing setup* (checks with
  status *OK* / *Check* / *Missing* / *By hand* and buttons, as megabox's
  payments setup page: Stripe key mode, webhook endpoint and events, portal
  configuration, Odoo connection, company, journal and credit note
  sequence, payment journal, tax, products).

## Notifications

Through the gateway's notification service (every channel the user prefers),
to the payer (and owners of the organization), type `billing.*`:
`billing.payment_failed` (portal link, the date features stop),
`billing.trial_ending` (3 days before), `billing.canceled`,
`billing.expired`.

### Invoice emails

Once a document is issued in Odoo, the gateway emails it to the customer's
billing email (the one Stripe collected), with the PDF attached, exactly once
(`billing_documents.emailed_at`). Invoices say the plan, the amount, the
period of each line, the trial end and when the subscription renews or ends,
and link to `/_/account`; credit notes show the refunded amount. A failed
send retries with the document's backoff and ends up in the *Odoo invoice
queue* like an Odoo failure. Without `notification.email` (or without a
customer email) the document is still issued, just not emailed; the setup
page's *Invoice emails* check turns red in the first case.

`billing.email` customizes it: `brandName` (header, subject, default: the
`server.url` host), `logoUrl`, `thanks`, `footer` and `templateFile`, a Go
`html/template` that replaces the built-in HTML. The template receives
`.Brand`, `.LogoURL`, `.Greeting`, `.Thanks`, `.Footer`, `.CreditNote`,
`.Number`, `.Date`, `.Total`, `.AccountURL`, `.Lines` (each with
`.Description`, `.Amount`, `.Period`) and `.Notes` (the subscription
sentences). The plain-text part is always the built-in one.

## Configuration

```yaml
billing:
  enabled: true
  stripe:
    secretKey: ${STRIPE_SECRET_KEY}
    webhookSecret: ${STRIPE_WEBHOOK_SECRET}
    apiUrl: ""                      # tests only (stripe-mock / fake)
  odoo:
    url: https://odoo.example.com
    database: ${ODOO_DB}
    apiKey: ${ODOO_API_KEY}
    companyId: 1
    salesJournalId: 1               # with Dedicated Credit Note Sequence
    paymentJournalId: 7             # bank journal "Stripe"
    taxId: 12                       # IVA 21 % included in price
    apiKeyExpiresAt: 2026-12-31     # optional: when the API key expires
  email:                            # optional: look of the invoice emails
    brandName: Academia
    logoUrl: https://academia.example.com/logo.png
    thanks: Gracias por tu compra.
    footer: Academia SL · C/ Mayor 1, Valencia
    templateFile: ./invoice-email.html
```

`billing.enabled` requires `server.url` (Checkout return URLs) and the Stripe
keys; without Odoo, documents stay pending. Validated by `tg validate`.

`apiKeyExpiresAt` (YYYY-MM-DD) makes the setup page's *Odoo API key expiry*
check real: OK, a warning in the last 30 days, missing once it has passed;
unset, the check only reminds you that Odoo 19 keys of non-admin users
expire.

### Problems in the dashboard

**Billing → Setup** shows, besides the connections, the *Odoo invoice queue*
(documents pending, retrying and failed, and how long the oldest has waited,
linking to the filtered **Invoices** list), the Stripe events waiting for
their subscription, and the API key expiry. In
**Invoices**, a failed document shows Odoo's last error and its attempt
count with a *Retry* button; a credit note whose invoice is not issued yet
(or failed) says so and offers *Retry its invoice*.

## Security

- The webhook is public and trusted only by its Stripe signature (tolerance
  5 minutes), body capped at 256 KiB.
- `/_/pay` POSTs carry the session CSRF token. A user can only pay for
  themselves or an organization they belong to (`subject` setting of the
  plan).
- Header `X-Entitlements` is always stripped from client requests.
- Stripe and Odoo secrets live only in the gateway config.
- Amounts always come from the plan (Checkout) and from Stripe's events
  (invoices), never from the browser.

## Testing

| Level | What |
|---|---|
| Unit | Each package against `httptest` fakes: Stripe adapter, Odoo JSON-2 client, entitlement rules (table tests over every status), webhook state machine (same event twice, out of order), outbox retries. |
| Stripe API shape | `stripe/stripe-mock` (Stripe's own mock, validates every request against Stripe's OpenAPI) in `e2e/docker-compose.yml`: every adapter call is made against it. |
| Odoo, real | `odoo:19.0` with Postgres in `e2e/docker-compose.yml`, initialised by `e2e/odoo/init.sh` (Spanish company, `l10n_es`, journal with credit note sequence, Stripe journal, VAT-included tax, API user and key): partner, invoice, payment, credit note, PDF, simplified invoice. Veri\*Factu registration is tested with `E2E_ODOO_VERIFACTU=1` against AEAT pre-production. |
| Whole flow | e2e test with a scripted Stripe (`httptest` server speaking the Stripe API for the calls the flow makes, signed webhooks from `webhook.GenerateTestSignedPayload`) and the real Odoo: pay link → checkout → webhook → entitlement in the header → Odoo invoice with PDF → refund → credit note; failed renewal → grace → expired; activation. |
| Live, opt-in | With `STRIPE_TEST_SECRET_KEY` (test mode; refuses live keys): the real Stripe API creates a product, prices, a customer, Checkout sessions and a portal session. No `stripe listen` is needed: webhooks are not part of it. |

## Applications

### Academia (first)

Done on Academia's branch `gateway-billing` (its PLAN.md §8). Academia sells
**modules**, so its features are per module: `academia.<module>` (today
`academia.access`).

- Its `billing/` package (Stripe Checkout, portal, webhook), the processed
  events table and the organization's Stripe customer and billing status
  are gone.
- A school ↔ a gateway organization (`Organization.GatewayOrgID`): created
  with the school, its admin as owner (or on first use for older schools);
  every user becomes a member on their first request (admins as owners).
- New school → a gateway grant of the paid modules' features for 14 days
  (the trial; the plans themselves have no Stripe trial).
- `ModuleService.Enabled`: the gateway admin's grants as before, or, for a
  paid module, the feature active for the school's gateway organization,
  read from `X-Entitlements` on requests or `GET /_/api/entitlements` with
  the admin token (cached a minute) for public links and background jobs.
- `GET /api/organizations/{id}/billing` shows each paid module's status and
  until, the pay links `/_/pay/academia-monthly?org=<id>&redirect=<settings>`
  (plans from `ACADEMIA_BILLING_PLANS`) and **Invoices and payments** =
  `/_/account`.
- Plans `academia-monthly` and `academia-yearly` (group `academia`, subject
  `org`, feature `academia.access`) are created by Academia's
  `scripts/gateway-billing-plans.sh` (or in the dashboard).
- The gateway route to Academia's API must be `authenticated: true` for the
  gateway to send `X-Entitlements` (otherwise Academia asks the API).

### megabox (later)

Plan `megabox-familia`: subscription, monthly, setup fee (the box),
`starts: on_activation`, `collect_shipping`, subject `user`, feature
`megabox.protection`. megabox keeps orders and shipping, reading paid
purchases from `GET /_/api/billing/purchases`, calls *activate* when the box
first goes online, and suspends a box whose owner no longer has
`megabox.protection`. Its `payments/` Stripe and Odoo code, `mail/` and the
Odoo module go away.

## Still open

- Stripe **webhooks** are not tested against real Stripe (the live test
  covers the API calls); events are tested with signed payloads.
- Veri\*Factu registration is tested against AEAT pre-production with
  `E2E_ODOO_VERIFACTU=1`, using Odoo's demo certificate; a real certificate
  is not.
- The entitlement cache is per process (one gateway instance).
- No dashboard form for grants: they are created through the API.
- Bank reconciliation of Stripe payouts is manual. The gateway records each
  payment in the Stripe journal (`paymentJournalId`), but Stripe pays out
  batches net of fees and refunds, so accounting must match each payout with
  its payments and book the fees by hand. It could be automated by importing
  the payouts as bank statements (Stripe balance transactions) or with an
  Odoo connector.
