# Notifications

Taronja Gateway can store and deliver notifications on behalf of the app(s)
it sits in front of, so each app doesn't have to build its own in-app
notification list, its own SMTP integration, and its own Telegram bot: the
gateway already owns user identity, sessions, and (optionally) SMTP/Telegram
credentials, so it's a natural place for delivery mechanics to live once.

**What triggers a notification and what it says is entirely the calling
app's decision.** The gateway never interprets `type` or `metadata` — they're
opaque, caller-defined values, stored and returned as-is. The gateway's job
is: store it, decide who to deliver it to and how, and let the user answer
it from wherever they saw it.

## What it does

- **Always stored in-app**, regardless of configuration — a notification
  exists and is listable/readable via the API the moment it's created, with
  no external delivery required.
- **Optionally delivered over one or more external channels** — today,
  email (via SMTP) and Telegram (via a bot). Delivery is best-effort and
  independent per channel: a failed or skipped email doesn't affect a
  Telegram delivery for the same notification, and neither affects the
  in-app record.
- **Failed deliveries retry automatically**, with backoff, for a handful of
  attempts before giving up — see "Retries" below. Every attempt (not just
  the latest) is kept, so `GET /api/notifications/{id}/deliveries` shows
  the complete history of what was tried, when, and with what result.
- **Answerable from any channel** — a notification can carry a small set of
  actions (e.g. "Approve" / "Deny"), rendered as clickable links in an email
  and as inline buttons in Telegram, in addition to being answerable from
  the in-app list. Whichever answer arrives first wins; a second attempt
  (a double-tapped button, a stale email link clicked twice) is rejected,
  not silently overwritten.
- **Channel-agnostic by design** — delivery is a small `notification.Provider`
  interface (`Channel() string`, `Send(ctx, req) (externalRef string, err error)`),
  the same shape as this codebase's OAuth2 `AuthenticationProvider`
  architecture (`providers/providers.go`): one shared service, several
  independent implementations, one registry. Adding a channel (WhatsApp,
  Slack, SMS, ...) means writing one `Provider` and registering it in
  `notification.NewService` — no change to the data model or the API.

## Data model

- `Notification` — one row per notification: `UserID`, `Type`, `Title`,
  `Body`, optional `URL`, caller-opaque `Metadata` and `Actions` (both
  stored as JSON, only interpreted enough to validate a response names a
  real action), `CreatedAt`, `ReadAt`, and — once answered —
  `RespondedActionID`/`RespondedAt`/`RespondedVia` (`"web"`, `"email"`, or
  `"telegram"`).
- `NotificationDelivery` — one row per delivery *attempt* (not per channel —
  a channel that's retried gets a new row each time, `AttemptNumber` one
  higher than the last), recording whether it was sent, failed, or skipped
  (and why). This, not `Notification` itself, is what "was this actually
  emailed?" answers, since one notification can be requested on channels
  the user has no recipient for. `NextRetryAt` is set on a failed attempt
  that hasn't yet exhausted the retry schedule; see "Retries" below.
- `NotificationChannelLink` — connects a gateway user to an external
  channel identity (today: a Telegram chat ID). Email needs no equivalent
  row — the gateway already knows every user's email from `User.Email`.
- `NotificationLinkCode` — a short-lived, single-use code powering the
  Telegram "connect" deep link (see below).

## Enabling it

Both channels are independent and optional; a gateway with neither
configured still has a fully working in-app notification system.

```yaml
notification:
  email:
    enabled: true
    host: smtp.example.com
    port: 587
    username: ${SMTP_USERNAME}
    password: ${SMTP_PASSWORD}
    from: notifications@example.com
    fromName: My App

  telegram:
    enabled: true
    botToken: ${TELEGRAM_BOT_TOKEN}   # from @BotFather
```

Email delivery is a plain, synchronous SMTP send per notification
(`net/smtp`) — no queue, no retry, no template engine, which is
appropriate for the volume a per-user notification system produces. A
high-volume deployment should put a relay with its own queuing in front of
this rather than expecting the gateway to grow one.

Telegram delivery long-polls Telegram's `getUpdates` API rather than
registering a webhook, deliberately: it means zero inbound network exposure
is required, so it works the same whether the gateway is reachable from the
internet or only from a private network.

## API

All endpoints are under the management prefix (`/_` by default, see
`sample/config.yaml`) — the paths below omit it.

Server-to-server (an app's backend calling the gateway when something
happens):

| Method & path | Auth | Purpose |
|---|---|---|
| `POST /api/notifications` | Admin-owned API token | Create a notification, optionally with `actions` and/or a `channels` list. Omitting `channels` attempts every channel this gateway has configured. |

Creation is admin-only, reusing the existing admin-owned API token
mechanism (`POST /api/users/{userId}/tokens`, same as every other
admin-only endpoint like the middleware/statistics APIs) rather than
inventing a new auth concept for this one feature.

User-facing (an app's frontend, same session cookie it already uses for
`/me`):

| Method & path | Purpose |
|---|---|
| `GET /api/notifications` | List the current user's notifications, newest first. `unreadOnly`, `limit`, `cursor` query params for a bell-icon dropdown / paginated list. |
| `GET /api/notifications/unread-count` | For a bell-icon badge. |
| `POST /api/notifications/{id}/read` | Mark one as read. |
| `POST /api/notifications/read-all` | Mark all as read. |
| `POST /api/notifications/{id}/respond` | Answer one of a notification's actions from the in-app list. |
| `GET /api/notifications/{id}/deliveries` | Full delivery history for one notification — every attempt, every channel, newest first (owner or admin only). |
| `GET /api/notifications/telegram/link` | Get a fresh `https://t.me/<bot>?start=<code>` deep link to connect the current user's Telegram chat. |

