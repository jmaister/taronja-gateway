package repositories

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// Delivery statuses as an admin sees them, matching notification.Service's
// rollup: a failed attempt with a retry scheduled is still pending. "none"
// means no external delivery at all (in-app only).
const (
	AdminStatusSent    = "sent"
	AdminStatusPending = "pending"
	AdminStatusFailed  = "failed"
	AdminStatusSkipped = "skipped"
	AdminStatusNone    = "none"
)

// AdminNotificationFilter narrows NotificationRepository.ListForAdmin. Zero
// values mean "no filter".
type AdminNotificationFilter struct {
	From, To *time.Time // created between, inclusive
	Type     string
	UserID   string
	User     string // case-insensitive substring of the recipient's username or email
	BatchID  string
	// Channel keeps notifications with a delivery on this channel; with
	// Status, the status is that channel's.
	Channel string
	// Status is one of the AdminStatus* values. Without Channel it is the
	// notification's overall status: pending if any channel is pending,
	// else failed if any failed, else sent if any was sent, else skipped
	// (every channel skipped) or none (no external delivery).
	Status    string
	Responded *bool
	Read      *bool
	Search    string // case-insensitive substring of the title or body
}

// AdminNotificationRow is a notification with its recipient's username and email.
type AdminNotificationRow struct {
	db.Notification `gorm:"embedded"`
	Username        string
	Email           string
}

// latestDelivery matches deliveries that are the newest attempt on their
// channel for the outer notification, excluding the response webhook (an
// internal event, not a delivery to the user) unless a channel is asked for.
func latestDelivery(channel string) (string, []any) {
	sql := "SELECT 1 FROM notification_deliveries d WHERE d.notification_id = notifications.id" +
		" AND d.rowid = (SELECT MAX(d2.rowid) FROM notification_deliveries d2 WHERE d2.notification_id = d.notification_id AND d2.channel = d.channel)"
	if channel != "" {
		return sql + " AND d.channel = ?", []any{channel}
	}
	return sql + " AND d.channel != '" + db.NotificationChannelResponseWebhook + "'", nil
}

// deliveryStatusSQL is the condition on d for one admin status.
var deliveryStatusSQL = map[string]string{
	AdminStatusSent:    "d.status = 'sent'",
	AdminStatusPending: "(d.status = 'pending' OR (d.status = 'failed' AND d.next_retry_at IS NOT NULL))",
	AdminStatusFailed:  "(d.status = 'failed' AND d.next_retry_at IS NULL)",
	AdminStatusSkipped: "d.status = 'skipped'",
}

// IsAdminStatus reports whether status is one of the AdminStatus* values.
func IsAdminStatus(status string) bool {
	_, ok := deliveryStatusSQL[status]
	return ok || status == AdminStatusNone
}

func (f AdminNotificationFilter) apply(q *gorm.DB) (*gorm.DB, error) {
	if f.From != nil {
		q = q.Where("notifications.created_at >= ?", f.From.UTC())
	}
	if f.To != nil {
		q = q.Where("notifications.created_at <= ?", f.To.UTC())
	}
	if f.Type != "" {
		q = q.Where("notifications.type = ?", f.Type)
	}
	if f.UserID != "" {
		q = q.Where("notifications.user_id = ?", f.UserID)
	}
	if f.User != "" {
		like := "%" + strings.ToLower(f.User) + "%"
		q = q.Where("(LOWER(users.username) LIKE ? OR LOWER(users.email) LIKE ?)", like, like)
	}
	if f.BatchID != "" {
		q = q.Where("notifications.batch_id = ?", f.BatchID)
	}
	if f.Search != "" {
		like := "%" + strings.ToLower(f.Search) + "%"
		q = q.Where("(LOWER(notifications.title) LIKE ? OR LOWER(notifications.body) LIKE ?)", like, like)
	}
	if f.Responded != nil {
		if *f.Responded {
			q = q.Where("notifications.responded_at IS NOT NULL")
		} else {
			q = q.Where("notifications.responded_at IS NULL")
		}
	}
	if f.Read != nil {
		if *f.Read {
			q = q.Where("notifications.read_at IS NOT NULL")
		} else {
			q = q.Where("notifications.read_at IS NULL")
		}
	}

	latest, args := latestDelivery(f.Channel)
	exists := func(status string) (string, []any) {
		return "EXISTS (" + latest + " AND " + deliveryStatusSQL[status] + ")", args
	}
	notExists := func(status string) (string, []any) {
		sql, a := exists(status)
		return "NOT " + sql, a
	}
	if f.Channel != "" && f.Status == "" {
		q = q.Where("EXISTS ("+latest+")", args...)
	}
	switch {
	case f.Status == "":
	case !IsAdminStatus(f.Status):
		return nil, fmt.Errorf("unknown status %q", f.Status)
	case f.Status == AdminStatusNone:
		q = q.Where("NOT EXISTS ("+latest+")", args...)
	case f.Channel != "":
		sql, a := exists(f.Status)
		q = q.Where(sql, a...)
	default:
		// The overall status: the first that applies of pending, failed,
		// sent, skipped.
		order := []string{AdminStatusPending, AdminStatusFailed, AdminStatusSent, AdminStatusSkipped}
		for _, before := range order {
			if before == f.Status {
				break
			}
			sql, a := notExists(before)
			q = q.Where(sql, a...)
		}
		sql, a := exists(f.Status)
		q = q.Where(sql, a...)
	}
	return q, nil
}

func (r *NotificationRepositoryDB) ListForAdmin(filter AdminNotificationFilter, limit, offset int) ([]AdminNotificationRow, int64, error) {
	base := r.db.Table("notifications").Joins("LEFT JOIN users ON users.id = notifications.user_id")
	q, err := filter.apply(base)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	err = q.Session(&gorm.Session{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	rows := []AdminNotificationRow{}
	err = q.Select("notifications.*, users.username AS username, users.email AS email").
		Order("notifications.created_at DESC, notifications.id DESC").
		Limit(limit).Offset(offset).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *NotificationRepositoryDB) ListDeliveriesForNotifications(ids []string) (map[string][]*db.NotificationDelivery, error) {
	out := make(map[string][]*db.NotificationDelivery, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var deliveries []*db.NotificationDelivery
	err := r.db.Where("notification_id IN ?", ids).Order("created_at DESC, rowid DESC").Find(&deliveries).Error
	if err != nil {
		return nil, err
	}
	for _, d := range deliveries {
		out[d.NotificationID] = append(out[d.NotificationID], d)
	}
	return out, nil
}

func (r *NotificationRepositoryDB) ListNotificationTypes() ([]string, error) {
	types := []string{}
	err := r.db.Model(&db.Notification{}).Distinct("type").Order("type").Pluck("type", &types).Error
	return types, err
}
