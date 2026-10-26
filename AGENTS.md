# Taronja Gateway — Project Context for AI Agents

## Project Overview

**Taronja Gateway** (`github.com/jmaister/taronja-gateway`) is a Go application and API reverse-proxy gateway that centralizes authentication, session management, request routing, static/SPA file serving, and traffic analytics in front of one or more backend services. It includes a bundled React admin dashboard for user management, API token issuance, and statistics. The gateway sits between client applications and backend services, injecting identity headers (`X-User-Id`, `X-User-Data`) so backends can trust the gateway instead of reimplementing auth.

**Key references:** [`doc/adr/0001-purpose.md`](./doc/adr/0001-purpose.md) (architecture decision), [`README.md`](./README.md) (user-facing feature list and configuration reference).

---

## Repository Layout at a Glance

```
taronja-gateway/
├── main.go                     # Thin entry point: embeds webapp/dist, hands build info to cli.Execute
├── cli/                        # Cobra commands, one file each (run, adduser, middleware, migrate, openapi, validate, version)
├── Makefile                    # Build/dev/test automation
├── go.mod, go.sum              # Go module def (1.27)
├── .env, .env.sample           # Environment (OAuth secrets, SMTP config)
├── modd.conf                   # Dev file-watch config
├── .goreleaser.yml             # Release build (cross-platform binaries)
│
├── gateway/                    # HTTP server assembly, routing, reverse proxy
├── handlers/                   # OpenAPI handler implementations (one per resource)
├── middleware/                 # Plumbing: Middleware/Chain, health, metrics; builtin/ holds the implementations
├── providers/                  # Auth providers (Basic, Google OAuth2, GitHub OAuth2, generic OIDC)
├── notification/               # Generic notification system: storage, email/Telegram delivery
├── featureflag/                # Feature flag service (cached values, X-Feature-Flags header)
├── billing/                    # Plans, Stripe Checkout, webhook state machine, entitlements, Odoo outbox; stripeapi/, odooapi/
├── accounts/                   # External login resolution (identities) and activity log
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
├── examples/                   # Example client apps (astro-hockey, react-newspaper) + docker-demo/docker-image (docker compose)
├── sample/                     # Sample config.yaml, example configs
├── doc/                        # Architecture decisions, config reference, middleware docs
├── scripts/                    # Install scripts, GoReleaser setup, migration helpers
├── test/                       # Load-test plan (JMeter)
├── e2e/                        # End-to-end tests (build tag e2e) + docker-compose.yml with Mailpit and Zitadel
└── .github/
    ├── copilot-instructions.md # GitHub Copilot conventions (Go style, test patterns)
    └── workflows/              # CI/CD pipelines
```

---

## Backend Modules (Go)

### `main.go` and `cli/` — CLI Entry Point

`main.go` only holds the `//go:embed webapp/dist` filesystem and the GoReleaser-injected version variables, and calls `cli.Execute`. Each Cobra command lives in its own file in `cli/` (`run.go`, `adduser.go`, `middleware.go`, `migrate.go`, `openapi.go`, `validate.go`, `version.go`, wiring in `root.go`).

- **Cobra CLI** with six subcommands:
  - `run --config <path>` — starts the gateway server (config file path required); handles `SIGINT`/`SIGTERM` by calling `http.Server.Shutdown()` with a `gracefulShutdownTimeout` (15s) deadline instead of dying mid-request — see "Graceful shutdown" below. `SIGHUP` is explicitly ignored (`signal.Ignore`), since its OS default would terminate the process outright
  - `adduser <username> <email> <password>` — direct CLI user creation
  - `version` — displays version/commit/build date (injected by GoReleaser)
  - `middleware list --config <path>` — prints the middleware chain a config resolves to and every available middleware with its dependencies, without starting the server (`cli/middleware.go`)
  - `migrate --config <path>` — prints a config file migrated to the version this build requires on stdout (unchanged if already current); writes nothing itself — redirect the output (`> config-v2.yaml`) to save it. Never touches the original. `run`, `validate` and `middleware list` all refuse to load a config file older than current and point here (see "Config file versioning" in the `config/` section below)
  - `validate --config <path>` — loads and validates a config file (`config.LoadConfig` + `middleware.ValidateConfigOnly`) and reports success/failure, without starting the server, opening a database connection, or making any network calls — same no-side-effects contract as `middleware list`/`migrate`. Prints a one-line summary on success (route count, prefix); a validation failure prints `FATAL: <error>` to stderr and exits 1
- Embeds the built React dashboard (`webapp/dist`) into the binary using `//go:embed`

**Graceful shutdown (`cli/run.go`'s `runGateway`):** `gateway.Server.ListenAndServe()` runs in a goroutine so `runGateway` can `select` between it exiting on its own (a real startup/runtime error) and a `SIGINT`/`SIGTERM` arriving (`signal.Notify`). On a signal, it calls `gateway.Server.Shutdown(ctx)` with a `gracefulShutdownTimeout`-bounded context, which lets `net/http` drain in-flight requests before the process exits — previously `ListenAndServe()` was simply never asked to stop, so every deploy/restart/Ctrl+C dropped whatever was mid-flight. `Server.Shutdown` was already used throughout the test suite (`gateway/gateway_test.go`); this just wires the same call into the actual `run` command.
- Loads environment variables from `.env` via `godotenv`

- `runGateway`'s own `godotenv.Load()` call used to `log.Fatal` on *any* error, including a merely-missing `.env` — meaning the gateway refused to start at all with no `.env` present (`addUser`'s own `godotenv.Load()` call, right below it, only ever warned). Found while building `examples/docker-demo` (a container with no `.env` file, relying on real environment variables via `docker-compose.yml`'s `environment:` block, wouldn't start). Fixed via the extracted, directly-testable `dotEnvLoadIsFatal(err)` — `main_test.go`'s `TestDotEnvLoadIsFatal`.

**The reduced-entropy "stable" fingerprint (`middleware/fingerprint/stable.go`):** added alongside JA4H in the same `ja4_fingerprint` middleware call (`JA4Middleware` in `middleware/builtin/performance.go`) rather than as a separate middleware, since it's cheap (a handful of header reads plus one SHA256, no cache needed unlike JA4H) and conceptually the same concern: computing something from request headers. `StableFingerprint` hashes only `User-Agent`/`Accept-Encoding`/`Accept-Language`/`Sec-Ch-Ua*` — deliberately excluding everything that makes JA4H noisy (header count, the header-name-set hash, method, cookie/referer presence, `Accept`, and even HTTP version, which JA4H uses but this doesn't, since some CDNs route static assets over a different HTTP version than the main document). Returns `""` if none of those headers are present at all, rather than hashing down to one fixed value for every such client. Not part of the JA4 spec family — named `StableFingerprint`, not anything JA4-prefixed, specifically so it's never mistaken for a standard this project doesn't actually implement.

**Fingerprint selection (`middleware/fingerprint/select.go`, `db.ClientInfo`):** the `ja4_fingerprint` middleware calls `fingerprint.Assign(req, ja4h)`, which stores exactly one fingerprint in the single `X-Taronja-Fingerprint` request header as `<type>:<value>`, overwriting anything the client sent. Priority is `TypeStable` ("stable") > `TypeJA4H` ("ja4h"); the header is cleared if none is available. `fingerprint.SelectFingerprint(req)` parses it back to `(value, type)`, and `session.NewClientInfo` stores those as `db.ClientInfo.Fingerprint`/`FingerprintType` — so every `Session`/`TrafficMetric` row and `X-User-Data` carries only those two fields.
- This is a genuine schema/API change, not just an internal rename: `db.TrafficMetricRepository.GetRequestCountByJA4Fingerprint` became `GetRequestCountByFingerprint` (same grouping, now on the consolidated column) plus a new `GetRequestCountByFingerprintType`; the OpenAPI spec's `RequestStatistics.requestsByJA4Fingerprint` became `requestsByFingerprint` + a new `requestsByFingerprintType`, and `RequestDetail` gained `fingerprint`/`fingerprint_type` (previously not surfaced there at all). `webapp/src/pages/RequestSummaryPage.tsx` and the hand-maintained `sdk/src/types.ts` (not auto-generated — see the "Duplicate API types" note in this file's frontend section) were updated to match by hand.
- While making this change, `db.Session`/`db.ClientInfo` also picked up proper `json:"..."` struct tags for the first time — every field there was previously untagged, so `X-User-Data` (built via a bare `json.Marshal(sessionObject)` in gateway.go) actually emitted Go's default PascalCase field names (`"JA4Fingerprint"`, `"IPAddress"`, ...), silently contradicting README.md's always-camelCase documented example. The new tags make the real output match the documented `fingerprint`/`fingerprintType`/`ipAddress`/etc. shape. Not fixed: `gorm.Model`'s own embedded fields (`ID`, `CreatedAt`, `UpdatedAt`, `DeletedAt`) still have no tags and so still leak into `X-User-Data` in PascalCase — suppressing an embedded struct's own fields from JSON requires shadowing it entirely, out of scope for what was actually asked here.

### `gateway/` and `gateway/deps/`

