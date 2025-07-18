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

