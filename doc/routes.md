# Routes

Define routing rules for incoming requests. Each route can:

- Proxy requests to backend services
- Serve static files
- Require authentication
- Control caching behavior

**Route Properties:**

- `name`: Human-readable route identifier
- `from`: URL path pattern to match (supports wildcards with `*`)
- `to`: Backend URL to proxy requests to. Accepts either a single URL
  (`to: https://api.example.com`) or a list of URLs
  (`to: [https://api-1.example.com, https://api-2.example.com]`) for load
  balancing — see [Load Balancing](#load-balancing) below
- `toFile`: Serve a single static file
- `toFolder`: Serve files from a directory
- A route sets exactly one of `to`, `toFile` and `toFolder`.
- `removeFromPath`: Prefix removed before forwarding (proxy routes only). See [removeFromPath](#removefrompath).
- `authenticated: true`: Require a logged-in user for this route
- `options.cacheControlSeconds`: Cache duration in seconds (0 = no-cache)

**Example Routes:**

```yaml
routes:
  # Serve a single file
  - name: Favicon
    from: /favicon.ico
    toFile: ./sample/webfiles/favicon.ico

  # Public API - no authentication required
  - name: Public API v1
    from: /api/v1/*
    removeFromPath: "/api/v1/"
    to: https://jsonplaceholder.typicode.com
    options:
      cacheControlSeconds: 300  # Cache for 5 minutes

  # Authenticated API route
  - name: Private API v2
    from: /api/v2/*
    removeFromPath: "/api/v2/"
    to: https://api.example.com
    authenticated: true
    options:
      cacheControlSeconds: 0  # No cache

  # Static files folder - public
  - name: CSS and JavaScript
    from: /assets/*
    toFolder: ./static/assets
    options:
      cacheControlSeconds: 604800  # Cache for 1 week

  # Another static folder - requires authentication
  - name: Protected Documents
    from: /documents/*
    toFolder: ./static/private-docs
    authenticated: true
    options:
      cacheControlSeconds: 3600  # Cache for 1 hour

  # Frontend application - no authentication
  - name: Public Frontend
    from: /
    toFolder: ./static/public
    options:
      cacheControlSeconds: 86400  # Cache for 1 day

  # Admin panel - requires authentication
  - name: Admin Dashboard
    from: /admin/*
    toFolder: ./static/admin
    authenticated: true
    options:
      cacheControlSeconds: 0  # No cache for dashboard
```

# removeFromPath

Strips a prefix from the request path before it is proxied, so a backend
mounted at `/` can be published under a sub-path.

```yaml
routes:
  - name: Users API
    from: /api/*
    removeFromPath: /api
    to: http://users:8080

  - name: Billing API (backend lives under /v2)
    from: /billing/*
    removeFromPath: /billing/
    to: http://billing:9000/v2
```

| Request | Route | Backend receives |
|---|---|---|
| `/api/users` | Users API | `/users` |
| `/api/users?page=2` | Users API | `/users?page=2` |
| `/api/` | Users API | `/` |
| `/api/a%20b` | Users API | `/a%20b` |
| `/billing/invoices/7` | Billing API | `/v2/invoices/7` |
| `/billing/` | Billing API | `/v2/` |

Without `removeFromPath` the full path is forwarded: `/billing/invoices/7`
would reach `http://billing:9000/v2/billing/invoices/7`. A bare `/api` never
reaches the backend: a `from: /api/*` route answers it with a `307` redirect
to `/api/`.

Caveats:

- **Proxy routes only.** A route with `toFolder` or `toFile` and
  `removeFromPath` fails validation.
- **Whole path segments.** `/api` and `/api/` are the same and strip `/api`
  and `/api/...`, never the start of `/apiary`.
- **Paths without the prefix pass unchanged.** Keep `removeFromPath` equal to
  the start of `from`; otherwise nothing is stripped.
- **The `to` path is added after stripping**, exactly once. A trailing slash
  on `to` makes no difference.
- **Responses are not rewritten.** If the backend redirects to `/users/x`,
  the browser goes to `/users/x`, not `/api/users/x`; links in the body are
  passed as-is too, and no `X-Forwarded-Prefix` header is sent. The backend
  must build public URLs itself, or use relative links.
- **Load balancing** (`to` as a list) takes the path from the first URL, so
  all targets must share it.

# Load Balancing

Give `to` a list instead of a single URL to spread requests across multiple
backend instances:

```yaml
- name: API (load balanced)
  from: /api/*
  to:
    - http://api-1.internal:8080
    - http://api-2.internal:8080
    - http://api-3.internal:8080
```

Requests are distributed round-robin across the list. If a backend's
connection attempt fails outright (refused, DNS failure, timeout), the
gateway automatically retries the same request against the next backend in
the list before giving up — a request only fails with `502 Bad Gateway` if
every listed backend is unreachable. This failover only reacts to
connection-level failures, never to a backend's response status code: a
backend returning its own `500` is a real answer, not treated as "down."

The listed URLs are expected to be interchangeable replicas of the same
backend — same path structure, differing only in scheme/host. Use separate
route entries (different `from:` patterns) to send different paths to
different places; that's routing, not load balancing.

A single URL (`to: http://backend:8080`) continues to work exactly as
before — this is purely additive.
