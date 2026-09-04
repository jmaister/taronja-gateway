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
| Authentication: Token         | ✅       | v0.0.9 |
| Authentication: JWT           | 🚧       |        |
| Authorization using RBAC      | 🚧       |        |
| HTTP Cache Control            | ✅       | v0.0.12 |
| Response Compression (brotli/zstd/gzip/deflate) | ✅ | v1.0.0 |
| Rate Limiter                  | ✅       | v0.0.22 |
| - Requess per minute per IP   | ✅       | v0.0.22 |
| - Avoid scanners with number of 404 limit | ✅       | v0.0.22 |
| - Severe path with wildcard limit (e.g. /admin/*.php) | ✅       | v0.0.22 |
| Hot config reload             | ✅       |        |
| Feature Flags                 | 🚧       |        |
| Circuit breaker               | 🚧       |        |
| Caching                       | 🚧       |        |
| Load Balancing                | ✅       | v1.0.0 |
| - Round-robin across multiple `to` backends | ✅ | v1.0.0 |
| - Automatic failover on connection failure | ✅ | v1.0.0 |
| TLS Termination (HTTPS)       | ✅       | v1.0.0 |
| - Automatic HTTP → HTTPS redirect | ✅  | v1.0.0 |
| - Zero-downtime certificate reload on renewal | ✅ | v1.0.0 |
| - Automatic certificates via ACME / Let's Encrypt | ✅ | v1.0.0 |
| robots.txt                    | 🚧       |        |
| more...                       | 🚧       |        |

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

# Commands

The Taronja Gateway CLI provides the following commands:

*   **Run the Gateway:**
    ```bash
    ./tg run --config ./sample/config.yaml
    ```
    This command starts the Taronja API Gateway using the configuration file specified by the `--config` flag. On `Ctrl+C` or a `SIGTERM` (e.g. from `docker stop` or a Kubernetes pod eviction), it shuts down gracefully — draining in-flight requests for up to 15 seconds before exiting, instead of dropping them.

    The config file can be reloaded without restarting: save it (auto-reload is on by default; disable with `--watch=false`) or send the process a `SIGHUP` (`kill -HUP <pid>`). Either re-reads the file and, if it's still valid, swaps in the new routes, middleware chain, and rate limiter for requests received from then on — in-flight requests keep running against whatever was already serving them. An invalid edit is logged and ignored; the gateway keeps running its last-good config.

    `.env` is re-read on every reload too (values there win over whatever the process started with), so an edited `${VARIABLE_NAME}` secret takes effect right along with a structural config change — though only for values that go through `.env` itself; a variable exported directly in the shell/process manager can never reach an already-running process, reload or not, since that's fixed for the life of the process at the OS level.

    `server.host`/`port` and the database connection can't be changed this way — those need a real restart. Editing them and reloading anyway isn't silently ignored: the gateway logs a warning naming the port it's still actually listening on vs. the new one from the file.

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
    ./tg migrate --config ./sample/config.yaml > ./sample/config-v2.yaml
    ```
    Prints the migrated config to stdout — redirect it to save it; never modifies the original or writes a file itself. `tg run` refuses to start against an outdated config file and tells you to run this. See [Config File Versioning](#config-file-versioning).

*   **Validate a config file:**
    ```bash
    ./tg validate --config ./sample/config.yaml
    ```
    Loads and validates the config — schema version, routes, admin settings, and the global middleware chain's dependencies — without starting the server, opening a database connection, or making any network calls. Prints `'<path>' is valid (version N): M route(s), management prefix "<prefix>".` on success, or `FATAL: <error>` (exit code 1) on the first problem found. Safe to run in CI or before deploying a config change.

# Configuration

Taronja Gateway uses a YAML configuration file to define server settings, routes, authentication providers, and other features. The configuration file can reference environment variables using the `${VARIABLE_NAME}` syntax.

## Basic Structure

```yaml
version: 1 # Config schema version — see "Config File Versioning" below

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

The config file declares a schema version:

```yaml
version: 1
```

On startup, the gateway logs the version it detected and what it currently
supports:

```
Config file version: 1 (current: 1)
```

A file with no `version:` key at all is also treated as version 1 — every
config file is version 1 as of this release, whether or not it says so
explicitly.

**The gateway refuses to start against an outdated config file.** If a
future release raises the schema version, `tg run` (and `tg middleware
list`) will fail immediately for a config file older than that release
supports, with an error telling you what to do:

```
FATAL: Failed to load configuration: config file 'config.yaml' is version 1, but this gateway requires version 2

Run this to upgrade it (it prints the migrated config; redirect it to a file):

    tg migrate --config config.yaml > config-v2.yaml

Then point --config at the new file.
```

`tg migrate` **prints** the migrated config to stdout — it never writes a
file itself, and never touches the original. Redirect the output to save
it, same as any other Unix command:

```bash
./tg migrate --config config.yaml > config-v2.yaml
./tg run --config config-v2.yaml
```

You choose the destination filename; `config-v<N>.yaml` (as suggested in
the error above) is just a convention, not a requirement. A config several
versions behind is migrated one step at a time internally (e.g.
v1→v2→v3), so a single `tg migrate` run always gets you all the way to the
version this build requires. Running it on a config that's already current
just prints the file back unchanged (with a note on stderr, so it doesn't
pollute the redirected output).

## Configuration Sections

### Server

Defines the gateway server settings.

- `host`: The host address to bind to (default: 127.0.0.1)
- `port`: The port number to listen on (default: 8080). The HTTPS port when `tls.enabled` is true.
- `url`: The full URL where the gateway is accessible
- `tls`: HTTPS termination settings — see [TLS / HTTPS](#tls--https) below
- `trustedProxies`: CIDR ranges or bare IPs (e.g. `["10.0.0.0/8", "127.0.0.1"]`) allowed to supply the real client IP via `X-Forwarded-For`/`X-Real-IP`/`X-Client-IP`. **Default: empty** — those headers are never honored, and every request's IP is simply its real TCP peer address. Only add an entry for a reverse proxy or load balancer you actually control and that sits directly in front of this gateway: honoring these headers from an untrusted source lets any client set its own "IP" to anything it wants, defeating IP-based rate limiting, analytics, and geolocation.

### TLS / HTTPS

The gateway can terminate HTTPS itself, on `server.port`. There are two ways
to give it a certificate — pick one (they're mutually exclusive; configuring
both is a config-load error):

| | [Option 1: your own certificate files](#option-1-your-own-certificate-files) | [Option 2: ACME / Let's Encrypt](#option-2-automatic-certificates-via-acme--lets-encrypt) |
|---|---|---|
| Config key | `certFile` / `keyFile` | `acme` |
| **Who obtains/renews the certificate** | **You** — `certbot`, a commercial CA, an internal PKI, etc., running independently of the gateway. The gateway only *notices the file changed* and hot-swaps it in; it never requests anything itself. | **The gateway itself**, automatically, for as long as it keeps running — no external tool, no cron job, nothing else to keep working. |
| **Network requirement** | None — works on a fully private/internal network, behind a firewall, with no public DNS at all. | The domain(s) must have **public DNS pointing at this gateway** and be **reachable from the internet** on port 80 and/or 443 — Let's Encrypt's own servers connect *to* the gateway to prove domain ownership. Won't work for internal-only services. |
| Wildcard domains (`*.example.com`) | Supported, if your certificate provider issues them | **Not supported** (needs a `dns-01` challenge; unimplemented here) |
| Best for | Internal/private services, an existing CDN- or org-issued certificate, wildcard certs | Public-facing services where you just want HTTPS with zero ongoing certificate management |

#### Option 1: Your own certificate files

```yaml
server:
  port: 443
  tls:
    enabled: true
    certFile: /etc/letsencrypt/live/example.com/fullchain.pem
    keyFile: /etc/letsencrypt/live/example.com/privkey.pem
```

- `enabled`: Turn on HTTPS termination. Requires `certFile` and `keyFile` (or `acme` — see Option 2). Default: `false` (plain HTTP).
- `certFile`: Path to the PEM certificate (or full chain — leaf cert followed by any intermediates).
- `keyFile`: Path to the PEM private key matching `certFile`.
- `redirectPort`: Plain-HTTP port the gateway also listens on, redirecting every request there to the HTTPS equivalent on `server.port`. Omit for the default (80); set to `0` to disable the redirect listener entirely (e.g. if something else already owns port 80 in front of the gateway).

A bad or unparseable cert/key pair is rejected at config-load time (`tg validate` catches it before deploy), the same way a bad admin/CORS/route setting is.

##### Certificate file format

`certFile` must be **PEM-encoded** (not DER/binary, not PKCS#12/`.pfx`) — a text file made of one or more blocks that look like this:

```
-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAJC1HiIAZAiIMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
... (many more base64-encoded lines) ...
-----END CERTIFICATE-----
```

Give it the **full chain** — your certificate's own `CERTIFICATE` block followed immediately by every intermediate CA certificate's block, leaf first — not just the leaf alone. A browser that already trusts the intermediate (cached from visiting another site) will work either way, but a browser or API client seeing it for the first time won't be able to build a trust path to a root CA without it, and will reject the connection. This is exactly what a `fullchain.pem` from `certbot` already contains — use that file, not `cert.pem` (which certbot also writes, containing the leaf only).

`keyFile` must also be PEM-encoded, containing exactly one private key matching the certificate, in any of these forms (Go's standard library auto-detects which one it is):

```
-----BEGIN PRIVATE KEY-----        (PKCS#8 — RSA, ECDSA, or Ed25519)
-----BEGIN RSA PRIVATE KEY-----    (PKCS#1 — RSA only)
-----BEGIN EC PRIVATE KEY-----     (SEC1 — ECDSA only)
-----END ...-----
```

**A passphrase-encrypted private key is not supported** — the gateway has no way to prompt for a passphrase at startup, so a key file with a `Proc-Type: 4,ENCRYPTED` header (or PKCS#8's own encrypted form) fails to load with an unhelpful parse error, not a clear "this key needs a passphrase" message. Strip the passphrase before pointing `keyFile` at it:

```bash
openssl rsa -in encrypted-key.pem -out privkey.pem       # RSA key
openssl ec  -in encrypted-key.pem -out privkey.pem       # EC key
```

**Generating a self-signed certificate for local testing** (browsers will show a trust warning for it — that's expected; it's for testing the gateway's TLS support itself, not for production):

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout privkey.pem -out fullchain.pem -days 365 -nodes \
  -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
```

**Verifying a cert/key pair actually match** (compares a hash of each side's public key — a mismatch here is exactly the kind of thing that produces `tls: private key does not match public key` at startup):

```bash
openssl x509 -noout -pubkey -in fullchain.pem | openssl md5
openssl pkey  -pubout       -in privkey.pem    | openssl md5
```

**Picking up a renewed certificate is automatic, with zero downtime — but you still need something else actually renewing it.** The gateway itself never requests or renews a certificate in this mode; that's `certbot` (or whatever issued it)'s job, typically on its own cron job/systemd timer, same as it would be for e.g. nginx. What the gateway does automatically is *notice* when that tool replaces `certFile`/`keyFile` and hot-swap the in-memory certificate the moment it does — no restart, no reload, no dropped connections; already-open connections keep using whatever certificate they negotiated. This is independent of [hot config reload](#commands): a renewed certificate file doesn't require touching `config.yaml` at all. (Compare this to Option 2 below, where the gateway handles the entire renewal itself.)

#### Option 2: Automatic certificates via ACME / Let's Encrypt

The gateway can obtain and renew its own certificate via the ACME protocol
(RFC 8555) — what Let's Encrypt and several other certificate authorities
speak — with no `certbot` or other external tool needed:

```yaml
server:
  port: 443
  tls:
    enabled: true
    acme:
      domains: ["example.com", "www.example.com"]
      email: admin@example.com
```

- `domains`: Every hostname the gateway should obtain a certificate for. Required — at least one. Must exactly match what clients connect with; **wildcard domains (`*.example.com`) are not supported** — those need a `dns-01` challenge, which this integration doesn't implement, so use Option 1 with a DNS-capable ACME client instead.
- `email`: Optional contact address the CA can use for expiry/problem notifications.
- `cacheDir`: Where the obtained certificate(s) and the ACME account key are persisted across restarts (created automatically). Default: `.autocert-cache` (relative to the working directory).
- `directoryURL`: Overrides the ACME server. Empty (the default) means Let's Encrypt's production directory. Point this at Let's Encrypt's **staging** directory while testing a setup, to avoid the much stricter production rate limits — staging certificates aren't trusted by real browsers, so switch back (or remove the override) once things work:
  ```yaml
      acme:
        domains: ["example.com"]
        directoryURL: https://acme-staging-v02.api.letsencrypt.org/directory
  ```

**Using this feature means accepting the CA's Terms of Service on your behalf** — there's no interactive prompt a long-running server could sensibly show, so the gateway accepts automatically, the same way every other ACME client integration (Caddy, Traefik, certbot's `--agree-tos`) does.

**Domain validation happens automatically**, via whichever of the two standard challenge types succeeds:
- **`tls-alpn-01`** — answered directly on the main HTTPS listener itself, no extra port needed. Some networks/CDNs in front of the gateway strip the ALPN protocol this needs, though, so it isn't always available.
- **`http-01`** — answered on the `redirectPort` listener (default 80, same one that redirects normal traffic to HTTPS), under `/.well-known/acme-challenge/`. This means `redirectPort` needs to stay enabled (the default) for `http-01` to be available as a fallback — setting it to `0` leaves `tls-alpn-01` as the only option.

**The first certificate for a new domain is requested lazily**, on that domain's first real TLS handshake — not at gateway startup. This means a misconfigured domain (DNS not yet pointed at this gateway, port 80/443 unreachable from the internet) surfaces as a failed handshake for a real client hitting it, not as a startup error — check the gateway's logs if HTTPS connections are failing right after enabling this. Renewal happens automatically in the background well before expiry, with no restart, reload, or file-watching involved (there's no cert/key file for you to manage at all in this mode).

#### A free bonus of terminating TLS yourself: TLS-level client fingerprinting

Whichever certificate source you use, enabling TLS also turns on **TLS-level JA4 fingerprinting** automatically — no extra config. Unlike [JA4H](doc/middleware/ja4-fingerprint.md) (computed per HTTP request from header count/order, which varies constantly between a page load and its own subresource/API requests — see that page for why), TLS JA4 is computed once per TLS connection from the client's actual TLS stack (cipher suites, extensions, ALPN, TLS version): a property of the client's OS/browser/TLS library, not of any individual request, so it stays the same across every request on that connection. It's only possible because the gateway itself sees the raw `ClientHello` — this is the concrete meaning of "if we control the TLS certificates" in practice: TLS terminated by something else in front of this gateway (a CDN, a load balancer) means this gateway never sees a `ClientHello` at all.

It's exposed the same way as every other fingerprint signal — via the single `fingerprint`/`fingerprintType` pair on every session/traffic-metric row and in `X-User-Data` (`fingerprintType: "ja4_tls"` when TLS produced it) — see [doc/middleware/ja4-fingerprint.md](doc/middleware/ja4-fingerprint.md#one-consolidated-fingerprint-not-three) for how the three fingerprinting signals get reduced to that single pair, and the [`X-User-Data` field reference](#field-reference) below for the exact JSON shape.

#### Restarting vs. reloading

Enabling or disabling TLS itself, switching between the two certificate sources above, or changing either one's settings (cert/key paths, ACME domains, `redirectPort`) **does** require a full restart — like `server.host`/`port`, that means rebinding the listening socket, which a config reload can't do. Editing these and reloading (`SIGHUP` or `--watch`) anyway isn't silently ignored: the gateway logs a warning and keeps serving on whatever TLS configuration it started with.

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

By default, the global middleware chain (compression, CORS, rate limiting,
JA4 fingerprinting, session extraction, traffic metrics, request logging —
see [doc/middleware/](doc/middleware/README.md) for what each one does) is
controlled by the `compression` / `cors` / `logging` / `analytics` /
`rateLimiter` flags above. For explicit control over which middleware runs
and in what order, add a `middleware:` section — when present it fully
replaces those flags:

```yaml
middleware:
  global:
    - name: compression
    - name: cors
      cors:
        allowedOrigins: ["https://app.example.com"]
    - name: rate_limiter
      rateLimiter:
        requestsPerMinute: 1000
        maxErrors: 10
        blockMinutes: 5
    - name: ja4_fingerprint
    - name: session_extraction
    - name: traffic_metrics
      trafficMetrics:
        excludeStaticAssets: true
    - name: logging
      enabled: false   # listed but disabled
```

See [doc/middleware/README.md#two-ways-to-enable-any-of-these](doc/middleware/README.md#two-ways-to-enable-any-of-these)
for a full comparison of the two forms (including the "fully replaces those
flags" gotcha above spelled out in more detail — it's easy to trip on when
adding this section just to reorder or reconfigure one middleware).

See [Middleware Architecture](#middleware-architecture) below for how to
inspect this at runtime, and `doc/middleware_development.md` for adding your
own middleware.

### Routes

Define routing rules for incoming requests. Each route can:

- Proxy requests to backend services
- Serve static files
- Require authentication
- Control caching behavior

**Route Properties:**

- `name`: Human-readable route identifier
- `from`: URL path pattern to match (supports wildcards with `*`)
- `to`: Backend URL to proxy requests to. Accepts either a single URL
  (`to: https://api.example.com`) or a list of URLs
  (`to: [https://api-1.example.com, https://api-2.example.com]`) for load
  balancing — see [Load Balancing](#load-balancing) below
- `toFile`: Serve a single static file
- `toFolder`: Serve files from a directory
- `static`: Set to `true` for static file serving
- `removeFromPath`: Remove prefix before forwarding to backend
- `authentication.enabled`: Require authentication for this route
- `options.cacheControlSeconds`: Cache duration in seconds (0 = no-cache)

**Example Routes:**

```yaml
routes:
  # Serve a single file
  - name: Favicon
    from: /favicon.ico
    toFile: ./sample/webfiles/favicon.ico
    static: true

  # Public API - no authentication required
  - name: Public API v1
    from: /api/v1/*
    removeFromPath: "/api/v1/"
    to: https://jsonplaceholder.typicode.com
    authentication:
      enabled: false
    options:
      cacheControlSeconds: 300  # Cache for 5 minutes

  # Authenticated API route
  - name: Private API v2
    from: /api/v2/*
    removeFromPath: "/api/v2/"
    to: https://api.example.com
    authentication:
      enabled: true
    options:
      cacheControlSeconds: 0  # No cache

  # Static files folder - public
  - name: CSS and JavaScript
    from: /assets/*
    toFolder: ./static/assets
    static: true
    options:
      cacheControlSeconds: 604800  # Cache for 1 week

  # Another static folder - requires authentication
  - name: Protected Documents
    from: /documents/*
    toFolder: ./static/private-docs
    static: true
    authentication:
      enabled: true
    options:
      cacheControlSeconds: 3600  # Cache for 1 hour

  # Frontend application - no authentication
  - name: Public Frontend
    from: /
    toFolder: ./static/public
    static: true
    options:
      cacheControlSeconds: 86400  # Cache for 1 day

  # Admin panel - requires authentication
  - name: Admin Dashboard
    from: /admin/*
    toFolder: ./static/admin
    static: true
    authentication:
      enabled: true
    options:
      cacheControlSeconds: 0  # No cache for dashboard
```

### Load Balancing

Give `to` a list instead of a single URL to spread requests across multiple
backend instances:

```yaml
- name: API (load balanced)
  from: /api/*
  to:
    - http://api-1.internal:8080
    - http://api-2.internal:8080
    - http://api-3.internal:8080
  authentication:
    enabled: false
```

Requests are distributed round-robin across the list. If a backend's
connection attempt fails outright (refused, DNS failure, timeout), the
gateway automatically retries the same request against the next backend in
the list before giving up — a request only fails with `502 Bad Gateway` if
every listed backend is unreachable. This failover only reacts to
connection-level failures, never to a backend's response status code: a
backend returning its own `500` is a real answer, not treated as "down."

The listed URLs are expected to be interchangeable replicas of the same
backend — same path structure, differing only in scheme/host. Use separate
route entries (different `from:` patterns) to send different paths to
different places; that's routing, not load balancing.

A single URL (`to: http://backend:8080`) continues to work exactly as
before — this is purely additive.

### Authentication Providers

Configure authentication methods for your gateway.

**Basic Authentication:**
```yaml
authenticationProviders:
  basic:
    enabled: true
```

**OAuth2 Providers:**
```yaml
authenticationProviders:
  google:
    clientId: ${GOOGLE_CLIENT_ID}
    clientSecret: ${GOOGLE_CLIENT_SECRET}
  github:
    clientId: ${GITHUB_CLIENT_ID}
    clientSecret: ${GITHUB_CLIENT_SECRET}
```

To obtain OAuth2 credentials:
- **Google**: [Google Cloud Console](https://console.cloud.google.com/)
- **GitHub**: [GitHub OAuth Apps](https://github.com/settings/developers)

#### Google OAuth2 sample

Authorized origin: `http://localhost:8080`
Authorized redirect URI: `http://localhost:8080/_/auth/google/callback`

#### GitHub OAuth2 sample

Authorized origin: `http://localhost:8080`
Authorized callback URL: `http://localhost:8080/_/auth/github/callback`

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

Configure email notifications for user actions.

```yaml
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from: noreply@example.com
      fromName: Taronja Gateway
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

## Development Builds

```bash
# Build the binary
make build

# Run tests
make test

# Generate test coverage report
make cover

# Run in development mode with automatic restart on file changes
make dev
```

## Release Process

Taronja Gateway uses [GoReleaser](https://goreleaser.com/) for building and publishing releases.

```bash
# Install GoReleaser
make setup-goreleaser

# Check GoReleaser configuration
make release-check

# Create a local snapshot release (for testing)
make release-local

# Build Docker image locally
make release-docker
```

## GitHub Releases

When a new version is ready to be released:

1. Tag the commit with a semantic version:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

2. Create a new release on GitHub, pointing to the created tag.

3. The GitHub action will automatically:
   - Build binaries for multiple platforms
   - Create Docker images
   - Generate coverage reports
   - Publish all artifacts to the GitHub release

## Geolocation Configuration

Configure IP geolocation services in your `config.yaml`:

```yaml
geolocation:
  iplocateApiKey: ${IPLOCATE_IO_API_KEY}  # Optional: Use iplocate.io
```

- **With API key**: Uses [iplocate.io](https://www.iplocate.io) (more accurate, requires API key)
- **Without API key**: Uses [freeipapi.com](https://freeipapi.com) (free, basic accuracy)

Geolocation data is cached for 7 days to optimize performance and reduce API calls.


# Authentication on the APIs

When a request is proxied to a backend route that has `authentication.enabled: true`, Taronja Gateway injects HTTP headers into the request so the backend service can identify the authenticated user. These headers are only set when a valid session exists.

## Headers Sent to Backend Routes

### Standard Proxy Headers

Every proxied request (authenticated or not) includes the following standard headers:

| Header              | Type     | Description                                                    |
|---------------------|----------|----------------------------------------------------------------|
| `X-Forwarded-Host`  | `string` | The original `Host` header from the client request.            |
| `X-Forwarded-Proto` | `string` | The protocol used by the client (`http` or `https`).           |
| `X-Forwarded-For`   | `string` | The client's IP address. Appended to existing values if present. |

### Authentication Headers

These headers are added only on routes with `authentication.enabled: true` and when the user has a valid session:

| Header        | Type     | Description                                                                 |
|---------------|----------|-----------------------------------------------------------------------------|
| `X-User-Id`   | `string` | The unique user ID (CUID) of the authenticated user.                        |
| `X-User-Data` | `string` | A JSON-serialized object containing the full session data (see structure below). |

## `X-User-Data` JSON Structure

The `X-User-Data` header contains a JSON-encoded session object with the following fields:

```json
{
  "token": "string",
  "userId": "string",
  "username": "string",
  "email": "string",
  "isAuthenticated": true,
  "isAdmin": false,
  "validUntil": "2026-02-28T12:00:00Z",
  "provider": "string",
  "closedOn": null,
  "lastActivity": "2026-02-27T10:30:00Z",
  "sessionName": "string",
  "createdFrom": "string",
  "ipAddress": "string",
  "userAgent": "string",
  "referrer": "string",
  "browserFamily": "string",
  "browserVersion": "string",
  "osFamily": "string",
  "osVersion": "string",
  "deviceFamily": "string",
  "deviceBrand": "string",
  "deviceModel": "string",
  "geoLocation": "string",
  "latitude": 0.0,
  "longitude": 0.0,
  "city": "string",
  "zipCode": "string",
  "country": "string",
  "countryCode": "string",
  "region": "string",
  "continent": "string",
  "fingerprint": "string",
  "fingerprintType": "string"
}
```

### Field Reference

| Field              | Type      | Description                                                      |
|--------------------|-----------|------------------------------------------------------------------|
| `token`            | `string`  | The session token identifier.                                    |
| `userId`           | `string`  | Unique user ID (CUID format).                                    |
| `username`         | `string`  | Username of the authenticated user.                              |
| `email`            | `string`  | Email address of the user.                                       |
| `isAuthenticated`  | `bool`    | Whether the session is authenticated.                            |
| `isAdmin`          | `bool`    | Whether the user has admin privileges.                           |
| `validUntil`       | `string`  | Session expiration timestamp (RFC 3339 / ISO 8601).              |
| `provider`         | `string`  | Authentication provider used (`basic`, `google`, `github`, etc). |
| `closedOn`         | `string?` | Timestamp when the session was closed, or `null` if active.      |
| `lastActivity`     | `string`  | Timestamp of the last user activity in this session.             |
| `sessionName`      | `string`  | Optional name assigned to the session.                           |
| `createdFrom`      | `string`  | How the session was created (e.g. `cookie`, `token`).            |
| `ipAddress`        | `string`  | Client IP address.                                               |
| `userAgent`        | `string`  | Client's User-Agent string.                                      |
| `referrer`         | `string`  | HTTP referrer.                                                   |
| `browserFamily`    | `string`  | Browser name (e.g. `Chrome`, `Firefox`).                         |
| `browserVersion`   | `string`  | Browser version string.                                          |
| `osFamily`         | `string`  | Operating system name.                                           |
| `osVersion`        | `string`  | Operating system version.                                        |
| `deviceFamily`     | `string`  | Device type (e.g. `desktop`, `mobile`).                          |
| `deviceBrand`      | `string`  | Device manufacturer.                                             |
| `deviceModel`      | `string`  | Device model name.                                               |
| `geoLocation`      | `string`  | General geolocation description.                                 |
| `latitude`         | `float`   | GPS latitude coordinate.                                         |
| `longitude`        | `float`   | GPS longitude coordinate.                                        |
| `city`             | `string`  | City name from geolocation.                                      |
| `zipCode`          | `string`  | Postal / ZIP code.                                               |
| `country`          | `string`  | Country name.                                                    |
| `countryCode`      | `string`  | ISO country code (2-3 characters).                               |
| `region`           | `string`  | State, province, or region.                                      |
| `continent`        | `string`  | Continent name.                                                  |
| `fingerprint`      | `string`  | The client's fingerprint value — see `fingerprintType` for which algorithm produced it. Whichever of the three available signals is most reliable wins; see [doc/middleware/ja4-fingerprint.md](doc/middleware/ja4-fingerprint.md#one-consolidated-fingerprint-not-three) for the full priority order and why. |
| `fingerprintType`  | `string`  | Which algorithm produced `fingerprint`: `ja4_tls` (TLS-level JA4 — most stable, only possible when `server.tls.enabled`), `stable` (reduced-entropy header-based fingerprint), or `ja4h` (HTTP-header JA4H — the noisiest of the three). Empty string if `fingerprint` is empty too. |

## Authentication Methods

Backend routes can receive authenticated requests via two methods:

1. **Session cookie** — The user logs in through the gateway (Basic auth or OAuth2), and a `tg_session_token` cookie is set. The gateway validates the cookie on each request and injects the headers above.

2. **Bearer token** — API clients can authenticate using a token in the `Authorization` header:
   ```
   Authorization: Bearer <token>
   ```
   The gateway validates the token, creates a session-like object, and injects the same `X-User-Id` and `X-User-Data` headers.

## Example: Reading Headers in a Backend Service

**Node.js / Express:**
```js
app.get('/api/resource', (req, res) => {
    const userId = req.headers['x-user-id'];
    const userData = JSON.parse(req.headers['x-user-data']);
  console.log(`User: ${userData.username} (${userId})`);
  res.json({ message: `Hello, ${userData.username}` });
});
```

**Go:**
```go
func handler(w http.ResponseWriter, r *http.Request) {
    userId := r.Header.Get("X-User-Id")
    userDataJson := r.Header.Get("X-User-Data")
    // Parse userDataJson as needed
    fmt.Fprintf(w, "User ID: %s", userId)
}
```

**Python / Flask:**
```python
@app.route('/api/resource')
def resource():
    user_id = request.headers.get('X-User-Id')
    user_data = json.loads(request.headers.get('X-User-Data', '{}'))
    return jsonify(message=f"Hello, {user_data.get('Username')}")
```

## Getting the Current User from the Frontend

Web applications served through the gateway can call the `/_/me` endpoint to retrieve information about the currently logged-in user. The endpoint uses the session cookie (`tg_session_token`) that the browser sends automatically.

**Endpoint:** `GET /_/me`

- Returns `200` with user data if the user is authenticated.
- Returns `401` if no valid session exists.

**Response (200):**

```json
{
  "authenticated": true,
  "username": "testuser",
  "email": "user@example.com",
  "name": "Test User",
  "picture": "https://example.com/picture.jpg",
  "givenName": "Test",
  "familyName": "User",
  "provider": "google",
  "isAdmin": false,
  "timestamp": "2026-02-27T12:00:00Z"
}
```

| Field           | Type      | Nullable | Description                                              |
|-----------------|-----------|----------|----------------------------------------------------------|
| `authenticated` | `bool`    | No       | Always `true` when the response is 200.                  |
| `username`      | `string`  | No       | Username of the authenticated user.                      |
| `email`         | `string`  | Yes      | Email address (format: email).                           |
| `name`          | `string`  | Yes      | Full display name.                                       |
| `picture`       | `string`  | Yes      | URL to the user's profile picture.                       |
| `givenName`     | `string`  | Yes      | First name.                                              |
| `familyName`    | `string`  | Yes      | Last name.                                               |
| `provider`      | `string`  | No       | Authentication provider (`basic`, `google`, `github`).   |
| `isAdmin`       | `bool`    | No       | Whether the user has admin privileges.                   |
| `timestamp`     | `string`  | No       | Server timestamp (RFC 3339 / ISO 8601).                  |

**Example: Fetching the current user from JavaScript:**

```js
const response = await fetch('/_/me', { credentials: 'include' });
if (response.ok) {
    const user = await response.json();
    console.log(`Logged in as ${user.username}`);
} else {
    console.log('Not authenticated');
}
```

## Login and Logout Links from a Web Page

You can add direct login/logout links in your frontend pages.

By default, the management prefix is `_`, so authentication URLs are under `/_/`.

### Login Links

Use the login page endpoint:

- `/_/login`

This page automatically shows all configured login options (Basic, Google, GitHub, etc.).

Optional redirect after login:

- `/_/login?redirect=/dashboard`

### Logout Link

- `/_/logout`

Optional redirect after logout:

- `/_/logout?redirect=/`
- `/_/logout?redirect=/goodbye`

### HTML Example

```html
<a href="/_/login?redirect=/dashboard">Login</a>
<a href="/_/logout?redirect=/">Logout</a>
```

### JavaScript Example

```js
function login() {
  window.location.href = '/_/login?redirect=/dashboard';
}

function logout() {
  window.location.href = '/_/logout?redirect=/';
}
```
