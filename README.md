# Taronja Gateway

Taronja Gateway is an API and application gateway.

It serves as an entry point for your API server and your frontend application, handling routing, authentication, sessions, and many more features, leaving your application code clean and focused on business logic.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Commands](#commands)
- [Configuration](#configuration)
- [Building and Releasing](#building-and-releasing)
- [Authentication on the APIs](#authentication-on-the-apis)
- [Getting the Current User from the Frontend](#getting-the-current-user-from-the-frontend)
- [Login and Logout Links from a Web Page](#login-and-logout-links-from-a-web-page)

# Features

Features table, shows what is implemented and what is planned.

| Feature                       | Status   |
|-------------------------------|----------|
| API Gateway                   | ✅       |
| Application Gateway           | ✅       |
| Management Dashboard          | ✅       |
| Logging                       | ✅       |
| Analytics and Traffic metrics | ✅       |
| - User Geo-location           | ✅       |
| - User fingerprint (JA4)      | ✅       |
| Sessions (Persistent)         | ✅       |
| User management               | ✅       |
| Authentication                | ✅       |
| Authentication: Basic         | ✅       |
| Authentication: OAuth2        | ✅       |
| - OAuth2: GitHub              | ✅       |
| - OAuth2: Google              | ✅       |
| Authentication: Token         | ✅       |
| Authentication: JWT           | 🚧       |
| Authorization using RBAC      | 🚧       |
| HTTP Cache Control            | ✅       |
| Rate Limiter                  | ✅       |
| - Requess per minute per IP   | ✅       |
| - Avoid scanners with number of 404 limit | ✅       |
| - Severe path with wildcard limit (e.g. /admin/*.php) | ✅       |
| Feature Flags                 | 🚧       |
| Circuit breaker               | 🚧       |
| Caching                       | 🚧       |
| Load Balancing                | 🚧       |
| robots.txt                    | 🚧       |
| more...                       | 🚧       |

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
    This command starts the Taronja API Gateway using the configuration file specified by the `--config` flag.

*   **Add a new user:**
    ```bash
    ./tg adduser <username> <email> <password>
    ```
    This command creates a new user in the database with the provided username, email, and password.

*   **Show the current version:**
    ```bash
    ./tg version
    ```

# Configuration

Taronja Gateway uses a YAML configuration file to define server settings, routes, authentication providers, and other features. The configuration file can reference environment variables using the `${VARIABLE_NAME}` syntax.

## Basic Structure

```yaml
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
