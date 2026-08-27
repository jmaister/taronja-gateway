# Docker demo

A self-contained `docker compose` setup with everything running: the
gateway, a proxied backend service, a static site, an authenticated route,
and the built-in admin dashboard — one example of each route type, plus a
few other features worth trying once it's up.

## Run it

```bash
cd examples/docker-demo
cp .env.sample .env   # optional — see "Configure Google authentication" below
docker compose up --build
```

Then open **http://localhost:8080**.

The first `--build` compiles the gateway from the repo root (`../../Dockerfile`
— the React admin dashboard, then the Go binary); it's the slow step, and
only needs repeating after you change gateway source, not after editing
`config/config.yaml` or the static files (see "Try hot config reload" below).

## What's running

| Service | What it is | Why it's here |
|---|---|---|
| `gateway` | Taronja Gateway, built from this repo | the thing being demoed |
| `web-service` | [`traefik/whoami`](https://github.com/traefik/whoami), a ~5MB server that echoes back the request it received | target of the proxy route — lets you actually see what the gateway forwards |

## Routes

| Path | Type | Config |
|---|---|---|
| `/`, `/second_page.html`, `/images/logo.png` | **static** | `toFolder: /srv/static-site`, `static: true` |
| `/secret/` | **static, authenticated** | same as above plus `authentication.enabled: true` |
| `/web/*` | **proxy** | `to: http://web-service:80` — a Compose service name, resolved by Docker's internal DNS, not `localhost` |
| `/_/admin/` | admin dashboard | built in; log in with `admin` / `admin123` |

All of it is wired up through the declarative `middleware:` chain (`cors`,
`rate_limiter`, `ja4_fingerprint`, `session_extraction`, `traffic_metrics`,
`logging`) in `config/config.yaml`, rather than the legacy boolean flags — see the
main [README's Middleware Architecture section](../../README.md#middleware-architecture).

## Configure Google authentication

1. Google Cloud Console → [Credentials](https://console.cloud.google.com/apis/credentials)
   → **+ Create Credentials** → **OAuth client ID** → Application type **Web application**.
2. Authorized redirect URI (must match exactly):
   ```
   http://localhost:8080/_/auth/google/callback
   ```
3. Copy the client ID and secret into `.env` (see `.env.sample`):
   ```
   GOOGLE_CLIENT_ID=...
   GOOGLE_CLIENT_SECRET=...
   ```
4. `docker compose up --build` (or, if it's already running, just
   `docker compose restart gateway` — hot reload picks up `.env` changes
   too, see below, but the container needs to actually re-read its
   environment, which only happens on a restart, not a config reload).
5. Visit any authenticated route (`/secret/`, `/_/admin/`) — a "Sign in
   with Google" button now appears on the login page.

GitHub works the same way (`GITHUB_CLIENT_ID`/`GITHUB_CLIENT_SECRET`,
callback `http://localhost:8080/_/auth/github/callback`), if you want both.

Neither is required — Basic auth (`admin`/`admin123`) always works, and the
rest of the demo doesn't depend on OAuth being configured.

## Other features worth trying

**Hot config reload** — edit `config/config.yaml` on the host (e.g. change
`cacheControlSeconds`, add a route, or change `requestsPerMinute`) and save
it; the running container picks it up without a restart (`--watch` is on by
default) — verified working here even with editors/tools (`sed -i`
included) that save by writing a new file and renaming it over the old one,
which is why this mounts the whole `config/` directory rather than just the
one file (a single-file bind mount doesn't survive that rename; a directory
one does). Watch it happen: `docker compose logs -f gateway`.

On Docker Desktop (macOS/Windows) specifically, the virtualized filesystem
bind mounts go through can still occasionally miss a change notification —
if a save doesn't seem to trigger a reload, this always works instead,
everywhere:
```bash
docker compose kill -s HUP gateway
```

**Rate limiter** — `config/config.yaml` sets a deliberately low
`requestsPerMinute: 60` on the whole gateway. Trigger it:
```bash
for i in $(seq 1 90); do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/web/; done
```
You'll see `200`s turn into `429`s partway through. Check current state from
the admin dashboard's Rate Limiter page (`/_/admin/`, log in first — its API
requires a session, unlike the plain `curl` above).

**CORS** — the proxy route allows any origin (`allowedOrigins: ["*"]`).
Inspect a preflight response:
```bash
curl -si -X OPTIONS http://localhost:8080/web/ \
  -H "Origin: https://example.com" \
  -H "Access-Control-Request-Method: GET" | grep -i access-control
```

**Traffic analytics** — every request through the middleware chain above is
recorded (`traffic_metrics`). Click around the site, then check
`/_/admin/` → Analytics for request counts, response times, and (per
`session_extraction`/`ja4_fingerprint`) client fingerprints.

**Cache-Control** — `curl -sI http://localhost:8080/` and look for
`Cache-Control: max-age=3600` — set via the static route's
`options.cacheControlSeconds`, not a middleware.

**Validate config without running anything** — the same checks `tg run`
applies at startup, without starting a server:
```bash
docker compose exec gateway tg validate --config /etc/taronja-gateway/config.yaml
docker compose exec gateway tg middleware list --config /etc/taronja-gateway/config.yaml
```

## Cleaning up

```bash
docker compose down        # stop, keep the sqlite DB (admin user, sessions) in its volume
docker compose down -v     # stop and delete it too — next `up` starts fresh
```
