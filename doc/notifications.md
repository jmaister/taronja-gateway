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
- **One call can notify several users at once** — `POST /api/notifications`
  takes a list of user IDs, not just one, all sharing the same
  type/title/body/actions but each getting their own independent
  notification: their own read state, their own delivery attempts, their
  own answer. Notifying every parent in a family is one call, not a loop of
  them on the caller's side. Every call, even a single-recipient one, gets
  a `batchId` grouping its recipients — see "Status" below.
- **A single sent/failed/pending status, per notification and for a whole
  batch** — computed on demand from the raw delivery attempts, not
  something the caller has to work out by reading `/deliveries` history
  themselves. See "Status" below.
- **Each user can pick their own preferred channel** — a user who'd rather
  get notifications by Telegram than email (or vice versa) sets that once
  (`PUT /api/notifications/preferences`), and every notification that
  doesn't name explicit channels uses it from then on, no matter which app
  or call created the notification. See "Per-user channel preference"
  below.
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
- **The app that created a notification learns when it's answered** — an
  optional outbound webhook (`notification.responseWebhook` in config)
  fires once, automatically, the moment a response is recorded, on any
  channel. Without it, a response just sits in the gateway's database with
  no way for the calling app to find out about it at all — there's no
  admin-readable "check this notification's state" endpoint either, so
  this webhook is the only way that information leaves the gateway today.
  See "Response webhook" below.
- **Channel-agnostic by design** — delivery is a small `notification.Provider`
  interface (`Channel() string`, `Send(ctx, req) (externalRef string, err error)`),
  the same shape as this codebase's OAuth2 `AuthenticationProvider`
  architecture (`providers/providers.go`): one shared service, several
  independent implementations, one registry. Adding a channel (WhatsApp,
  Slack, SMS, ...) means writing one `Provider` and registering it in
  `notification.NewService` — no change to the data model or the API.

## Data model

