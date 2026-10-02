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
must match `<server.url><management.prefix>/auth/<provider>/callback`
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

Microsoft (Entra ID / Azure AD), Facebook, and Apple ("Sign in with
Apple") setup instructions moved to the `wip/microsoft-apple-facebook-auth`
branch along with their implementation — untested, coming back to this
later (see the Features table's footnote above).
