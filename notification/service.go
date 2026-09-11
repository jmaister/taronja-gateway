package notification

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/lucsky/cuid"
)

// Sentinel errors Service methods return — handlers/api_notifications.go
// maps each to an HTTP status, and telegram.go's humanizeRespondError maps
// each to a short message shown back in the Telegram app.
var (
	ErrNotFound             = errors.New("notification not found")
	ErrForbidden            = errors.New("not your notification")
	ErrInvalidAction        = errors.New("not a valid action for this notification")
	ErrAlreadyResponded     = errors.New("notification already responded to")
	ErrTokenExpired         = errors.New("respond link has expired")
	ErrChannelNotConfigured = errors.New("channel not configured")
)

// respondTokenTTL bounds how long an email's answer links keep working —
// generous enough that a notification sitting unread in an inbox for a few
// days can still be answered, short enough that a token isn't usable
// indefinitely.
const respondTokenTTL = 7 * 24 * time.Hour

// linkCodeTTL bounds how long a Telegram "connect" deep link stays valid —
// short, since it's meant to be followed within the same session it was
// generated in (the user clicks "Connect Telegram" and follows the link
// right away), not saved for later.
const linkCodeTTL = 15 * time.Minute

// DefaultListLimit and MaxListLimit bound ListNotifications' page size —
// see List.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// retryBackoffSchedule bounds how many times a failed delivery is retried
// and how long after each failure the next attempt waits. Not
// user-configurable (the same reasoning as gateway/deps' trafficMetrics
// batch settings) — this gateway's own delivery volume is far too low for
// the exact numbers to matter to an operator; the shape (a handful of
// attempts, a growing gap between them) is the only thing that does.
// Index 0 is the wait after the *first* attempt fails, before the 2nd is
// tried; the schedule is exhausted (no further retries, the delivery
// stays "failed" permanently) once an attempt number exceeds
// len(retryBackoffSchedule).
var retryBackoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// retryWorkerInterval is how often RunRetryWorker checks for deliveries
// whose NextRetryAt has passed. Coarser than the shortest backoff step
// above, deliberately — there's no reason to poll faster than the fastest
// thing that could possibly be due.
const retryWorkerInterval = 30 * time.Second

// nextRetryAt returns when a delivery that just failed on attemptNumber
// should be retried next, or nil if the schedule is exhausted.
func nextRetryAt(attemptNumber int, now time.Time) *time.Time {
	if attemptNumber > len(retryBackoffSchedule) {
		return nil
	}
	t := now.Add(retryBackoffSchedule[attemptNumber-1])
	return &t
}

// Service is the notification system's business logic: creating
// notifications, listing/reading them, delivering them over whichever
// Providers are configured, and recording responses from any channel. It
// holds no HTTP concerns at all — handlers/api_notifications.go is the
// only caller, translating this package's types and errors to and from
// the OpenAPI-generated request/response shapes.
type Service struct {
	repo     db.NotificationRepository
	userRepo db.UserRepository

	providers map[string]Provider
	telegram  *TelegramProvider // also present in providers when configured; kept separately for Telegram-only operations (linking) with no generic Provider equivalent

	// respondBaseURL is the absolute URL email answer links are built
	// from, e.g. "https://gateway.example.com/_/notifications/respond".
	// Empty when config.ServerConfig.URL isn't set — Create still works,
	// it just can't offer clickable email actions (see deliver).
	respondBaseURL string

	// pendingDeliveries tracks the in-flight background goroutines Create
	// spawns for each recipient/channel delivery attempt (see Create's doc
	// comment for why those run in the background rather than inline).
	// Production code never waits on it — delivery outcomes are meant to
	// be inspected later via GetNotificationStatus/GetBatchStatus/
	// ListDeliveries, never by blocking on this. Tests that need a
	// deterministic point to assert on a delivery row use
	// waitForPendingDeliveries (service_test.go) instead of sleeping or
	// polling.
	pendingDeliveries sync.WaitGroup
}

