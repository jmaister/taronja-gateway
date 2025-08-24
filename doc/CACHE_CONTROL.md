# Cache Control Configuration

The Taronja Gateway now supports configuring HTTP cache control headers for routes through the `options` field in route configuration.

## Configuration

Add an `options` section to any route in your configuration file:

```yaml
routes:
  - name: Static Assets
    from: /assets/*
    toFolder: ./static/assets
    static: true
    options:
      cacheControlSeconds: 86400  # Cache for 1 day
  
  - name: API Endpoint
    from: /api/data
    to: http://backend.example.com/data
    options:
      cacheControlSeconds: 0  # Explicitly disable caching
  
  - name: Default Route
    from: /
    toFolder: ./public
    static: true
    # No options = no cache control header
```

## Cache Control Values

The `cacheControlSeconds` field supports the following values:

- **`null` or not specified**: No Cache-Control header will be set
- **`0`**: Sets `Cache-Control: no-cache` header
- **Positive integer**: Sets `Cache-Control: max-age=<seconds>` header
- **Negative integer**: No Cache-Control header will be set (same as null)

## Examples

### Static Files with Long Cache
```yaml
- name: Images
  from: /images/*
  toFolder: ./static/images
  static: true
  options:
    cacheControlSeconds: 604800  # 1 week (7 * 24 * 60 * 60)
```

### API with Short Cache
```yaml
- name: User Data API
  from: /api/user/*
  to: http://api.example.com/user
  options:
    cacheControlSeconds: 300  # 5 minutes
```

### No Cache for Dynamic Content
```yaml
- name: Authentication Check
  from: /api/auth/check
  to: http://auth.example.com/check
  options:
    cacheControlSeconds: 0  # Always fresh
```

### No Cache Header (Default Behavior)
```yaml
- name: Proxy
  from: /proxy/*
  to: http://backend.example.com
  # No options means browser/proxy default behavior
```

## Common Cache Durations

| Duration | Seconds | Use Case |
|----------|---------|----------|
| 5 minutes | 300 | Frequently changing API data |
| 1 hour | 3600 | Semi-static content |
| 1 day | 86400 | Static assets (CSS, JS) |
| 1 week | 604800 | Images, fonts |
| 1 month | 2592000 | Rarely changing assets |
| 1 year | 31536000 | Immutable assets with versioning |

## Future Extensions

The `options` field is designed to be extensible. Future versions may include additional options such as:
- Custom cache headers
- CORS configuration
- Rate limiting settings
- Compression options
