# Authentication Providers

Configure authentication methods for your gateway.

**Basic Authentication:**
```yaml
authenticationProviders:
  basic:
    enabled: true
```

**OAuth2 Providers:** each one below is independent and optional — enable
as many side by side as you like (the login page shows a button for each
configured one). The redirect/callback URL you register with the provider
must match `<server.url>/_/auth/<provider>/callback`
exactly (scheme, host, port, and path) — the samples below assume the
defaults (`http://localhost:8080`, prefix `/_`).

## Google

Get credentials: [Google Cloud Console → APIs & Services → Credentials](https://console.cloud.google.com/apis/credentials) → **Create Credentials → OAuth client ID** (application type "Web application").

- **Credentials needed:** Client ID, Client Secret
- **Authorized JavaScript origin:** `http://localhost:8080`
- **Authorized redirect URI:** `http://localhost:8080/_/auth/google/callback`

```yaml
authenticationProviders:
  google:
    clientId: ${GOOGLE_CLIENT_ID}
    clientSecret: ${GOOGLE_CLIENT_SECRET}
```

## GitHub

Get credentials: [GitHub → Settings → Developer settings → OAuth Apps](https://github.com/settings/developers) → **New OAuth App**.

- **Credentials needed:** Client ID, Client Secret
- **Homepage URL:** `http://localhost:8080`
- **Authorization callback URL:** `http://localhost:8080/_/auth/github/callback`

```yaml
authenticationProviders:
  github:
    clientId: ${GITHUB_CLIENT_ID}
    clientSecret: ${GITHUB_CLIENT_SECRET}
```

## Any OpenID Connect provider (Zitadel, Keycloak, Authentik, Authelia, Pocket ID, ...)

List one entry per provider under `oidc`. Only the issuer and the client
credentials are needed: the endpoints come from
`<issuer>/.well-known/openid-configuration`.

```yaml
authenticationProviders:
  oidc:
    - name: zitadel                  # URL id: /_/auth/zitadel/login and /callback
      label: Zitadel                 # button: "Login with Zitadel" (default: name)
      issuer: https://auth.example.com
      clientId: ${ZITADEL_CLIENT_ID}
      clientSecret: ${ZITADEL_CLIENT_SECRET}   # empty for a public (PKCE) client
      # scopes: [openid, email, profile]       # the default
      # trustEmail: false                      # see below
      # signup: false                          # see "Self-registration"
      # logoutAtProvider: false                # see "Already logged in"
```

At the provider, create a web application with the authorization code flow
and register the redirect URI `<server.url>/_/auth/<name>/callback`, e.g.
`http://localhost:8080/_/auth/zitadel/callback`. In Zitadel: a project, then
an app of type **Web** with authentication method **Basic** (with a secret)
or **PKCE** (without one); enable **Development Mode** to allow an `http://`
redirect URI while testing locally.

How it works:

- **PKCE (S256) is always used**, with or without a client secret.
- **Discovery happens on the first login**, not at startup, and is retried
  until it succeeds: the gateway starts and serves everything else while the
  provider is down, and its login answers 503 meanwhile. The discovery
  document must name the configured issuer.
- **The user comes from the userinfo endpoint**: `sub` (the stable ID),
  `email`, `email_verified`, `preferred_username`, `name`, `given_name`,
  `family_name`, `picture`, `locale`.
- **The email must be verified** (`email_verified: true`, or the string
  `"true"`). For a provider that never sends the claim, `trustEmail: true`
  treats its emails as verified — only for a provider you control, since its
  users could then claim any address.
- Logins are matched by `sub`, and same-email accounts are linked as
  described in [accounts.md](accounts.md#how-an-external-login-is-resolved).
- `name` must be lowercase letters, digits or `-`, and not `basic`, `google`,
  or `github`.

### Self-registration

The gateway has no sign-up form: let the provider do it. With `signup: true`
the login page shows a "Crear cuenta" link, and
`/_/auth/<name>/login?signup=1&redirect=/start` sends the authorization
request with `prompt=create`, which opens the provider's registration form
(without `signup: true` the parameter is ignored). The `redirect` is kept as
in a normal login. In Zitadel, enable **Register allowed** in the
organization's *Login Behavior* settings and require verified emails
(*Login Behavior → Verify email*), so the gateway sees `email_verified: true`
and only links accounts of verified emails.

The end-to-end tests run this against a real Zitadel (`e2e/zitadel_test.go`).

## Already logged in

`/_/login` redirects on the server to `?redirect=` (or `/`) when the request
already has a valid gateway session, so a logged-in user never sees the login
page. One gateway session covers every route and app behind the same host, and it
renews itself while used: only `management.session.durationSeconds` of
inactivity (15 days by default) logs the user out.
The login page itself never redirects to an external provider on its own:
users always choose how to log in. By default logout ends the gateway session
only; the session at the provider stays, so the next login may not ask for a
password. Set `logoutAtProvider: true` on an OIDC provider to also end the
session there: `/_/logout` then redirects the browser to the provider's
`end_session_endpoint` with `client_id` and `post_logout_redirect_uri`
(`<server.url>` plus the `?redirect=` path, default `/`). Register that URI at the
provider next to the callback URI (for Zitadel: `<server.url>/` under post
logout redirect URIs). If the provider cannot be reached or has no
`end_session_endpoint`, logout falls back to the normal redirect. The browser
must navigate to `/_/logout` (not `fetch` it) for the redirect to work.

Microsoft (Entra ID / Azure AD), Facebook, and Apple ("Sign in with
Apple") setup instructions moved to the `wip/microsoft-apple-facebook-auth`
branch along with their implementation — untested, coming back to this
later (see the Features table's footnote above).
