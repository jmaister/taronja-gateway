# Accounts: one user per person across logins

Users log in with basic username/password or with an OAuth/OIDC provider
(Google, GitHub, any OpenID Connect provider). The gateway keeps one user per
person: each external login is stored in `user_identities` (provider + the
provider's stable subject), and every login goes through
`accounts.Service.ResolveExternalLogin`.

## How an external login is resolved

1. A login already linked to a user logs in that user, even if its email
   changed since.
2. Otherwise, if a user has the same email, the login is linked automatically
   only when both sides have proven the email: the provider says it is
   verified, and the account's email is confirmed (or the account has no
   password). The admin account from the config file is never linked
   automatically, since its email was never proven. Otherwise the login page
   shows an error (`?error=link_required`) and the user logs in their usual way.
3. Otherwise a new user is created, if the provider verified the email
   (`?error=email_not_verified` if not).

Users created before this feature keep logging in as before: migration
`0004_accounts` copies each Google/GitHub user's provider ID into
`user_identities`.

## Activity log

`auth_events` records logins (with the method), failed password logins,
account creation and automatic linking, with the client IP and user agent.
There is no page for it yet.

## Account page

`/_/account` shows the signed-in user's email and, when billing is enabled, their
subscriptions and documents with a button to the Stripe customer portal (see
[billing.md](billing.md)). Forms are protected against CSRF with a token derived
from the session. There is no password change, session list or manual linking.

## Where the code is

- `accounts/` — the flows (`Service`), with no HTTP handlers.
- `providers/providers.go` — the OAuth/OIDC callback calls `ResolveExternalLogin`.
- Tables `user_identities` and `auth_events` (migration `0004_accounts`).
- Tests: `accounts/service_test.go`, `providers/providers_test.go`
  (`TestCallbackResolvesAccounts`) and `e2e/zitadel_test.go`.
