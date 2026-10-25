# Billing: implementation plan

How [billing.md](billing.md) gets built. The coordinator (this session) writes
the contracts every task depends on, then runs small sub-agents (Haiku) with
one goal each, a closed list of files to touch and a command that proves the
task is done. Tasks in the same wave touch different files and run in
parallel, each in its own git worktree. The coordinator reviews each diff,
merges it, runs `go test ./...` and `make e2e`, and fixes what a task missed
before the next wave. After wave 0 no task edits the contracts: the OpenAPI
spec, `api/api.gen.go`, the migration, `db/schema.go`, `config/config.go` or
the interfaces in `billing/contracts.go`; a task that needs a change stops and
reports.

## Package layout

| Package | Holds |
|---|---|
| `billing/` | the service: contracts (types, the `Stripe` and `Odoo` interfaces, fakes), entitlements, checkout, webhook state machine, activation, refunds, Odoo outbox, notifications |
| `billing/stripeapi/` | `Stripe` implemented with `stripe-go/v87` |
| `billing/odooapi/` | `Odoo` implemented over the JSON-2 API |
| `db/repositories/` | `organization*.go`, `billing*.go` repositories |
| `handlers/` | `api_organizations.go`, `api_billing_*.go`, `api_entitlements.go` |
| `gateway/` | `pay_routes.go` (pay pages, webhook route), header injection in `proxy.go`, wiring in `setup.go` |
| `static/` | `pay.html`, `pay_done.html`, the account page's billing section |
| `webapp/src/pages/` | `OrganizationsPage`, `PlansPage`, `SubscriptionsPage`, `PurchasesPage`, `InvoicesPage`, `BillingSetupPage` |
| `e2e/` | `stripe-mock`, Odoo + Postgres, `e2e/odoo/init.sh`, `billing_test.go` |

## Wave 0 — contracts (coordinator)

- `config.BillingConfig` (+ validation, `doc/CONFIG.md`).
- Migration `0006_billing.up.sql` and the models in `db/schema.go`.
- Repository interfaces in `db/repositories/billing.go` and
  `organizationrepository.go` (implementations in wave 1).
- `billing/contracts.go`: `Subject`, `Plan`, statuses, `Stripe` and `Odoo`
  interfaces with their parameter and result types; `billing/fakes.go`:
  in-memory fakes of both for service tests.
- Every operation of billing.md in `api/taronja-gateway-api.yaml`, `make gen`,
  and handler stubs answering 501 so the build passes.
- `e2e/docker-compose.yml`: `stripe-mock` and Odoo service definitions.

## Wave 1 — adapters and storage (3 in parallel)

| Task | Files | Done when |
|---|---|---|
| 1a Stripe adapter: customers, products and prices (lookup keys, `tax_behavior: inclusive`), Checkout Sessions in both modes, subscriptions with `trial_end` and a saved payment method, portal configuration and sessions, refunds, webhook parsing into `billing.Event` for the 7 event types, idempotency keys | `billing/stripeapi/**` | unit tests against `httptest` pass; `go test -tags e2e ./billing/stripeapi/` passes against stripe-mock |
| 1b Odoo JSON-2 client: partner upsert by `ref`, product upsert by `default_code`, invoice and credit note create/post/pay/send idempotently by `ref`, `refund_sequence` check, PDF attachment, setup checks; `e2e/odoo/init.sh` (company ES, `l10n_es`, sales journal with credit note sequence, Stripe bank journal, 21 % VAT-included tax, user `taronja-billing` with an API key, written to `e2e/.odoo/env`) | `billing/odooapi/**`, `e2e/odoo/**` | unit tests against a fake JSON-2 server pass; `go test -tags e2e ./billing/odooapi/` passes against `odoo:19.0` and `odoo:20.0`, including a simplified invoice for a partner without VAT and a credit note in its own series |
| 1c Repositories: organizations and members; plans with features; customers; checkouts; subscriptions; purchases; grants; documents (due queue, retries); stripe events (insert-once) | `db/repositories/organization*.go`, `db/repositories/billing*.go` (not `billing.go`) | `go test ./db/...` passes, one test file per repository |

## Wave 2 — core services (4 in parallel)

