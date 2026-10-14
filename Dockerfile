# Builds the `tg` binary with the React admin dashboard embedded into it
# (main.go's `//go:embed webapp/dist`), exactly like a GoReleaser release
# build (see .goreleaser.yml: CGO_ENABLED=0, -tags=purego — the project uses
# modernc.org/sqlite, a pure-Go driver, specifically so this works).
#
# Used by examples/docker-demo/docker-compose.yml, and by
# .github/workflows/docker-release.yml to publish
# ghcr.io/jmaister/taronja-gateway on every GitHub Release (see that
# workflow and .goreleaser.yml's comment for why this is a separate
# workflow rather than goreleaser's own `dockers:` integration).
#
# Build from the repo root (the compose file does this via `context: ../..`):
#   docker build -t taronja-gateway .
# Release builds pass VERSION and COMMIT (shown by `tg version`) and build
# linux/amd64 and linux/arm64: the first two stages run on the build machine
# and cross-compile; only the last one runs per platform.

# --- Stage 1: build the admin dashboard (webapp/dist) ---
FROM --platform=$BUILDPLATFORM node:24-alpine AS webapp-builder
WORKDIR /src

# The TypeScript API client (webapp/src/apiclient) is generated from the
# OpenAPI spec, not committed (gitignored), by `npm run gen:api`.
COPY api/taronja-gateway-api.yaml api/taronja-gateway-api.yaml
COPY webapp/package.json webapp/package-lock.json webapp/
RUN cd webapp && npm ci
COPY webapp/ webapp/
RUN cd webapp && npm run gen:api
RUN cd webapp && npm run build

# --- Stage 2: build the Go binary ---
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS go-builder
ARG TARGETOS TARGETARCH
ARG VERSION=Dev
ARG COMMIT=none
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# api/api.gen.go is committed (generated from the same OpenAPI spec, checked
# by CI to be up to date) so it doesn't need regenerating here — only the
# embedded dashboard build output does.
COPY --from=webapp-builder /src/webapp/dist ./webapp/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags=purego \
    -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.buildOS=$TARGETOS -X main.buildArch=$TARGETARCH" \
    -o /out/tg .

# --- Stage 3: minimal runtime image ---
FROM alpine:3.24
# ca-certificates: needed for outbound HTTPS (OAuth token exchange with
# Google/GitHub, IP geolocation lookups) — without it those calls fail with
# "certificate signed by unknown authority", not an obviously-related error.
RUN apk add --no-cache ca-certificates
COPY --from=go-builder /out/tg /usr/local/bin/tg
# The sqlite DB file (db/db.go: "taronja-gateway.db", relative to the
# working directory) lands here — mount a volume at /data to persist it
# across container restarts, as examples/docker-demo/docker-compose.yml does.
WORKDIR /data
ENTRYPOINT ["tg"]
CMD ["--help"]
