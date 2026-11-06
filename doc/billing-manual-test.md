# Billing: manual test guide

Follow it by hand or hand it to a browser-automation agent. It exercises the
whole payment workflow against **Stripe test mode**, a real Odoo and a mail
catcher. Reference: [billing.md](billing.md). Automated tests: [e2e.md](e2e.md).

## Rules (for people and agents)

- Use only Stripe **test** keys (`sk_test_…`). The setup page must show
  "test mode: no real payments". If it says "live mode", stop.
- Never paste keys, signing secrets or the Odoo API key into chat, logs, the
  page or a commit. Put them in environment variables or an untracked config.
- Test cards only. Any future expiry, any CVC, any postal code.
- An agent must wait for each element (Stripe pages load slowly, `/_/pay/done`
  polls until the webhook arrives; allow up to 30 s) instead of sleeping, and
  must take the observed text, not the expected one, as the result.
- Record each step as PASS or FAIL with what was seen. Do not fix on the fly:
  finish the run, then look at "If something fails".

| Card | Behaviour |
|---|---|
| `4242 4242 4242 4242` | pays |
| `4000 0000 0000 0341` | attaches, then declines every charge (failed renewal) |
| `4000 0025 0000 3155` | asks for 3D Secure: click "Complete" in the dialog |
| `4000 0000 0000 9995` | declined at once (insufficient funds) |

## 0. Setup

1. Start the e2e services: `make e2e-up` (Mailpit on http://localhost:18025,
   Odoo with the e2e initialisation, stripe-mock). Odoo ids are in
   `e2e/.odoo/env`.
