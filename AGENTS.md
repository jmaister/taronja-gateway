# Taronja Gateway — Project Context for AI Agents

## Project Overview

**Taronja Gateway** (`github.com/jmaister/taronja-gateway`) is a Go application and API reverse-proxy gateway that centralizes authentication, session management, request routing, static/SPA file serving, and traffic analytics in front of one or more backend services. It includes a bundled React admin dashboard for user management, API token issuance, and statistics. The gateway sits between client applications and backend services, injecting identity headers (`X-User-Id`, `X-User-Data`) so backends can trust the gateway instead of reimplementing auth.

**Key references:** [`doc/adr/0001-purpose.md`](./doc/adr/0001-purpose.md) (architecture decision), [`README.md`](./README.md) (user-facing feature list and configuration reference).

---

## Repository Layout at a Glance

```
taronja-gateway/
├── main.go                     # CLI entry point (Cobra: run, adduser, version commands)
├── Makefile                    # Build/dev/test automation
├── go.mod, go.sum              # Go module def (1.27)
├── .env, .env.sample           # Environment (OAuth secrets, SMTP config)
├── modd.conf                   # Dev file-watch config
├── .goreleaser.yml             # Release build (cross-platform binaries)
│
├── gateway/                    # HTTP server assembly, routing, reverse proxy
├── handlers/                   # OpenAPI handler implementations (one per resource)
├── middleware/                 # Chain builder, auth, cache, logging, rate limit, metrics, JA4H
├── providers/                  # Auth providers (Basic, Google OAuth2, GitHub OAuth2)
├── session/                    # Session store, client-info parsing, IP geolocation
├── auth/                       # API bearer token service
├── db/                         # GORM models, repositories (User, Session, Token, etc.), SQLite
├── config/                     # YAML config loader, struct definitions, validation
├── encryption/                 # Password hashing utilities
├── static/                     # Embedded login page, logo assets
├── api/                        # Generated OpenAPI server types/interfaces
│
├── sdk/                        # Published npm package: taronja-gateway-react-sdk
├── webapp/                     # React/Vite/TS admin dashboard (built into binary)
├── examples/                   # Example client apps (astro-hockey, react-newspaper) + middleware-plugin (Go) + docker-demo (docker compose)
├── sample/                     # Sample config.yaml, example configs
├── doc/                        # Architecture decisions, config reference, middleware docs
├── scripts/                    # Install scripts, GoReleaser setup, migration helpers
├── test/                       # Load-test plan (JMeter)
└── .github/
    ├── copilot-instructions.md # GitHub Copilot conventions (Go style, test patterns)
    └── workflows/              # CI/CD pipelines
```

---

## Backend Modules (Go)

### `main.go` — CLI Entry Point

- **Cobra CLI** with six subcommands:
  - `run --config <path> [--watch]` — starts the gateway server (config file path required); handles `SIGINT`/`SIGTERM` by calling `http.Server.Shutdown()` with a `gracefulShutdownTimeout` (15s) deadline instead of dying mid-request — see "Graceful shutdown" below. Also reloads the config file without restarting on `SIGHUP`, or automatically on save when `--watch` (default `true`) is left on — see "Hot config reload" below
  - `adduser <username> <email> <password>` — direct CLI user creation
  - `version` — displays version/commit/build date (injected by GoReleaser)
  - `middleware list --config <path>` — prints the resolved global middleware chain and status for a config file, without starting the server (see Phase 4 notes in the `middleware/` section below)
  - `migrate --config <path>` — prints a config file migrated to the version this build requires on stdout (unchanged if already current); writes nothing itself — redirect the output (`> config-v2.yaml`) to save it. Never touches the original. `run` and `middleware list` both refuse to load a config file older than current and point here (see "Config file versioning" in the `config/` section below)
  - `validate --config <path>` — loads and validates a config file (`config.LoadConfig` + `middleware.ValidateConfigOnly`) and reports success/failure, without starting the server, opening a database connection, or making any network calls — same no-side-effects contract as `middleware list`/`migrate`. Prints a one-line summary on success (route count, prefix); a validation failure prints `FATAL: <error>` to stderr and exits 1
- Embeds the built React dashboard (`webapp/dist`) into the binary using `//go:embed`

