# Backend Integration

How backends and frontends behind the gateway identify the logged-in user, and how to log users in and out.

## Authentication on the APIs

When a request is proxied to a backend route that has `authentication.enabled: true`, Taronja Gateway injects HTTP headers into the request so the backend service can identify the authenticated user. These headers are only set when a valid session exists.

### Headers Sent to Backend Routes

#### Standard Proxy Headers

Every proxied request (authenticated or not) includes the following standard headers:

| Header              | Type     | Description                                                    |
|---------------------|----------|----------------------------------------------------------------|
| `X-Forwarded-Host`  | `string` | The original `Host` header from the client request.            |
| `X-Forwarded-Proto` | `string` | The protocol the client's connection to this gateway actually used — `https` only when TLS terminated here or a trusted upstream proxy said so, `http` otherwise. |
| `X-Forwarded-For`   | `string` | The client's real IP address, resolved the same way `X-Real-IP`/`X-Client-IP` are — trusted only from a loopback/private-range peer, never taken from a direct client at face value. |

**`X-Forwarded-Host` is not verified.** Unlike `X-Forwarded-Proto`/`X-Forwarded-For` above, this gateway forwards the client's `Host` header exactly as received, with no check that it matches anything this gateway is actually configured to serve — any direct client can set it to whatever they want. If a backend route builds an absolute URL from this header (a password-reset link, an OAuth redirect, a cache key), that URL is only as trustworthy as the client who sent the request — treat it the same way you'd treat any other unauthenticated, client-supplied input, not as something this gateway already validated for you.

#### Authentication Headers

These headers are added only on routes with `authentication.enabled: true` and when the user has a valid session:

| Header        | Type     | Description                                                                 |
|---------------|----------|-----------------------------------------------------------------------------|
| `X-User-Id`   | `string` | The unique user ID (CUID) of the authenticated user.                        |
| `X-User-Data` | `string` | A JSON-serialized object containing the full session data (see structure below). |

### `X-User-Data` JSON Structure

The `X-User-Data` header contains a JSON-encoded session object with the following fields:

```json
{
  "token": "string",
  "userId": "string",
  "username": "string",
  "email": "string",
  "isAuthenticated": true,
  "isAdmin": false,
  "validUntil": "2026-02-28T12:00:00Z",
  "provider": "string",
  "closedOn": null,
  "lastActivity": "2026-02-27T10:30:00Z",
  "sessionName": "string",
  "createdFrom": "string",
  "ipAddress": "string",
  "userAgent": "string",
  "referrer": "string",
  "browserFamily": "string",
  "browserVersion": "string",
  "osFamily": "string",
  "osVersion": "string",
  "deviceFamily": "string",
  "deviceBrand": "string",
  "deviceModel": "string",
  "geoLocation": "string",
  "latitude": 0.0,
  "longitude": 0.0,
  "city": "string",
  "zipCode": "string",
  "country": "string",
  "countryCode": "string",
  "region": "string",
  "continent": "string",
  "fingerprint": "string",
  "fingerprintType": "string"
}
```

#### Field Reference

| Field              | Type      | Description                                                      |
|--------------------|-----------|------------------------------------------------------------------|
| `token`            | `string`  | The session token identifier.                                    |
| `userId`           | `string`  | Unique user ID (CUID format).                                    |
| `username`         | `string`  | Username of the authenticated user.                              |
| `email`            | `string`  | Email address of the user.                                       |
| `isAuthenticated`  | `bool`    | Whether the session is authenticated.                            |
| `isAdmin`          | `bool`    | Whether the user has admin privileges.                           |
| `validUntil`       | `string`  | Session expiration timestamp (RFC 3339 / ISO 8601).              |
| `provider`         | `string`  | Authentication provider used (`basic`, `google`, `github`, etc). |
| `closedOn`         | `string?` | Timestamp when the session was closed, or `null` if active.      |
| `lastActivity`     | `string`  | Timestamp of the last user activity in this session.             |
| `sessionName`      | `string`  | Optional name assigned to the session.                           |
| `createdFrom`      | `string`  | How the session was created (e.g. `cookie`, `token`).            |
| `ipAddress`        | `string`  | Client IP address.                                               |
| `userAgent`        | `string`  | Client's User-Agent string.                                      |
| `referrer`         | `string`  | HTTP referrer.                                                   |
| `browserFamily`    | `string`  | Browser name (e.g. `Chrome`, `Firefox`).                         |
| `browserVersion`   | `string`  | Browser version string.                                          |
| `osFamily`         | `string`  | Operating system name.                                           |
| `osVersion`        | `string`  | Operating system version.                                        |
| `deviceFamily`     | `string`  | Device type (e.g. `desktop`, `mobile`).                          |
| `deviceBrand`      | `string`  | Device manufacturer.                                             |
| `deviceModel`      | `string`  | Device model name.                                               |
| `geoLocation`      | `string`  | General geolocation description.                                 |
| `latitude`         | `float`   | GPS latitude coordinate.                                         |
| `longitude`        | `float`   | GPS longitude coordinate.                                        |
| `city`             | `string`  | City name from geolocation.                                      |
| `zipCode`          | `string`  | Postal / ZIP code.                                               |
| `country`          | `string`  | Country name.                                                    |
| `countryCode`      | `string`  | ISO country code (2-3 characters).                               |
| `region`           | `string`  | State, province, or region.                                      |
| `continent`        | `string`  | Continent name.                                                  |
| `fingerprint`      | `string`  | The client's fingerprint value — see `fingerprintType` for which algorithm produced it. Whichever of the three available signals is most reliable wins; see [doc/middleware/ja4-fingerprint.md](middleware/ja4-fingerprint.md#three-signals-one-header) for the full priority order and why. |
| `fingerprintType`  | `string`  | Which algorithm produced `fingerprint`: `ja4_tls` (TLS-level JA4 — most stable, only possible when `server.tls.enabled`), `stable` (reduced-entropy header-based fingerprint), or `ja4h` (HTTP-header JA4H — the noisiest of the three). Empty string if `fingerprint` is empty too. |

