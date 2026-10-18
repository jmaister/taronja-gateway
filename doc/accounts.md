# Accounts: linking, password, sessions, activity

Users log in with OAuth/OIDC providers (Google, GitHub, any OpenID Connect
provider) or with basic username/password. The gateway keeps one user per
person, however many logins they use, and gives them an account page.

## Pages

| Path | What |
|------|------|
| `/_/login` | Login with the enabled methods. |
| `/_/account` | The signed-in user's account page. |

Link to `/_/account` from your app (e.g. a "My account" menu entry) so users
can find it.

## Linking several logins to one account

Each user can have several external logins (Google, GitHub, OIDC) in
`user_identities`. On an OAuth/OIDC login:

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

- Email and whether it is confirmed.
- Change the password (or set one, for users who only used OAuth). The current
  password is required if there is one; other sessions are signed out.
- Linked accounts, with Unlink and "Link ..." buttons.
- Open sessions (browser, OS, IP, location, last activity), each with a revoke
  button, and "Sign out all other sessions". Sessions renew while used and
  end after `management.session.durationSeconds` (15 days by default)
  without use, or when revoked here, or on a password change.
- Recent activity: logins (with the method), failed password logins, account
  creation, password changes, linking, revoked sessions.

Forms are protected against CSRF with a token derived from the session.

## Where the code is

- `accounts/` — the flows (`Service`), with no HTTP handlers.
- `gateway/account_routes.go` — the pages; templates are `static/*.html`
  rendered inside `static/auth_layout.html`.
- `providers/providers.go` — OAuth callback (`ResolveExternalLogin`) and the
  link round trip (`Link`, `finishLink`).
- Tables `user_identities` and `auth_events` (migration `0004_accounts`).
- Tests: `accounts/service_test.go`, `providers/providers_test.go`
  (`TestCallbackLinksAccounts`) and the end-to-end tests in
  `e2e/accounts_test.go` and `e2e/zitadel_test.go`.
