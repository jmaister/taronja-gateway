package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
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
// handlers.CreateNotification) supplies to create one notification.
type CreateInput struct {
	UserID   string
	Type     string
	Title    string
	Body     string
	URL      *string
	Metadata map[string]interface{}
	Actions  []Action
	// Channels restricts delivery to these external channels. Empty means
	// "every channel this gateway has configured" — the caller doesn't
	// need to know which channels exist to get sensible default delivery,
	// matching the "keep providers generic" goal: adding a channel never
	// requires every existing caller to start naming it explicitly.
	Channels []string
}

// Create stores a notification and attempts delivery on every requested
// (or, if none named, every configured) external channel. Delivery
// failures never fail Create itself — the in-app record is the source of
// truth and always succeeds if the database write does; each channel's
// outcome is recorded on its own NotificationDelivery row instead (see
// deliver), inspectable independently of whether Create returned an
// error.
func (s *Service) Create(ctx context.Context, in CreateInput) (*db.Notification, error) {
	metadataJSON, err := EncodeMetadata(in.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encoding metadata: %w", err)
	}
	actionsJSON, err := EncodeActions(in.Actions)
	if err != nil {
		return nil, fmt.Errorf("encoding actions: %w", err)
	}

	n := &db.Notification{
		UserID:   in.UserID,
		Type:     in.Type,
		Title:    in.Title,
		Body:     in.Body,
		URL:      in.URL,
		Metadata: metadataJSON,
		Actions:  actionsJSON,
	}
	if err := s.repo.CreateNotification(n); err != nil {
		return nil, err
	}

	channels := in.Channels
	if len(channels) == 0 {
		for channel := range s.providers {
			channels = append(channels, channel)
		}
	}
	for _, channel := range channels {
		s.deliver(ctx, n, in.Actions, channel)
	}

	return n, nil
}

// deliver attempts one channel's delivery of n and always records the
// outcome, even when the channel is unknown or the user has no recipient
// for it — see db.NotificationDeliveryStatusSkipped's doc comment for why
// that's worth recording rather than silently doing nothing.
func (s *Service) deliver(ctx context.Context, n *db.Notification, actions []Action, channel string) {
	provider, configured := s.providers[channel]
	if !configured {
		s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusSkipped, "channel not configured on this gateway", "")
		return
	}

	recipient, err := s.resolveRecipient(n.UserID, channel)
	if err != nil {
		s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusSkipped, err.Error(), "")
		return
	}

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

	externalRef, err := provider.Send(ctx, req)
	if err != nil {
		s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusFailed, err.Error(), "")
		return
	}
	s.recordDelivery(n.ID, channel, db.NotificationDeliveryStatusSent, "", externalRef)
}

func (s *Service) recordDelivery(notificationID, channel, status, errMsg, externalRef string) {
	_ = s.repo.CreateDelivery(&db.NotificationDelivery{
		NotificationID: notificationID,
		Channel:        channel,
		Status:         status,
		Error:          errMsg,
		ExternalRef:    externalRef,
	})
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

// RespondViaWeb records userID's answer to notification id from the
// authenticated in-app list — the session cookie is userID's proof of
// identity, the same as every other /_/ endpoint that reads
// ctx.Value(session.SessionKey).
func (s *Service) RespondViaWeb(id, userID, actionID string) (label string, err error) {
	n, err := s.repo.GetNotification(id)
	if err != nil {
		return "", ErrNotFound
	}
	if n.UserID != userID {
		return "", ErrForbidden
	}
	return s.recordValidatedResponse(n, actionID, db.NotificationChannelWeb)
}

// RespondViaToken records a response from the public, unauthenticated
// email answer link — rawToken (the URL's ?token=) is itself the proof of
// identity here, since there's no session cookie on an email click; see
// db.Notification.RespondTokenHash's doc comment.
func (s *Service) RespondViaToken(rawToken, actionID string) (label string, err error) {
	n, err := s.repo.FindNotificationByRespondTokenHash(hashToken(rawToken))
	if err != nil {
		return "", ErrNotFound
	}
	if n.RespondTokenExpiresAt == nil || time.Now().UTC().After(*n.RespondTokenExpiresAt) {
		return "", ErrTokenExpired
	}
	return s.recordValidatedResponse(n, actionID, db.NotificationChannelEmail)
}

// RespondViaTelegram records a response from a Telegram inline-keyboard
// tap. chatID is whichever chat the tap came from — proof of identity here
// is that chatID resolves (via NotificationChannelLink) to the same user
// the notification was actually sent to, not merely "some linked user";
// otherwise anyone who discovers a notification ID could answer someone
// else's notification.
func (s *Service) RespondViaTelegram(notificationID, actionID, chatID string) (label string, err error) {
	n, err := s.repo.GetNotification(notificationID)
	if err != nil {
		return "", ErrNotFound
	}
	link, err := s.repo.FindChannelLinkByExternalID(db.NotificationChannelTelegram, chatID)
	if err != nil || link.UserID != n.UserID {
		return "", ErrForbidden
	}
	return s.recordValidatedResponse(n, actionID, db.NotificationChannelTelegram)
}

func (s *Service) recordValidatedResponse(n *db.Notification, actionID, via string) (label string, err error) {
	actions, err := DecodeActions(n.Actions)
	if err != nil {
		return "", err
	}
	action := findAction(actions, actionID)
	if action == nil {
		return "", ErrInvalidAction
	}
	if err := s.repo.RecordResponse(n.ID, actionID, via, time.Now()); err != nil {
		if errors.Is(err, db.ErrNotificationAlreadyResponded) {
			return "", ErrAlreadyResponded
		}
		return "", err
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
