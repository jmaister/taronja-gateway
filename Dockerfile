# Builds the `tg` binary with the React admin dashboard embedded into it
# (main.go's `//go:embed webapp/dist`), exactly like a GoReleaser release
# build (see .goreleaser.yml: CGO_ENABLED=0, -tags=purego — the project uses
# modernc.org/sqlite, a pure-Go driver, specifically so this works).
#
# Used by examples/docker-demo/docker-compose.yml. Not currently wired into
# the GitHub release pipeline — .goreleaser.yml's `dockers:` section
# referencing this file is still commented out ("Docker image builds are
# disabled due to issues"); this Dockerfile exists for local/demo use.
#
# Build from the repo root (the compose file does this via `context: ../..`):
#   docker build -t taronja-gateway .

# --- Stage 1: build the admin dashboard (webapp/dist) ---
FROM node:22-alpine AS webapp-builder
WORKDIR /src

# The TypeScript API client (webapp/src/apiclient) is generated from the
# OpenAPI spec, not committed (gitignored) — see AGENTS.md's "API Client
# Generation" section for why this must run from inside webapp/ after
# `npm install`, not a bare `npx --yes @hey-api/openapi-ts`.
COPY api/taronja-gateway-api.yaml api/taronja-gateway-api.yaml
COPY webapp/package.json webapp/package-lock.json webapp/
RUN cd webapp && npm ci
COPY webapp/ webapp/
RUN cd webapp && npx @hey-api/openapi-ts -i ../api/taronja-gateway-api.yaml -o src/apiclient -c @hey-api/client-fetch
RUN cd webapp && npm run build

# --- Stage 2: build the Go binary ---
FROM golang:1.26-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# api/api.gen.go is committed (generated from the same OpenAPI spec, checked
# by CI to be up to date) so it doesn't need regenerating here — only the
# embedded dashboard build output does.
COPY --from=webapp-builder /src/webapp/dist ./webapp/dist
RUN CGO_ENABLED=0 go build -tags=purego -o /out/tg .

# --- Stage 3: minimal runtime image ---
FROM alpine:3.20
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
