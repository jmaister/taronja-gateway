# Taronja Gateway

Taronja Gateway is an API and application gateway.

It serves as an entry point for your API server and your frontend application, handling routing, authentication, sessions, and many more features, leaving your application code clean and focused on business logic.

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
| Feature Flags                 | 🚧       |
| Rate Limiting                 | 🚧       |
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
  host: 127.0.0.1
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

