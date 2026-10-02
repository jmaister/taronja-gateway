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
| - OAuth2: Microsoft (Entra ID / Azure AD)¹ | 🚧 |        |
| - OAuth2: Facebook¹           | 🚧       |        |
| - OAuth2: Apple (Sign in with Apple)¹ | 🚧 |        |
| Authentication: Token         | ✅       | v0.0.9 |
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
| Feature Flags                 | 🚧       |        |
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

### Try it with Docker

Prefer to see it running before installing anything? [`examples/docker-demo`](examples/docker-demo/) is a `docker compose up --build` away from a full stack with one of each route type (static, authenticated static, reverse proxy), the admin dashboard, and Google/GitHub OAuth wired up to `.env` — see its README.

### Docker Image

Every [GitHub Release](https://github.com/jmaister/taronja-gateway/releases) publishes `ghcr.io/jmaister/taronja-gateway:<version>` and `:latest` (see `.github/workflows/docker-release.yml`) — pull it directly instead of building the `Dockerfile` yourself:

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

*   **List the global middleware chain for a config file:**
    ```bash
    ./tg middleware list --config ./sample/config.yaml
    ```
    Prints every global middleware's status, dependencies, and (where implemented) health for the given config — without starting the server. See [Middleware Architecture](#middleware-architecture).

*   **Migrate a config file to the current schema version:**
    ```bash
    ./tg migrate --config ./sample/config.yaml > ./sample/config-v1.1.yaml
    ```
    Prints the migrated config to stdout — redirect it to save it; never modifies the original or writes a file itself. `tg run` refuses to start against an outdated (or too new) config file and tells you to run this (or upgrade the binary). See [Config File Versioning](#config-file-versioning).

*   **Validate a config file:**
    ```bash
    ./tg validate --config ./sample/config.yaml
    ```
    Loads and validates the config — schema version, routes, admin settings, and the global middleware chain's dependencies — without starting the server, opening a database connection, or making any network calls. Prints `'<path>' is valid (version X.Y): M route(s), management prefix "<prefix>".` on success, or `FATAL: <error>` (exit code 1) on the first problem found. Safe to run in CI or before deploying a config change.

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
  prefix: _
  logging: true
  analytics: true
  session:
    secondsDuration: 86400  # Session duration in seconds (24 hours)
  admin:
    enabled: true
    username: admin
    password: admin123  # Automatically hashed for security

routes:
  - name: API Route
    from: /api/v1/*
    removeFromPath: "/api/v1/"
    to: https://api.example.com
    authentication:
      enabled: false
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
    smtp:
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

### Tracing

The gateway can export an [OpenTelemetry](https://opentelemetry.io/) span per request over OTLP/HTTP. It is disabled by default and configured at the top level of the config file:

```yaml
tracing:
  enabled: true
  endpoint: localhost:4318   # OTLP/HTTP collector host:port, no scheme
  insecure: true             # plain HTTP to endpoint, not HTTPS
```

See [doc/middleware/tracing.md](doc/middleware/tracing.md) for the full reference and a local Jaeger walkthrough.

### Management

Controls the management dashboard and gateway features. Every setting below
that belongs to a specific global middleware links to that middleware's
full reference page — see [doc/middleware/](doc/middleware/README.md) for
all of them together (options, dependencies, chain order).

- `prefix`: URL prefix for management endpoints (default: `_`)
- `logging`: Enable/disable request logging — see [`logging`](doc/middleware/logging.md)
- `compression`: Enable brotli/zstd/gzip/deflate response compression, negotiated per-request from the client's `Accept-Encoding` header — no other options. See [`compression`](doc/middleware/compression.md)
- `analytics`: Enable/disable traffic analytics and metrics — turns on the
  [`ja4_fingerprint`](doc/middleware/ja4-fingerprint.md),
  [`session_extraction`](doc/middleware/session-extraction.md), and
  [`traffic_metrics`](doc/middleware/traffic-metrics.md) middlewares together
- `excludeStaticAssets`: Skip traffic-metrics collection for static asset requests (CSS/JS/images/fonts/...). Default: `false`. Has no effect unless `analytics` is also `true`. Reduces per-request overhead and stats volume on asset-heavy sites; the Request Details report can still filter by request type either way (see below) — see [`traffic_metrics`](doc/middleware/traffic-metrics.md)
- `session.secondsDuration`: Session timeout in seconds (e.g., 86400 = 24 hours)
- `admin.enabled`: Enable the admin dashboard
- `admin.username`: Username for dashboard access
- `admin.password`: Password for dashboard access (automatically hashed)
- `rateLimiter.*`: Requests-per-minute, error-count, and vulnerability-scan
  limits per client IP — see [`rate_limiter`](doc/middleware/rate-limiter.md)
  for the full option list and scan-path wildcard syntax
- `cors.allowedOrigins`: Origins allowed to make cross-origin requests to the management API (e.g. `["https://app.example.com"]`). Omit or leave empty to disable CORS entirely — the default, since the dashboard is always served same-origin. A literal `"*"` allows any origin, but only when `allowCredentials` is not also `true` (browsers reject that combination; the gateway rejects it at startup instead of shipping a CORS setup that silently doesn't work)
- `cors.allowCredentials`: Send `Access-Control-Allow-Credentials: true`, letting browsers include cookies on cross-origin requests
- `cors.allowedMethods` / `cors.allowedHeaders` / `cors.maxAgeSeconds`: Preflight response details; sensible defaults are used if omitted — see [`cors`](doc/middleware/cors.md) for the full default values

### Middleware (optional, advanced)

By default the global middleware chain is controlled by the `compression` / `cors` / `logging` / `analytics` / `rateLimiter` flags above. To control exactly which middleware runs and in what order, add a `middleware:` section; when present it fully replaces those flags.

```yaml
middleware:
  global:
    - name: compression
    - name: rate_limiter
    - name: logging
```

See [doc/middleware/configuration.md](doc/middleware/configuration.md) for the full example, and [Middleware Architecture](#middleware-architecture) for runtime inspection.

### Routes

Each route maps a `from` path pattern to a backend (`to`, a single URL or a list for load balancing), a single file (`toFile`), or a folder (`toFolder`). Common properties:

- `name`: human-readable route identifier
- `from`: URL path pattern to match (supports `*` wildcards)
- `to`: backend URL, or a list of URLs to load balance across
- `toFile` / `toFolder` / `static`: serve static files
- `removeFromPath`: prefix to remove before forwarding to the backend
- `authentication.enabled`: require authentication for this route
- `options.cacheControlSeconds`: cache duration in seconds (0 = no-cache)

See [doc/routes.md](doc/routes.md) for examples and load balancing, and [doc/CACHE_CONTROL.md](doc/CACHE_CONTROL.md) for caching.

### Authentication Providers

Basic (username/password) authentication and the Google and GitHub OAuth2 providers are configured under `authenticationProviders`. Each is independent and optional, and the login page shows a button for every enabled one.

```yaml
authenticationProviders:
  basic:
    enabled: true
```

See [doc/authentication-providers.md](doc/authentication-providers.md) for the OAuth2 setup and callback URLs.

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

The gateway's global middleware chain (compression, rate limiting, JA4
fingerprinting, session extraction, traffic metrics, request logging) is
built from a small Factory + Registry system rather than hardcoded
conditionals, so it can be inspected, configured declaratively, monitored,
and extended. For what each
individual middleware does, its config options, and its dependencies, see
[doc/middleware/](doc/middleware/README.md) — one reference page per
middleware. This section is about the system they're all built on:

- **Inspect** what's active for a config file without starting the server:
  ```bash
  ./tg middleware list --config ./sample/config.yaml
  ```
- **Configure declaratively** with an optional `middleware:` YAML section —
  see [Middleware (optional, advanced)](#middleware-optional-advanced) above.
- **Monitor** a running gateway (admin session required):
  - `GET <prefix>/api/middleware` — status, dependencies, and health of every
    global middleware
  - `GET <prefix>/api/middleware/{name}/metrics` — request count, error
    count, and average duration for one middleware
- **Extend** by adding your own middleware — see
  [`doc/middleware_development.md`](doc/middleware_development.md) for the
  guide and [`examples/middleware-plugin/`](examples/middleware-plugin/) for
  a complete, tested, third-party-style example.

Full design rationale and phase-by-phase history: [`doc/refactor01.md`](doc/refactor01.md).

# Building and Releasing

`make build`, `make test`, `make cover` and `make dev` cover day-to-day development. Releases are built and published with [GoReleaser](https://goreleaser.com/). See [doc/building-and-releasing.md](doc/building-and-releasing.md).


# Authentication on the APIs

Proxied requests to routes with `authentication.enabled: true` carry `X-User-Id` and `X-User-Data` headers, set only when a valid session (cookie) or bearer token exists. See [doc/backend-integration.md](doc/backend-integration.md) for the header reference, the `X-User-Data` JSON structure, and a backend example.

# Getting the Current User from the Frontend

A frontend served through the gateway can fetch the current user from the gateway's session API. See [doc/backend-integration.md](doc/backend-integration.md#getting-the-current-user-from-the-frontend).

# Login and Logout Links from a Web Page

Plain links to the gateway's login and logout endpoints are enough. See [doc/backend-integration.md](doc/backend-integration.md#login-and-logout-links-from-a-web-page).