2. Write a config (not committed) with `server.url: http://localhost:8080`,
   `billing.enabled: true`, `billing.timezone: Europe/Madrid`,
   `billing.stripe.secretKey: ${STRIPE_SECRET_KEY}`,
   `billing.stripe.webhookSecret: ${STRIPE_WEBHOOK_SECRET}`, the `billing.odoo`
   block from `e2e/.odoo/env`, and `notifications.email` pointing to Mailpit's
   SMTP port. See [billing.md](billing.md#configuration).
3. Forward Stripe's webhooks (separate terminal):
   `stripe listen --forward-to localhost:8080/_/billing/stripe/webhook`.
   It prints a `whsec_…` secret: export it as `STRIPE_WEBHOOK_SECRET` before
   starting the gateway.
4. `tg run --config <config>`; create an admin user and a normal user
   (`user@example.test`) and log in as admin.
5. Open `/_/admin` → **Billing → Setup**. Expect: Stripe key warning "test
   mode", Odoo checks green. Click the fix button on each warning that has one
   (portal, plans). Stripe webhook and emails rows are manual checks. The
   payment methods row lists the methods Checkout offers and warns while a
   delayed one (SEPA, iDEAL…) is on: turn it off in the Stripe dashboard.
6. **Billing → Plans → New**: key `test-monthly`, 1000 cents/month (VAT
   included), one feature. Save. Setup shows "Plans synced" green. Make a
   second plan `test-pro`, 2000 cents/month, same group, for the plan change.

## 1. Subscribe

1. Log in as `user@example.test`. Open `/_/pay/test-monthly`.
   Expect the plan name, "10,00 €", "Pagar".
2. Click **Pagar**. Expect Stripe Checkout in Spanish with the address and tax
   id fields.
3. Pay with `4242…`, name "Test User", address in Spain, no tax id.
4. Expect `/_/pay/done` to say the payment is confirmed (after the webhook).
5. Check: `/_/account` lists the plan as active; the Stripe CLI terminal shows
   `checkout.session.completed`, `invoice.paid` answered 200; Odoo has a posted,
   paid invoice for 10,00 €; Mailpit has an email with the PDF attached;
   **Billing → Invoices** shows the document as `done`.
6. Open the PDF link on `/_/account`. Expect the invoice PDF.
7. Repeat step 1 of this section: expect "Gestionar suscripción" instead of
   "Pagar".

## 2. Variants of the pay link

- `/_/pay/test-monthly?redirect=/_/account&direct=1`, pay with `4242…`:
  expect a 302 straight to `/_/account?checkout=<id>&status=…` without the
  waiting page (needs a user without a live subscription: cancel or use a
  second user).
- `3155` card: complete 3D Secure; the result is the same as a normal payment.
- `9995` card: Stripe shows the decline and no subscription appears.
- Close Checkout without paying: no subscription, no invoice.

## 3. Fiscal data

1. As a user without a subscription, `POST /_/api/me/billing/customer`
   (or the app's form) with a company: `kind: company`, `taxId: B12345674`,
   Spanish address. Expect 200.
2. The same with `taxId: B12345679`: expect 400 (bad control character).
3. Pay: Checkout opens on that customer. The Odoo partner has the NIF and
   address, the invoice is addressed to it.

## 4. Change plan (portal)

1. `/_/account` → **Gestionar suscripción** (opens Stripe's portal).
2. Switch to `test-pro`. Expect the upgrade to be charged pro rata at once,
   an invoice for it in Odoo and by email, and the new plan on `/_/account`.
3. Switch back: expect the change scheduled for the period end, no charge.

## 5. Failed renewal and grace

1. In the Stripe dashboard (test mode) create a **test clock** and a customer
   with `4000 0000 0000 0341`, or use the Live test
   `TestLive_PlanChange` as a model. Subscribe that customer to the plan's
   price.
2. Advance the clock past one period. The renewal fails.
3. Expect `invoice.payment_failed` in the CLI, a warning email, and the
   subscription `past_due` with entitlements still active
   ([Grace period](billing.md#grace-period)).
4. Advance past the grace period: the feature is no longer active
   (`GET /_/api/entitlements`).

## 6. Cancel

1. In the portal choose **Cancel subscription**. Expect it to end at the period
   end: the status stays active with the end date shown, then expires.
2. Admin: **Billing → Subscriptions → Cancel** on another subscription cancels
   it in Stripe too.

## 7. Refund and credit note

1. **Billing → Purchases** shows only purchases (one-time plans or first
   payments with setup fee/`on_activation`). Create a one-time plan
   `test-once` (3000 cents) and buy it with `4242…`.
2. Partial refund 1000 cents. Expect `charge.refunded` in the CLI, a credit
   note of 10,00 € in Odoo, an email with its PDF, the purchase "partially
   refunded". Clicking Refund twice quickly must refund once.
3. Refund the rest (2000): credit note 20,00 €, purchase "refunded". A second
   click while the first refund is still in progress answers 409, not a second refund.
4. Refund of a subscription's first payment with "cancel subscription":
   the subscription ends at once.

## 8. Activation (megabox-style)

1. Plan with `starts: on_activation`; buy it. The subscription is
   `awaiting_activation`, the first period is paid.
   Note: a one-time plan's Checkout offers every payment method enabled in the
   Stripe account (Apple Pay, Link, PayPal, iDEAL…), not only cards: choose
   "Tarjeta" to type `4242…`.
2. `POST /_/api/billing/subscriptions/{id}/activate` with the admin token
   (id from `GET /_/api/billing/subscriptions?subject=…`). Expect 200 and an
   active Stripe subscription. Call it again: same result, no second
   subscription in Stripe.
3. Cancel an unactivated one: it ends, nothing is created in Stripe.

## 9. Setup screen states

Stop things on purpose and reload **Billing → Setup**, then restore them:

| Do | Expect |
|---|---|
| Unset the secret key, restart | Stripe key "not set" |
| Delete the portal configuration in Stripe | portal warning, fix button recreates it |
| Stop Odoo, pay | invoice stays `pending/retrying`, queue check warns; after starting Odoo it becomes `done` |
| Set `apiKeyExpiresAt` within 30 days | API key warning |

## 10. Account page

`/_/account` as a payer: subscriptions with status and next date, invoices
with PDF links, credit notes marked "Abono". An organization owner sees the
organization's; another user must get 403 on its portal and PDFs.

## If something fails

| Symptom | Look at |
|---|---|
| `/_/pay/done` never confirms | The Stripe CLI is not forwarding, or the secret differs from `billing.stripe.webhookSecret` (the webhook answers 400) |
| Subscription active but no invoice | **Billing → Setup**, "Odoo invoice queue"; **Billing → Invoices?state=failed**, retry |
| No email | Mailpit up and `notifications.email` set; "Invoice emails" check |
| Events "waiting for their subscription" | Resend the event from Stripe → Developers → Events |
| Wrong dates | `billing.timezone` |

Gateway log: `tg.log`. Never copy keys or full tax ids from it into reports.

## Quick checks

| After | Check | Where |
|---|---|---|
| Pay | Subscription active | `/_/account`, `GET /_/api/entitlements` |
| Pay | Invoice posted and paid | Odoo, **Billing → Invoices** |
| Pay | Email with PDF | Mailpit |
| Refund | Credit note in the `RINV/…` series | Odoo |
| Failed renewal | `past_due`, still entitled | `/_/api/billing/subscriptions` |
| Grace over | Not entitled | `/_/api/entitlements` |
| Cancel | `cancel_at_period_end` | Stripe, portal |

## Not covered here

Webhooks from Stripe to a public endpoint (use `stripe listen` locally),
Veri\*Factu with a real certificate, and how Stripe's pages look in every
browser. See [Still open](billing.md#still-open).
