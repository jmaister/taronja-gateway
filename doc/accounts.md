# Accounts: sign-up, password reset, email login, linking

With an SMTP server configured, the gateway runs user accounts itself, so a
small app does not need a separate identity provider.

```yaml
server:
  url: https://example.com        # needed: the emailed links point here
authenticationProviders:
  basic:
    enabled: true
    signup: true                  # /_/signup
  emailLogin:
    enabled: true                 # "Email me a login link" on /_/login
notification:
  email:
    enabled: true
    host: smtp.example.com
    port: 587
    username: ${SMTP_USERNAME}
    password: ${SMTP_PASSWORD}
    from: noreply@example.com
    fromName: Example
```

`tg validate` refuses `signup` or `emailLogin` without `notification.email`
and `server.url`. Password reset needs no option: it is on whenever password
login, `notification.email` and `server.url` are.

## Pages

| Path | What |
|------|------|
| `/_/login` | Login. Shows "Forgot your password?", "Create one" and the email login form when they are enabled. |
| `/_/signup` | Email + password. Sends a confirmation link. |
| `/_/verify-email?token=` | Confirms an email; for a sign-up, creates the account and logs in. |
| `/_/forgot-password` | Sends a reset link. |
| `/_/reset-password?token=` | Sets a new password and logs in. |
| `/_/auth/email/start` (POST) | Sends a login link and a 6-digit code; shows the code form. |
| `/_/auth/email/verify?token=` | Logs in with the emailed link. |
| `/_/account` | The signed-in user's account page. |

Link to `/_/account` from your app (e.g. a "My account" menu entry) so users
can find it.

## How each flow behaves

**Sign-up.** Submitting the form only sends an email: the account is created
when the confirmation link is used, with the password typed at sign-up. If the
person who owns the address did not sign up, they ignore the email and no
account ever exists, so nobody can squat someone else's email. Signing up with
an email that already has an account shows the same "check your inbox" page and
emails the owner a "you already have an account" notice with login and reset
links, so the form does not reveal who is registered.

**Password reset.** The forgot-password page answers the same for known and
unknown emails. The link works once, for 1 hour; using it confirms the email,
sets the new password, invalidates older reset links and signs out every
session.

**Email login.** The email holds a link and a 6-digit code, both valid once for
15 minutes. The code only works in the browser that asked for it (a cookie
binds them) and locks after 5 wrong tries; the link works in any browser. A
login proves the user owns the email, so it marks it confirmed. With `signup`
on, an unknown email gets a new, passwordless account on its first login;
without it, nothing is sent (the page still looks the same).

**Emailed links.** Opening a link shows a page with a button; the token is only
used when the button is pressed. Mail scanners that open links to check them
therefore cannot use them up.

**Rate limit.** At most one email of each kind per address per minute.

## Linking several logins to one account

Each user can have several external logins (Google, GitHub) in
`user_identities`. On an OAuth login:

1. A login already linked to a user logs in that user, even if its email
   changed since.
2. Otherwise, if a user has the same email, the login is linked automatically
   only when both sides have proven the email: the provider says it is
   verified, and the account's email is confirmed (or the account has no
   password). The admin account from the config file is never linked
   automatically, since its email was never proven. Otherwise the login page
   asks the user to log in their usual way and link the provider from the
   account page.
3. Otherwise a new user is created, if the provider verified the email.

From the account page a signed-in user can link a provider whose email differs
from theirs, and unlink one. Unlinking is refused if it would leave no way to
log in. After an unlink, that provider is only linked again explicitly, never
automatically.

Users created before this feature keep logging in as before: migration
`0004_accounts` copies each Google/GitHub user's provider ID into
`user_identities`.

## Account page

- Email and whether it is confirmed, with a button to send a confirmation email.
- Change the password (or set one, for users who only used OAuth or email
  login). The current password is required if there is one; other sessions are
  signed out.
- Linked accounts, with Unlink and "Link Google/GitHub" buttons.
- Open sessions (browser, OS, IP, location, last activity), each with a revoke
  button, and "Sign out all other sessions".
- Recent activity: logins (with the method), failed password logins, sign-up,
  email confirmations, password changes and resets, linking, revoked sessions.

Forms are protected against CSRF with a token derived from the session.

## Where the code is

- `accounts/` — the flows (`Service`), with no HTTP handlers.
- `gateway/account_routes.go` — the pages; templates are `static/*.html`
  rendered inside `static/auth_layout.html`.
- `providers/providers.go` — OAuth callback (`ResolveExternalLogin`) and the
  link round trip (`Link`, `finishLink`).
- Tables `auth_tokens` (hashed one-time tokens and codes), `user_identities`
  and `auth_events` (migration `0004_accounts`).
- Tests: `accounts/service_test.go`, `providers/providers_test.go`
  (`TestCallbackLinksAccounts`) and the Mailpit end-to-end tests in
  `e2e/accounts_test.go`.
