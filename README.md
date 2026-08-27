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
| Rate Limiter                  | ✅       | v0.0.22 |
| - Requess per minute per IP   | ✅       | v0.0.22 |
| - Avoid scanners with number of 404 limit | ✅       | v0.0.22 |
| - Severe path with wildcard limit (e.g. /admin/*.php) | ✅       | v0.0.22 |
| Hot config reload             | ✅       |        |
| Feature Flags                 | 🚧       |        |
| Circuit breaker               | 🚧       |        |
| Caching                       | 🚧       |        |
| Load Balancing                | 🚧       |        |
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
version: 2 # Config schema version — see "Config File Versioning" below

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
version: 2
```

On startup, the gateway logs the version it detected and what it currently
supports:

```
Config file version: 2 (current: 2)
```

A file with no `version:` key is treated as version 1 — the implicit version
of every config written before this feature existed.

**The gateway refuses to start against an outdated config file.** If the
version it detects is older than it supports, `tg run` (and `tg middleware
list`) fail immediately with an error telling you what to do:

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
- `port`: The port number to listen on (default: 8080)
- `url`: The full URL where the gateway is accessible

### Management

Controls the management dashboard and gateway features.

- `prefix`: URL prefix for management endpoints (default: `_`)
- `logging`: Enable/disable request logging
- `analytics`: Enable/disable traffic analytics and metrics
- `session.secondsDuration`: Session timeout in seconds (e.g., 86400 = 24 hours)
- `admin.enabled`: Enable the admin dashboard
- `admin.username`: Username for dashboard access
- `admin.password`: Password for dashboard access (automatically hashed)
- `cors.allowedOrigins`: Origins allowed to make cross-origin requests to the management API (e.g. `["https://app.example.com"]`). Omit or leave empty to disable CORS entirely — the default, since the dashboard is always served same-origin. A literal `"*"` allows any origin, but only when `allowCredentials` is not also `true` (browsers reject that combination; the gateway rejects it at startup instead of shipping a CORS setup that silently doesn't work)
- `cors.allowCredentials`: Send `Access-Control-Allow-Credentials: true`, letting browsers include cookies on cross-origin requests
- `cors.allowedMethods` / `cors.allowedHeaders` / `cors.maxAgeSeconds`: Preflight response details; sensible defaults are used if omitted

### Middleware (optional, advanced)

By default, the global middleware chain (CORS, rate limiting, JA4
fingerprinting, session extraction, traffic metrics, request logging) is
controlled by the `cors` / `logging` / `analytics` / `rateLimiter` flags
above. For explicit control over which middleware runs and in what order, add
a `middleware:` section — when present it fully replaces those flags:

```yaml
middleware:
  global:
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
    - name: logging
      enabled: false   # listed but disabled
```

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
- `to`: Backend URL to proxy requests to
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

The gateway's global middleware chain (rate limiting, JA4 fingerprinting,
session extraction, traffic metrics, request logging) is built from a small
Factory + Registry system rather than hardcoded conditionals, so it can be
inspected, configured declaratively, monitored, and extended:

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
  "ja4Fingerprint": "string"
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
| `ja4Fingerprint`   | `string`  | JA4H HTTP fingerprint of the client.                             |

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
