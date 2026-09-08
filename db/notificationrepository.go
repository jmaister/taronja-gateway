package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotificationAlreadyResponded is returned by RecordResponse when the
// notification already has a response recorded — first response wins, a
// second attempt (a double-tapped Telegram button, a stale email link
// clicked twice) is rejected rather than silently overwriting the first.
var ErrNotificationAlreadyResponded = errors.New("notification already responded to")

// ErrNotificationLinkCodeInvalid is returned by ConsumeLinkCode when the
// code doesn't exist or has expired — deliberately one error for both
// cases, since distinguishing them to whoever typed/followed the code
// (a Telegram user) would only ever say "try connecting again" either way.
var ErrNotificationLinkCodeInvalid = errors.New("invalid or expired notification link code")

// NotificationRepository defines the interface for notification
// persistence: the Notification rows themselves, their per-channel
// delivery attempts, and the channel-linking (Telegram account connection)
// side of the system. See db.Notification's doc comment for the data
// model and notification.Service for the business logic built on top of
// this.
type NotificationRepository interface {
	CreateNotification(n *Notification) error
	GetNotification(id string) (*Notification, error)
	FindNotificationByRespondTokenHash(tokenHash string) (*Notification, error)
	// ListNotifications returns up to limit notifications for userID,
	// newest first, optionally restricted to unread ones. cursor, when
	// non-nil, excludes the given notification and everything newer than
	// it (keyset pagination on CreatedAt/ID) — pass the last item's ID from
	// a previous page to get the next one.
	ListNotifications(userID string, unreadOnly bool, limit int, cursor *string) ([]*Notification, error)
	CountUnread(userID string) (int64, error)
	// MarkRead sets ReadAt to now for one notification owned by userID, if
	// not already read. A no-op (not an error) if already read, missing,
	// or owned by someone else — GetNotification-then-check-ownership is
	// the caller's job when it needs to tell those apart (e.g. to return
	// 404 vs 403 from the API).
	MarkRead(id, userID string, now time.Time) error
	MarkAllRead(userID string, now time.Time) error
	// RecordResponse sets RespondedActionID/RespondedAt/RespondedVia for
	// one notification, if it has no response yet. Returns
	// ErrNotificationAlreadyResponded if it already does, or
	// gorm.ErrRecordNotFound if id doesn't exist.
	RecordResponse(id, actionID, via string, now time.Time) error
	// SetRespondToken stores the hash of a freshly generated email
	// answer-link token for one notification, replacing any previous one
	// (a re-sent email gets a fresh token; the old link stops working).
	SetRespondToken(id, tokenHash string, expiresAt time.Time) error

	CreateDelivery(d *NotificationDelivery) error
	// FindLatestDelivery returns the most recent delivery attempt for one
	// notification on one channel, or gorm.ErrRecordNotFound if there's
	// none — used to find which Telegram message to edit once a button on
	// it is answered.
	FindLatestDelivery(notificationID, channel string) (*NotificationDelivery, error)
	// ListDeliveries returns every delivery attempt for one notification,
	// across every channel, newest first — the full delivery history a
	// caller inspects via GET /api/notifications/{id}/deliveries.
	ListDeliveries(notificationID string) ([]*NotificationDelivery, error)
	// FindDeliveriesDueForRetry returns every delivery row whose
	// NextRetryAt is set and has passed — see NotificationDelivery's doc
	// comment for the "at most one non-null NextRetryAt per
	// (NotificationID, Channel)" invariant this relies on to stay a plain
	// index scan rather than a latest-row-per-group query.
	FindDeliveriesDueForRetry(now time.Time) ([]*NotificationDelivery, error)
	// ClearNextRetry sets one delivery row's NextRetryAt to NULL —  called
	// as the first step of processing a due retry, before attempting the
	// resend, so a crash mid-retry can't leave the same row perpetually
	// due (see notification.Service.RetryFailedDeliveries).
	ClearNextRetry(deliveryID string) error

	// UpsertChannelLink connects userID to externalID on channel, replacing
	// any previous link for that (userID, channel) pair — re-linking (e.g.
	// after starting a fresh Telegram chat) overwrites, it doesn't add a
	// second row.
	UpsertChannelLink(userID, channel, externalID string) error
	FindChannelLink(userID, channel string) (*NotificationChannelLink, error)
	// FindChannelLinkByExternalID is the reverse lookup: given an inbound
	// Telegram chat ID, which gateway user (if any) linked it. Used to
	// authorize a button tap — the tapping chat must resolve to the same
	// user the notification was sent to.
	FindChannelLinkByExternalID(channel, externalID string) (*NotificationChannelLink, error)

	CreateLinkCode(l *NotificationLinkCode) error
	// ConsumeLinkCode looks up code and deletes it in the same operation
	// (single-use) if found and unexpired, returning
	// ErrNotificationLinkCodeInvalid otherwise (not found, or found but
	// expired — either way the code deletes itself here so an expired one
	// doesn't linger).
	ConsumeLinkCode(code string, now time.Time) (*NotificationLinkCode, error)
}

// NotificationRepositoryDB is a database implementation of
// NotificationRepository.
type NotificationRepositoryDB struct {
	db *gorm.DB
}

// NewNotificationRepositoryDB creates a new database notification
// repository.
func NewNotificationRepositoryDB(db *gorm.DB) *NotificationRepositoryDB {
	return &NotificationRepositoryDB{db: db}
}

func (r *NotificationRepositoryDB) CreateNotification(n *Notification) error {
	return r.db.Create(n).Error
}

