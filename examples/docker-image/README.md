# Docker image example

The `docker-demo` example builds the gateway from source (`../../Dockerfile`)
on every `docker compose up --build`. This one instead pulls the
already-built [`ghcr.io/jmaister/taronja-gateway`](https://github.com/jmaister/taronja-gateway/pkgs/container/taronja-gateway)
image published on every [GitHub Release](https://github.com/jmaister/taronja-gateway/releases)
— the way you'd actually deploy it on a platform that runs containers for
you (Dokploy, Coolify, a Kubernetes cluster, plain `docker run` on a VPS,
...), pointing its one proxy route at a second service by its
Compose-internal address (`http://web-service:80`), the same mechanism
that reaches any other service on whatever platform's internal network
you deploy this alongside.

## Run it

```bash
cd examples/docker-image
docker compose up
```

Then open **http://localhost:18199**.

No `--build` needed — `docker compose up` pulls the image the first time
and reuses it after that. `docker compose pull` fetches a newer `:latest`
explicitly.

## What's running

| Service | What it is | Why it's here |
|---|---|---|
| `gateway` | `ghcr.io/jmaister/taronja-gateway:latest`, pulled, not built | the thing being demoed |
| `web-service` | [`traefik/whoami`](https://github.com/traefik/whoami) | stands in for "one of your other services" — target of the proxy route |

## Routes

| Path | Type | Config |
|---|---|---|
| `/` | **static** | `toFolder: /srv/static-site`, `static: true` |
| `/web/*` | **proxy** | `to: http://web-service:80` — a Compose service name, resolved by Docker's internal DNS |
| `/_/admin/` | admin dashboard | built in; log in with `admin` / `admin123` |

## Run it against a locally built image

Testing a change to the `Dockerfile` (or anything it embeds) before it's
actually released? Build and tag it under the exact name this compose
file references, from the repo root:

```bash
cd ../..
docker build -t ghcr.io/jmaister/taronja-gateway:latest .
cd examples/docker-image
docker compose up
```

Docker only pulls a tag it doesn't already have locally, so this runs
your local build instead of fetching the real published image — nothing
in `docker-compose.yml` needs to change either way. `docker compose pull`
would overwrite the local tag with the real one again, so don't run that
in between.

## Cleaning up

```bash
docker compose down        # stop, keep the sqlite DB (admin user, sessions) in its volume
docker compose down -v     # stop and delete it too — next `up` starts fresh
```