**Graceful shutdown (`main.go`'s `runGateway`):** `gateway.Server.ListenAndServe()` runs in a goroutine so `runGateway` can `select` between it exiting on its own (a real startup/runtime error) and a `SIGINT`/`SIGTERM` arriving (`signal.Notify`). On a signal, it calls `gateway.Server.Shutdown(ctx)` with a `gracefulShutdownTimeout`-bounded context, which lets `net/http` drain in-flight requests before the process exits — previously `ListenAndServe()` was simply never asked to stop, so every deploy/restart/Ctrl+C dropped whatever was mid-flight. `Server.Shutdown` was already used throughout the test suite (`gateway/gateway_test.go`); this just wires the same call into the actual `run` command.
- Loads environment variables from `.env` via `godotenv`

**Hot config reload (`gateway/reload.go`, `main.go`'s `watchConfigFile`):** `run` re-reads and re-applies the config file without restarting the process, triggered two ways — always via `SIGHUP` (`signal.Notify(reload, syscall.SIGHUP)`), and additionally by saving the file when `--watch` is on (default), via an `fsnotify` watch on the file's *containing directory* (not the file itself — the standard workaround for editors/deploy tools that save by writing a temp file and renaming it over the original, which would silently stop a direct file watch after the first save) with events debounced 200ms and filtered to the target filename.
- `Gateway.ReloadConfig(path)` → `applyConfig(cfg)` does `config.LoadConfig` (so an outdated/invalid file is rejected exactly like `run`'s own startup check) → `middleware.ValidateAllMiddleware` → `buildRuntime` (a fresh mux, rate limiter, middleware registry/chain, auth/cache middleware — the same construction `NewGatewayWithDependencies` uses for the *first* config, factored out so both paths build identically) → `ensureAdminUser` → swap the built runtime into the `Gateway` struct's fields → `configureRoutes` (registers everything onto the new mux) → publish the new handler. Every fallible step runs *before* anything on `Gateway` is mutated, so a bad reload (parse error, failed validation) returns an error and leaves the previous generation serving unchanged — never a half-applied config.
- What a reload can *never* change: `server.host`/`port` (would mean rebinding the listening socket) and the database/session-store connection (`Dependencies` isn't rebuilt) — both require an actual process restart. Editing either and reloading anyway doesn't fail or get silently dropped: `warnIfImmutableFieldsChanged` logs a clear warning naming the port still actually listening vs. the new one from the file, and the new value *is* still stored in `GatewayConfig` (there's no sane "partially apply this config" representation) even though the running listener never moves.
- `ReloadConfig` also re-reads `.env` first (`reloadDotEnv`, using `godotenv.Overload` — not `Load`, which is a no-op for every key `.env` already set at process startup, i.e. all of them by reload time) before calling `config.LoadConfig`, so an edited secret behind a `${VAR}` placeholder takes effect on the same reload as a structural YAML change, not just the latter. This only reaches as far as `.env` itself — a value exported directly in the parent shell/process manager's environment (never funneled through `.env`) can't be picked up by an already-running process at all, reload or not; that's a fixed-at-`execve` OS property, not something any application code can work around.
- The one live-per-request read of a field a reload also writes — the login page handler reading `GatewayConfig` — goes through `Gateway.currentConfig()` (package-internal) / `Gateway.CurrentConfig()` (exported, for callers like `main.go`), both guarded by a `configMu sync.RWMutex`; nothing else needs the lock because nothing else reads those fields outside of route registration, which `reloadMu sync.Mutex` already serializes against concurrent reloads. `gateway/reload_test.go`'s `TestReloadConfig_ConcurrentWithLiveRequests` is a `go test -race` regression test for exactly this — proven to actually fail under `-race` by temporarily reverting `currentConfig()` to a bare field read before restoring it, the same regression-proving pattern used elsewhere in this codebase.
- The server's actual `http.Server.Handler` is a `reloadableHandler` (a mutex-guarded `http.Handler`, not `atomic.Value` — the chain's returned concrete type isn't guaranteed consistent across rebuilds, which `atomic.Value` requires) so in-flight requests keep running against whichever generation was already serving them; only requests received after `Store` finishes see the new config.
- `runGateway`'s own `godotenv.Load()` call used to `log.Fatal` on *any* error, including a merely-missing `.env` — meaning the gateway refused to start at all with no `.env` present (`addUser`'s own `godotenv.Load()` call, right below it, only ever warned). Found while building `examples/docker-demo` (a container with no `.env` file, relying on real environment variables via `docker-compose.yml`'s `environment:` block, wouldn't start). Fixed via the extracted, directly-testable `dotEnvLoadIsFatal(err)` — `main_test.go`'s `TestDotEnvLoadIsFatal`.

**TLS termination (`config/tls.go`, `gateway/tls.go`, `main.go`'s `watchCertFiles`):** `server.tls.enabled` turns on HTTPS on `server.port`, via exactly one of two mutually-exclusive certificate sources — `certFile`/`keyFile` (PEM, static) or `acme` (automatic issuance/renewal) — enforced in `config.LoadConfig`. Static-file paths are resolved relative to the working directory (same as route `toFile`/`toFolder`) and validated with a throwaway `tls.LoadX509KeyPair` just to confirm the pair parses — this is why `tg validate` catches a bad cert/key before deploy, the same as a bad admin/CORS/route setting. ACME's config is only validated for shape (domains present, `cacheDir` defaulted/resolved) since actually obtaining a certificate is a network operation `tg validate`'s no-side-effects contract can't perform.
- **Static files:** `gateway.certReloader` (unexported) holds the live `*tls.Certificate` behind an `atomic.Pointer`, served via `tls.Config.GetCertificate` rather than `tls.Config.Certificates` — this is what makes the certificate hot-swappable without rebinding the listening socket. `newCertReloader` loads once at construction (failing `NewGatewayWithDependencies` outright if the pair doesn't parse — belt-and-suspenders alongside `config.LoadConfig`'s own check, but the gateway's copy is what actually serves).
  - `Gateway.ReloadTLSCertificate()` (exported) re-runs `certReloader.Reload()`; a failed reload (e.g. a renewal tool caught mid-write) leaves the previous certificate serving unchanged, logging the error rather than disrupting anything — same fail-safe philosophy as config reload. `main.go`'s `watchCertFiles` drives this: an `fsnotify` watch on `certFile`/`keyFile`'s containing director{y,ies} (same write-then-rename-tolerant pattern as `watchConfigFile`, debounced 200ms), started alongside the config-file watcher whenever `--watch` is on and TLS is enabled with a static cert. This is deliberately independent of config reload entirely — renewing a certificate on disk doesn't touch `config.yaml`, so it was never going to be `ReloadConfig`'s job.
- **ACME:** `gateway.acmeManager` (unexported `*autocert.Manager`, from `golang.org/x/crypto/acme/autocert` — already part of the `golang.org/x/crypto` module this project depends on for password hashing, so no new `go.mod` entry was needed) is built by `newACMEManager` with `HostPolicy: autocert.HostWhitelist(cfg.Domains...)` (never left nil — an unset `HostPolicy` would let anyone connecting by IP with an arbitrary SNI hostname trigger a real certificate request, both a forgery risk and a fast way to exhaust the CA's rate limit), `Cache: autocert.DirCache(cfg.CacheDir)`, `Prompt: autocert.AcceptTOS` (no interactive prompt is possible for a long-running server, so ToS acceptance is automatic — documented prominently in the README). `acmeTLSConfig` wraps `manager.TLSConfig()` (which already sets `GetCertificate` and `NextProtos: ["h2", "http/1.1", acme.ALPNProto]` — the last one is what makes `tls-alpn-01` challenges work automatically on the main listener, no separate port needed) and additionally pins `MinVersion` the same as the static-file path. When `gateway.RedirectServer` exists, its `Handler` gets wrapped in `manager.HTTPHandler(existingHandler)` so `http-01` challenges under `/.well-known/acme-challenge/` are answered directly while every other request still falls through to the normal HTTPS redirect unchanged — this is why `redirectPort: 0` with ACME isn't rejected, just leaves `tls-alpn-01` as the only available challenge type. `Gateway.ReloadTLSCertificate()` is a no-op in ACME mode (the existing `g.tlsCertReloader == nil` check already covers it) since `autocert.Manager` renews itself lazily, in the background, from inside `GetCertificate` — nothing for `main.go`'s file watcher to do, so it's skipped entirely (`config.Server.TLS.ACME == nil` guards starting it). Constructing the manager makes no network calls itself; registration/issuance only happen lazily on a real TLS handshake for a configured domain, so `NewGatewayWithDependencies` stays side-effect-free the same way the static-file path already was — the tests in `gateway/tls_test.go` covering ACME wiring rely on exactly this to stay fast and offline (see that file's own comment on why a full issuance flow is inherently untestable outside of a real public domain and a reachable port 80/443).
- Enabling/disabling TLS, switching between the two certificate sources, or changing either one's settings is treated exactly like `server.host`/`port` in `warnIfImmutableFieldsChanged` (`reflect.DeepEqual` for the `*ACMEConfig` comparison, since it holds a slice): fixed at process construction, since switching what protocol/certificate-source an already-bound listener uses isn't something a reload can do. Only a static certificate's *content* is reload-safe.
- `gateway.RedirectServer` (nil unless TLS is enabled and `EffectiveRedirectPort()` is nonzero, i.e. `server.tls.redirectPort` isn't explicitly `0`) is a second `http.Server` answering plain HTTP with a `308 Permanent Redirect` to the HTTPS equivalent (`httpsRedirectHandler` in `gateway/tls.go`) — 308 rather than 301 specifically to preserve method/body on a redirected non-GET request. `main.go`'s `runGateway` starts it in its own goroutine feeding the same `serverErr` channel as the main listener (buffered to 2) — a bind failure on the redirect port (e.g. port 80 already in use) is treated as fatal, same as the main listener failing, on the reasoning that silently running without a redirect the config asked for is worse than failing loudly; set `redirectPort: 0` to opt out of the listener entirely instead. It's shut down alongside `gateway.Server` on the same graceful-shutdown path.
- `MinVersion: tls.VersionTLS12` is hardcoded in both `newTLSConfig` and `acmeTLSConfig` (`gateway/tls.go`), not configurable — matches the floor every major gateway (nginx, Traefik, Envoy) still defaults to.

**TLS-level JA4 fingerprinting (`gateway/ja4tls.go`):** wired automatically whenever `server.tls.enabled` — regardless of certificate source — with no config flag of its own, since it needs nothing beyond "the gateway sees the real `ClientHello`," which is already guaranteed by TLS being on. Built on `github.com/exaring/ja4plus` (a new, genuinely direct dependency — not part of `golang.org/x/crypto` — added via `go get`/`go mod tidy`; computes JA4 purely from the already-parsed `tls.ClientHelloInfo` fields Go's stdlib exposes, no raw-byte ClientHello parsing needed, unlike JA3).
- `Gateway.tlsJA4 *tlsJA4` is constructed in `NewGatewayWithDependencies` *before* the first `applyConfig` call (which wraps the built handler with `tlsJA4.middleware` — see `reload.go`), even though the actual `*tls.Config`/`ConnState` wiring has to happen *after*, since it needs `gateway.Server` to already exist. Splitting construction from wiring this way is what lets `applyConfig`'s handler-wrapping and the later TLS-specific wiring both reference the same instance without an ordering conflict.
- Three moving pieces from `ja4plus.JA4Middleware`, each independently required per its own doc comment: `StoreFingerprintFromClientHello` wired into `tls.Config.GetConfigForClient` (returning `(nil, nil)` afterward, which per `crypto/tls`'s documented behavior keeps the rest of the already-selected `Config` — including `GetCertificate` — untouched, so this composes cleanly with both the static-file and ACME TLS setups); `ConnStateCallback` wired into `http.Server.ConnState` (evicts a closed connection's stored fingerprint — without this, every connection's entry leaks for the life of the process); `Wrap` — called from `tlsJA4.middleware`, which additionally reads `ja4plus.JA4FromContext` and sets it as the `X-Taronja-JA4-TLS` request header (`fingerprint.JA4TLSHeaderName`), so downstream code (`session.NewClientInfo`) reads it the same "off the request" way it already reads JA4H, rather than needing a second access pattern.
- `tlsJA4.middleware` wraps `rt.handler` from the *outside*, in `applyConfig`, not through `MiddlewareRegistryV2` — deliberately not one of the seven global middlewares, since it's a TLS-connection-level concern (like `RedirectServer`/`certReloader`/`acmeManager` themselves) rather than an HTTP-request one, with no independent enable/disable beyond `server.tls.enabled` itself. Wrapping outside `rt.handler` also guarantees it runs before `session_extraction`/`traffic_metrics`, which need the header already set.
- `gateway/ja4tls_test.go` verifies this against a real TLS handshake (not just wiring assertions): the backend behind a TLS-enabled test gateway actually receives a well-formed `t13...` JA4 string (Go's TLS 1.3 client), and a plain-HTTP gateway never builds a `tlsJA4` at all.

**The reduced-entropy "stable" fingerprint (`middleware/fingerprint/stable.go`):** added alongside JA4H in the same `ja4_fingerprint` middleware call (`middleware/ja4.go`'s `JA4Middleware`, `middleware/performance.go`'s `OptimizedJA4Middleware`) rather than as a separate middleware, since it's cheap (a handful of header reads plus one SHA256, no cache needed unlike JA4H) and conceptually the same concern: computing something from request headers. `StableFingerprint` hashes only `User-Agent`/`Accept-Encoding`/`Accept-Language`/`Sec-Ch-Ua*` — deliberately excluding everything that makes JA4H noisy (header count, the header-name-set hash, method, cookie/referer presence, `Accept`, and even HTTP version, which JA4H uses but this doesn't, since some CDNs route static assets over a different HTTP version than the main document). Returns `""` if none of those headers are present at all, rather than hashing down to one fixed value for every such client. Not part of the JA4 spec family — named `StableFingerprint`, not anything JA4-prefixed, specifically so it's never mistaken for a standard this project doesn't actually implement.

### `gateway/` and `gateway/deps/`

**`gateway/gateway.go`** — Core HTTP server assembly:
- Builds `http.Server` on Go 1.22+ `net/http.ServeMux`
- Registers management API routes (`/​_/me`, `/​_/login`, `/​_/logout`, `/​_/users`, `/​_/tokens`, `/​_/statistics`, `/​_/counters`, etc.)
- Registers user-defined routes from config (reverse proxy, static file, SPA routes) with pattern conversion for wildcards
- Implements reverse-proxy logic via `net.http/httputil.ReverseProxy` with path rewriting, SPA 404-fallback, and header injection (`X-Forwarded-*`, `X-User-Id`, `X-User-Data`)
- Serves admin dashboard at `<management.prefix>/admin/` (default `/_/admin/`) with SPA index.html fallback

**`gateway/deps/`** — Dependency-injection container:
- `Dependencies` struct holds DB repositories, session store, token service, rate limiter, cache, geolocation config
- `NewProduction()` — wires all prod dependencies (DB, real geolocation API, etc.)
- `NewTest()` / `NewTestWithName(name)` — in-memory SQLite DB per test for isolation, used in unit tests

### `config/` — Configuration Loading

- **Format:** YAML, loaded via `config.LoadConfig(filePath)`
- **Env-var expansion:** Supports `${VAR_NAME}` interpolation from OS environment (sourced from `.env`)
- **Defaults applied** then validated post-unmarshal (port required, admin credentials required if admin enabled)
- **Main config sections:**
  - `server` — host, port, external URL
  - `management` — API prefix (default `/_`), admin credentials, session config, rate limiter rules, CORS settings, geolocation provider
  - `routes` — list of `RouteConfig` (reverse proxy, static file, SPA routes)
  - `authenticationProviders` — Basic, Google OAuth2, GitHub OAuth2 config
  - `branding`, `geolocation`, `notification.email.smtp`
  - `version` — config schema version (see below)
- **Examples:** [`sample/config.yaml`](./sample/config.yaml), inline in [`README.md`](./README.md), auto-generated reference in [`doc/CONFIG.md`](./doc/CONFIG.md) (regenerate with `make config-docs`, which runs `gomarkdoc` via `go run` — no separate install step, and no `@latest`: it's a pinned `tool (...)` dependency in `go.mod`, same as `oapi-codegen` below, specifically so a new gomarkdoc release can't reformat this committed file and fail CI's "generated files up to date" check with no actual doc-comment change behind it — this happened once already)

**Config file versioning (`config/version.go`):** `GatewayConfig.Version` (yaml `version:`) declares the config schema version; `config.CurrentConfigVersion` is what this build expects. `LoadConfig` always logs the detected version (`Config file version: %d (current: %d)`). An absent/zero `version:` field is treated as `legacyConfigVersion` (1) — every file written before this feature existed.
- **`LoadConfig` refuses to run against an outdated config file** — `checkConfigVersion` returns an error (not just a log line) if the file's version is older than `CurrentConfigVersion`, naming `tg migrate --config <path>` in the error text so the failure is actionable. This is deliberate: an earlier version of this feature migrated the file automatically on every load, but a config silently rewritten out from under someone (even non-destructively) is worse than a hard failure telling them exactly what to run. A version *newer* than `CurrentConfigVersion` is still just a logged warning, not fatal — there's no way to downgrade a config, and refusing to start over an unrecognized-but-harmless newer field would be more disruptive than useful.
- **`tg migrate --config <path>`** (`main.go`'s `migrateConfigFile`, calling `config.MigrateConfigContent`) is the only supported way to move an outdated config forward, and it never writes a file — it prints the result to stdout, the same way any Unix filter would, and leaves saving it (via `> file`) entirely up to the caller. It reads the file, and — if older than current — migrates its *original, pre-`${VAR}`-expansion* bytes (`configMigrations`, one step per version, chained by `migrateConfigToCurrent`); already-current (or newer) input comes back unchanged (`fromVersion >= CurrentConfigVersion`), which the CLI reports as a note on stderr so it doesn't pollute a redirected stdout. `versionedConfigPath` (`config.yaml` → `config-v2.yaml`) now only suggests a filename in messages — nothing enforces it, since the user names the output themselves.
- The only migration today (`migrateV1ToV2`) just stamps the `version:` field via a **text-level** edit (`setTopLevelVersionField`, regex-matched at column 0 so it can't match a same-named key nested under another section) rather than an `Unmarshal`-then-`Marshal` round trip through `GatewayConfig` — that would silently drop every comment and reorder every key in the user's file. A future migration that needs to restructure the document, not just add/update a field, will need a different strategy (e.g. `yaml.Node` AST editing) to keep that property. `configMigrations` is a `map[int]configMigration` keyed by the version being migrated *from*, applied sequentially by `migrateConfigToCurrent` — a config several versions behind steps through each intervening migration in one `tg migrate` run (v1→v2→v3→...), not just the first one.

### `db/` — Database Layer (GORM + SQLite)

**ORM & Driver:**
- GORM (`gorm.io/gorm`) with pure-Go SQLite driver (`modernc.org/sqlite` — **no CGO**, builds with `CGO_ENABLED=0`)
- Auto-migration on init; no separate migration files (schema-as-code via GORM struct tags)

**Models** (`db/schema.go`):
- `User` — username, email, password hash, OAuth provider identity, email confirmation flag
- `Session` — persistent session tied to `tg_session_token` cookie; embeds `ClientInfo` (IP, User-Agent, geolocation, device fingerprint)
- `TrafficMetric` — per-request analytics record; embeds `ClientInfo`
- `Token` — API bearer tokens (SHA-256 hashed, CUID ID, scopes, usage counter, lazy-expiring per ADR 0006)
- `Counter` — generic user "counter" transactions (credits/points ledger)
- `ClientInfo` — shared embedded struct (IP address, parsed User-Agent, browser/OS/device info, geolocation, JA4 fingerprint)

**Repositories** (`db/*repository.go` + `_test.go`):
- One interface + GORM implementation per model (`UserRepository`, `SessionRepository`, `TokenRepository`, `TrafficMetricRepository`, `CountersRepository`)
- Each has unit tests; no mocks — tests use the in-memory SQLite DB

### `auth/` — API Token Service

**`auth/token_service.go`:**
- Issues and validates Bearer API tokens for programmatic access (not session cookies)
- Tokens are SHA-256 hashed at rest, stored with a CUID ID, optional scopes, and usage counters
- Implements "lazy expiration" — a token never truly expires at issued time; expiration checked on validation (ADR 0006)
- `TokenService` interface + database-backed implementation

### `session/` — Session Management and Client Info

**`session/session.go`:**
- `SessionStore` interface + `SessionStoreDB` implementation (backed by `db.Session` table)
- Validates `tg_session_token` cookie and Bearer API tokens
- Session includes `IsAdmin` flag, provider name, expiry, and rich client metadata

**`clientinfo.go`:**
- Parses User-Agent via `github.com/ua-parser/uap-go` to extract browser, OS, device type
- JA4H fingerprinting support (via `middleware/ja4.go`)

**`ipgeo.go`:**
- IP geolocation via `iplocate.io` or fallback `freeipapi.com`
- 7-day cache to minimize external API calls

### `middleware/` — HTTP Middleware Chain

**Architecture:** composable chain builder pattern (see [`doc/middlewares.md`](./doc/middlewares.md) for deep dive).

**Global chain** (applied to all routes in order):
1. JA4H fingerprinting (`ja4.go`)
2. Session extraction (`session_extraction.go`)
3. Traffic metrics recording (`trafficmetric.go`)
4. Request logging (`logging.go`)

**Per-route chain** (applied selectively):
1. HTTP cache-control headers (`cache.go`)
2. Session-based authentication/authorization (`auth.go`)

**Notable middlewares:**
- **Rate limiter** (`ratelimiter.go`) — in-memory IP-based rate limiting, configurable per route/path, scanner detection (blocks known scanner User-Agents)
- **Cache control** (`cache.go`) — sets HTTP cache-control headers on responses
- **Traffic metrics** (`trafficmetric.go`) — records each request as a `TrafficMetric` row for analytics
- **Auth** (`auth.go`) — enforces route-level auth (redirects to login for static/SPA, 401 for API)

**Factory + Registry pattern (Phases 1–3 of `doc/refactor01.md`):** the global chain is built declaratively via `MiddlewareRegistryV2` (`registry_v2.go`) instead of a hardcoded `if` ladder:
- Each existing global middleware (`rate_limiter`, `ja4_fingerprint`, `session_extraction`, `traffic_metrics`, `logging`, `cors`, `compression`) has a `MiddlewareFactory` implementation in `factory.go`, exposing `Create()`, `GetName()`, `GetDescription()`, `GetDependencies()`, `GetDefaultConfig()`. Middleware names are shared constants in `config` (`config.MiddlewareNameRateLimiter`, etc.) so `config` can validate them without importing `middleware`.
- `MiddlewareRegistryV2.BuildChain([]MiddlewareSpec)` looks up each spec's factory, verifies its declared `GetDependencies()` were already built earlier in the same call (e.g. `traffic_metrics` depends on `ja4_fingerprint` — it reads the JA4H header via `session.NewClientInfo` — and on `session_extraction`, which puts the session on the request context that `traffic_metrics` also reads), wraps the created middleware with request-metrics instrumentation (see below), and appends it to a `ChainBuilder`. `ValidateSpecs`/`ValidateGlobalChainSpecs` run the same name+dependency check without instantiating anything, so config can be validated before real dependencies (session store, DB repos, rate limiter instance) exist. `session_extraction`'s own declared dependency on `ja4_fingerprint` preserves the original hardcoded chain's ordering rather than a verified direct read — see the comment on `NewSessionExtractionFactory` in `factory.go`.
- `MiddlewareRegistryV2.GetStatus()` reports each registered factory as `"active"` (included in the last `BuildChain` call) or `"available"` (registered but not built), plus a `Health` (nil unless the factory implements `HealthChecker`).
- `ResolveGlobalChainSpecs(gatewayConfig)` turns config into an ordered `[]MiddlewareSpec`: if `gatewayConfig.Middleware.Global` is **non-nil** (a `middleware:` section with a `global:` key present — this checks nil, not length, since YAML unmarshals an absent section to `nil` but an explicit `global: []` to a non-nil empty slice, letting a config explicitly declare zero global middleware rather than that being indistinguishable from "no section") it's used directly, entry order and `enabled` flags respected, and a per-entry `rateLimiter:` override takes priority over `management.rateLimiter`; otherwise it falls back to translating the legacy `management.analytics`/`logging`/`rateLimiter` flags, so existing config files are unaffected. `BuildGlobalChainV2()` and `BuildGlobalChain()` (which now just delegates to it, logging and falling back to an empty chain on error since it has no error return) both go through this.
- `gateway.go`'s `createHTTPServer()` calls `middleware.NewGlobalMiddlewareRegistry()` then `BuildGlobalChainFromConfigV2()` (rather than the all-in-one `BuildGlobalChainV2()`) so it can keep the registry itself, not just the chain — stored on `Gateway.MiddlewareRegistry` and passed into `handlers.NewStrictApiServer` for the status/metrics endpoints below. `RateLimiterFactory` reuses the gateway's existing `*RateLimiter` instance (via its `Handler` method) when one is supplied, rather than constructing a new stateless middleware, so request stats stay consistent with what the management API reports. Because of that reuse, the shared instance's *own* config — not whatever `cfg` `Create` receives — is what actually takes effect; `createHTTPServer` builds it with `middleware.EffectiveRateLimiterConfig(config)` (which resolves a per-entry override the same way `ResolveGlobalChainSpecs` does) rather than `config.Management.RateLimiter` directly, or a per-entry override would be silently ignored (this was a real bug — see `TestCreateHTTPServer_RateLimiterHonorsPerEntryOverride` in `gateway/middleware_wiring_test.go`).
- `middleware.ValidateMiddlewareChainConfig()` (called from `ValidateAllMiddleware`) resolves and validates the chain spec at startup, so a typo'd middleware name or a missing dependency (e.g. `session_extraction` without `ja4_fingerprint`) fails fast with a clear error instead of at request time.
- Only `rate_limiter` has real per-entry YAML configuration (`config.MiddlewareEntryConfig.RateLimiter`) today, since it's the only middleware with tunable runtime options; the others just take `name`/`enabled`. New middleware should get a factory + a name constant in `config`, not another `if` branch.
- `BuildChain` resets `r.built`/`r.metrics` at the start of every call, so `GetStatus`/`GetMetrics`/`GetAllMetrics` always reflect only the most recently built chain — calling `BuildChain` again on the same registry (there's no current caller that does, but nothing stops one) does not leave middleware dropped from the new spec list still reporting "active" from an earlier call.

**Health checks and metrics (Phase 3):**
- `health.go` defines `HealthChecker` (`HealthCheck() MiddlewareHealth`), an interface a `MiddlewareFactory` can optionally implement. Most built-ins are stateless per request and have nothing to check, so `MiddlewareRegistryV2.GetHealth(name)`/`GetStatus()` report `Status: "unknown"` for those rather than a fabricated "healthy" — only `RateLimiterFactory` implements it today, reporting `"healthy"` plus the tracked/blocked IP counts from `RateLimiter.Stats()`.
- `metrics.go` wraps every middleware `BuildChain` creates with `instrumentMiddleware`, which records request count, error count (status ≥ 500), and elapsed wall-clock time per middleware name (in-memory only, reset on restart). Because middlewares nest, a given middleware's `averageDurationMs` includes everything downstream of it too — it isn't an isolated cost. `GetMetrics(name)`/`GetAllMetrics()` read the counters back as a `MiddlewareMetricsSnapshot`.
- `GET <prefix>/api/middleware` (`handlers/api_middleware.go`, admin-only, same pattern as the rate-limiter stats/config endpoints) returns `GetStatus()` as a list. `GET <prefix>/api/middleware/{name}/metrics` returns one middleware's `MiddlewareMetricsSnapshot`, 404 for an unknown name. Both are defined in `api/taronja-gateway-api.yaml` and generated via `make generate` (`oapi-codegen`) into `api/api.gen.go`.
- No dashboard UI was added for these endpoints (Phase 3's "if applicable" dashboard task was left for whenever the webapp actually needs it) — they're consumable today via the API directly.

**Documentation and tooling (Phase 4):**
- [`doc/middleware_development.md`](../doc/middleware_development.md) is the guide for implementing a new `MiddlewareFactory` (and optionally `HealthChecker`), registering it, and wiring it into a gateway instance either as a built-in (`middleware/factory.go` + `NewGlobalMiddlewareRegistry`) or as a separate module.
- [`examples/middleware-plugin/`](../examples/middleware-plugin/) is a complete, compiled, tested third-party-style example (`request_id`, an `X-Request-Id` tracing middleware) that only imports `middleware`'s public API — proof the extension path in the guide actually works, not just prose. Run with `go test ./examples/middleware-plugin/...`.
- [`examples/docker-demo/`](../examples/docker-demo/) is a `docker compose` stack (`docker compose up --build`) with everything running at once: a static route, an authenticated static route, a proxy route to a separate container (`traefik/whoami`), the declarative `middleware:` chain, and Google/GitHub OAuth wired to `.env`. Built from a new repo-root `Dockerfile` (multi-stage: webapp build → Go build with `webapp/dist` embedded → minimal alpine runtime) — not the same thing as `.goreleaser.yml`'s still-disabled `dockers:` release section, though it could become the `dockerfile:` that references someday. Its `config/` directory (not just the one file) is bind-mounted deliberately — see the compose file's own comment — because Docker's single-file bind mounts don't survive a host editor/tool replacing the file via write-then-rename, which would silently defeat `--watch` hot reload.
- `tg middleware list --config <path>` (new Cobra command in `main.go`, implemented by `listMiddleware`) prints the resolved chain and `GetStatus()` for a config file. It builds a `MiddlewareRegistryV2` with every dependency `nil` (session store, repos, rate limiter) since introspection never calls into them — safe against a real config file, opens no DB connection, starts no server.
- [`doc/refactor01-release-notes.md`](../doc/refactor01-release-notes.md) is a human-readable summary of the whole refactor (all 4 phases) suitable for a PR description or release notes — GoReleaser generates its own changelog from commit messages at release time (see `.goreleaser.yml`), so this file exists for the case that needs more than a commit list.
- `CLAUDE.md` intentionally stays a one-line pointer to this file rather than duplicating any of the above.

**Follow-ups from self-review (Phase 5):** two gaps found by re-auditing phases 3–4 (see `doc/refactor01.md` Phase 5) rather than bugs:
- `GET <prefix>/api/middleware/metrics` (`handlers/api_middleware.go`'s `GetAllMiddlewareMetrics`) returns every middleware's `MiddlewareMetricsSnapshot` in one call via `MiddlewareRegistryV2.GetAllMetrics()`, which existed since Phase 3 but had no endpoint — a dashboard previously needed one request per middleware.
- `webapp/src/pages/MiddlewarePage.tsx` (route `/middleware`, nav entry in `Sidebar.tsx`) is the dashboard page Phase 3 marked "(if applicable)" and skipped; modeled directly on `RateLimiterStatsPage.tsx`'s layout/auto-refresh pattern. Uses `useMiddlewareStatus()` + `useMiddlewareMetrics()` (`services/services.ts`), merging the two responses client-side by middleware name.
- Two other gaps found in the same review — per-middleware YAML config beyond `rate_limiter`, and a dedicated dependency-graph data structure — are deliberately **not** implemented; see Phase 5 in `doc/refactor01.md` for why.

**CORS middleware (`middleware/cors.go`, `config/cors.go`):** the sixth built-in global middleware, added when a later architecture review flagged that nothing added CORS response headers at all. `config.CORSConfig` (`management.cors`, and per-entry `middleware.global[].cors`, same override pattern as `rateLimiter`) is disabled by default — `IsEnabled()` is false unless `allowedOrigins` is non-empty, matching the pre-CORS behavior exactly when the section is omitted. `CORSMiddleware` handles both real cross-origin requests (adds `Access-Control-Allow-Origin`/`-Credentials`) and preflight `OPTIONS` requests (also adds `-Methods`/`-Headers`/`-Max-Age` and responds `204` directly, never reaching the wrapped handler). `ValidateCORSMiddleware` (`middleware/validation.go`) rejects `allowedOrigins: ["*"]` combined with `allowCredentials: true` at config-load time — browsers reject that combination outright, so it's caught before deploy rather than silently not working in any browser. In the legacy (non-`middleware:`-section) config path, CORS runs *first among the request-rejecting middlewares*, before rate limiting or auth — a preflight request is never meant to reach application logic, so it needs to be answered before anything that might reject it.

**Compression middleware (`middleware/compression.go`):** the seventh built-in global middleware, `management.compression` (default `false`). Deliberately option-free (see the doc comment on `CompressionMiddleware` — no algorithm choice, no level, no threshold, no per-route opt-out), it gzip/deflate-compresses response bodies via standard `Accept-Encoding` content negotiation. It runs *before* CORS — i.e. as the outermost middleware in the legacy chain — because it works entirely by wrapping the `http.ResponseWriter` passed down through every other middleware and the final route handler; being outermost is what lets it compress the bytes that actually reach the socket regardless of which inner middleware or handler produced them, while `traffic_metrics`/`logging` (which also wrap the writer, further in) still observe the *uncompressed* byte count the handler produced, since compression only affects what gets forwarded past them, not what they record. `compressingResponseWriter` defers the compress/don't-compress decision to the first `Write`/`WriteHeader` call, so it can see whatever `Content-Type`/`Content-Length`/`Content-Encoding` the handler already set. Requests that must never be touched — `Range`, `HEAD`, `Connection: Upgrade` (would break `http.Hijacker` access for WebSocket) — bypass the wrapper entirely rather than being handled by it and declining to compress, so those capabilities are never at risk of being silently dropped.

**`tg validate` (`main.go`'s `validateConfigFile`):** loads a config via `config.LoadConfig`, then additionally runs `middleware.ValidateConfigOnly` — the subset of `ValidateAllMiddleware`'s checks that validate the config file itself (routes, admin, rate limiter, CORS, the middleware dependency graph) without dereferencing `deps.Dependencies`, since `tg validate` (like `middleware list` and `migrate`) never opens a database connection. `ValidateAllMiddleware`'s other checks (`ValidateAnalyticsMiddleware`, `ValidateAuthenticationMiddleware`, `ValidateDependencies`) are deliberately excluded — they check that dependency injection wired real objects, not a property of the config file, so they're meaningless without a real `deps.Dependencies`.

**Declarative `middleware:` config example** (opt-in; omit this section entirely to keep using `management.analytics`/`logging`/`rateLimiter`/`cors`):
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
      enabled: false
```

### `providers/` — Authentication Providers

**`providers.go`:**
- `AuthenticationProvider` interface with Login/Callback/Logout flow
- OAuth2 lifecycle: state + redirect cookies → provider redirect → code exchange → fetch user info → find-or-create `User` by email → create `Session` → set cookie

**Implementations:**
- `basicAuthentication.go` — username/password against `User.PasswordHash`
- `google.go` — Google OAuth2 (redirect to Google, callback at `/_/callback`, exchanges auth code for user info)
- `github.go` — GitHub OAuth2 (similar flow)

### `handlers/` and `api/` — OpenAPI API Implementation

**API Spec:** [`api/taronja-gateway-api.yaml`](./api/taronja-gateway-api.yaml) (OpenAPI 3.x)

**Code generation:** `api/api.gen.go` auto-generated by `oapi-codegen` (run via `make gen`). Defines:
- `StrictApiServer` interface — one method per endpoint
- Request/response types (embedded in generated code, not a separate types file)
- Server interface that handler implementations must satisfy

**Handler implementations** (`handlers/api_*.go`):
- One file per major resource: `api_users.go`, `api_tokens.go`, `api_me.go`, `api_logout.go`, `api_health.go`, `api_statistics.go`, `api_counters.go`, `api_openapi.go`
- `api_impl.go` defines `StrictApiServer` struct with all repo/service/store dependencies; each handler method signature satisfies the generated interface
- **API-first workflow:** when adding a new endpoint, edit the OpenAPI spec first, regenerate with `make gen`, then implement in a handler file

### `encryption/` — Password Hashing

**`encryption/password.go`:**
- `GeneratePasswordHash(plaintext)` — hashes passwords using `golang.org/x/crypto`
- `IsPasswordHashed(hash)` — checks if a string is already hashed (avoids double-hashing)

### `static/` — Embedded Assets

**`static/static.go`:**
- Uses `//go:embed` to bundle `login.html` template and logo/favicon assets
- Served under `<management.prefix>/static/` (default `/_/static/`)

---

## Frontend Modules (TypeScript/React)

The frontend lives under two sibling directories: **`sdk/`** (published npm package) and **`webapp/`** (admin dashboard, built into the Go binary).

### `sdk/` — Published npm Package: `taronja-gateway-react-sdk`

**Purpose:** Reusable React client library for applications that integrate with Taronja Gateway. Currently co-located in this repo but designed to move to an external `taronja-gateway-clients` repository later (see [`sdk/README.md`](./sdk/README.md), [`doc/SDK_RELEASE.md`](./doc/SDK_RELEASE.md)).

**Key exports** (`sdk/src/index.ts`):
- `createTaronjaClient(baseUrl?, opts?)` — dependency-free fetch wrapper for gateway API calls (`getCurrentUser`, `getLoginUrl`, `logout`, user admin, tokens, statistics, rate limiter, counters endpoints)
- `TaronjaAuthProvider` — React context provider for session state, polls `/_/me` every 5 minutes (configurable) to keep session fresh
- `useTaronjaAuth()` — hook to read current user and auth state
- `useTaronjaClient()` — hook to get the configured API client
- `RequireAuth`, `RequireAdmin` — components and HOCs for route/UI protection
- `withTaronjaAuth`, `withTaronjaAdmin` — higher-order components (legacy pattern, prefer hooks)
- Shared types: `CurrentUser`, `UserResponse`, `TokenResponse`, `RequestStatistics`, counter types, etc. (hand-maintained, distinct from the generated webapp types)
- Error type: `TaronjaGatewayError`
- Utilities: `getUserDisplayName`, `getUserInitials`, `getUserAvatar`

**Publishing:** npm package (`taronja-gateway-react-sdk`), released via GitHub Actions workflow (`.github/workflows/sdk-release.yml`), published to npm registry.

**Testing:** Unit tests in `sdk/src/client.test.ts`, `sdk/src/utils.test.ts` (Vitest).

### `webapp/` — React Admin Dashboard

**Stack:** React 19, Vite, TypeScript, TanStack Query (React Query), react-router-dom v7, Tailwind CSS v4

**Build & Dev:**
- Dev server: `npm run dev` (Vite dev server)
- Build: `npm run build` → `dist/` (production build, ~300KB minified)
- This `dist/` is embedded into the Go binary at build time and served at `/_/admin/*`

**Directory Structure:**

```
webapp/src/
├── main.tsx                    # Bootstrap: QueryClientProvider → ThemeProvider → TaronjaAuthProvider → App
├── App.tsx                     # Routes + AdminLayoutRoutes wrapper (auth guard, admin check, layout)
├── index.css                   # Tailwind v4 import + CSS custom-property design tokens + component layer
├── vite-env.d.ts, vite.config.ts, tsconfig.json
├── apiclient/                  # ⚠️ AUTO-GENERATED OpenAPI client (do NOT hand-edit)
│   ├── client.gen.ts, sdk.gen.ts, types.gen.ts, index.ts
│   └── client/, core/          # Generated fetch internals
├── services/
│   └── services.ts             # React Query hooks wrapping apiclient (useUsers, useCurrentUser, useRequestStatistics, etc.)
├── components/
│   ├── layout/                 # Sidebar.tsx, Header.tsx, MainLayout.tsx (app shell)
│   ├── ui/                     # Design system: Badge, Button, Card, FormField, Input, PageHeader, StatusPill
│   ├── theme/                  # ThemeSwitcher.tsx (light/dark + color palette selection)
│   ├── charts/                 # SampleBarChart.tsx (chart.js 4 + react-chartjs-2)
│   ├── RequestsWorldMap.tsx, LazyRequestsWorldMap.tsx  # maplibre-gl world map
│   └── [other components]
├── pages/                      # One file per route
│   ├── HomePage, ProfilePage, NotFoundPage
│   ├── RequestSummaryPage, RequestsDetailsPage, RateLimiterStatsPage
│   ├── UsersListPage, CreateUserPage, UserInfoPage
│   ├── CountersManagementPage
│   └── MiddlewarePage             # status/health/metrics for the global middleware chain (doc/refactor01.md Phase 5)
├── contexts/
│   └── ThemeContext.tsx        # Light/dark mode + color palette provider (drives html[data-theme]/[data-palette])
├── lib/, utils/                # cn() classnames helper, formatting helpers, coordinates
├── assets/                     # Static files (maplibre style, icons)
└── test/
    └── setup.ts                # Vitest setup (currently minimal — no test files in webapp yet)
```

**Key Architectural Details:**

1. **Two API-calling mechanisms coexist** (watch for this when editing):
   - **SDK's `createTaronjaClient`** — used for **auth concerns only** (`/me`, `/login`, `/logout`) in `TaronjaAuthProvider`
   - **Generated OpenAPI client** + **React Query hooks** — used for **all data fetching** (users, tokens, statistics, counters, etc.)
   
   These are two separate fetch implementations that happen to target the same backend. The SDK is dependency-free; the generated client is auto-generated from the OpenAPI spec.

2. **Routing:** `react-router-dom` v7 with `basename="/_/admin"`:
   - Protected routes require auth via `AdminLayoutRoutes` wrapper
   - Admin-only check via `useTaronjaAuth()` + `currentUser.isAdmin`
   - Full route list in [`App.tsx`](./webapp/src/App.tsx)

3. **Styling:** Tailwind v4 via `@tailwindcss/vite` plugin (config-first, not PostCSS). CSS custom properties for theme tokens (`--color-bg`, `--color-fg`, `--color-primary`, etc.) swapped via `html[data-theme='dark']` and `html[data-palette='...']` attributes. Four color palettes: taronja (orange), blue, violet, emerald.

4. **Same-origin API access in production:** The built `webapp/dist` is embedded into the Go binary and served from the same origin as the API (`/_/admin/*` + `/_/*` routes), so no CORS or proxy configuration needed. **Dev mode:** `npm run dev` runs Vite dev server; the dev setup assumes the Go server is running separately and reachable (no proxy configured in `vite.config.ts` — you must either run the Go server or manually proxy API requests).

5. **State management:** No Redux/Zustand; uses React Query cache (server state) + React Context (local UI state like theme).

**API Client Generation:**
- OpenAPI spec: [`api/taronja-gateway-api.yaml`](./api/taronja-gateway-api.yaml)
- Tool: `@hey-api/openapi-ts` (via `make gen`, executed in root Makefile)
- Command (`make gen`'s actual recipe): `cd webapp && npm install && npx @hey-api/openapi-ts -i ../api/taronja-gateway-api.yaml -o src/apiclient -c @hey-api/client-fetch` — run from inside `webapp/`, not the repo root
- Regenerated whenever the spec changes; files carry auto-generated headers (`// This file is auto-generated`)
- `webapp/src/apiclient` is gitignored (a generated artifact, same as `webapp/dist`), so a fresh checkout has no client at all until `make gen` (or the command above) is run once
- **Fixed toolchain gotcha, keep it fixed**: `@hey-api/openapi-ts` is a pinned `devDependency` in `webapp/package.json` (not invoked via bare `npx --yes` from the repo root) specifically because that resolves its own isolated `typescript` dependency — which as of writing pulls in `typescript@7.x`, the from-scratch Go-based compiler rewrite, incompatible with `@hey-api/openapi-ts`'s TS AST codegen (`Cannot read properties of undefined (reading 'SyntaxKind')` / `'LineFeed'`, across multiple `@hey-api/openapi-ts` versions). Running it from inside `webapp/` as a local devDependency makes npm resolve its `typescript` peer against the workspace's own pinned `typescript` (6.0.3, which works) instead. If this starts failing again after a `@hey-api/openapi-ts` or `typescript` version bump, that's almost certainly the same class of issue recurring — don't revert to bare `npx --yes` from root as a "fix." `.github/workflows/ci.yml` and `.github/workflows/release.yml` both had this exact bare `npx --yes` bug live (unnoticed since it happened to work on whatever GitHub Actions' runners resolved) until they were fixed to match — see their "Generate API client"/"Generate Go API code and TypeScript client" steps.
- **CI enforces more than it used to**: `ci.yml` now also runs `gofmt -l` (against `git ls-files '*.go'`, not a bare recursive `gofmt -l .`, since that would also pick up vendored `.go` files under `webapp/node_modules` that aren't ours to reformat), `go vet ./...`, `npm run lint`, and `npx tsc --noEmit` for webapp — and a "Check generated files are up to date" step that regenerates `api/api.gen.go` and `doc/CONFIG.md` and fails the build on any diff. None of these existed before; concretely, `tsc --noEmit` would have caught two real bugs (a missing `UserResponse.createdAt`/`updatedAt` mapping, a JSON-import type error) that shipped silently earlier in this project's history because nothing ran it.

**Dependencies** (key packages in [`webapp/package.json`](./webapp/package.json)):
- React 19, react-dom 19, TypeScript 6
- Vite 8, @vitejs/plugin-react, @tailwindcss/vite
- react-router-dom 7.14, @tanstack/react-query 5
- chart.js 4.5, maplibre-gl 5.24
- date-fns 4.1
- taronja-gateway-react-sdk 0.0.24 (local, points to `../sdk` in dev)
- vitest 4.1, jsdom 29 (no test files in webapp yet; SDK has tests)
- ESLint 9, @tailwindcss/vite

**⚠️ Known inconsistency:** [`webapp/.copilot-instructions.md`](./webapp/.copilot-instructions.md) describes DaisyUI conventions, but DaisyUI is **not** in `webapp/package.json` — verify before using DaisyUI classes in new code (the project currently uses custom `components/ui/` instead).

---

## Build, Run, and Test Commands

### Go Backend (Root Level)

| Command | Purpose |
|---------|---------|
| `make setup` | `go mod download` + `cd webapp && npm install` |
| `make dev` | Start dev file-watcher (`modd`) for live-reload on .go/.tsx/.css changes |
| `make build` | Regenerate API client (`make gen`), build webapp (`npm run build`), build Go binary with embedded assets; produces `tg` executable |
| `make run` | Run gateway with sample config (`tg run --config sample/config.yaml`) |
| `make test` | `go test -cover ./...` — runs all Go unit tests with coverage; uses testify + in-memory DBs |
| `make gen` | Regenerate Go OpenAPI server (`oapi-codegen`) + TS API client (`@hey-api/openapi-ts`) |
| `make config-docs` | Generate [`doc/CONFIG.md`](./doc/CONFIG.md) from `config/` package comments via `gomarkdoc` |
| `make cover` | Run tests + open coverage HTML report |
| `make jmeter` | Run JMeter load test (`test/test-plan.jmx`) |

### React Webapp (under `webapp/`)

| Command | Purpose |
|---------|---------|
| `npm run dev` | Start Vite dev server (assumes Go backend running separately) |
| `npm run build` | Vite build → `dist/` |
| `npm run preview` | Preview production build locally |
| `npm run test` | Run Vitest (currently no tests in webapp; SDK tests run separately) |
| `npm run lint` | ESLint check |
| `npm run analyze` | Build + generate bundle-size HTML report (`dist/bundle-stats.html`) |

### CI/CD

- **GitHub Actions** (`.github/workflows/`):
  - `ci.yml` — on push/PR: setup Go 1.27 + Node 22, regenerate API clients, build SDK/webapp, `go build`, `go test -cover`, post coverage table as PR comment, run `goreleaser check`
  - `sdk-release.yml` — publish SDK to npm (triggered on tag or manual workflow dispatch)
  - `release.yml` — build release binaries and Docker images via GoReleaser (disabled/commented out Docker section)

---

## Conventions to Follow

These practices appear in the codebase and are documented more fully in [`.github/copilot-instructions.md`](./.github/copilot-instructions.md). AI assistants should follow these:

### Go Code

- **Formatting:** 4-space indent (see [`.editorconfig`](./.editorconfig))
- **Error handling:** Separate the error assignment and the `if err != nil` check onto different lines; do not use inline `if err := ...; err != nil { ... }` chains
- **Naming:** Repository/type names must match the underlying concept (e.g., `TrafficMetricRepository` not `StatsRepository`)
- **HTTP server:** Use standard library `net/http` directly; no web framework
- **Database:** GORM for ORM, pure-Go SQLite driver (no CGO)
- **Testing:** Use testify assertions, colocate tests with source (`_test.go`), **no mocks** — use actual in-memory or DB repository implementations
- **Avoid:** No throwaway debug main functions; use tests instead
- **API-first:** OpenAPI spec at [`api/taronja-gateway-api.yaml`](./api/taronja-gateway-api.yaml) drives code generation. Implement handlers in `handlers/` (one file per root endpoint); the server interface is auto-generated (`api.StrictApiServer`)
- **Commands:** Use `make build`, `make run`, `make test`, `make gen`; never run `git add/commit/push` automatically

### React/TypeScript Code

- **Stack:** TypeScript + Vite + React 19 + React Router v7 + Tailwind v4
- **Components:** Function components only; no `React.FC` type annotation
- **State:** Use React Query for server state, React Context for local UI state; no Redux/Zustand
- **Formatting:** 4-space indent (matches Go)
- **Imports:** Use `taronja-gateway-react-sdk` for auth (`TaronjaAuthProvider`, `useTaronjaAuth`); use generated `apiclient` + `services.ts` hooks for data fetching
- **Routes:** Protect routes via `AdminLayoutRoutes` and `useAuth()`; route list in `App.tsx`

---

## Known Inconsistencies to Verify (Don't Assume)

1. **DaisyUI mismatch:** [`webapp/.copilot-instructions.md`](./webapp/.copilot-instructions.md) describes DaisyUI 5 component conventions (e.g., `.btn`, `.card`, `.modal`), but DaisyUI is **not** a dependency in [`webapp/package.json`](./webapp/package.json). The project uses custom `components/ui/` components instead. Before writing UI code that references DaisyUI classes, verify current practice.

2. **`webapp/README.md`:** Generic Vite boilerplate left from `npm create vite`; **not project-specific documentation**. Don't treat it as truth about the project.

3. **Duplicate API types:** `sdk/src/types.ts` (hand-maintained SDK types) vs. `webapp/src/apiclient/types.gen.ts` (auto-generated OpenAPI types) are two separate type sources. Check which one a file imports before assuming a type's structure.

---

## Where to Go Deeper

For more detailed information on specific areas:

- **Purpose & Architecture:** [`doc/adr/0001-purpose.md`](./doc/adr/0001-purpose.md), [`README.md`](./README.md)
- **Configuration reference:** [`doc/CONFIG.md`](./doc/CONFIG.md) (auto-generated from code comments)
- **Middleware design:** [`doc/middlewares.md`](./doc/middlewares.md) (execution order, chain builder patterns)
- **Architecture decisions:** [`doc/adr/*.md`](./doc/adr/) (numbered decisions on purpose, login, webapp tech, stats, tokens, JWT, rate limiter, OAuth, etc.)
- **SDK documentation:** [`sdk/README.md`](./sdk/README.md)
- **SDK release process:** [`doc/SDK_RELEASE.md`](./doc/SDK_RELEASE.md)
- **Auth header contract:** [`README.md`](./README.md) section "Using the Gateway" (detailed header formats, backend integration examples in Node/Go/Python/JS)
- **Performance notes:** [`PERFORMANCE_ANALYSIS.md`](./PERFORMANCE_ANALYSIS.md)

---

## Quick Links by Task

| Task | Key Files |
|------|-----------|
| Add a new API endpoint | Edit [`api/taronja-gateway-api.yaml`](./api/taronja-gateway-api.yaml) → `make gen` → implement in [`handlers/api_*.go`](./handlers/) |
| Add a database model | Add struct to [`db/schema.go`](./db/schema.go) + repository interface/impl in [`db/*repository.go`](./db/) |
| Add a route (proxy/static) | Edit config YAML, see [`sample/config.yaml`](./sample/config.yaml) and [`config/config.go`](./config/config.go) for schema |
| Configure auth provider | Edit config YAML `authenticationProviders` section; implementations in [`providers/*.go`](./providers/) |
| Build for release | `make release-local` (GoReleaser, cross-platform binaries); requires version tag |
| Publish SDK to npm | `.github/workflows/sdk-release.yml` (automated on tag or manual dispatch); details in [`doc/SDK_RELEASE.md`](./doc/SDK_RELEASE.md) |
| Add a UI component | Create in [`webapp/src/components/`](./webapp/src/components/), use in pages under [`webapp/src/pages/`](./webapp/src/pages/) |
| Add a dashboard page | Create in [`webapp/src/pages/`](./webapp/src/pages/), add route in [`webapp/src/App.tsx`](./webapp/src/App.tsx), add menu entry in [`webapp/src/components/layout/Sidebar.tsx`](./webapp/src/components/layout/Sidebar.tsx) |
| Fetch data in React | Use `services.ts` hooks (e.g., `useUsers()`, `useRequestStatistics()`) from [`webapp/src/services/services.ts`](./webapp/src/services/services.ts); these wrap the generated OpenAPI client |
| Add unit tests (Go) | Create `*_test.go` in same package, use testify + in-memory DB via `gateway/deps.NewTestWithName(testName)` |
| Add unit tests (SDK/TS) | Create `*.test.ts` in `sdk/src/`, run `npm test` from `sdk/` |