// NewService builds a Service from configuration, registering a Provider
// for each external channel that's actually configured (see
// NewEmailProvider/NewTelegramProvider's nil-when-unconfigured
// convention) — a deployment with neither block set still gets a fully
// working in-app notification system, just with no external delivery.
func NewService(cfg config.NotificationConfig, repo db.NotificationRepository, userRepo db.UserRepository, respondBaseURL string) *Service {
	providers := map[string]Provider{}
	var telegram *TelegramProvider

	if email := NewEmailProvider(cfg.Email); email != nil {
		providers[email.Channel()] = email
	}
	if tg := NewTelegramProvider(cfg.Telegram); tg != nil {
		providers[tg.Channel()] = tg
		telegram = tg
	}
	if webhook := NewResponseWebhookProvider(cfg.ResponseWebhook); webhook != nil {
		providers[webhook.Channel()] = webhook
	}

	return &Service{
		repo:           repo,
		userRepo:       userRepo,
		providers:      providers,
		telegram:       telegram,
		respondBaseURL: respondBaseURL,
	}
}

// TelegramPoller returns a poller for this Service's Telegram provider, or
// nil if Telegram isn't configured — see TelegramPoller.Run.
func (s *Service) TelegramPoller() *TelegramPoller {
	return NewTelegramPoller(s.telegram, s)
}

// CreateInput is what a caller (typically server-to-server, see
// handlers.CreateNotification) supplies to create one notification per
// recipient in UserIDs — all sharing the same Type/Title/Body/URL/
// Metadata/Actions, but each getting an independent db.Notification row,
// so each recipient reads, responds to, and (if configured) is delivered
// theirs entirely independently of the others.
type CreateInput struct {
	UserIDs  []string
	Type     string
	Title    string
	Body     string
	URL      *string
	Metadata map[string]interface{}
	Actions  []Action
	// Channels restricts delivery to these external channels, for every
	// recipient alike. Empty means each recipient's own preferred channel
	// (see SetPreferredChannel) if they've set one, else every channel
	// this gateway has configured — see resolveChannelsForUser. The
	// caller never needs to know which channels exist, or what any given
	// recipient prefers, to get sensible default delivery.
	Channels []string
}