| Task | Files | Done when |
|---|---|---|
| 2a Organizations API and memberships; `/_/me` returns organizations | `handlers/api_organizations.go`, `handlers/api_me.go`, tests | handler tests pass: admin only, members unique, roles |
| 2b Entitlements: rules of billing.md as a pure function over subscriptions, purchases and grants; cache with invalidation; `GET /entitlements`; grants API; `X-Entitlements` on proxied requests (stripped from clients); entitlements in `/_/me` | `billing/entitlements*.go`, `handlers/api_entitlements.go`, `handlers/api_billing_grants.go`, `gateway/proxy.go` (header only), tests | table test over every status; gateway test: header present for a member, absent and stripped otherwise |
| 2c Plans: validation, create and update syncing Stripe (product, prices, lookup key move) and Odoo (products), archive; plans API | `billing/plans*.go`, `handlers/api_billing_plans.go`, tests | tests with the fakes: a price change makes a new Price and moves the lookup key |
| 2d Odoo outbox: due documents, the six steps through `billing.Odoo`, backoff, failed after 10, retry; documents API and PDF with its access rules | `billing/odoosync*.go`, `handlers/api_billing_documents.go`, tests | tests with the fake Odoo: no duplicates on retry, credit note waits for its invoice |

## Wave 3 — flows (4 in parallel)

| Task | Files | Done when |
|---|---|---|
| 3a Pay pages: `/_/pay/{plan}` (login, subject choice, already subscribed → portal), POST creates the checkout, `/_/pay/done` polls | `gateway/pay_routes.go`, `static/pay*.html`, `billing/checkout*.go`, tests | gateway tests: logged out → login; non-member org refused; POST without CSRF 403; checkout params per plan kind |
| 3b Webhook: signature, dedupe, every event of billing.md's table, documents queued | `billing/webhook*.go`, the route in `gateway/pay_routes.go` is 3a's: 3b exports `billing.WebhookHandler` only | tests: each event; same event twice; `invoice.paid` before `checkout.session.completed` |
| 3c Activation, cancel, refunds, subscriptions and purchases API | `billing/activation*.go`, `billing/refund*.go`, `handlers/api_billing_subscriptions.go`, `handlers/api_billing_purchases.go`, tests | tests with the fake Stripe: activate twice = once; partial refund then full refund |
| 3d Notifications `billing.*` and the daily trial-ending job | `billing/notify*.go`, tests | tests with the real notification service and the in-app channel |

## Wave 4 — screens (3 in parallel)

| Task | Files | Done when |
|---|---|---|
| 4a Dashboard: Organizations, Plans (copy pay link), Subscriptions, Purchases, Invoices; services hooks; sidebar | `webapp/src/pages/{Organizations,Plans,Subscriptions,Purchases,Invoices}Page*.tsx`, `webapp/src/services/billing.ts`, `App.tsx`, `Sidebar.tsx` | `npx tsc --noEmit`, `npm run lint`, `npx vitest run`, `npm run build` |
| 4b Account page *Billing* section and portal; `/me/billing` handlers | `static/account.html`, `gateway/account_routes.go` (billing section only), `handlers/api_me_billing.go`, tests | gateway test renders the section for a payer and an owner |
| 4c Billing setup page and checks | `billing/setup*.go`, `handlers/api_billing_setup.go`, `webapp/src/pages/BillingSetupPage.tsx` | tests with the fakes; page builds |

## Wave 5 — end to end and Academia (coordinator)

- Wiring in `gateway/setup.go` and `cli/run.go`; `make e2e` with stripe-mock,
  real Odoo, scripted Stripe events: the whole-flow test of billing.md.
- Docs: README, AGENTS.md, `doc/notifications.md` (types), `sample/config.yaml`.
- Academia: the changes in billing.md's *Academia* section, its tests and
  e2e; plans created by Academia's local setup script.

## Sub-agent prompt

Each task prompt carries: the goal; the sections of billing.md it implements;
the interfaces it implements or calls (`billing/contracts.go`, the repository
interfaces); the files it may create or edit (nothing else); the commands
that prove it is done; the repo rules (AGENTS.md: 4-space indent in TS, error
checks on their own line in Go, testify, no mocks of our own code — fakes of
external services only, real SQLite in tests); and "do not change the
contracts, the OpenAPI spec, migrations or other packages; if blocked, stop
and report what you need". Each agent commits its work in its worktree with a
message naming the task.