Public, unauthenticated (only reachable via the link an email actually
sends):

| Method & path | Purpose |
|---|---|
| `GET /api/notifications/respond?token=...&action=...` | Answer one of a notification's actions from an email link. Returns a small self-contained HTML confirmation page, not JSON. |

See `api/taronja-gateway-api.yaml`'s `Notification` tag for the full
request/response schemas.

## The email answer link, and why it needs no login

An email has no session cookie to authenticate a click with. When a
notification with actions is delivered by email, the gateway generates a
random, single-use token (the same random-bytes-then-hash pattern
`auth.TokenService` uses for API tokens — the raw token goes in the email
link, only its hash is ever stored), and the link is
`GET /api/notifications/respond?token=<raw>&action=<id>`. The token *is*
the credential: whoever has the link can answer that one notification, and
only that one, once. Tokens expire after 7 days.

## Connecting Telegram

A gateway user connects their Telegram chat once:

1. The frontend calls `GET /api/notifications/telegram/link` and renders
   the returned `deepLink` as a button or QR code.
2. The user opens it and presses **Start** in Telegram.
3. The gateway's long-polling loop sees the `/start <code>` message,
   validates the code (15-minute expiry, single use), and records a
   `NotificationChannelLink` connecting that chat to the user.

From then on, any notification requesting (or defaulting to) the
`telegram` channel is delivered there too, with actions rendered as inline
keyboard buttons. Tapping one is authorized by checking that the tapping
chat is linked to the *same user* the notification was sent to — not just
"some linked user" — otherwise anyone who discovered a notification ID
could answer someone else's notification.

## Retries

A failed delivery isn't the end of the story. A background worker
(`notification.Service.RunRetryWorker`, started unconditionally alongside
the gateway — it applies to any channel, not just one) checks every 30
seconds for deliveries due for another attempt, and retries them with
backoff:

| Attempt | Waits before it, after the previous one failed |
|---|---|
| 1 (the original) | — |
| 2 | 1 minute |
| 3 | 5 minutes |
| 4 | 30 minutes |
| 5 | 2 hours |

After the 5th attempt fails, the delivery stays `failed` permanently — no
6th attempt is scheduled. This schedule isn't configurable (see
`notification/service.go`'s `retryBackoffSchedule` — the same reasoning as
`gateway/deps`' traffic-metrics batch settings: this gateway's own
notification volume is far too low for the exact numbers to matter to an
operator).

Each retry **appends a new `NotificationDelivery` row** rather than
overwriting the failed one — `GET /api/notifications/{id}/deliveries`
shows every attempt, so "this failed twice, then succeeded on the third
try" is visible after the fact, not just "it's fine now." A retry that
succeeds where the original failed uses a channel-appropriate fresh
attempt — for an email with actions, that means a brand new answer-link
token, since the raw token from a failed attempt was never persisted
(only its hash) and so can't be reused.

A delivery that becomes unresolvable by the time its retry comes due (the
channel was disabled, or — for Telegram — the user's link was removed) is
recorded as `skipped` on that attempt instead of `failed`, and a `skipped`
attempt never schedules a further retry of its own.

## Testing

No live SMTP server or Telegram bot is needed to test this:

- `notification/email_test.go` runs a minimal real SMTP server (the actual
  wire protocol — EHLO/MAIL FROM/RCPT TO/DATA — not a mocked
  `net/smtp` internals) and asserts on what `EmailProvider.Send` actually
  sends.
- `notification/telegram_test.go` runs a real HTTP server implementing
  enough of Telegram's Bot API to exercise `TelegramProvider` and
  `TelegramPoller` end to end — including the `/start` linking flow and a
  simulated button tap, decoded from real JSON.
- `notification/service_test.go` and `db/notificationrepository_test.go`
  cover the business logic and persistence layer against a real SQLite test
  database — including the retry schedule's exhaustion boundary and a full
  fail-then-succeed retry cycle, by directly backdating a delivery's
  `NextRetryAt` (the shortest real backoff is a minute, far too slow for a
  unit test to actually wait out) rather than mocking time.
- `handlers/api_notifications_test.go` covers the HTTP handlers'
  auth/ownership/status-code decisions directly.

Run just this feature's tests with:

```sh
go test ./notification/... ./db/... -run Notification -v
go test ./handlers/... -run Notification -v
```

## Notes

- **Fixed at startup, like tracing and TLS.** `notification.NewService`,
  the retry worker, and the Telegram poller (if configured) are constructed
  once, from whatever `notification.*` said at startup
  (`gateway.InitNotifications`, called from `main.go`). A config reload
  (SIGHUP or file-watch) that changes `notification.*` is stored but has no
  effect until a full restart.
- **Delivery failures never fail creation.** The in-app record is the
  source of truth and always succeeds if the database write does; each
  channel's outcome is recorded on its own `NotificationDelivery` row,
  inspectable independently of whether the creation call itself succeeded.
- **No multi-tenancy / multi-app scoping.** Like the rest of this gateway,
  one deployment serves one app (or a small set of trusted apps sharing one
  admin token) — there's no `Source`/`AppID` field scoping notifications to
  a particular calling app. This can be added later, non-breaking, if a
  real multi-app deployment needs it.

## See also

- [README.md#authentication-providers](../README.md#authentication-providers) —
  the OAuth2 provider architecture this feature's `Provider`
  interface/registry design mirrors.
- `doc/TODO.md` — background on why this was added and the design
  decisions (channel scoping, auth, email template) made along the way.