// Create stores one notification per recipient, all sharing one freshly
// generated batchID — even a single-recipient call gets one, a "batch of
// one" (see db.Notification.BatchID's doc comment) — and kicks off delivery
// on each recipient's own resolved channels (see resolveChannelsForUser) in
// the background, one goroutine per recipient/channel pair. Create itself
// returns as soon as the in-app records are written, without waiting for
// any of those deliveries to finish: handlers.CreateNotification calls this
// directly from an HTTP handler, and a caller notifying many recipients
// over a slow channel (an SMTP server with a slow greeting, a flaky
// webhook) would otherwise hold that request open for as long as every
// send takes, combined. Each delivery uses a context detached from ctx
// (context.WithoutCancel) for exactly this reason — ctx is usually an
// *http.Request's, which is canceled the moment the handler returns, and a
// canceled context would abort an in-flight send that Create no longer
// waits for anyway.
//
// Delivery failures never fail Create itself — the in-app record is the
// source of truth and always succeeds if its database write does; each
// channel's outcome is recorded on its own NotificationDelivery row
// instead (see deliver), inspectable independently of whether Create
// returned an error, or in aggregate via GetBatchStatus. Because delivery
// is now asynchronous, a NotificationDelivery row for a given recipient
// may not exist yet the instant Create returns — callers that need to
// observe the outcome of a specific send (rather than just that Create
// accepted the notification) should poll GetNotificationStatus/
// GetBatchStatus rather than assume delivery is complete.
//
// One recipient's notification failing to be stored (a rare DB-level
// failure — there's no per-recipient validation that could fail first)
// doesn't stop the rest: Create keeps going and returns every
// notification that *did* get created, alongside the joined set of
// per-recipient errors, if any — a caller notifying five people shouldn't
// lose the other four over one bad row.
func (s *Service) Create(ctx context.Context, in CreateInput) (notifications []*db.Notification, batchID string, err error) {
	metadataJSON, err := EncodeMetadata(in.Metadata)
	if err != nil {
		return nil, "", fmt.Errorf("encoding metadata: %w", err)
	}
	actionsJSON, err := EncodeActions(in.Actions)
	if err != nil {
		return nil, "", fmt.Errorf("encoding actions: %w", err)
	}
	batchID, err = cuid.NewCrypto(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generating batch id: %w", err)
	}

	notifications = make([]*db.Notification, 0, len(in.UserIDs))
	var errs []error
	for _, userID := range in.UserIDs {
		n := &db.Notification{
			UserID:   userID,
			BatchID:  batchID,
			Type:     in.Type,
			Title:    in.Title,
			Body:     in.Body,
			URL:      in.URL,
			Metadata: metadataJSON,
			Actions:  actionsJSON,
		}
		if err := s.repo.CreateNotification(n); err != nil {
			errs = append(errs, fmt.Errorf("user %s: %w", userID, err))
			continue
		}
		notifications = append(notifications, n)

		deliverCtx := context.WithoutCancel(ctx)
		for _, channel := range s.resolveChannelsForUser(userID, in.Channels) {
			s.pendingDeliveries.Add(1)
			go func(n *db.Notification, channel string) {
				defer s.pendingDeliveries.Done()
				s.deliver(deliverCtx, n, in.Actions, channel)
			}(n, channel)
		}
	}
	return notifications, batchID, errors.Join(errs...)
}

// resolveChannelsForUser decides which external channels to attempt
// delivery on for one recipient: explicit, when the caller named one in
// CreateInput.Channels (applies to every recipient of that Create call
// alike); else that recipient's own preferred channel, so "each user can
// decide to receive notifications by one different provider" holds even
// when several recipients are notified in the same call; else every
// channel this gateway has configured — the original default from before
// per-user preferences existed.
func (s *Service) resolveChannelsForUser(userID string, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}
	if preferred, err := s.repo.GetPreferredChannel(userID); err == nil && preferred != "" {
		return []string{preferred}
	}
	channels := make([]string, 0, len(s.providers))
	for channel := range s.providers {
		// response_webhook isn't a channel a notification is ever
		// delivered *by* — it's an internal, one-time event fired after a
		// response is recorded (see recordValidatedResponse) — so it must
		// never be picked up by the "every configured channel" default a
		// notification's own creation uses.
		if channel == db.NotificationChannelResponseWebhook {
			continue
		}
		channels = append(channels, channel)
	}
	return channels
}

// SetPreferredChannel sets (or, with channel == "", clears) userID's
// preferred delivery channel — consulted by Create whenever a caller
// doesn't name explicit Channels (see resolveChannelsForUser). Not
// validated against which channels this gateway currently has configured:
// the same "an unknown/unconfigured channel is recorded as skipped, not
// rejected" philosophy Create's own Channels list already follows (see
// db.NotificationDeliveryStatusSkipped), so a preference set today keeps
// working unchanged if the gateway's configuration changes later.
func (s *Service) SetPreferredChannel(userID, channel string) error {
	return s.repo.SetPreferredChannel(userID, channel)
}

// GetPreferredChannel returns userID's preferred delivery channel, or ""
// if they haven't set one.
func (s *Service) GetPreferredChannel(userID string) (string, error) {
	return s.repo.GetPreferredChannel(userID)
}