- `Notification` — one row per notification: `UserID`, `BatchID` (groups
  every recipient's row from one `Create` call — see "Status" below),
  `Type`, `Title`, `Body`, optional `URL`, caller-opaque `Metadata` and
  `Actions` (both stored as JSON, only interpreted enough to validate a
  response names a real action), `CreatedAt`, `ReadAt`, and — once
  answered — `RespondedActionID`/`RespondedAt`/`RespondedVia` (`"web"`,
  `"email"`, or `"telegram"`).
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
- `NotificationPreference` — one row per user who's set a preferred
  delivery channel; see "Per-user channel preference" below.

## Enabling it

Every channel is independent and optional (SMS and WhatsApp are
configured as described in [SMS and WhatsApp](#sms-and-whatsapp)); a gateway with neither
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
| `POST /api/notifications` | Admin-owned API token | Create one notification per entry in `userIds`, optionally with `actions` and/or a `channels` list. Omitting `channels` uses each recipient's own preferred channel if they've set one (see "Per-user channel preference"), else every channel this gateway has configured. |

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
| `GET /api/notifications/{id}/status` | One rolled-up sent/failed/pending status, plus the per-channel breakdown behind it (owner or admin only). |
| `GET /api/notifications/batches/{batchId}/status` | Rolled-up sent/failed/pending counts across every recipient of one create call (admin only — see "Status"). |
| `GET /api/notifications/telegram/link` | Get a fresh `https://t.me/<bot>?start=<code>` deep link to connect the current user's Telegram chat. |
| `GET /api/notifications/preferences` | Get the current user's preferred delivery channel, if they've set one. |
| `PUT /api/notifications/preferences` | Set (or, with an empty/omitted value, clear) it. |

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

## SMS and WhatsApp

Both deliver to the user's **phone number**, which an admin sets through
the users API in E.164 format (`"+34600123456"`; spaces and dashes are
stripped, anything else is a 400):

```
POST  /api/users              {"username": "...", "email": "...", "phone": "+34600123456"}   # password is optional
PATCH /api/users/{userId}     {"phone": "+34600123456"}     # "" removes it
```

`GET /api/users[/{id}]` returns `phone` and `phoneVerified` (false for a
number an admin entered; such numbers are still used). Users without a
number get a `skipped` delivery on these channels, and
`GET /api/users/{id}/notifications/channels/status` reports them
`not_connected`.

Sending is the same as for every other channel: `POST /api/notifications`
with `"channels": ["sms"]` or `["whatsapp"]`, or no channels and the user's
preference (an admin can set it for users who never log in, see below).

### Notification page links

SMS and WhatsApp have no room for one link per action, so they carry one
short link to the notification's page, `<server.url>/_/n/<token>`. The
page shows the body, the notification's URL and a button per action; the
answer is recorded when a button is pressed (a POST), so link scanners that
open the link cannot answer. Answers count as given on that channel
(`respondedVia: "sms"`), first answer wins as everywhere else, and links
expire after 7 days like the email links. Without `server.url` no link is
sent.

### SMS (BulkGate)

```yaml
notification:
  sms:
    enabled: true
    backend: bulkgate               # the only backend today
    bulkgate:
      applicationId: ${BULKGATE_APPLICATION_ID}
      applicationToken: ${BULKGATE_APPLICATION_TOKEN}
      senderId: gText               # alphanumeric sender...
      senderIdValue: Academia       # ...shown as "Academia"
    maxSegments: 2                  # default 2, at most 4
    maxPerMinute: 60                # default 60
```

Create the application in BulkGate under *Modules & APIs → API
administration*. `senderId` is BulkGate's sender type (`gSystem` by
default; `gText` for an alphanumeric name, `gOwn` for your number, ...);
which ones a country allows is up to BulkGate and the carriers.

The message is the title, the body and the page link (when the notification
has actions or a URL), each on its own line. Length decides the cost:

- A single SMS holds 160 GSM-7 characters, or 153 per segment once split.
  **One character outside GSM-7 switches the whole message to Unicode**:
  70 characters, 67 per segment. Spanish á, í, ó and ú are outside it; ñ,
  é and ü are in it.
- The body (then the title) is shortened with "..." (or "…" in Unicode) to
  fit `maxSegments`; the link is never cut.

### WhatsApp (Meta Cloud API)

Meta only lets a business start a conversation with an **approved message
template**: free-form text is rejected unless the user wrote to you in the
last 24 hours (error 131047). So every notification is sent as one
template, the same for every notification:

```yaml
notification:
  whatsapp:
    enabled: true
    phoneNumberId: ${WHATSAPP_PHONE_NUMBER_ID}   # the number's ID in WhatsApp Manager
    accessToken: ${WHATSAPP_ACCESS_TOKEN}        # system user token with whatsapp_business_messaging
    template:
      name: taronja_notification
      language: es                               # as approved
      linkButton: true                           # the template has the URL button below
    # apiVersion: v23.0                           # default
    maxPerMinute: 60
```

Create the template in WhatsApp Manager → *Message templates*:

- **Category:** Utility (school notices, reminders). Marketing templates
  cost more and need opt-in.
- **Language:** the one in `template.language`.
- **Body:** text with exactly two variables, `{{1}}` for the title and
  `{{2}}` for the body. Meta rejects a body that starts or ends with a
  variable or has little fixed text around them, so wrap them, e.g.
  `New notice: *{{1}}*` / `{{2}}` / `Reply from the link below.` on three
  lines (in the template's language).
  The gateway puts newlines, tabs and runs of spaces in them on one line,
  because Meta rejects those in variables (error 132018), and limits them
  to 200 and 700 characters.
- **Button (with `linkButton: true`):** a *Visit website* button with a
  dynamic URL `https://<your server.url>/_/n/{{1}}`, e.g. labelled "Open".
  The gateway fills `{{1}}` with the page token on every message, so the
  user can open the notification and answer its actions.

A different template shape is possible with `template.bodyVariables`: `1`
puts "title: body" in a single `{{1}}`, `0` sends a fixed-text template
(e.g. "You have a new notice", usually with the link button) without the
notification's text.

Wait for approval before enabling it; until then Meta rejects the
messages (error 132001), and they are not retried.

### Cost and errors

Both channels cost money per message, so:

- `maxPerMinute` caps messages for the whole gateway; one over the cap
  fails with "rate limit" and the retry worker sends it in a later minute.
- Errors that retrying cannot fix are **not retried**: an invalid number,
  bad credentials, a number without WhatsApp, a template that does not
  match. They are recorded as `failed` with "(not retried)". Network
  errors, provider outages, throttling and (BulkGate) low credit are
  retried on the usual schedule.
- `sent` means the provider accepted the message (its ID is stored), not
  that it reached the phone.

## Per-user channel preference

A user sets which single channel they'd rather be notified on:

```
PUT /api/notifications/preferences
{"preferredChannel": "telegram"}
```

An admin sets it for another user, e.g. a contact who never logs in, with
`PUT /api/users/{userId}/notifications/preferences` and the same body. That
endpoint only accepts `email`, `telegram`, `sms`, `whatsapp` or empty.

From then on, any `POST /api/notifications` call that notifies this user
**without naming explicit `channels`** uses only their preferred channel,
instead of every channel the gateway has configured. An explicit
`channels` list on the create call always wins over the preference — a
caller that genuinely needs a specific channel for one particular
notification (e.g. "this one has to go by email, it has an attachment-style
link that doesn't make sense as a Telegram button") isn't blocked by
whatever the user picked as their everyday default.

`preferredChannel` isn't validated against which channels this gateway
currently has configured, the same "an unknown/unconfigured channel is
recorded as skipped, not rejected" philosophy the `channels` field on
notification creation already follows — see
`GET /api/notifications/{id}/deliveries` to see when that's actually
happened to a user's own notifications. `PUT` with an empty (or omitted)
`preferredChannel` clears the preference back to "no preference — use
every configured channel," the original default from before per-user
preferences existed.

## Response webhook

Without this, a response a user gives is only ever visible to that same
user's own frontend — the app that created the notification has no way to
learn about it, and there's no admin-readable endpoint to poll for it
either (`GET /api/notifications/{id}` requires the session cookie of the
notification's own owner; there's deliberately no admin bypass on that
one, unlike the delivery-history endpoint). The response webhook is the
one way that information leaves the gateway.

```yaml
notification:
  responseWebhook:
    enabled: true
    url: https://your-app.example.com/webhooks/taronja-notifications
    secret: ${NOTIFICATION_WEBHOOK_SECRET}   # optional but recommended
```

Once configured, every time a user answers a notification — from the
in-app list, an email link, or a Telegram button, it doesn't matter which —
the gateway POSTs this once, automatically:

```json
{
  "notificationId": "cm...",
  "userId": "cm...",
  "type": "music_track_added",
  "metadata": {"folderId": "f-123"},
  "respondedActionId": "approve",
  "respondedVia": "web",
  "respondedAt": "2026-01-02T03:04:05Z"
}
```

`metadata` is exactly whatever the caller attached at creation time —
this is deliberately minimal, only the information the caller couldn't
already know from having created the notification itself (which action,
via which channel, when), plus enough to correlate it back
(`notificationId`, `userId`, `type`, `metadata`) without a second lookup.

If `secret` is set, the request carries `X-Taronja-Signature:
sha256=<hex>` — an HMAC-SHA256 of the raw request body, keyed with
`secret` — so the receiver can verify the call actually came from this
gateway and not something forging it. Verify it the same way any
GitHub-/Stripe-style webhook signature is checked: compute the same HMAC
over the raw bytes you received (before any JSON parsing) and compare.
Omitting `secret` is only reasonable on a network where forging this
request isn't a real concern.

**This isn't a delivery channel a notification is ever sent *to*** — it
never appears in a `channels` list, is never a user's preferred channel,
and is never part of the "every configured channel" default. It's an
internal, one-time event, fired only from the moment a response is
recorded — see `notification.Service.recordValidatedResponse`. It's also,
deliberately, implemented as an ordinary `notification.Provider` under the
hood (`ResponseWebhookProvider`), which is what gets it the exact same
automatic retry-with-backoff and delivery-history treatment as email or
Telegram, for free: a failed webhook call shows up in
`GET /api/notifications/{id}/deliveries` with `channel: "response_webhook"`
and retries on the same schedule as any other channel (see "Retries"
below) — there's no separate, weaker reliability story for this one.

## Status

`GET /api/notifications/{id}/deliveries` (above) gives you every raw
delivery attempt; most callers don't want to reduce that themselves just
to answer "is this done, and did it work." `GET /api/notifications/{id}/status`
does that reduction for you:

```json
{
  "overall": "pending",
  "channels": {"email": "sent", "telegram": "pending"}
}
```

Each channel's own status is the outcome of its *most recent* attempt:

| Channel status | Meaning |
|---|---|
| `sent` | Delivered successfully. |
| `failed` | The most recent attempt failed and the retry schedule is exhausted — permanent. |
| `pending` | Either the most recent attempt is still in-flight (the provider's Send call has not yet returned), or it failed but a retry is still scheduled — not permanent yet. |
| `skipped` | The channel was never usable for this user/config (see `NotificationDeliveryStatusSkipped`) — never attempted, so it's neither a success nor a failure. |

`overall` rolls those up worst-first: **pending beats failed beats sent**.
A notification with even one channel still retrying is `pending` overall,
even if every other channel already succeeded; only once nothing is left
retrying does a remaining failure make it `failed`. `skipped` channels are
excluded from the rollup entirely — a channel the gateway never had a
recipient or config for doesn't drag down an otherwise fully-successful
notification, and a notification with *only* skipped channels (or no
external channels attempted at all) rolls up to `sent`: the in-app record,
which always exists regardless of configuration, is the baseline success
every external channel is additive to.

For a whole multi-recipient `POST /api/notifications` call,
`GET /api/notifications/batches/{batchId}/status` (**admin only** — a
batch can span several different users' own notifications, so there's no
single owning user this could be scoped to the way the per-notification
endpoint is) gives the same rollup as *counts* across every recipient:

```json
{"batchId": "cm...", "total": 5, "sent": 3, "failed": 1, "pending": 1}
```

`batchId` is on every `Notification` (including in
`CreateNotificationResponse`) — even a single-recipient create call gets
one, so there's no special case for "was this actually a batch."

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
- `notification/webhook_test.go` runs a real HTTP server to verify
  `ResponseWebhookProvider`'s actual payload and HMAC signature, plus
  end-to-end `Service` tests proving the webhook fires exactly once per
  response (never as part of a notification's own initial delivery), stays
  fully invisible when unconfigured, and gets a real fail-then-retry-then-
  succeed cycle through the exact same retry machinery email/Telegram use.
- `notification/service_test.go` and `db/repositories/notificationrepository_test.go`
  cover the business logic and persistence layer against a real SQLite test
  database — including the retry schedule's exhaustion boundary, a full
  fail-then-succeed retry cycle (backdating a delivery's `NextRetryAt`
  directly rather than mocking time, since the shortest real backoff is a
  minute — far too slow for a unit test to actually wait out), multi-
  recipient `Create` producing independent notifications sharing one
  `batchID`, preference resolution (explicit `channels` beats a stored
  preference beats "every configured channel"), and the status rollup
  itself — every combination of sent/failed/pending/skipped channels,
  worst-first, plus the batch-level counts across a mix of recipients.
- `handlers/api_notifications_test.go` covers the HTTP handlers'
  auth/ownership/status-code decisions directly.

Run just this feature's tests with:

```sh
go test ./notification/... ./db/... -run Notification -v
go test ./handlers/... -run Notification -v
```

## Notes

- **Setup.** `notification.NewService`, the retry worker, and the
  Telegram poller (if configured) are constructed once, from
  `notification.*` (`gateway.InitNotifications`, called from `cli/run.go`).
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