**`gateway/`** — HTTP server assembly, one file per concern:
- `gateway.go` — `Gateway` struct, constructor, admin-user bootstrap, template parsing; `setup.go` builds the middleware chain, mux and rate limiter
- `management_routes.go` — management endpoints (`/​_/me`, `/​_/login`, `/​_/logout`, `/​_/users`, `/​_/tokens`, `/​_/statistics`, `/​_/counters`, etc.), the admin dashboard at `/_/admin/` and the login routes
- `routes.go` — user-defined routes from config (proxy, static, SPA) and wildcard pattern conversion
- `proxy.go` — reverse proxy via `net/http/httputil.ReverseProxy` with path rewriting and header injection (`X-Forwarded-*`, `X-User-Id`, `X-User-Data`); `loadbalancer.go` picks backends
- `static.go` / `spa.go` — static file handler and SPA 404-fallback to `index.html`

**`gateway/deps/`** — Dependency-injection container:
- `Dependencies` struct holds DB repositories, session store, token service, rate limiter, cache, geolocation config
- `NewProduction()` — wires all prod dependencies (DB, real geolocation API, etc.)
- `NewTest()` / `NewTestWithName(name)` — in-memory SQLite DB per test for isolation, used in unit tests

### `config/` — Configuration Loading

- **Format:** YAML, loaded via `config.LoadConfig(filePath)`
- **Env-var expansion:** Supports `${VAR_NAME}` interpolation from OS environment (sourced from `.env`)
- **Defaults** (`server.host: 127.0.0.1`, `management.session.durationSeconds: 1296000`, 15 days) are set before a strict decode; `LoadConfig` then requires `server.port` and calls `GatewayConfig.Validate()`, the single place for config rules (admin credentials, middleware entries, each route has exactly one of `to`/`toFolder`/`toFile`, non-negative cache seconds). `builtin.ValidateConfigOnly` (`tg validate`) and gateway startup call the same `Validate()`, so the rules never diverge.
- **Main config sections:**
  - `server` — host (default `127.0.0.1`), port, external URL
  - `management` — admin credentials and session lifetime. The management endpoints are always under `config.ManagementPrefix` (`/_`); it is not configurable.
  - `routes` — list of `RouteConfig`: a route is static when it sets `toFolder` or `toFile` (`RouteConfig.IsStatic()`), a proxy when it sets `to`; `authenticated: true` requires a login
  - `authenticationProviders` — Basic, Google OAuth2, GitHub OAuth2 config
  - `middleware` — global middleware list (see the `middleware/` section)
  - `branding`, `geolocation`, `notification`
  - `version` — config schema version, `"1.0"` quoted or unquoted (see below)