func (r *NotificationRepositoryDB) GetNotification(id string) (*Notification, error) {
	var n Notification
	if err := r.db.Where("id = ?", id).First(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NotificationRepositoryDB) FindNotificationByRespondTokenHash(tokenHash string) (*Notification, error) {
	var n Notification
	if err := r.db.Where("respond_token_hash = ? AND respond_token_hash != ''", tokenHash).First(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NotificationRepositoryDB) ListNotifications(userID string, unreadOnly bool, limit int, cursor *string) ([]*Notification, error) {
	q := r.db.Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if cursor != nil {
		var after Notification
		if err := r.db.Select("created_at", "id").Where("id = ?", *cursor).First(&after).Error; err != nil {
			// An unknown/stale cursor behaves like "no more pages" rather
			// than an error — a page fetched right as its cursor row was
			// deleted (it can't be, notifications aren't deleted today,
			// but the query shouldn't assume that) is more useful failing
			// closed (empty) than crashing the caller.
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return []*Notification{}, nil
			}
			return nil, err
		}
		q = q.Where("(created_at < ?) OR (created_at = ? AND id < ?)", after.CreatedAt, after.CreatedAt, after.ID)
	}
	var notifications []*Notification
	err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&notifications).Error
	return notifications, err
}

func (r *NotificationRepositoryDB) CountUnread(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&Notification{}).Where("user_id = ? AND read_at IS NULL", userID).Count(&count).Error
	return count, err
}

func (r *NotificationRepositoryDB) MarkRead(id, userID string, now time.Time) error {
	return r.db.Model(&Notification{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", id, userID).
		Update("read_at", now.UTC()).Error
}

func (r *NotificationRepositoryDB) MarkAllRead(userID string, now time.Time) error {
	return r.db.Model(&Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", now.UTC()).Error
}

func (r *NotificationRepositoryDB) RecordResponse(id, actionID, via string, now time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var n Notification
		if err := tx.Where("id = ?", id).First(&n).Error; err != nil {
			return err
		}
		if n.RespondedAt != nil {
			return ErrNotificationAlreadyResponded
		}
		utcNow := now.UTC()
		return tx.Model(&Notification{}).Where("id = ? AND responded_at IS NULL", id).Updates(map[string]interface{}{
			"responded_action_id": actionID,
			"responded_at":        utcNow,
			"responded_via":       via,
		}).Error
	})
}

func (r *NotificationRepositoryDB) SetRespondToken(id, tokenHash string, expiresAt time.Time) error {
	return r.db.Model(&Notification{}).Where("id = ?", id).Updates(map[string]interface{}{
		"respond_token_hash":       tokenHash,
		"respond_token_expires_at": expiresAt.UTC(),
	}).Error
}

func (r *NotificationRepositoryDB) CreateDelivery(d *NotificationDelivery) error {
	return r.db.Create(d).Error
}

func (r *NotificationRepositoryDB) FindLatestDelivery(notificationID, channel string) (*NotificationDelivery, error) {
	var d NotificationDelivery
	err := r.db.Where("notification_id = ? AND channel = ?", notificationID, channel).
		Order("created_at DESC").First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *NotificationRepositoryDB) ListDeliveries(notificationID string) ([]*NotificationDelivery, error) {
	var deliveries []*NotificationDelivery
	err := r.db.Where("notification_id = ?", notificationID).
		Order("created_at DESC").Find(&deliveries).Error
	return deliveries, err
}

func (r *NotificationRepositoryDB) FindDeliveriesDueForRetry(now time.Time) ([]*NotificationDelivery, error) {
	var deliveries []*NotificationDelivery
	err := r.db.Where("next_retry_at IS NOT NULL AND next_retry_at <= ?", now.UTC()).
		Find(&deliveries).Error
	return deliveries, err
}

func (r *NotificationRepositoryDB) ClearNextRetry(deliveryID string) error {
	return r.db.Model(&NotificationDelivery{}).Where("id = ?", deliveryID).
		Update("next_retry_at", nil).Error
}

func (r *NotificationRepositoryDB) UpsertChannelLink(userID, channel, externalID string) error {
	var existing NotificationChannelLink
	err := r.db.Where("user_id = ? AND channel = ?", userID, channel).First(&existing).Error
	if err == nil {
		return r.db.Model(&NotificationChannelLink{}).Where("id = ?", existing.ID).Update("external_id", externalID).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.Create(&NotificationChannelLink{
		UserID:     userID,
		Channel:    channel,
		ExternalID: externalID,
	}).Error
}

func (r *NotificationRepositoryDB) FindChannelLink(userID, channel string) (*NotificationChannelLink, error) {
	var l NotificationChannelLink
	if err := r.db.Where("user_id = ? AND channel = ?", userID, channel).First(&l).Error; err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *NotificationRepositoryDB) FindChannelLinkByExternalID(channel, externalID string) (*NotificationChannelLink, error) {
	var l NotificationChannelLink
	if err := r.db.Where("channel = ? AND external_id = ?", channel, externalID).First(&l).Error; err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *NotificationRepositoryDB) CreateLinkCode(l *NotificationLinkCode) error {
	return r.db.Create(l).Error
}

func (r *NotificationRepositoryDB) ConsumeLinkCode(code string, now time.Time) (*NotificationLinkCode, error) {
	var found *NotificationLinkCode
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var l NotificationLinkCode
		if err := tx.Where("code = ?", code).First(&l).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotificationLinkCodeInvalid
			}
			return err
		}
		if err := tx.Where("code = ?", code).Delete(&NotificationLinkCode{}).Error; err != nil {
			return err
		}
		if now.UTC().After(l.ExpiresAt) {
			return ErrNotificationLinkCodeInvalid
		}
		found = &l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
