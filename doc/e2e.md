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
| [Mailpit](https://mailpit.axllent.org/) | Catches every email the gateway sends. Tests read it back through Mailpit's API. | SMTP `11025`, web UI and API `18025` | `E2E_MAILPIT_SMTP_PORT`, `E2E_MAILPIT_HTTP_PORT` (compose) and `E2E_MAILPIT_URL` (tests) |

Open http://localhost:18025 after `make e2e-up` to see the emails a test run sent.

## What is covered

- `mailpit_test.go`: a notification created through the API arrives by SMTP
  with the right sender, recipient, encoded non-ASCII subject, body and action
  links; opening an action link without a session records the answer once;
  a multi-recipient notification sends one email per user and the batch
  status reports them sent.
- `accounts_test.go`: the account page's password change, session revocation, CSRF check and activity list.
- `featureflags_test.go`: flags created and toggled by an admin show up on the
  public `/_/flags` endpoint and in the `X-Feature-Flags` header a proxied
  backend receives, and a client cannot forge that header.

Tests use unique usernames/emails per run, so they do not need a clean Mailpit.
