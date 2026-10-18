# End-to-end tests

`e2e/` holds tests that run the real gateway against real external services,
instead of the in-process fakes the unit tests use. They are behind the `e2e`
build tag, so `make test` skips them.

```sh
make e2e        # start the services, run the tests, stop the services
make e2e-up     # only start the services (then: go test -tags e2e ./e2e/...)
make e2e-down   # stop them
```

CI runs `make e2e` in its own job (`.github/workflows/ci.yml`).

## How a test runs

Each test calls `startGateway` (`e2e/harness_test.go`) with a YAML config. The
harness writes it to a file, loads it with `config.LoadConfig` (the same path
`tg run` takes), and serves the gateway in-process on a random port.
`{{URL}}`/`{{PORT}}` in the YAML are replaced with that address. `newClient` is
one browser with its own cookie jar; `login` signs in through the real basic
login form.

## Services (`e2e/docker-compose.yml`)

| Service | Purpose | Host ports | Override |
|---------|---------|------------|----------|
| [Zitadel](https://zitadel.com/) + Postgres | OpenID Connect provider for the generic OIDC login. | `18180` | `E2E_ZITADEL_PORT` (compose) and `E2E_ZITADEL_URL` (tests) |
| [Mailpit](https://mailpit.axllent.org/) | Catches every email the gateway sends. Tests read it back through Mailpit's API. | SMTP `11025`, web UI and API `18025` | `E2E_MAILPIT_SMTP_PORT`, `E2E_MAILPIT_HTTP_PORT` (compose) and `E2E_MAILPIT_URL` (tests) |

Open http://localhost:18025 after `make e2e-up` to see the emails a test run sent.

Zitadel starts with two service accounts whose tokens it writes to
`e2e/.zitadel/` (gitignored; `make e2e-up` creates it and runs the container
as your user so the files are readable): `admin.pat` creates a project, an
OIDC app and users for each test, and `login-client.pat` lets the tests act as
Zitadel's login UI. A test starts the login at the gateway, follows the
redirect to Zitadel's authorize endpoint, takes the auth request ID from the
redirect to the (never started) login UI, signs the user in through
`/v2/sessions` and `/v2/oidc/auth_requests/{id}`, and follows the returned
callback URL back to the gateway. The e2e Zitadel uses bcrypt cost 4 instead
of 14, which makes each password operation take milliseconds instead of
seconds. First start takes about 40 seconds.

## What is covered

- `mailpit_test.go`: a notification created through the API arrives by SMTP
  with the right sender, recipient, encoded non-ASCII subject, body and action
  links; opening an action link without a session records the answer once;
  a multi-recipient notification sends one email per user and the batch
  status reports them sent.
- `accounts_test.go`: the account page's password change, session revocation, CSRF check and activity list.
- `zitadel_test.go`: login through Zitadel creates the user, and logging in
  again reuses it; an unverified Zitadel email is refused; a Zitadel login
  is not linked by itself to an existing account that never confirmed its
  email, and the account owner links it from the account page instead.
- `phone_channels_test.go`: SMS and WhatsApp against the **real** BulkGate
  and Meta Graph APIs (there is no emulator for either). With invalid
  credentials, which needs no account, the gateway must parse each
  provider's real error and not retry it; `E2E_OFFLINE=1` skips these
  without internet. Two live tests deliver a real message when credentials
  are set, and are skipped otherwise:
  `E2E_BULKGATE_APPLICATION_ID`, `E2E_BULKGATE_APPLICATION_TOKEN` and
  `E2E_SMS_TO` (BulkGate's 14-day trial is free), and
  `E2E_WHATSAPP_PHONE_NUMBER_ID`, `E2E_WHATSAPP_ACCESS_TOKEN` and
  `E2E_WHATSAPP_TO` (a Meta developer app's free test number sends the
  pre-approved `hello_world` template to up to five allowlisted numbers;
  `E2E_WHATSAPP_TEMPLATE`, `E2E_WHATSAPP_LANGUAGE` and
  `E2E_WHATSAPP_BODY_VARIABLES` pick your own template). The exact request
  payloads are checked against fake servers in
  `notification/phone_channels_test.go` and through the HTTP API in
  `gateway/phone_notifications_test.go`.
- `featureflags_test.go`: flags created and toggled by an admin show up on the
  public `/_/flags` endpoint and in the `X-Feature-Flags` header a proxied
  backend receives, and a client cannot forge that header.

Tests use unique usernames/emails per run, so they do not need a clean Mailpit.
