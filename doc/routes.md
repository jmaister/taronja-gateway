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
  authentication:
    enabled: false
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
