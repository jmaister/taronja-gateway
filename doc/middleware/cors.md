# cors

Adds CORS headers for allowed origins and answers preflight `OPTIONS`
requests with `204`. Applies to every route.

```yaml
middleware:
  - name: cors
    config:
      allowedOrigins: ["https://app.example.com"]
      allowCredentials: true
```

| Option | Default | Description |
|---|---|---|
| `allowedOrigins` | `[]` | Exact origins. `"*"` allows any, but not with `allowCredentials`. Empty adds no headers. |
| `allowCredentials` | `false` | Sends `Access-Control-Allow-Credentials: true`. |
| `allowedMethods` | `GET, POST, PUT, PATCH, DELETE, OPTIONS` | Methods in preflight responses. |
| `allowedHeaders` | `Content-Type, Authorization` | Headers in preflight responses. |
| `maxAgeSeconds` | `600` | Preflight cache time. |

**Depends on:** nothing. Place it before `rate_limiter` so preflights are not
counted.

Requests from other origins pass through without CORS headers (browsers
enforce CORS). With an explicit origin list or credentials, the request's
origin is echoed back with `Vary: Origin`.