- **Examples:** [`sample/config.yaml`](./sample/config.yaml), inline in [`README.md`](./README.md), auto-generated reference in [`doc/CONFIG.md`](./doc/CONFIG.md) (regenerate with `make config-docs`, which runs `gomarkdoc` via `go run` — no separate install step, and no `@latest`: it's a pinned `tool (...)` dependency in `go.mod`, same as `oapi-codegen` below, specifically so a new gomarkdoc release can't reformat this committed file and fail CI's "generated files up to date" check with no actual doc-comment change behind it — this happened once already)

**Config file versioning (`config/version.go`):** `GatewayConfig.Version` (yaml `version:`) declares the config schema version as `MAJOR.MINOR`; `config.CurrentConfigVersion` (`"1.0"`) is what this build expects. An absent `version:` is `legacyConfigVersion` (`"0.0"`), every pre-v1.0.0 file.
- **`LoadConfig` refuses to start on any version mismatch**, older or newer: older names `tg migrate --config <path>`, newer says to upgrade the binary. The version is read with a lenient probe first so an old file gets that message rather than an unknown-field error.
- **Strict decoding**: after the version check, `LoadConfig` decodes with `KnownFields(true)`, so a key this version does not know (a typo, or a leftover `management.rateLimiter`) fails with its line number instead of being ignored. Middleware options are decoded strictly by each middleware.
- **The `0.0 → 1.0` migration** lives in its own package, `config/v0` (`v0.UpgradeToV1`; `config.migrateLegacyToV1` calls it and stamps `version: 1.0`). It flattens `notification.email.smtp.*`, applies the 1.0 field renames (`config/v0/fields.go`: drops `management.prefix` and route `static`, `secondsDuration` → `durationSeconds`, route `authentication.enabled` → `authenticated`) and moves the middleware settings into the root `middleware:` list (`config/v0/middleware.go`): an old `middleware.global` list keeps its order, otherwise the `management.*` flags and the root `tracing:` section become middleware in the order they used to run, only the ones that were on. Edits go through `yaml.Node` (`editYAML`), which keeps comments and `${VAR}` placeholders and leaves a file with nothing to move byte-for-byte unchanged; output uses 2-space indentation.
- **`tg migrate --config <path>`** (`cli/migrate.go`'s `migrateConfigFile`, calling `config.MigrateConfigContent`) is the only supported way to move an outdated config forward, and it never writes a file — it prints the result to stdout, the same way any Unix filter would, and leaves saving it (via `> file`) entirely up to the caller. It reads the file, and — if older than current — migrates its *original, pre-`${VAR}`-expansion* bytes (`configMigrations`, one step per version, chained by `migrateConfigToCurrent`); already-current (or newer) input comes back unchanged (`fromVersion >= CurrentConfigVersion`), which the CLI reports as a note on stderr so it doesn't pollute a redirected stdout. `versionedConfigPath` (`config.yaml` → `config-v2.yaml`) now only suggests a filename in messages — nothing enforces it, since the user names the output themselves.
- Adding a version: append it to `configVersionSequence`, and add a `configMigrations` entry only when content must change, with the transformation in its own package named after the version it upgrades from (as `config/v0` does); otherwise the step just stamps the new version (`setTopLevelVersionField`, a text-level edit).

### `db/` — Database Layer (GORM + SQLite)

**Layout:**
- `db/` — connection (`db.go`: `Init`, `SetupTestDB`, `GetConnection`, `NowUTC`) and the schema (`schema.go`: models, `Models`, `HashSessionToken`).
- `db/migrations/` — the numbered `.sql` files only. `db.go` embeds them (`go:embed` can only reach files under the embedding package's directory, so the `db` package owns them).
- `db/dbmigrations/` — package `dbmigrations`: `Run` (golang-migrate plus the one-time legacy bridge) and the v0.0.24 fixture in `testdata/`. It must not import `db` (which calls it), so `db.MigrationSchema()` passes it what it needs: the embedded SQL files, the models and `HashSessionToken`. Its external tests reach unexported functions through `export_test.go`.
- `db/queries/` — query building with no repository state: `TrafficFilter` and dimensions, time-series granularity and bucketing SQL, `ParseStoredTimestamp`.
- `db/repositories/` — every repository interface and GORM implementation, with their tests.

**ORM & Driver:**
- GORM (`gorm.io/gorm`) with pure-Go SQLite driver (`modernc.org/sqlite` — **no CGO**, builds with `CGO_ENABLED=0`)
- Schema migrations via [golang-migrate](https://github.com/golang-migrate/migrate) (`db/dbmigrations/migrate.go`), not GORM's AutoMigrate on every startup — see "Data migrations" below for why, and for the one thing golang-migrate's plain-SQL model can't do (rewriting values already on disk) that this project still handles separately

**Models** (`db/schema.go`):
- `User` — username, email, password hash, OAuth provider identity, email confirmation flag
- `Session` — persistent session tied to `tg_session_token` cookie; embeds `ClientInfo` (IP, User-Agent, geolocation, device fingerprint)
- `TrafficMetric` — per-request analytics record; embeds `ClientInfo`
- `Token` — API bearer tokens (SHA-256 hashed, CUID ID, scopes, usage counter, lazy-expiring per ADR 0006)
- `Counter` — generic user "counter" transactions (credits/points ledger)
- `ClientInfo` — shared embedded struct (IP address, parsed User-Agent, browser/OS/device info, geolocation, JA4 fingerprint)

**Repositories** (`db/repositories/*repository.go` + `_test.go`):
- One interface + GORM implementation per model (`UserRepository`, `SessionRepository`, `TokenRepository`, `TrafficMetricRepository`, `CountersRepository`)
- Each has unit tests; no mocks — tests use the in-memory SQLite DB

**Data migrations (`db/dbmigrations/migrate.go` + `db/migrations/*.sql` + `db/dbmigrations/legacy.go`):** answers "how does an existing database behave when opened by a newer version of the gateway." Schema changes going forward — new tables, columns, indexes — are plain, numbered SQL files under `db/migrations/` (embedded into the binary via `//go:embed`), applied and tracked by [golang-migrate](https://github.com/golang-migrate/migrate)'s own `schema_migrations` table via `dbmigrations.Run`. This replaced a hand-rolled system: GORM's `AutoMigrate` (schema-as-code from Go struct tags, called unconditionally on every startup) plus an indefinitely-growing, hand-written list of Go functions, each tracked by SQLite's `PRAGMA user_version`, for anything `AutoMigrate` couldn't do — it never renames a column, changes an existing column's stored values, or otherwise transforms data already on disk. `AutoMigrate` itself isn't gone, but it no longer runs on every startup: it's now one step inside `applyLegacyDataMigrations`, a one-time bridge `dbmigrations.Run` runs only the first time it ever sees a given database (detected via `needsLegacyBridge` — `PRAGMA user_version` not already at the sentinel `legacyBridgeDoneVersion`, a value neither a brand-new database (`0`) nor any real database that finished under the old system (`1`-`4`) could already be at). That bridge runs *before* golang-migrate's own `0001_initial_schema.up.sql`, not after: SQLite has no "`ALTER TABLE ADD COLUMN IF NOT EXISTS`", so a `CREATE TABLE IF NOT EXISTS`/`CREATE INDEX IF NOT EXISTS` migration can create a table that doesn't exist at all, but can't retrofit a missing column (or an index on one) onto a table that already exists in some older shape — confirmed directly: reversing this order breaks on `db/dbmigrations/testdata/v0.0.24.db`'s genuinely old `sessions` table with `SQL logic error: no such column: fingerprint`, since AutoMigrate hadn't had a chance to add it yet. `AutoMigrate` remains the one thing here that knows how to diff a live struct against an existing table and add what's missing; running it first means `0001_initial_schema.up.sql`'s `IF NOT EXISTS` clauses only ever see an already-current schema in practice, whichever path got a given database there — which also means that file never does real work today, and exists as an explicit, readable baseline instead (the same reason a codebase migrating onto a real migration tool usually writes one).
- After AutoMigrate, the bridge runs the same three data-repair functions the old system did, unchanged, in their original order — `migrateTimestampsToUTC`, `migrateLegacyFingerprintColumns`, `migrateSessionTokensToHashed` — the exact thing neither `AutoMigrate` nor a plain SQL migration file can express: rewriting values already on disk in a way that depends on runtime schema introspection (does this table still have a column the current release doesn't declare?). `db/dbmigrations/legacy.go` (what this file used to be called `db/dbmigrations/legacy.go`) still holds all three. Recorded as done (`PRAGMA user_version = legacyBridgeDoneVersion`) only after every step succeeds, so a process killed mid-bridge, or a step that errors outright, retries the *entire* bridge wholesale on the next startup rather than silently treating partial work as complete — safe, since every step already guards itself against work it's already done (the exact bug this replaces: a shipped version of `migrateSessionTokensToHashed` backfilled `token_hash` but never dropped the legacy `token` column, and the old fine-grained per-migration `PRAGMA` tracking recorded that as done anyway, permanently — see `TestRunMigrations_HealsDatabaseStuckMidOldSystem`).
- `dbmigrations.Run` is called from `Init`, on a short-lived connection dedicated to migrating and closed before the real, long-lived, pooled serving connection ever opens (`db.go`) — the gateway never has a connection open against the database for anything else while it runs. Also called from `SetupTestDB`, on the *same* connection about to be used for serving, deliberately not a separate one: a `file::memory:` test database only exists for as long as at least one connection to it stays open, so migrating on a connection that's closed afterward would silently discard the schema. Tests exercise one migration function directly after seeding legacy-shaped data (see `migrations_test.go`) rather than going through the full bridge. `db/dbmigrations/fixture_v0_test.go` complements those with a real, historical SQLite file (`db/dbmigrations/testdata/v0.0.24.db`, carved out of the top-level `.gitignore`'s `*.db` rule specifically for that directory) built by actually running the v0.0.24 tagged release and generating real traffic — running `dbmigrations.Run` against genuine old data catches shapes nobody would think to reproduce synthetically (the `storedTimestampOffsetPrefix` fallback in `db/queries/timeformat.go`, and the SQL-migration-ordering bug above, both exist because of exactly this). See `db/dbmigrations/testdata/README.md` for how to regenerate or extend it.
- **`migrateTimestampsToUTC`** backfills every pre-existing row in `utcTimestampColumns` (one entry per timestamp column across `User`, `Session`, `TrafficMetric`, `Token`, `Counter`) to UTC. This exists because `db.utcNowFunc` and `db/schema.go`'s `BeforeSave` hooks (see the UTC bullet below) only change what gets *written from now on* — a database with rows from before upgrading keeps them in whatever zone the host's `time.Now()` returned at the time, and left alone, that reintroduces the exact `BETWEEN`-is-a-text-comparison bug those fixes exist to prevent, just at the boundary between old and new data instead of within one call site. Reads each column via a plain `SELECT` into a string (not straight into `time.Time` — see the function's own comment for exactly why) and reparses it with `parseStoredTimestamp` (`db/queries/timeformat.go`, shared with `CountersRepositoryDB.toUserCounterSummary`'s identical parsing need), which needed two real fixes of its own, both confirmed against an actual pre-upgrade database created by literally checking out the last pre-UTC-fix commit and running it, not synthesized: a real system zone's abbreviation (e.g. `"BST"`) round-trips through this driver's own decode-and-reformat-to-RFC3339 step fine, but `Token.ExpiresAt` — API-caller-supplied, from request JSON — commonly carries a bare numeric offset with no named zone at all (`"+05:00"`), which Go represents as a `FixedZone` whose name is the offset text itself; that shape breaks the driver's decode heuristic, which falls back to returning the *raw* stored text verbatim instead of a normalized string, so `parseStoredTimestamp` needs `storedTimestampOffsetPrefix`, a regex fallback that extracts just the `"date time[.fraction] ±HHMM"` prefix and ignores whatever trailing zone-name text follows (real, synthetic, or absent) — the numeric offset alone already fully determines the correct instant. A genuinely old database (built and run from a commit predating this project's UTC work entirely) surfaced a second variant of the same problem: a `time.Time` that still carries its monotonic clock reading (any value built from a bare `time.Now()` that was never subjected to an operation stripping it) renders that as a trailing `" m=±<seconds>"` suffix after the zone name — `storedTimestampOffsetPrefix`'s prefix-only match handles this the same way, for free, since it already discards everything after the numeric offset regardless of shape. A row that still fails to parse is logged and left as-is rather than aborting the whole migration.
- **A genuinely old database (checked out from `f765cbe`, July 2025 — before `Token`/`Counter` existed, before the fingerprint consolidation, before `TrafficMetric.IsStaticAsset`) surfaced an unrelated, far more serious bug during this same review: `AutoMigrate` panicked the whole gateway on startup**, not a soft failure — `ALTER TABLE traffic_metrics ADD is_static_asset NOT NULL` fails outright on SQLite ("Cannot add a NOT NULL column with default value NULL") the moment the table already has one row, since `IsStaticAsset`'s gorm tag was `not null` with no `default`. Any deployment with existing `traffic_metrics` data upgrading past the commit that introduced `IsStaticAsset` would have hit this and been completely unable to start. Fixed by adding `default:false` to the tag — SQLite's `ALTER TABLE` only accepts a `NOT NULL` column addition on a non-empty table when a default is present to backfill existing rows with, and `false` is the semantically correct backfill value here (every pre-existing row predates this column, when every request was still recorded). `TestAutoMigrate_AddingNotNullColumnToExistingRows_NeedsADefault` (`db/automigrate_compat_test.go`) guards this specifically — AutoMigrate an old-shaped `TrafficMetric` (a copy of the struct from before `IsStaticAsset`, defined in the test), insert a row, then `AutoMigrate` the real current struct and confirm it doesn't error. Still directly relevant even though `AutoMigrate` no longer runs on every startup: it's the one call site (inside `applyLegacyDataMigrations`) this hazard can still happen at. This is a general hazard, not specific to this one column: **any future `not null` field added to an already-shipped table needs an explicit `default:...` tag**, or it reproduces this exact startup panic for anyone with existing data — AutoMigrate itself gives no warning at write-the-struct-tag time, only a runtime failure against a populated table.
- **Separately, that same old database exposed a real data-completeness gap from the earlier fingerprint-consolidation commit (`a559a93`)**: old `Session`/`TrafficMetric`/`Token` rows had their historical value sitting in the now-unused `ja4_fingerprint`/`ja4_tls_fingerprint`/`stable_fingerprint` columns, while the new `fingerprint`/`fingerprint_type` columns AutoMigrate added for them were `NULL` — `AutoMigrate` only ever adds columns, never migrates data between them, so nothing carried the old values over, and the application only ever reads `fingerprint`/`fingerprint_type` now. **`migrateLegacyFingerprintColumns`** fixes this: for each of `sessions`/`traffic_metrics`/`tokens` (every table `ClientInfo` has ever been embedded in), a single `UPDATE ... SET fingerprint = CASE ... END, fingerprint_type = CASE ... END WHERE fingerprint IS NULL OR fingerprint = ''` picks the first non-empty of `ja4_tls_fingerprint` (`fingerprint.TypeJA4TLS`) / `stable_fingerprint` (`fingerprint.TypeStable`) / `ja4_fingerprint` (`fingerprint.TypeJA4H`) — the same priority order `fingerprint.SelectFingerprint` itself uses, so a backfilled row gets the same choice the application would have made had it processed that request after the consolidation. Pure SQL, not a row-by-row Go loop like the UTC migration: this is just picking among already-stored strings, none of the timezone-parsing concerns that one has. Guarded per column with `Migrator().HasColumn` — a table created after `a559a93` never had any of the three old columns at all, and this must no-op cleanly for it rather than erroring on a nonexistent column. Confirmed against the same real pre-consolidation database: a session's real captured JA4H value round-tripped into `fingerprint`/`fingerprint_type="ja4h"` correctly, leaving the orphaned `ja4_fingerprint` column (harmlessly) in place.
- Adding a future *schema* change means a new numbered SQL file under `db/migrations/` (e.g. `0002_something.up.sql`) — never edit an already-released one, the same rule as any migration tool. Adding a future *data repair* that genuinely needs runtime schema introspection (the one thing a SQL file can't express) means a new Go function called from `applyLegacyDataMigrations` — but that function only ever runs again for a database that hasn't finished the legacy bridge at all (`PRAGMA user_version != legacyBridgeDoneVersion`); a database already past it needs a real, normal migration instead, the same as any schema change from here on.

**Time-series bucketing (`db/queries/timeseries.go`):** `TrafficMetricRepositoryDB.GetTimeSeries(start, end, granularity)` powers the `/api/statistics/timeseries` endpoint's "traffic over time" graphs — one `TimeSeriesPoint` (request count, unique fingerprints, new vs. returning visitors, unique authenticated users, error count, average response time) per bucket, backfilled with zero-valued points for buckets with no data so the series has no gaps for a chart to render oddly. Aggregation (`COUNT`/`COUNT DISTINCT`/`SUM`/`AVG`, `GROUP BY` bucket) happens in SQL, not by fetching every matching row into Go — an earlier version did the latter, and it doesn't scale: it pulls every matching row across the wire and holds a per-bucket hash set in application memory for every distinct-count field, when the database can compute the same result set-side and transfer only a handful of aggregated rows.
- **Getting SQL-side bucketing to actually work took two real fixes, both confirmed by hand against a live DB before trusting either — this is the one thing to know before touching this file.** The obvious approach (`strftime(...)` on the `timestamp` column in a `GROUP BY`) silently returns `NULL`/empty for every row: `modernc.org/sqlite` stores a `time.Time` as Go's default `.String()` representation (`"2026-06-10 08:00:00 +0200 CEST"` — offset and zone name included), which isn't a format any of SQLite's date/time functions recognize. `sqliteBucketExpr`/`stripUTCFormatSuffix` fix this by taking `substr(timestamp, 1, 19)` first — stripping the offset/zone suffix leaves `"YYYY-MM-DD HH:MM:SS"`, a format SQLite does parse — but that only gives the *correct* bucket if the stripped prefix is already UTC wall-clock time, which is the second fix: `TrafficMetric.BeforeCreate` (`db/schema.go`) normalizes every stored `Timestamp` to UTC, and `session.NewTrafficMetric` calls `.UTC()` explicitly too, belt-and-suspenders. Skipping either half of this reintroduces a real bug, not a theoretical one — see the next point.
- **`WHERE timestamp BETWEEN ? AND ?` has the exact same string-comparison fragility, in the other direction, and it bit this codebase for real**: the `timestamp` column has TEXT affinity, `modernc.org/sqlite` binds a `time.Time` query parameter as that same default `.String()` form, and SQLite compares TEXT operands lexicographically — there is no numeric/chronological comparison happening underneath a `BETWEEN` on this column, ever, in any version of this code. Once `TrafficMetric.BeforeCreate` started normalizing *storage* to UTC without every query's *bounds* getting the same treatment, comparing a UTC-stored row's `"...+0000 UTC"` string against a caller's un-normalized `"...+0100 BST"` bound (anything built from a plain `time.Now()`, which carries the server process's local zone) silently matched the wrong rows. This broke three existing tests immediately (`TestBatchingTrafficMetricRepository_Flushes*`, three `handlers/api_statistics_test.go` cases) the moment it was introduced — caught by the existing real-SQLite test suite, running in this environment's actual non-UTC system timezone (`Europe/London`), not by reasoning about it. The fix is `.UTC()` at the top of every `TrafficMetricRepositoryDB` method that takes a date range (all of them — see `FindByDateRange`'s comment, which the rest point back to) — normalizing centrally via a GORM callback/plugin was considered and rejected: mutating already-built query parameters correctly requires depending on GORM internals (exactly when `Statement.Vars` is populated relative to a hookable callback phase) that are much easier to get subtly wrong than 13 one-line, individually-testable fixes.
- **This whole schema now stores every timestamp as UTC, not just `TrafficMetric.Timestamp`** — auditing the rest of `db/schema.go` after fixing the two points above found `gorm.Model`'s `CreatedAt`/`UpdatedAt`/`DeletedAt` (on every model that embeds it: `User`, `Session`, `TrafficMetric`, `Counter`), `Token`'s `autoCreateTime`/`autoUpdateTime` fields, `Session.ValidUntil`/`LastActivity`/`ClosedOn`, `User.PasswordResetExpires`/`EmailConfirmationExpires`, and `Token.ExpiresAt`/`LastUsedAt`/`RevokedAt` were all still stored in whatever zone `time.Now()` (or GORM's own default clock, itself `time.Now()`) returned on the host — no active bug from this specifically (every comparison against these fields happens in Go via `.Before()`/`.After()`, correct regardless of zone), but a real inconsistency and foot-gun for the next raw SQL comparison someone adds. Fixed in two layers: `db/db.go`'s `gorm.Config.NowFunc` is now `utcNowFunc` (`time.Now().UTC()`) on both the production and `SetupTestDB` connections, covering every `gorm.Model`/`autoCreateTime`/`autoUpdateTime` field automatically (including future models, with no per-model hook needed); and new `BeforeSave` hooks on `Session` and `Token` (plus an extension of `User`'s existing one) normalize the fields the application sets itself outside GORM's own clock — `NowFunc` never touches those. `Token.ExpiresAt` is the one that actually matters most in practice: it's API-caller-supplied (`handlers/api_tokens.go`, from request JSON), not always a server-side `time.Now()`, so this hook is what normalizes whatever offset a client sends. Three raw single-column/map-based `Update`s bypass hooks entirely — a hook fires against an empty `&Model{}` that never sees the value actually being set — so `SessionRepository.CloseSession`, `TokenRepository.RevokeToken`, and `TokenRepository.IncrementUsageCount` normalize their own values at the call site instead. `db/schema_test.go` has one test per hook/call-site fix, each building a `time.Time` in a fixed `+05:00` zone (deliberately neither UTC nor this host's own local zone) and asserting it round-trips out of a real SQLite connection with a zero UTC offset. This only changes what gets written from here on — a database with rows from before this change needs the "Data migrations" section above to actually fix those.
- **New vs. returning visitors** (`NewVisitors`/`ReturningVisitors` on `TimeSeriesPoint`) needs a second query, `getNewVisitorsByBucket`: "how many fingerprints active in this bucket were seen for the very first time, ever" isn't answerable from rows inside `[start, end]` alone. Migration `0002_fingerprint_first_seen` adds a `fingerprint_first_seen` table (fingerprint → earliest timestamp), backfilled from `traffic_metrics` and kept current by an `AFTER INSERT` trigger on `traffic_metrics`, so single and batch inserts both maintain it with no Go code. The query reads only the rows whose first appearance falls in `[start, end]`, so its cost shrinks with the range. When the `TrafficFilter` narrows beyond the time range, a fingerprint counts only if an `EXISTS` subquery finds a matching row in the same bucket, which keeps `NewVisitors <= UniqueFingerprints`. `ReturningVisitors` is `UniqueFingerprints - NewVisitors`.
- `ValidateTimeSeriesRange` caps how large a `[start, end]` span each granularity accepts (minute: 24h, hour: 31d, day: 366d, week: 2y, month: 5y) so a minute-bucketed request over a year can't backfill hundreds of thousands of empty points — called from the handler, not enforced by `GetTimeSeries` itself.
- Week buckets start on Monday (`truncateToBucket`'s ISO-week alignment: back up `(weekday+6)%7` days from the given date), matching common convention over Sunday-first weeks. `truncateToBucket`/`nextBucket` are only used to generate the Go-side list of expected bucket boundaries for backfilling — the actual aggregation grouping is `sqliteBucketExpr`'s SQL expression, not these.
- `db.TimeSeriesGranularity` is a plain string type, not the OpenAPI-generated `api.TimeSeriesGranularity` — keeping `db` from depending on `api` (which itself already depends on nothing in `db` beyond what's needed for generated types); `handlers/api_statistics.go`'s `GetRequestTimeSeries` converts between the two at the boundary.

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

**Sliding sessions (`session/renewal.go`):** `SessionStoreDB.RenewFor` (set in `gateway/setup.go` from `management.session.durationSeconds`; 0 = off) makes `ValidateSession` push a used session's `ValidUntil` to now+RenewFor, at most once an hour (`needsRenewal`). `ValidateSession` has no ResponseWriter, so it records the renewal in the request context, and `session.RenewCookies` — wrapped around the whole gateway handler in `setup.go`, independent of the `middleware:` list — re-sends the cookie with the new Max-Age just before the headers go out, unless the handler set its own session cookie (login/logout). Its writer implements Flush/Hijack/Unwrap so streaming and WebSocket upgrades keep working.

**`clientinfo.go`:**
- Parses User-Agent via `github.com/ua-parser/uap-go` to extract browser, OS, device type
- JA4H fingerprinting support (via `middleware/builtin/ja4.go`)
- `GetClientIP` only honors `X-Forwarded-For`/`X-Real-IP`/`X-Client-IP` when the request's actual TCP peer (`r.RemoteAddr`) is loopback or an RFC 1918/RFC 4193 private-range address (`isTrustedProxy`, via stdlib `net.IP.IsLoopback()`/`IsPrivate()` — the same default Rails' `ActionDispatch::RemoteIp` uses, no configuration involved). This used to trust those headers from any client unconditionally, which is a real, previously-shipped bug: it let any direct client spoof its own IP for geolocation, logging, and `middleware/builtin/ratelimiter.go`'s IP-based rate limiting, all at once — traced back from a real deployment's logs showing a JNDI-exploit probe's own crafted string logged as the "client IP". A configurable `server.trustedProxies` allowlist was tried first and scrapped: the deployment shapes it needed to cover (a proxy on the same host or same private network) are exactly what loopback-or-private already answers for free, with nothing for an operator to understand or configure.

**`ipgeo.go`:**
- IP geolocation via `iplocate.io` or fallback `freeipapi.com`
- 7-day cache to minimize external API calls

### `middleware/` — Global and route middleware

See [`doc/middlewares.md`](./doc/middlewares.md) for the design, [`doc/middleware/`](./doc/middleware/README.md) for each middleware's options, and [`doc/middleware_development.md`](./doc/middleware_development.md) for adding one.

**Middleware (global chain).** The root `middleware:` list in the config is the whole global chain, in order; nothing runs without it. `config.MiddlewareConfig` is `{name, enabled, config}` with `config` kept as a raw `*yaml.Node` (custom `UnmarshalYAML`, since the strict decoder would otherwise match its keys against `yaml.Node`'s fields); the config package knows no middleware names or options and only rejects a missing or duplicated name.

- Each built-in middleware is one file in `middleware/builtin/` with its name constant (`NameCORS`, `NameRateLimiter`, ...), its options type with a `Validate()` method, its `Definition` and its middleware: `tracing.go`, `compression.go`, `cors.go`, `ratelimiter.go`, `ja4.go`, `session.go` (`session_extraction`), `trafficmetric.go`, `logging.go`.
- `builtin/definition.go`: `Definition` is a plain struct (`Name`, `Description`, `DependsOn`, `Decode(*yaml.Node) (any, error)`, `New(opts, GlobalDeps)`, optional `Health(GlobalDeps)`); `definitions` lists every built-in (adding a middleware = one line here). `decodeOptions(defaults)` decodes options over defaults, rejects unknown keys and calls `Validate`; `noOptions` rejects any `config:` block; `stateless(mw)` is the `New` of option-less, service-less middleware.
- `builtin/global_chain.go`: `ResolveGlobalChain(cfg)` checks names and dependency order and decodes each entry's options, needing no runtime services (startup, `tg validate`, `tg middleware list`, `gateway.InitTracing`); `GlobalChain.Build(GlobalDeps)` runs once at startup, creating each middleware wrapped with metrics. The chain is immutable afterwards, so the admin API reads `Status`/`Metrics`/`AllMetrics` without locking.
- Code outside the chain that depends on a middleware's options reads them with `GlobalOptions[C](chain, name)` or `chain.Enabled(name)`, so it always agrees with the chain: `gateway/setup.go` builds the shared `*RateLimiter` from the rate_limiter options (the definition reuses that instance so the chain and the admin API see the same state), `gateway.InitTracing` sets up the OTLP exporter, `gateway/proxy.go` wraps the proxy transport with `otelhttp` when tracing is on, and `gateway/management_routes.go` passes `excludeStaticAssets` to the statistics API.
- Middleware needing runtime services (`session_extraction`, `traffic_metrics`) return an error from `New` when they are missing, so a misconfigured gateway fails at startup.
- Dependencies are declared reads: `traffic_metrics` needs `session_extraction` (session on the request context) and `ja4_fingerprint` (fingerprint header read by `session.NewClientInfo`); `session_extraction` needs `ja4_fingerprint` only to keep the original order.

**Route middleware.** `WrapRoute` (`route_chain.go`) wraps each route's handler: authentication first when the route sets `authenticated: true` (`auth.go`: redirect to login for static/SPA routes, 401 for proxy routes), then `CacheControlMiddleware` (`cache.go`). These are configured on the route, not in `middleware:`, and are not instrumented.

**Sessions.** `session.go` holds `ValidateSessionFromRequest`, `SessionExtractionMiddleware`, `SessionMiddleware`/`StrictSessionMiddleware` (dashboard and OpenAPI routes) and `CheckAdminAccess`, the single admin check also used by `handlers/session_guard.go`.

**Notable middleware:**
- **Rate limiter** — in-memory per-IP limits and vulnerability-scan detection. Every block is also written to `db.BlockedClient` as a persistent record (the in-memory entry is dropped once a block expires), exposed via `GET /_/api/rate-limiter/blocked` (optional `ip`/`country` filters) and the "Blocked Clients History" table on `RateLimiterStatsPage.tsx`, which filters by country through the `?country=` URL parameter.
- **Compression** — option-free brotli/zstd/gzip/deflate via `Accept-Encoding` (tie-break `br > zstd > gzip > deflate`), pure-Go libraries, zstd with `WithEncoderConcurrency(1)`. It wraps the response writer and decides on the first write; `Range`, `HEAD` and `Connection: Upgrade` requests bypass it. Middleware after it still records the uncompressed size.
- **CORS** — disabled unless `allowedOrigins` is set; answers preflights with 204 without reaching the handler; `Validate` rejects `"*"` with `allowCredentials`.
- **Tracing** — `otelhttp.NewHandler` per request; the exporter and global `TracerProvider` are set up once by `gateway.InitTracing` from `cli/run.go`. Tested with the in-memory exporter (`middleware/builtin/tracing_test.go`) and a fake OTLP collector (`gateway/tracing_test.go`).

**Health and metrics.** A definition may set `Health` (only the rate limiter does). `metrics.go` records request count, 5xx count and duration per middleware (nested, so a middleware's duration includes everything after it). Exposed by `GET /_/api/middleware`, `.../middleware/metrics` and `.../middleware/{name}/metrics` (`handlers/api_middleware.go`) and shown on the dashboard's Middleware page (`webapp/src/pages/MiddlewarePage.tsx`).

**Examples.** [`examples/docker-demo/`](../examples/docker-demo/) runs a full stack with `docker compose up --build` (static, authenticated and proxy routes, a `middleware:` list, OAuth from `.env`), built from the repo-root `Dockerfile` that `.github/workflows/docker-release.yml` publishes to `ghcr.io/jmaister/taronja-gateway`. [`examples/docker-image/`](../examples/docker-image/) is the same idea pulling that published image.

### `providers/` — Authentication Providers

**`providers.go`:**
- `AuthenticationProvider` interface with Login/Callback/Logout flow
- OAuth2 lifecycle: state + redirect cookies → provider redirect → code exchange → fetch user info → find-or-create `User` by email → create `Session` → set cookie
- `UserDataFetcher.FetchUserData(r *http.Request, token *oauth2.Token)` — takes the whole request and token, not just an access-token string, so a provider without a REST "get current user" endpoint can instead decode/verify an ID token and read a one-time extra form value off the callback request itself; every provider currently registered ignores both extra parameters and just uses `token.AccessToken`.
- Two optional `AuthenticationProvider` fields, both nil/empty for every provider currently registered: `ClientSecretFunc func() (string, error)` — called to (re)generate `OAuthConfig.ClientSecret` immediately before every token exchange, for a provider whose secret isn't a static string; `ResponseMode string` — sent as the OAuth2 `response_mode` authorization parameter when non-empty. `Callback` reads `state`/`code` via `r.FormValue` (populated from the URL query for a GET callback, or a POST body for `response_mode=form_post`), so both callback shapes go through the same code path.

**Implementations:**
- `basicAuthentication.go` — username/password against `User.PasswordHash`
- `google.go` — Google OAuth2 (redirect to Google, callback at `/_/callback`, exchanges auth code for user info)
- `github.go` — GitHub OAuth2 (similar flow)

Microsoft, Facebook, and Apple ("Sign in with Apple") providers
(`microsoft.go`/`facebook.go`/`apple.go`) existed here too, but moved to
the `wip/microsoft-apple-facebook-auth` branch — implemented but untested,
to come back to later. That branch's `apple.go` is the worked example for
a provider needing the `ClientSecretFunc`/`ResponseMode`/ID-token-instead-
of-REST shape above (its client secret is a JWT the gateway signs itself,
`ResponseMode: "form_post"`, and no REST "get current user" call at all —
`FetchUserData` decodes the ID token from the exchange response instead,
verified against Apple's published public keys); `microsoft.go`/
`facebook.go` are the "normal-shaped" examples alongside `google.go`.

**Generic OIDC (`oidc.go`):** `authenticationProviders.oidc` is a list; each entry becomes an `AuthenticationProvider` named by its `name` (validated in `config.validateOIDCProviders`). Two optional `AuthenticationProvider` fields exist for it: `Prepare` (runs discovery before the endpoints are used; an error answers 503) and `UsePKCE` (S256 challenge in `startAuth`, verifier kept in the `tg_pkce_verifier` cookie for `Callback`). Discovery (`oidcDiscovery.load`) is lazy, cached once it succeeds, retried until then, and checks the document's `issuer`; it fills `oauthConfig.Endpoint` once under its mutex (and `AuthStyleInParams` for a public client with no secret). `OIDCUserDataFetcher` maps the userinfo response; `email_verified` may be a bool or the string "true"; `trustEmail` overrides it. Tests: `oidc_test.go` (fake provider that checks the PKCE verifier) and `e2e/zitadel_test.go` (real Zitadel).

**Login page (`gateway/management_routes.go`'s `handleLoginPage`):** a request with a valid session is redirected on the server to the sanitized `?redirect=` (never back to `/_/login`, which would loop); otherwise the page is rendered. It never hands over to an external provider by itself (an auto-login option was tried and dropped: it needed loop and re-login-after-logout workarounds). Logout never signs the user out at the provider.

**Adding another OAuth2 provider is usually a small, self-contained addition** — the generic `AuthenticationProvider` above already owns the entire flow (state/CSRF, redirect cookie, code exchange, user find-or-create, session creation, login/logout routing); a new provider only supplies the provider-specific glue, mirroring `google.go`/`github.go` (or, if it turns out to need the `ClientSecretFunc`/`ResponseMode`/ID-token-instead-of-REST shape, the wip branch's `apple.go`):
1. A `Name() string` type (e.g. `type XxxProvider struct{}`).
2. A `UserDataFetcher` implementation: normally one HTTP call to the provider's own "get current user" API, mapping its response into the shared `UserInfo` struct.
3. `RegisterXxxAuth(mux, sessionStore, gatewayConfig, userRepo)`: builds the `oauth2.Config` (ClientID/Secret from a new `config.AuthenticationProviders` field, RedirectURL, Scopes, and an `oauth2.Endpoint`) and wires it through `NewAuthenticationProvider`/`RegisterEndpoints`. `golang.org/x/oauth2/endpoints` ships ready-made `AuthURL`/`TokenURL` pairs for 40+ services (Microsoft, GitLab, Discord, Slack, Facebook, LinkedIn, Spotify, ...), and several (Google, GitHub, Microsoft, Slack, GitLab, ...) get their own dedicated subpackage instead — check both before hand-writing an endpoint.
4. Wire the new config field into `providers.RegisterProviders`, `config.GatewayConfig.HasAnyAuthentication`, `config.AuthenticationProviders.PrintOAuthCallbackURLs`, and `config.loginPageData`/`NewLoginPageData`.
5. A login button in `static/login.html` (a `{{if .AuthenticationProviders.Xxx.Enabled}}` block, a brand SVG in `static/` — embedded automatically via `static.StaticAssetsFS`'s `//go:embed *`) and a `.oauth-xxx` CSS rule.
6. Docs: README's "Authentication Providers" section and `sample/config.yaml`.

None of this touches the OAuth2 flow itself for a "normal-shaped" provider — see `google.go`/`github.go` for a complete example of exactly this list.

### `notification/` — Generic Notification System

Full reference: [doc/notifications.md](./doc/notifications.md). Same
shared-interface-plus-registry shape as `providers/`: notification.Service
owns storage/business logic, and a small `Provider` interface
(`Channel() string`, `Send(ctx, req) (externalRef string, err error)`) is
implemented once per external delivery channel — `email.go` (SMTP,
`net/smtp`), `telegram.go` (Telegram Bot API over plain HTTP, plus
`TelegramPoller` long-polling `getUpdates` for account-linking `/start`
messages and inline-keyboard button taps — no webhook, so no inbound
network exposure is required), and `webhook.go`'s `ResponseWebhookProvider`
— an *outbound* HTTP callback (HMAC-signed if `config.ResponseWebhookConfig.Secret`
is set) fired once when a user responds to a notification, on any channel,
so the app that created it learns about the response without polling —
there's no admin-readable "check a notification's state" endpoint, so this
webhook is deliberately implemented as an ordinary `Provider` specifically
to inherit `deliver`/`recordDelivery`'s retry-with-backoff and delivery-
history treatment for free, rather than needing its own weaker reliability
story. It's excluded from `resolveChannelsForUser`'s "every configured
channel" default (see `db.NotificationChannelResponseWebhook`'s doc
comment) since it isn't a channel a notification is ever delivered *to* —
it's only ever invoked directly, once, from
`Service.recordValidatedResponse`, right after a response is recorded (the
in-memory `Notification` is updated with the new `RespondedActionID`/
`RespondedVia`/`RespondedAt` there, since `RecordResponse` only wrote them
to the database, not this copy — `ResponseWebhookProvider.Send` reads them
straight off `req.Notification`).

- `notification.go` — the `Action` type (one possible answer to a
  notification) and JSON (un)marshaling helpers for the caller-opaque
  `Metadata`/`Actions` fields `db.Notification` stores as raw strings.
- `provider.go` — the `Provider` interface, `SendRequest`, and
  `generateOpaqueToken`/`hashToken` (the same random-bytes-then-hash
  pattern `auth.TokenService` uses for API tokens — used for the
  Telegram-linking code and the email respond-link token, both of which
  need to hand a raw secret to something outside the gateway's own auth
  while only ever persisting its hash).
- `service.go` — `Service.Create` takes `CreateInput.UserIDs []string`, not
  a single ID, and returns `(notifications, batchID, err)`: it stores one
  `db.Notification` per recipient (all sharing the same type/title/body/
  actions plus one freshly generated `BatchID` — even a single-recipient
  call gets one, "a batch of one" — each independently read/answered/
  delivered), collecting per-recipient DB failures via `errors.Join` rather
  than letting one bad row abort the whole batch.
  `GetNotificationStatus`/`GetBatchStatus` compute a rolled-up
  sent/failed/pending `Status` on demand from `ListDeliveries`'s raw
  attempts (worst-first: `StatusPending` beats `StatusFailed` beats
  `StatusSent`; a `db.NotificationDeliveryStatusSkipped` channel is
  excluded from the rollup entirely rather than counting as a failure) —
  see `channelStatus`/`overallStatus`. `StatusPending` has two origins:
  a raw `db.NotificationDeliveryStatusPending` row (delivery in-flight,
  provider Send not yet returned) and a `db.NotificationDeliveryStatusFailed`
  row whose retry is still scheduled. `GetBatchStatus` is admin-only (a
  batch can span several different users' own notifications, so there's no
  single owning user to scope it to the way `GetNotificationStatus` is).
  For each recipient it
  then attempts delivery on `resolveChannelsForUser`'s answer: the request's
  explicit `Channels` if given, else that *recipient's own* preferred
  channel (`SetPreferredChannel`/`GetPreferredChannel`,
  `db.NotificationPreference` — "each user can decide to receive
  notifications by one different provider") if they've set one, else every
  configured channel — the pre-preferences default. A channel that isn't
  configured, or that the user has no recipient for, is recorded as
  `db.NotificationDeliveryStatusSkipped` rather than failing `Create`. A
  failed delivery isn't final: `RetryFailedDeliveries` (polled every 30s by
  `RunRetryWorker`, started unconditionally in `gateway.InitNotifications`)
  resends anything past its `NextRetryAt`, appending a *new*
  `NotificationDelivery` row each time (`AttemptNumber` incrementing) —
  deliveries are an append-only log, never mutated in place, so the full
  history survives every retry. `retryBackoffSchedule` (1m/5m/30m/2h, not
  config-exposed) bounds how many times and how far apart; past the last
  entry a delivery just stays `failed`. `RespondViaWeb`/`RespondViaToken`/
  `RespondViaTelegram` are the three ways a response gets recorded (in-app
  session, email link, Telegram button), converging on the same
  `recordValidatedResponse` — first response wins,
  `db.NotificationRepository.RecordResponse` rejects a second one.
- `db/repositories/notificationrepository.go` — the repository interface/impl, plus
  `NotificationChannelLink` (user ↔ external chat ID),
  `NotificationLinkCode` (the short-lived `/start <code>` linking code),
  and `NotificationPreference` (one row per user with a stored channel
  preference — `GetPreferredChannel` returns `""`, never
  `gorm.ErrRecordNotFound`, for "no row" and "empty preference" alike, so
  `resolveChannelsForUser` doesn't need a not-found special case)
  persistence. `FindDeliveriesDueForRetry`/`ClearNextRetry` rely on an
  invariant documented on `db.NotificationDelivery`: at most one row per
  (NotificationID, Channel) ever has a non-null `NextRetryAt` at a time,
  which keeps the due-retry query a plain index scan instead of a
  latest-row-per-group query.
- `handlers/api_notifications.go` — the OpenAPI `StrictApiServer` methods.
  `CreateNotification` (server-to-server) is admin-only, checked the same
  way `CreateToken`/`ListTokens` are (`sessionObj.IsAdmin`), reusing the
  existing admin-owned API token mechanism rather than a new auth concept.
  `RespondToNotificationByToken` (the public email answer link) is listed
  in `middleware.OperationWithNoSecurity` since it has no session to check —
  the opaque `token` query parameter is the credential instead, verified
  inside the handler. `ListNotificationDeliveries` (owner or admin) exposes
  the full retry/delivery history built above. `GetNotificationStatus`
  (owner or admin) and `GetNotificationBatchStatus` (admin only) expose
  `Service`'s computed status rollup. `GetNotificationPreference`/
  `SetNotificationPreference` are self-service (the caller's own session,
  no admin check) — a user manages only their own preference, never
  someone else's.

**Phone channels (`sms.go`, `whatsapp.go`, `smstext.go`, `phone.go`):** SMS (BulkGate Simple API) and WhatsApp (Meta Cloud API, always the one configured approved template) deliver to `db.UserPhone` (its own `user_phones` table: a `users` column added by SQL would collide with the legacy bridge's AutoMigrate on new databases). Two optional `Provider` interfaces keep `Service` channel-agnostic: `PhoneAddressedProvider` (recipient = the user's phone; channel status connected when one is on file) and `LinkProvider` (`WantsLink` → `buildSendRequest` creates a `db.NotificationLinkToken` and passes `LinkURL`/`LinkToken` for the `/_/n/<token>` page in `gateway/notification_page.go`, which answers by POST through `Service.RespondViaLink`). Both need `Service.ConfigurePhoneChannels(phones, server.url)`, called in `gateway.InitNotifications` and `deps.NewTestWithName`. A provider returns `permanent(err)` for errors retrying cannot fix; `recordFailure` then records the failure with no `NextRetryAt`, so paid messages are not paid for again. `minuteLimiter` caps each paid channel (`maxPerMinute`); over it the send fails as retryable. Admin API: `PATCH /api/users/{id}` (name, phone), optional password and `phone` on `POST /api/users`, and `PUT /api/users/{id}/notifications/preferences` (validated against `notification.DeliveryChannels`). Tests: `notification/phone_channels_test.go` (fake BulkGate/Meta servers) and `gateway/phone_notifications_test.go` (through the HTTP API).

**Admin history (`notification/admin.go`, `db/repositories/notificationadmin.go`, `handlers/api_admin_notifications.go`):** `GET /api/admin/notifications` filters in SQL (`AdminNotificationFilter.apply`) and paginates with a total. Status filters look at each channel's latest attempt (`MAX(rowid)` per notification and channel, since an attempt writes a pending row and then its outcome) and mirror `channelStatus` (a failed attempt with `next_retry_at` is pending); without a channel, the overall status is the first of pending, failed, sent, skipped that applies, or none, and `adminStatus` computes the same in Go for each returned row. Dashboard: `webapp/src/pages/NotificationsPage.tsx` with the filters in the URL (`lib/notificationFilters.ts`).

**Adding a new delivery channel** (WhatsApp, Slack, SMS, ...) means writing
a new `Provider` implementation and registering it in
`notification.NewService` — no change to `db.Notification`, the OpenAPI
spec, or any other provider. If the channel needs its own account-linking
step (most will, the way Telegram does — there's no notion of "this
gateway user's WhatsApp number" otherwise), mirror
`NotificationChannelLink`/`NotificationLinkCode` and the `/start` flow in
`telegram.go`.

### `accounts/` — External Logins and Activity

Full reference: [doc/accounts.md](./doc/accounts.md). `accounts.Service` (built in `gateway/setup.go`'s `newAccountsService`, stored as `Gateway.Accounts`) resolves OAuth/OIDC logins to users and records the activity log, with no HTTP handlers. The only account page (`/_/account`, in `gateway/account_routes.go`) exists for billing.
- **Tables** (migration `0004_accounts`): `user_identities` (provider + subject per external login; backfilled from `users.provider/provider_id` for Google/GitHub), `auth_events` (activity log).
- **OAuth login** goes through `ResolveExternalLogin` (in `providers.AuthenticationProvider.Callback`): identity by (provider, subject) first, then same-email auto-link only when `canAutoLink` (provider-verified email, account email confirmed or no password, not the config admin), else `ErrLinkRequired` → `/_/login?error=link_required` (messages in `gateway/login_errors.go`). An unverified provider email never creates a user (`?error=email_not_verified`).
- Model IDs for `UserIdentity` are set in `BeforeCreate`, not `BeforeSave`: GORM runs `BeforeSave` on `Model(&X{}).Update(...)` too, and an ID set there ends up in the `WHERE`, so the update silently matches nothing.

### `featureflag/` — Feature Flags

Zero-config on/off switches (`db.FeatureFlag`, migration `0003_feature_flags`; `Type` is always `boolean` today, kept so typed values can be added as new types plus a value column). `featureflag.Service` wraps `repositories.FeatureFlagRepository`, validates keys (`^[a-z0-9][a-z0-9._-]{0,99}$`, safe for a comma-separated header and a URL segment) and caches the key→value map in memory; every write through the service drops the cache (a generation counter stops a load that overlapped a write from re-caching stale values). Built in `deps.NewProduction`/`NewTestWithName` as `Dependencies.FeatureFlags`, handed to `StrictApiServer` with `SetFeatureFlags`.
- Admin API: `/api/feature-flags` (list/create) and `/api/feature-flags/{key}` (get/PATCH/delete), `handlers/api_featureflags.go`, 401/403 via `requireAdminAccess`.
- Public read: `GET /flags` (`GetFeatureFlagValues`, listed in `OperationWithNoSecurity`) — at the management root next to `/me`, not under `/api/feature-flags/`, so it cannot collide with a flag key.
- Backends: `gateway/proxy.go` drops any client-sent `X-Feature-Flags` and sets it to the enabled keys, sorted, on every proxied request.
- Dashboard `webapp/src/pages/FeatureFlagsPage.tsx` (+ `services/featureFlags.ts`); SDK `client.getFeatureFlags()` and `useFeatureFlags`/`useFeatureFlag` (`sdk/src/flags.tsx`).

### `billing/` — Billing and Entitlements

Spec: [doc/billing.md](./doc/billing.md). `billing.Service` (built by `gateway.InitBilling` when `billing.enabled`, stored as `Dependencies.Billing`, handed to `StrictApiServer` with `SetBilling`) holds the repositories (`repositories.BillingRepository`, `OrganizationRepository`), the `billing.Stripe` and `billing.Odoo` interfaces (`billing/contracts.go`) and the notification service. One file per area: `plans.go` (validation, Stripe product/prices with lookup keys, Odoo products, portal switch groups), `checkout.go` (subject choice, Checkout params per plan kind, trial once per plan group, portal URL), `webhook.go` (each event id recorded with its effect in one `Transaction`; `ErrRetryLater` makes Stripe resend events that arrive before what they refer to), `activation.go` (activate, cancel, refund), `entitlements.go` (pure `Compute` + per-subject cache, memberships cached per user, both with a 24 h TTL, per process (single gateway instance) and cleared by `InvalidateUser`/`InvalidateAll` on any billing change; each `Feature` carries `grace`/`graceUntil`, and the header an optional `grace` map; plans are EUR only), `odoosync.go` (outbox: due documents, backoff 1m/5m/30m/2h/6h, failed after 10, credit notes wait for their invoice), `notify.go` (`billing.*` notifications; `NotifyDue` hourly windows), `setup.go` (setup page checks/actions, including the Odoo outbox queue and `billing.odoo.apiKeyExpiresAt`; credit notes stuck behind their invoice get a `blockedReason` in the admin documents list).
- `billing/stripeapi`: stripe-go v87 `stripe.NewClient` V1 services; tests with an httptest Stripe and `-tags e2e` against stripe-mock.
- `billing/odooapi`: JSON-2 client; every `IssueMove` step checks state first, so a retry finishes a half-done move. Odoo 19 only (Odoo 20 lacks OIDC for now). Tests against a fake JSON-2 server and `-tags e2e` against the Odoo of `e2e/docker-compose.yml` (`e2e/odoo/init.sh` + `setup.py`).
- Live Stripe test: `STRIPE_TEST_SECRET_KEY=sk_test_… go test -tags e2e -run TestLive ./billing/stripeapi/` (no `stripe listen` needed). Tests use `billing.FakeStripe`/`FakeOdoo` (`fakes.go`) with real SQLite; `e2e/billing_test.go` runs the whole flow with signed webhook events.
- Gateway: `gateway/billing_routes.go` (`/_/pay/{plan}`, `/_/pay/done`, `POST /_/billing/stripe/webhook`, the account page's Billing section and portal button), `X-Entitlements` set in `gateway/proxy.go`; organizations API in `handlers/api_organizations.go`; the organization logo (blob in `organizations.logo`, PNG/JPEG/WebP up to 512 KiB, sniffed from the bytes, served with ETag by `handlers/api_organization_logo.go`).

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
- `handlers/session_guard.go` holds `requireSession` / `requireAdminSession`; use them instead of reading `session.SessionKey` from the context by hand
- **Traffic filters:** every statistics endpoint (summary, request details, time series) accepts the same query parameters, defined once under `components/parameters` (`Traffic*`) in the OpenAPI spec: `start_date`, `end_date`, `is_static`, `status_code`, `country`, `device_type`, `platform`, `browser`, `username`, `fingerprint`, `fingerprint_type`. `handlers/traffic_filter.go` turns them into a `db.TrafficFilter` (`db/queries/trafficfilter.go`), which every `TrafficMetricRepository` statistics method takes and applies with `Apply`. To add a filter: add the field and its condition to `TrafficFilter`, add the parameter to the spec and the three `Params` mappings in `handlers/traffic_filter.go`, and in the webapp add the field to `lib/trafficFilter.ts` (state, `trafficDimensionFields`, `trafficFilterToQuery`); `TrafficFilterBar` renders the control from that list, and `useTrafficFilter` holds the state shared by the summary and details pages. Values shown in the pages are clickable filters through `components/FilterToggle.tsx` (a second click clears, via `lib/toggleValue.ts`): every breakdown row on the summary page, and the countries in the details page's map and table. `CountBy(dimension, filter)` replaces the per-column count methods; new group-by columns go in `dimensionSpecs`.

**`GET /_/api/statistics/timeseries`** (`handlers/api_statistics.go`'s `GetRequestTimeSeries`, admin-only): request/visitor counts bucketed over time — see `db/queries/timeseries.go` for the bucketing implementation. Defaults `granularity` to `"day"` and, when `start_date` is also omitted, picks a default span proportional to the chosen granularity (24h for minute/hour/day, ~12 weeks for week, 12 months for month) rather than one fixed "last 30 days" default like `GetRequestStatistics` uses — a fixed 30-day default would be a near-single-point series at month granularity. Validates the resolved range via `db.ValidateTimeSeriesRange` before querying, returning `400` (not `500`) for an unrecognized granularity or a span too large for it. Powers `webapp/src/pages/RequestSummaryPage.tsx`'s "Traffic Over Time" section (`TrafficOverTimeSection`, using the new `webapp/src/components/charts/TimeSeriesChart.tsx`) — a preset dropdown (Last hour/24h/7d/30d/12mo, each with its own matching granularity, the same pattern Grafana/Cloudflare/Vercel Analytics use) driving three charts: requests+unique-fingerprints+unique-users, errors, and average response time. The chart itself uses chart.js's plain `CategoryScale` with pre-formatted date-fns labels rather than the built-in `TimeScale`, since this project doesn't have `chartjs-adapter-date-fns` as a dependency and adding one wasn't necessary to get a correct axis.

### `encryption/` — Password Hashing

**`encryption/password.go`:**
- `GeneratePasswordHash(plaintext)` — hashes passwords using `golang.org/x/crypto`
- `IsPasswordHashed(hash)` — checks if a string is already hashed (avoids double-hashing)

### `static/` — Embedded Assets

**`static/static.go`:**
- Uses `//go:embed` to bundle `login.html` template and logo/favicon assets
- Served under `/_/static/`

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
│   ├── charts/                 # TimeSeriesChart.tsx (chart.js 4 + react-chartjs-2, real data); SampleBarChart.tsx (unused hardcoded-data demo, predates TimeSeriesChart)
│   ├── RequestsWorldMap.tsx, LazyRequestsWorldMap.tsx  # maplibre-gl world map
│   └── [other components]
├── pages/                      # One file per route
│   ├── HomePage, ProfilePage, NotFoundPage
│   ├── RequestSummaryPage, RequestsDetailsPage, RateLimiterStatsPage
│   ├── UsersListPage, CreateUserPage, UserInfoPage
│   ├── NotificationsPage            # admin notification history: filters in the URL, pagination, delivery attempts per row
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
- Command: `cd webapp && npm run gen:api` (what `make gen`, CI, the release workflow, GoReleaser and the Dockerfile all run)
- Regenerated whenever the spec changes; files carry auto-generated headers (`// This file is auto-generated`)
- `webapp/src/apiclient` is gitignored (a generated artifact, same as `webapp/dist`), so a fresh checkout has no client at all until `make gen` (or the command above) is run once
- **Toolchain gotcha**: `@hey-api/openapi-ts` builds code with TypeScript's JavaScript compiler API, which TypeScript 7 (the Go rewrite) no longer has (`Cannot read properties of undefined (reading 'AnyKeyword')`). The webapp type-checks with TypeScript 7, so `gen:api` runs the generator through `npx -p @hey-api/openapi-ts@<v> -p typescript@6.0.3`, which installs it with its own TypeScript 6. npm `overrides` cannot do this (`typescript` is a peer dependency). Bump the two versions in that script together.
- **CI enforces more than it used to**: `ci.yml` now also runs `gofmt -l` (against `git ls-files '*.go'`, not a bare recursive `gofmt -l .`, since that would also pick up vendored `.go` files under `webapp/node_modules` that aren't ours to reformat), `go vet ./...`, `npm run lint` (oxlint), and `npx tsc --noEmit` for webapp — and a "Check generated files are up to date" step that regenerates `api/api.gen.go` and `doc/CONFIG.md` and fails the build on any diff. None of these existed before; concretely, `tsc --noEmit` would have caught two real bugs (a missing `UserResponse.createdAt`/`updatedAt` mapping, a JSON-import type error) that shipped silently earlier in this project's history because nothing ran it.

**Dependencies** (key packages in [`webapp/package.json`](./webapp/package.json)):
- React 19, react-dom 19, TypeScript 6
- Vite 8, @vitejs/plugin-react, @tailwindcss/vite
- react-router-dom 7.14, @tanstack/react-query 5
- chart.js 4.5, maplibre-gl 5.24
- date-fns 4.1
- taronja-gateway-react-sdk 0.0.24 (from npm, not the local `../sdk`)
- vitest 4.1, jsdom 29 (no test files in webapp yet; SDK has tests)
- TypeScript 7, oxlint (`.oxlintrc.json`), @tailwindcss/vite

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
| `make e2e` | Starts `e2e/docker-compose.yml` (Mailpit, Zitadel), runs `go test -tags e2e ./e2e/...` against an in-process gateway (notifications, OIDC login, feature flags), stops the services. See [`doc/e2e.md`](./doc/e2e.md) |
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
| `npm run lint` | oxlint (`.oxlintrc.json`) |
| `npm run gen:api` | Regenerate `src/apiclient` from the OpenAPI spec |
| `npm run analyze` | Build + generate bundle-size HTML report (`dist/bundle-stats.html`) |

### CI/CD

- **GitHub Actions** (`.github/workflows/`):
  - `ci.yml` — on push/PR: setup Go 1.27 + Node 22, regenerate API clients, build SDK/webapp, `go build`, `go test -cover`, post coverage table as PR comment, run `goreleaser check`
  - `sdk-release.yml` — publish SDK to npm (triggered on tag or manual workflow dispatch)
  - `clients.yml` — generate and push the OpenAPI-generated clients to a separate repo (triggered on tag or manual workflow dispatch)
  - `release.yml` — build release binaries via GoReleaser (its own `dockers:` integration is unused — see `.goreleaser.yml`'s comment; the image is built separately, below)
  - `docker-release.yml` — build and push `ghcr.io/jmaister/taronja-gateway:<version>` (linux/amd64 and linux/arm64, `tg version` stamped) on every GitHub Release, plus `:latest` unless it is a pre-release, with a `workflow_dispatch` build-only dry run
  - `docker-pr.yml` — build and push `ghcr.io/jmaister/taronja-gateway:pr-<number>` on every push to a same-repo (non-fork) PR, plus a daily job that deletes `pr-*` images older than 5 days (needs a `GHCR_CLEANUP_TOKEN` repo secret — see the workflow's own comment for why `GITHUB_TOKEN` isn't enough for this specific operation)

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
- **Performance notes:** [`doc/PERFORMANCE_ANALYSIS.md`](./doc/PERFORMANCE_ANALYSIS.md)

---

## Quick Links by Task

| Task | Key Files |
|------|-----------|
| Add a new API endpoint | Edit [`api/taronja-gateway-api.yaml`](./api/taronja-gateway-api.yaml) → `make gen` → implement in [`handlers/api_*.go`](./handlers/) |
| Add a database model | Add struct to [`db/schema.go`](./db/schema.go) + repository interface/impl in [`db/repositories/*repository.go`](./db/repositories/) |
| Add a route (proxy/static) | Edit config YAML, see [`sample/config.yaml`](./sample/config.yaml) and [`config/config.go`](./config/config.go) for schema |
| Configure auth provider | Edit config YAML `authenticationProviders` section; implementations in [`providers/*.go`](./providers/) |
| Add a notification delivery channel | Implement `notification.Provider` in [`notification/*.go`](./notification/), register it in `notification.NewService`; see [doc/notifications.md](./doc/notifications.md) |
| Build for release | `make release-local` (GoReleaser, cross-platform binaries); requires version tag |
| Publish SDK to npm | `.github/workflows/sdk-release.yml` (automated on tag or manual dispatch); details in [`doc/SDK_RELEASE.md`](./doc/SDK_RELEASE.md) |
| Add a UI component | Create in [`webapp/src/components/`](./webapp/src/components/), use in pages under [`webapp/src/pages/`](./webapp/src/pages/) |
| Add a dashboard page | Create in [`webapp/src/pages/`](./webapp/src/pages/), add route in [`webapp/src/App.tsx`](./webapp/src/App.tsx), add menu entry in [`webapp/src/components/layout/Sidebar.tsx`](./webapp/src/components/layout/Sidebar.tsx) |
| Fetch data in React | Use `services.ts` hooks (e.g., `useUsers()`, `useRequestStatistics()`) from [`webapp/src/services/services.ts`](./webapp/src/services/services.ts); these wrap the generated OpenAPI client |
| Add unit tests (Go) | Create `*_test.go` in same package, use testify + in-memory DB via `gateway/deps.NewTestWithName(testName)` |
| Add unit tests (SDK/TS) | Create `*.test.ts` in `sdk/src/`, run `npm test` from `sdk/` |
