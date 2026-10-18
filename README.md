# Taronja Gateway

<p align="center">
  <img src="doc/images/logo.png" alt="Taronja Gateway logo" width="240">
</p>

Taronja Gateway is an API and application gateway.

It serves as an entry point for your API server and your frontend application, handling routing, authentication, sessions, and many more features, leaving your application code clean and focused on business logic.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Commands](#commands)
- [Configuration](#configuration)
- [Middleware Architecture](#middleware-architecture)
- [Building and Releasing](#building-and-releasing)
- [Authentication on the APIs](#authentication-on-the-apis)
- [Getting the Current User from the Frontend](#getting-the-current-user-from-the-frontend)
- [Login and Logout Links from a Web Page](#login-and-logout-links-from-a-web-page)
- [Feature Flags](#feature-flags)

# Features

Features table, shows what is implemented and what is planned.

| Feature                       | Status   | Since  |
|-------------------------------|----------|--------|
| API Gateway                   | ✅       | v0.0.1 |
| Application Gateway           | ✅       | v0.0.1 |
| Management Dashboard          | ✅       | v0.0.3 |
| Logging                       | ✅       | v0.0.1 |
| Analytics and Traffic metrics | ✅       | v0.0.4 |
| - User Geo-location           | ✅       | v0.0.4 |
| - User fingerprint (JA4)      | ✅       | v0.0.8 |
| - Traffic-over-time graphs (per minute/hour/day/week/month) | ✅ | v1.0.0 |
| Sessions (Persistent)         | ✅       | v0.0.1 |
| User management               | ✅       | v0.0.3 |
| Authentication                | ✅       | v0.0.1 |
| Authentication: Basic         | ✅       | v0.0.1 |
| Authentication: OAuth2        | ✅       | v0.0.1 |
| - OAuth2: GitHub              | ✅       | v0.0.1 |
| - OAuth2: Google              | ✅       | v0.0.1 |
| - OpenID Connect: any provider (Zitadel, Keycloak, Authentik, ...) | ✅ | unreleased |
| - OAuth2: Microsoft (Entra ID / Azure AD)¹ | 🚧 |        |
| - OAuth2: Facebook¹           | 🚧       |        |
| - OAuth2: Apple (Sign in with Apple)¹ | 🚧 |        |
| Authentication: Token         | ✅       | v0.0.9 |
| Several logins per account (account linking) | ✅ | unreleased |
| Account page: password, sessions, recent activity | ✅ | unreleased |
| Authentication: JWT           | 🚧       |        |
| Authorization using RBAC      | 🚧       |        |
| HTTP Cache Control            | ✅       | v0.0.12 |
| Response Compression (brotli/zstd/gzip/deflate) | ✅ | v1.0.0 |
| Distributed Tracing (OpenTelemetry) | ✅ | v1.0.0 |
| Notifications (in-app, email, Telegram) | ✅ | v1.0.0 |
| - Multi-recipient, per-user delivery channel, automatic retries | ✅ | v1.0.0 |
| - Sent/failed/pending status, per notification and per batch | ✅ | v1.0.0 |
| - Outbound webhook on user response                | ✅ | v1.0.0 |
| Rate Limiter                  | ✅       | v0.0.22 |
| - Requests per minute per IP  | ✅       | v0.0.22 |
| - Avoid scanners with number of 404 limit | ✅       | v0.0.22 |
| - Severe path with wildcard limit (e.g. /admin/*.php) | ✅       | v0.0.22 |
| - Persistent block-event history, with a per-country attacker map | ✅ | v1.0.0 |
| Feature Flags (on/off, no config needed) | ✅ | unreleased |
| Circuit breaker               | 🚧       |        |
| Caching                       | 🚧       |        |
| Load Balancing                | ✅       | v1.0.0 |
| - Round-robin across multiple `to` backends | ✅ | v1.0.0 |
| - Automatic failover on connection failure | ✅ | v1.0.0 |
| robots.txt                    | 🚧       |        |
| more...                       | 🚧       |        |

¹ Implemented in v1.0.0 but untested; moved to the
`wip/microsoft-apple-facebook-auth` branch to come back to later.

# Installation

### Quick Install (All Platforms)

```bash
curl -fsSL https://github.com/jmaister/taronja-gateway/raw/main/scripts/install.sh | bash
```

This script detects your OS and architecture, downloads the latest release, and installs it to your system path.

### Windows Installation

```bat
powershell -Command "Invoke-WebRequest -Uri 'https://github.com/jmaister/taronja-gateway/raw/main/scripts/install.bat' -OutFile 'install.bat'" && install.bat
```

The Windows installer places the binary in `%USERPROFILE%\bin`. Add this directory to your PATH to use `tg` from anywhere.

### Installing a specific version

Both installers take an optional version, e.g. to go back to an earlier release:

```bash
curl -fsSL https://github.com/jmaister/taronja-gateway/raw/main/scripts/install.sh | bash -s -- 0.0.24
```

```bat
install.bat 0.0.24
```

A config file must match the gateway's config version: keep a copy of your old config before migrating it.

### Try it with Docker

Prefer to see it running before installing anything? [`examples/docker-demo`](examples/docker-demo/) is a `docker compose up --build` away from a full stack with one of each route type (static, authenticated static, reverse proxy), the admin dashboard, and Google/GitHub OAuth wired up to `.env` — see its README.

### Docker Image

Every [GitHub Release](https://github.com/jmaister/taronja-gateway/releases) publishes `ghcr.io/jmaister/taronja-gateway:<version>` and `:latest` for linux/amd64 and linux/arm64 (Apple Silicon); pre-releases get only `:<version>` (see `.github/workflows/docker-release.yml`) — pull it directly instead of building the `Dockerfile` yourself:

```bash
docker run --rm -p 8080:8080 \
  -v $(pwd)/config:/etc/taronja-gateway:ro \
  -v gateway-db:/data \
  ghcr.io/jmaister/taronja-gateway:latest \
  run --config /etc/taronja-gateway/config.yaml
```

Useful for deploying behind a platform that runs containers for you (a Docker Compose stack, Dokploy, Coolify, Kubernetes, ...): give it a config file with `routes[].to` pointing at your other services' addresses on that platform's network, and a volume at `/data` so the sqlite DB (admin user, sessions, traffic metrics) survives a redeploy. [`examples/docker-image`](examples/docker-image/) is a runnable Compose version of exactly this — pulling the image rather than building `examples/docker-demo`'s equivalent `Dockerfile` locally.

# Commands

The Taronja Gateway CLI provides the following commands:

*   **Run the Gateway:**
    ```bash
    ./tg run --config ./sample/config.yaml
    ```
    This command starts the Taronja API Gateway using the configuration file specified by the `--config` flag. On `Ctrl+C` or a `SIGTERM` (e.g. from `docker stop` or a Kubernetes pod eviction), it shuts down gracefully — draining in-flight requests for up to 15 seconds before exiting, instead of dropping them.

*   **Add a new user:**
    ```bash
    ./tg adduser <username> <email> <password>
    ```
    This command creates a new user in the database with the provided username, email, and password.

*   **Show the current version:**
    ```bash
    ./tg version
    ```

*   **List the middleware chain for a config file:**
    ```bash
    ./tg middleware list --config ./sample/config.yaml
    ```
    Prints the middleware the config enables, in order, and every available middleware with its dependencies — without starting the server. See [Middleware](#middleware).

*   **Migrate a config file to the current schema version:**
    ```bash
    ./tg migrate --config ./sample/config.yaml > ./sample/config-v1.1.yaml
    ```
    Prints the migrated config to stdout — redirect it to save it; never modifies the original or writes a file itself. `tg run` refuses to start against an outdated (or too new) config file and tells you to run this (or upgrade the binary). See [Config File Versioning](#config-file-versioning).

*   **Validate a config file:**
    ```bash
    ./tg validate --config ./sample/config.yaml
    ```
    Loads and validates the config — schema version, routes, admin settings, and the global middleware chain's dependencies — without starting the server, opening a database connection, or making any network calls. Prints `'<path>' is valid (version X.Y): M route(s), management prefix "/_".` on success, or `FATAL: <error>` (exit code 1) on the first problem found. Safe to run in CI or before deploying a config change.

# Configuration

Taronja Gateway uses a YAML configuration file to define server settings, routes, authentication providers, and other features. The configuration file can reference environment variables using the `${VARIABLE_NAME}` syntax.

## Basic Structure

```yaml
version: "1.0" # Config schema version — see "Config File Versioning" below

name: Example Gateway Configuration

server:
  host: 0.0.0.0 # Bind to all interfaces, 127.0.0.1 for localhost only
  port: 8080
  url: http://localhost:8080

management:
  session:
    durationSeconds: 1296000  # Inactivity before logout, in seconds (15 days, the default); renewed on use
  admin:
    enabled: true
    username: admin
    password: admin123  # Automatically hashed for security

middleware:                 # run top to bottom, see doc/middleware/
  - name: ja4_fingerprint   # the three analytics middleware feed the dashboard
  - name: session_extraction
  - name: traffic_metrics
  - name: logging

routes:
  - name: API Route
    from: /api/v1/*
    removeFromPath: "/api/v1/"
    to: https://api.example.com
    options:
      cacheControlSeconds: 3600

authenticationProviders:
  basic:
    enabled: true
  google:
    clientId: ${GOOGLE_CLIENT_ID}
    clientSecret: ${GOOGLE_CLIENT_SECRET}
  github:
    clientId: ${GITHUB_CLIENT_ID}
    clientSecret: ${GITHUB_CLIENT_SECRET}

branding:
  logoUrl: /static/logo.png

geolocation:
  iplocateApiKey: ${IPLOCATE_IO_API_KEY}

notification:
  email:
    enabled: true
    host: smtp.example.com
    port: 587
    username: ${SMTP_USERNAME}
    password: ${SMTP_PASSWORD}
    from: ${SMTP_FROM}
    fromName: ${SMTP_FROM_NAME}
```

## Config File Versioning

The config file declares a schema version in `MAJOR.MINOR` format (`version: "1.0"`). The gateway refuses to start when the file's version differs from the one it supports, in either direction. Run `tg migrate` to upgrade an older file. See [doc/config-versioning.md](doc/config-versioning.md) for the version rules and migration steps.

## Configuration Sections

### Server

Defines the gateway server settings.

- `host`: The host address to bind to (default: 127.0.0.1)
- `port`: The port number to listen on (default: 8080).
- `url`: The full URL where the gateway is accessible

### Management

Controls the admin dashboard and user sessions. The gateway's own endpoints always live under `/_` (login `/_/login`, dashboard `/_/admin/`, API `/_/api/`), so avoid using `/_` in your routes.

- `session.durationSeconds`: How long a session survives without use, in seconds. Default: 1296000 (15 days). Sessions are sliding: each use renews them to now + this (at most once an hour) and the cookie is re-sent, so active users stay logged in
- `admin.enabled`: Enable the admin dashboard
- `admin.username`: Username for dashboard access
- `admin.password`: Password for dashboard access (automatically hashed)

### Middleware

Global middleware runs on every request before it reaches a route:
compression, CORS, rate limiting, analytics, logging and tracing. It is
listed under the root `middleware:` key and runs in list order; each entry
has a `name`, an optional `enabled: false` and an optional `config` block
with that middleware's options. With no `middleware:` key, none runs.

```yaml
middleware:
  - name: compression
  - name: rate_limiter
    config:
      requestsPerMinute: 300
      maxErrors: 20
      blockMinutes: 15
  - name: ja4_fingerprint
  - name: session_extraction
  - name: traffic_metrics
    config:
      excludeStaticAssets: true
  - name: logging
```

| Middleware | What it does |
|---|---|
| [`tracing`](doc/middleware/tracing.md) | OpenTelemetry span per request, exported over OTLP/HTTP |
| [`compression`](doc/middleware/compression.md) | brotli/zstd/gzip/deflate response compression |
| [`cors`](doc/middleware/cors.md) | CORS headers and preflights |
| [`rate_limiter`](doc/middleware/rate-limiter.md) | Per-IP request and error limits, scanner blocking |
| [`ja4_fingerprint`](doc/middleware/ja4-fingerprint.md) | JA4H fingerprint per request |
| [`session_extraction`](doc/middleware/session-extraction.md) | Attaches the caller's session to the request |
| [`traffic_metrics`](doc/middleware/traffic-metrics.md) | Records requests for the statistics dashboard |
| [`logging`](doc/middleware/logging.md) | Logs each request |

See [doc/middleware/](doc/middleware/README.md) for every option, the
dependencies between middleware and ordering advice.

### Routes

Each route maps a `from` path pattern to a backend (`to`, a single URL or a list for load balancing), a single file (`toFile`), or a folder (`toFolder`). Common properties:

- `name`: human-readable route identifier
- `from`: URL path pattern to match (supports `*` wildcards)
- `to`: backend URL, or a list of URLs to load balance across
- `toFile` / `toFolder`: serve a file or a folder (a route sets exactly one of `to`, `toFile`, `toFolder`)
- `removeFromPath`: prefix removed before forwarding (proxy routes, whole path segments only; a path in `to` is prepended after)
- `authenticated: true`: require a logged-in user
- `options.cacheControlSeconds`: cache duration in seconds (0 = no-cache)

See [doc/routes.md](doc/routes.md) for examples and load balancing, and [doc/CACHE_CONTROL.md](doc/CACHE_CONTROL.md) for caching.

### Authentication Providers

Basic (username/password) authentication, the Google and GitHub OAuth2 providers, and any number of OpenID Connect providers (Zitadel, Keycloak, Authentik, Authelia, Pocket ID, ...) are configured under `authenticationProviders`. Each is independent and optional, and the login page shows a button for every enabled one.

```yaml
authenticationProviders:
  basic:
    enabled: true
```

See [doc/authentication-providers.md](doc/authentication-providers.md) for the OAuth2 setup and callback URLs.

The gateway also manages accounts for OAuth/OIDC logins: linking Google, GitHub and OIDC logins to one account, and an account page at `/_/account` where users change their password, see and sign out their sessions, and review recent activity. See [doc/accounts.md](doc/accounts.md).

### Branding

Customize the login page and dashboard appearance.

```yaml
branding:
  logoUrl: /static/logo.png
```

### Geolocation

Configure IP geolocation services for analytics.

```yaml
geolocation:
  iplocateApiKey: ${IPLOCATE_IO_API_KEY}
```

- With API key: Uses [iplocate.io](https://www.iplocate.io) for accurate results
- Without API key: Falls back to [freeipapi.com](https://freeipapi.com)

### Notifications

The gateway can store and deliver notifications on behalf of the apps it sits in front of: in-app always, plus email and Telegram if configured. See [doc/notifications.md](doc/notifications.md) for the data model, API, retry schedule, and delivery flows.

```yaml
notification:
  email:
    enabled: true
    host: smtp.example.com
    port: 587
    username: ${SMTP_USERNAME}
    password: ${SMTP_PASSWORD}
    from: noreply@example.com
    fromName: Taronja Gateway

  telegram:
    enabled: true
    botToken: ${TELEGRAM_BOT_TOKEN}   # from @BotFather
```

## Environment Variables

Use environment variables to keep sensitive data out of your config file:

```yaml
google:
  clientId: ${GOOGLE_CLIENT_ID}
  clientSecret: ${GOOGLE_CLIENT_SECRET}
```

Set environment variables before running the gateway:

```bash
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
./tg run --config ./config.yaml
```

## Example Configuration

See the complete example configuration in `sample/config.yaml`.

# Middleware Architecture

Every middleware is one file in `middleware/builtin/` holding its name, its
options type with validation, and a `Definition`;
`middleware/builtin/global_chain.go` turns the `middleware:` list into a
chain, checking names, options and dependencies. See
[doc/middlewares.md](doc/middlewares.md) for the design.

- **Inspect** what's active for a config file without starting the server:
  ```bash
  ./tg middleware list --config ./sample/config.yaml
  ```
- **Monitor** a running gateway (admin session required):
  - `GET /_/api/middleware` — status, dependencies, and health of every
    middleware
  - `GET /_/api/middleware/{name}/metrics` — request count, error
    count, and average duration for one middleware
- **Add a built-in middleware** — see
  [`doc/middleware_development.md`](doc/middleware_development.md).

# Building and Releasing

`make build`, `make test`, `make cover` and `make dev` cover day-to-day development. `make e2e` runs the end-to-end tests against real services in Docker (see [doc/e2e.md](doc/e2e.md)). Releases are built and published with [GoReleaser](https://goreleaser.com/). See [doc/building-and-releasing.md](doc/building-and-releasing.md).


# Authentication on the APIs

Proxied requests to routes with `authenticated: true` carry `X-User-Id` and `X-User-Data` headers, set only when a valid session (cookie) or bearer token exists. See [doc/backend-integration.md](doc/backend-integration.md) for the header reference, the `X-User-Data` JSON structure, and a backend example.

# Getting the Current User from the Frontend

A frontend served through the gateway can fetch the current user from the gateway's session API. See [doc/backend-integration.md](doc/backend-integration.md#getting-the-current-user-from-the-frontend).

# Login and Logout Links from a Web Page

Plain links to the gateway's login and logout endpoints are enough. See [doc/backend-integration.md](doc/backend-integration.md#login-and-logout-links-from-a-web-page).

# Feature Flags

Feature flags need no configuration: create them in the dashboard (**Feature Flags** page) or the admin API, and flip them at runtime without redeploying. Today a flag is on or off.

- **Frontends** read `GET /_/flags`, which is public (no login): `{"flags": {"new-checkout": true, "dark-mode": false}}`. With the React SDK: `const enabled = useFeatureFlag('new-checkout')`.
- **Backends** behind proxy routes get the enabled keys in the `X-Feature-Flags` request header, sorted and comma-separated (`dark-mode,new-checkout`). The gateway drops any copy of this header the client sent.
- **Admins** manage flags with `GET/POST /_/api/feature-flags` and `GET/PATCH/DELETE /_/api/feature-flags/{key}`.

Keys are 1-100 lowercase letters, digits, `-`, `_` or `.`. Since `/_/flags` is public, do not put secrets in flag keys.