// deliver attempts the first delivery of n on channel and always records
// the outcome, even when the channel is unknown or the user has no
// recipient for it — see db.NotificationDeliveryStatusSkipped's doc
// comment for why that's worth recording rather than silently doing
// nothing. A failure here doesn't end the story: RetryFailedDeliveries
// picks it up later if the schedule allows (see recordDelivery).
func (s *Service) deliver(ctx context.Context, n *db.Notification, actions []Action, channel string) {
	const firstAttempt = 1

	provider, configured := s.providers[channel]
	if !configured {
		s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusSkipped, "channel not configured on this gateway", "", firstAttempt)
		return
	}

	recipient, err := s.resolveRecipient(n.UserID, channel)
	if err != nil {
		s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusSkipped, err.Error(), "", firstAttempt)
		return
	}

	req := s.buildSendRequest(n, actions, channel, recipient)
	externalRef, err := provider.Send(ctx, req)
	if err != nil {
		s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusFailed, err.Error(), "", firstAttempt)
		return
	}
	s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusSent, "", externalRef, firstAttempt)
}

// buildSendRequest assembles a SendRequest for one delivery attempt,
// shared between deliver (attempt 1) and retryOne (every attempt after).
// For an email with actions, this generates a *fresh* respond token every
// single attempt, retries included — the raw token only ever exists in
// memory for the duration of one Send call (only its hash is persisted),
// so a retried send needs its own new one rather than reusing whatever the
// failed attempt tried to use.
func (s *Service) buildSendRequest(n *db.Notification, actions []Action, channel, recipient string) SendRequest {
	req := SendRequest{Notification: n, Actions: actions, Recipient: recipient}
	if len(actions) > 0 && channel == db.NotificationChannelEmail {
		// A failure generating/storing the token isn't fatal to delivery —
		// it just falls back to a plain, unanswerable email rather than
		// failing the whole send over it (EmailProvider.Send only adds
		// action links when both RespondBaseURL and RespondToken are set).
		if token, hash, err := generateOpaqueToken(24); err == nil {
			if err := s.repo.SetRespondToken(n.ID, hash, time.Now().Add(respondTokenTTL)); err == nil {
				req.RespondBaseURL = s.respondBaseURL
				req.RespondToken = token
			}
		}
	}
	return req
}

// recordDelivery persists one delivery attempt. When status is "failed"
// and the retry schedule isn't exhausted yet, this also sets NextRetryAt —
// the only place that field is ever set — so RetryFailedDeliveries picks
// it up later without deliver/retryOne needing to know anything about
// scheduling themselves.
func (s *Service) recordDelivery(notificationID, channel, status, errMsg, externalRef string, attemptNumber int) {
	delivery := &db.NotificationDelivery{
		NotificationID: notificationID,
		Channel:        channel,
		Status:         status,
		Error:          errMsg,
		ExternalRef:    externalRef,
		AttemptNumber:  attemptNumber,
	}
	if status == db.NotificationDeliveryStatusFailed {
		delivery.NextRetryAt = nextRetryAt(attemptNumber, time.Now())
	}
	_ = s.repo.CreateDelivery(delivery)
}

// RetryFailedDeliveries resends every delivery whose NextRetryAt has
// passed, recording a new NotificationDelivery row per attempt (see
// db.NotificationDelivery's doc comment for why retries append rather than
// mutate the original row). Meant to be called periodically — see
// RunRetryWorker — but exported and safe to call directly too (tests do;
// a future manual "retry now" admin action could).
func (s *Service) RetryFailedDeliveries(ctx context.Context) (attempted int, err error) {
	due, err := s.repo.FindDeliveriesDueForRetry(time.Now())
	if err != nil {
		return 0, err
	}
	for _, delivery := range due {
		// Clear first: if anything below panics or the process is killed
		// mid-retry, this row must not stay perpetually "due" and get
		// retried forever in the next tick.
		if clearErr := s.repo.ClearNextRetry(delivery.ID); clearErr != nil {
			log.Printf("notification: failed to clear retry marker on delivery %s: %v", delivery.ID, clearErr)
			continue
		}
		s.retryOne(ctx, delivery)
		attempted++
	}
	return attempted, nil
}

