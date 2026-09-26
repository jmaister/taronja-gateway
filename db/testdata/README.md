# db/testdata

Real, historical SQLite database files, kept as test fixtures — the
`*.db` exception carved out of the repo's `.gitignore` for exactly this
directory (see the top-level `.gitignore`).

Every migration test elsewhere in this package (`migrations_test.go`,
`automigrate_compat_test.go`) reproduces an old schema shape
*programmatically* — a Go struct mirroring the old columns, AutoMigrate'd
against a fresh in-memory database. That's precise and fast, but it can
only exercise what the person writing the test remembered to reproduce.
The `parseStoredTimestamp`/`storedTimestampOffsetPrefix` fallback in
`db/timeformat.go` exists because of a shape (a `time.Time` still carrying
its monotonic clock reading, serialized as
`2026-09-03 22:30:04.74361174 +0100 BST m=+13.399250358`) that nobody
would have thought to reproduce synthetically — it only turned up by
actually running an old build against a real database. Fixtures here make
that kind of discovery repeatable instead of a one-off manual exercise:
open the real file, run today's `AutoMigrate` + `applyDBMigrations`
against it, and see what breaks.

## v0.0.24.db

Built from the last tagged release before this project's `v1` schema
work began (everything in `db/migrations.go` — the UTC timestamp
normalization and the JA4H/TLS-JA4/stable fingerprint consolidation — was
written after this tag, so this file predates both). Reproduce it like
this:

```sh
git worktree add /tmp/v0-checkout v0.0.24
cd /tmp/v0-checkout
mkdir -p webapp/dist && echo '<html></html>' > webapp/dist/index.html  # satisfy the go:embed, webapp assets aren't needed
go build -tags=purego -o /tmp/tg-v0 .

# A config with the sample routes, an unused port, and rateLimiter.vulnerabilityScan
# configured (max404: 3) — see sample/config.yaml for the shape; point toFile/toFolder
# at /tmp/v0-checkout/sample/webfiles since routes are resolved relative to cwd.
cd /tmp/v0-seed && touch .env   # v0.0.24 hard-fails startup without one, even empty
/tmp/tg-v0 run --config config.yaml &

# Real traffic, generating a spread of status codes and two different
# JA4H fingerprints (different User-Agents):
curl http://127.0.0.1:8199/                                    # 200
curl http://127.0.0.1:8199/static/logo.png                     # 200
curl http://127.0.0.1:8199/secret                               # 307 (auth redirect)
curl http://127.0.0.1:8199/does-not-exist                       # 404
curl -c cookies.txt -X POST http://127.0.0.1:8199/_/auth/basic/login \
  -d "username=admin&password=admin123"                        # 302, real Session row
curl -b cookies.txt http://127.0.0.1:8199/secret -A "different-UA"  # 307, second fingerprint
for i in 1 2 3 4 5; do curl http://127.0.0.1:8199/.env; done    # 404 x4, then 429 (block)

# Then stop the server and copy the resulting ./taronja-gateway.db here.
```

Generated on a host running `Europe/London` (BST, `+01:00`) — deliberately
not UTC, so every stored timestamp genuinely needs
`migrateTimestampsToUTC`'s work, not just a no-op check. Contents at the
time it was captured (`db/migration_v0_fixture_test.go` asserts against
exactly this):

- One admin user (`tg_admin_provider`), one session, 11 `traffic_metrics`
  rows (status codes 200/307/404/302/429... — the 429 itself isn't
  persisted anywhere in v0.0.24, since the blocked-clients registry is
  `v1` work; only the requests *leading up to* the block are).
- Two distinct `ja4_fingerprint` values (one per `User-Agent` used) on both
  `sessions` and `traffic_metrics` — the sole legacy fingerprint column
  that exists at this tag; `ja4_tls_fingerprint`/`stable_fingerprint` were
  added later, so `migrateLegacyFingerprintColumns` backfills from JA4H
  only here.
- No `is_static_asset` column on `traffic_metrics` (added after this tag)
  and no `blocked_clients` table at all (added this session) — both must
  appear after `AutoMigrate`, empty/defaulted, without erroring.
- `PRAGMA user_version` is `0` — this file predates the migration-tracking
  mechanism itself, the same as any other pre-upgrade database.

Never overwrite this file casually — `migration_v0_fixture_test.go` pins
specific IDs, timestamps, and fingerprint values read out of it. If it
ever needs regenerating (e.g. a new legacy column shape needs covering),
regenerate it following the recipe above and update that test's expected
values together, in the same change.