### Authentication Methods

Backend routes can receive authenticated requests via two methods:

1. **Session cookie** — The user logs in through the gateway (Basic auth or OAuth2), and a `tg_session_token` cookie is set. The gateway validates the cookie on each request and injects the headers above.

2. **Bearer token** — API clients can authenticate using a token in the `Authorization` header:
   ```
   Authorization: Bearer <token>
   ```
   The gateway validates the token, creates a session-like object, and injects the same `X-User-Id` and `X-User-Data` headers.

### Example: Reading Headers in a Backend Service

**Node.js / Express:**
```js
app.get('/api/resource', (req, res) => {
    const userId = req.headers['x-user-id'];
    const userData = JSON.parse(req.headers['x-user-data']);
  console.log(`User: ${userData.username} (${userId})`);
  res.json({ message: `Hello, ${userData.username}` });
});
```

**Go:**
```go
func handler(w http.ResponseWriter, r *http.Request) {
    userId := r.Header.Get("X-User-Id")
    userDataJson := r.Header.Get("X-User-Data")
    // Parse userDataJson as needed
    fmt.Fprintf(w, "User ID: %s", userId)
}
```

**Python / Flask:**
```python
@app.route('/api/resource')
def resource():
    user_id = request.headers.get('X-User-Id')
    user_data = json.loads(request.headers.get('X-User-Data', '{}'))
    return jsonify(message=f"Hello, {user_data.get('Username')}")
```

### Getting the Current User from the Frontend

Web applications served through the gateway can call the `/_/me` endpoint to retrieve information about the currently logged-in user. The endpoint uses the session cookie (`tg_session_token`) that the browser sends automatically.

**Endpoint:** `GET /_/me`

- Returns `200` with user data if the user is authenticated.
- Returns `401` if no valid session exists.

**Response (200):**

```json
{
  "authenticated": true,
  "username": "testuser",
  "email": "user@example.com",
  "name": "Test User",
  "picture": "https://example.com/picture.jpg",
  "givenName": "Test",
  "familyName": "User",
  "provider": "google",
  "isAdmin": false,
  "timestamp": "2026-02-27T12:00:00Z"
}
```

| Field           | Type      | Nullable | Description                                              |
|-----------------|-----------|----------|----------------------------------------------------------|
| `authenticated` | `bool`    | No       | Always `true` when the response is 200.                  |
| `username`      | `string`  | No       | Username of the authenticated user.                      |
| `email`         | `string`  | Yes      | Email address (format: email).                           |
| `name`          | `string`  | Yes      | Full display name.                                       |
| `picture`       | `string`  | Yes      | URL to the user's profile picture.                       |
| `givenName`     | `string`  | Yes      | First name.                                              |
| `familyName`    | `string`  | Yes      | Last name.                                               |
| `provider`      | `string`  | No       | Authentication provider (`basic`, `google`, `github`).   |
| `isAdmin`       | `bool`    | No       | Whether the user has admin privileges.                   |
| `timestamp`     | `string`  | No       | Server timestamp (RFC 3339 / ISO 8601).                  |

**Example: Fetching the current user from JavaScript:**

```js
const response = await fetch('/_/me', { credentials: 'include' });
if (response.ok) {
    const user = await response.json();
    console.log(`Logged in as ${user.username}`);
} else {
    console.log('Not authenticated');
}
```

### Login and Logout Links from a Web Page

You can add direct login/logout links in your frontend pages.

By default, the management prefix is `_`, so authentication URLs are under `/_/`.

#### Login Links

Use the login page endpoint:

- `/_/login`

This page automatically shows all configured login options (Basic, Google, GitHub, etc.).

Optional redirect after login:

- `/_/login?redirect=/dashboard`

#### Logout Link

- `/_/logout`

Optional redirect after logout:

- `/_/logout?redirect=/`
- `/_/logout?redirect=/goodbye`

#### HTML Example

```html
<a href="/_/login?redirect=/dashboard">Login</a>
<a href="/_/logout?redirect=/">Logout</a>
```

#### JavaScript Example

```js
function login() {
  window.location.href = '/_/login?redirect=/dashboard';
}

function logout() {
  window.location.href = '/_/logout?redirect=/';
}
```