func (s *Service) retryOne(ctx context.Context, delivery *db.NotificationDelivery) {
	nextAttempt := delivery.AttemptNumber + 1

	n, err := s.repo.GetNotification(delivery.NotificationID)
	if err != nil {
		log.Printf("notification: retry skipped, notification %s no longer exists: %v", delivery.NotificationID, err)
		return
	}
	actions, err := DecodeActions(n.Actions)
	if err != nil {
		log.Printf("notification: retry skipped, could not decode actions for notification %s: %v", n.ID, err)
		return
	}

	provider, configured := s.providers[delivery.Channel]
	if !configured {
		s.recordDelivery(n.ID, delivery.Channel, db.NotificationDeliveryStatusSkipped, "channel not configured on this gateway", "", nextAttempt)
		return
	}
	recipient, err := s.resolveRecipient(n.UserID, delivery.Channel)
	if err != nil {
		s.recordDelivery(n.ID, delivery.Channel, db.NotificationDeliveryStatusSkipped, err.Error(), "", nextAttempt)
		return
	}

	req := s.buildSendRequest(n, actions, delivery.Channel, recipient)
	externalRef, err := provider.Send(ctx, req)
	if err != nil {
		s.recordDelivery(n.ID, delivery.Channel, db.NotificationDeliveryStatusFailed, err.Error(), "", nextAttempt)
		return
	}
	s.recordDelivery(n.ID, delivery.Channel, db.NotificationDeliveryStatusSent, "", externalRef, nextAttempt)
}

// RunRetryWorker polls for due retries every retryWorkerInterval until ctx
// is cancelled. Started unconditionally at gateway startup (see
// gateway.InitNotifications) — unlike TelegramPoller, this doesn't depend
// on any particular provider being configured, since retries apply to any
// channel; a gateway with nothing configured simply never has anything
// due.
func (s *Service) RunRetryWorker(ctx context.Context) {
	ticker := time.NewTicker(retryWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.RetryFailedDeliveries(ctx); err != nil {
				log.Printf("notification: retry worker failed to query due deliveries: %v", err)
			}
		}
	}
}

// resolveRecipient maps a (user, channel) pair to the channel-specific
// address Provider.Send needs — an email address, a Telegram chat ID.
// Adding a channel whose recipient isn't yet one of these two kinds of
// identity means adding a case here, nowhere else.
func (s *Service) resolveRecipient(userID, channel string) (string, error) {
	switch channel {
	case db.NotificationChannelEmail:
		user, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
		if err != nil || user == nil || user.Email == "" {
			return "", fmt.Errorf("no email address on file for this user")
		}
		return user.Email, nil
	case db.NotificationChannelTelegram:
		link, err := s.repo.FindChannelLink(userID, db.NotificationChannelTelegram)
		if err != nil {
			return "", fmt.Errorf("user has not linked a telegram chat")
		}
		return link.ExternalID, nil
	case db.NotificationChannelResponseWebhook:
		// Not a per-user address — ResponseWebhookProvider always POSTs to
		// its one configured URL regardless of which user responded. A
		// non-empty placeholder just satisfies deliver's "did we resolve a
		// recipient" check; Send never reads req.Recipient for this channel.
		return "response_webhook", nil
	default:
		return "", fmt.Errorf("unknown channel %q", channel)
	}
}

// List returns up to limit notifications for userID, newest first,
// clamping limit to (0, MaxListLimit] and defaulting a non-positive value
// to DefaultListLimit — callers never need to duplicate that clamping.
func (s *Service) List(userID string, unreadOnly bool, limit int, cursor *string) ([]*db.Notification, error) {
	if limit <= 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	return s.repo.ListNotifications(userID, unreadOnly, limit, cursor)
}

func (s *Service) UnreadCount(userID string) (int64, error) {
	return s.repo.CountUnread(userID)
}

func (s *Service) MarkRead(id, userID string) error {
	return s.repo.MarkRead(id, userID, time.Now())
}

func (s *Service) MarkAllRead(userID string) error {
	return s.repo.MarkAllRead(userID, time.Now())
}

// Get returns notification id, provided it belongs to userID — used by
// handlers.RespondToNotification to return the updated notification after
// RespondViaWeb records a response, without a second, unrelated query
// (e.g. re-listing) standing in for "fetch this one row again."
func (s *Service) Get(id, userID string) (*db.Notification, error) {
	n, err := s.repo.GetNotification(id)
	if err != nil {
		return nil, ErrNotFound
	}
	if n.UserID != userID {
		return nil, ErrForbidden
	}
	return n, nil
}

// ListDeliveries returns notificationID's full delivery history — every
// channel, every attempt (including ones superseded by a later retry),
// newest first — the data behind GET /api/notifications/{id}/deliveries.
// Restricted to the notification's own owner or an admin, the same
// ownership rule Get uses.
func (s *Service) ListDeliveries(notificationID, userID string, isAdmin bool) ([]*db.NotificationDelivery, error) {
	n, err := s.repo.GetNotification(notificationID)
	if err != nil {
		return nil, ErrNotFound
	}
	if n.UserID != userID && !isAdmin {
		return nil, ErrForbidden
	}
	return s.repo.ListDeliveries(notificationID)
}

// Status values GetNotificationStatus/GetBatchStatus report — distinct
// from the raw, per-attempt db.NotificationDeliveryStatus* values a
// NotificationDelivery row itself takes. StatusPending specifically means
// "the most recent attempt on this channel failed, but a retry is still
// scheduled" — it never appears as a NotificationDelivery.Status value,
// only as this package's interpretation of one.
const (
	StatusSent    = "sent"
	StatusFailed  = "failed"
	StatusPending = "pending"
)

// channelStatus reduces one channel's most recent delivery attempt to a
// single status: the outcome of that attempt, with a failed one promoted
// to StatusPending if a retry is still scheduled. Returns
// db.NotificationDeliveryStatusSkipped (not a Status constant above) for a
// skipped attempt — skipped channels are reported in the per-channel
// breakdown but deliberately excluded from overallStatus's rollup.
func channelStatus(latest *db.NotificationDelivery) string {
	switch latest.Status {
	case db.NotificationDeliveryStatusSent:
		return StatusSent
	case db.NotificationDeliveryStatusFailed:
		if latest.NextRetryAt != nil {
			return StatusPending
		}
		return StatusFailed
	default: // db.NotificationDeliveryStatusSkipped
		return db.NotificationDeliveryStatusSkipped
	}
}

// overallStatus rolls up a set of per-channel statuses (as channelStatus
// produces — "skipped" included) into one Status, worst-first: pending
// beats failed beats sent. Skipped channels are excluded from the rollup
// entirely — a channel the gateway never had a working recipient or
// config for shouldn't make an otherwise-successful notification look
// failed, or an otherwise-pending one look worse. A notification with no
// non-skipped channels at all — including one with no external delivery
// ever attempted — rolls up to StatusSent: the in-app record, which
// always exists regardless of configuration, is the baseline success
// every external channel is additive to.
func overallStatus(channelStatuses map[string]string) string {
	sawFailed := false
	for _, status := range channelStatuses {
		switch status {
		case StatusPending:
			return StatusPending
		case StatusFailed:
			sawFailed = true
		}
	}
	if sawFailed {
		return StatusFailed
	}
	return StatusSent
}

// channelStatuses returns notificationID's latest attempt on every channel
// it's ever been attempted on, reduced via channelStatus, keyed by channel
// name.
func (s *Service) channelStatuses(notificationID string) (map[string]string, error) {
	deliveries, err := s.repo.ListDeliveries(notificationID)
	if err != nil {
		return nil, err
	}
	// ListDeliveries returns newest first, across every channel and every
	// attempt — the first row seen per channel is that channel's latest.
	statuses := make(map[string]string, len(deliveries))
	for _, d := range deliveries {
		if _, seen := statuses[d.Channel]; seen {
			continue
		}
		statuses[d.Channel] = channelStatus(d)
	}
	return statuses, nil
}

// GetNotificationStatus returns notificationID's overall status (see
// overallStatus) and the per-channel breakdown behind it. Restricted to
// the notification's own owner or an admin, the same ownership rule
// ListDeliveries uses.
func (s *Service) GetNotificationStatus(notificationID, userID string, isAdmin bool) (overall string, channels map[string]string, err error) {
	n, err := s.repo.GetNotification(notificationID)
	if err != nil {
		return "", nil, ErrNotFound
	}
	if n.UserID != userID && !isAdmin {
		return "", nil, ErrForbidden
	}
	channels, err = s.channelStatuses(notificationID)
	if err != nil {
		return "", nil, err
	}
	return overallStatus(channels), channels, nil
}

// BatchStatusCounts summarizes one batch's notifications by their overall
// GetNotificationStatus — see GetBatchStatus.
type BatchStatusCounts struct {
	Total   int
	Sent    int
	Failed  int
	Pending int
}

// GetBatchStatus rolls up every notification created together in one
// Create call (see db.Notification.BatchID) into counts of how many are
// Sent/Failed/Pending overall. Admin-only: a batch can span several
// different users' own notifications (that's the whole point of a
// multi-recipient Create), so there's no single owning user this could be
// scoped to the way GetNotificationStatus/ListDeliveries are — only the
// caller who created the batch (necessarily an admin, since
// CreateNotification itself is admin-only) is in a position to ask "how
// did the whole thing go."
func (s *Service) GetBatchStatus(batchID string, isAdmin bool) (BatchStatusCounts, error) {
	if !isAdmin {
		return BatchStatusCounts{}, ErrForbidden
	}
	notifications, err := s.repo.ListByBatchID(batchID)
	if err != nil {
		return BatchStatusCounts{}, err
	}
	if len(notifications) == 0 {
		return BatchStatusCounts{}, ErrNotFound
	}
	counts := BatchStatusCounts{Total: len(notifications)}
	for _, n := range notifications {
		channels, err := s.channelStatuses(n.ID)
		if err != nil {
			return BatchStatusCounts{}, err
		}
		switch overallStatus(channels) {
		case StatusSent:
			counts.Sent++
		case StatusFailed:
			counts.Failed++
		case StatusPending:
			counts.Pending++
		}
	}
	return counts, nil
}

// RespondViaWeb records userID's answer to notification id from the
// authenticated in-app list — the session cookie is userID's proof of
// identity, the same as every other /_/ endpoint that reads
// ctx.Value(session.SessionKey).
func (s *Service) RespondViaWeb(ctx context.Context, id, userID, actionID string) (label string, err error) {
	n, err := s.repo.GetNotification(id)
	if err != nil {
		return "", ErrNotFound
	}
	if n.UserID != userID {
		return "", ErrForbidden
	}
	return s.recordValidatedResponse(ctx, n, actionID, db.NotificationChannelWeb)
}

// RespondViaToken records a response from the public, unauthenticated
// email answer link — rawToken (the URL's ?token=) is itself the proof of
// identity here, since there's no session cookie on an email click; see
// db.Notification.RespondTokenHash's doc comment.
func (s *Service) RespondViaToken(ctx context.Context, rawToken, actionID string) (label string, err error) {
	n, err := s.repo.FindNotificationByRespondTokenHash(hashToken(rawToken))
	if err != nil {
		return "", ErrNotFound
	}
	if n.RespondTokenExpiresAt == nil || time.Now().UTC().After(*n.RespondTokenExpiresAt) {
		return "", ErrTokenExpired
	}
	return s.recordValidatedResponse(ctx, n, actionID, db.NotificationChannelEmail)
}

// RespondViaTelegram records a response from a Telegram inline-keyboard
// tap. chatID is whichever chat the tap came from — proof of identity here
// is that chatID resolves (via NotificationChannelLink) to the same user
// the notification was actually sent to, not merely "some linked user";
// otherwise anyone who discovers a notification ID could answer someone
// else's notification.
func (s *Service) RespondViaTelegram(ctx context.Context, notificationID, actionID, chatID string) (label string, err error) {
	n, err := s.repo.GetNotification(notificationID)
	if err != nil {
		return "", ErrNotFound
	}
	link, err := s.repo.FindChannelLinkByExternalID(db.NotificationChannelTelegram, chatID)
	if err != nil || link.UserID != n.UserID {
		return "", ErrForbidden
	}
	return s.recordValidatedResponse(ctx, n, actionID, db.NotificationChannelTelegram)
}

// recordValidatedResponse records a response (from any of the three
// Respond* entry points above) and, if a response webhook is configured,
// fires it — see notification.ResponseWebhookProvider. The webhook fire
// is best-effort and never changes what's returned to the caller who
// actually answered the notification: a failure there gets Service's
// normal retry-with-backoff treatment (recordDelivery/RetryFailedDeliveries),
// same as any other delivery failure, not surfaced as an error here.
func (s *Service) recordValidatedResponse(ctx context.Context, n *db.Notification, actionID, via string) (label string, err error) {
	actions, err := DecodeActions(n.Actions)
	if err != nil {
		return "", err
	}
	action := findAction(actions, actionID)
	if action == nil {
		return "", ErrInvalidAction
	}
	now := time.Now()
	if err := s.repo.RecordResponse(n.ID, actionID, via, now); err != nil {
		if errors.Is(err, db.ErrNotificationAlreadyResponded) {
			return "", ErrAlreadyResponded
		}
		return "", err
	}

	if _, configured := s.providers[db.NotificationChannelResponseWebhook]; configured {
		// Reflect the just-recorded response in memory so
		// ResponseWebhookProvider.Send (via buildSendRequest, which reads
		// these three fields straight off the Notification) sees it —
		// repo.RecordResponse only updated the database row, not this
		// in-memory copy.
		utcNow := now.UTC()
		n.RespondedActionID = &actionID
		n.RespondedVia = &via
		n.RespondedAt = &utcNow
		s.deliver(ctx, n, actions, db.NotificationChannelResponseWebhook)
	}

	return action.Label, nil
}

// GetTelegramLinkCode issues a fresh one-time linking code for userID and
// returns the "https://t.me/<bot>?start=<code>" deep link a frontend can
// render as a button/QR code — following it and hitting "Start" in
// Telegram is what actually connects the chat (see TelegramPoller's
// handling of "/start").
func (s *Service) GetTelegramLinkCode(ctx context.Context, userID string) (deepLink string, expiresAt time.Time, err error) {
	if s.telegram == nil {
		return "", time.Time{}, ErrChannelNotConfigured
	}
	code, _, err := generateOpaqueToken(18)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(linkCodeTTL)
	if err := s.repo.CreateLinkCode(&db.NotificationLinkCode{
		Code: code, UserID: userID, Channel: db.NotificationChannelTelegram, ExpiresAt: expiresAt,
	}); err != nil {
		return "", time.Time{}, err
	}
	username, err := s.telegram.Username(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", username, code), expiresAt, nil
}

// LinkTelegramChat consumes a linking code and connects its user to
// chatID — called by TelegramPoller when a "/start <code>" message
// arrives.
func (s *Service) LinkTelegramChat(code, chatID string) error {
	linkCode, err := s.repo.ConsumeLinkCode(code, time.Now())
	if err != nil {
		return err
	}
	return s.repo.UpsertChannelLink(linkCode.UserID, db.NotificationChannelTelegram, chatID)
}
