package notification

import (
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
)

// Admin listing page sizes.
const (
	DefaultAdminPageSize = 50
	MaxAdminPageSize     = 200
)

// AdminNotification is one row of the admin notification list.
type AdminNotification struct {
	repositories.AdminNotificationRow
	// Channels maps each channel the notification was attempted on to its
	// latest status (see channelStatus), including the response webhook.
	Channels map[string]string
	// Status is the overall status as AdminNotificationFilter.Status
	// defines it; the response webhook does not count.
	Status string
}

// AdminList returns one page (1-based) of the notifications matching filter,
// and how many match in total. pageSize is clamped to MaxAdminPageSize.
func (s *Service) AdminList(filter repositories.AdminNotificationFilter, page, pageSize int) ([]AdminNotification, int64, error) {
	if pageSize <= 0 {
		pageSize = DefaultAdminPageSize
	}
	if pageSize > MaxAdminPageSize {
		pageSize = MaxAdminPageSize
	}
	if page < 1 {
		page = 1
	}
	rows, total, err := s.repo.ListForAdmin(filter, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	deliveries, err := s.repo.ListDeliveriesForNotifications(ids)
	if err != nil {
		return nil, 0, err
	}

	out := make([]AdminNotification, 0, len(rows))
	for _, row := range rows {
		channels := map[string]string{}
		for _, d := range deliveries[row.ID] { // newest first: the first per channel is its latest
			if _, seen := channels[d.Channel]; !seen {
				channels[d.Channel] = channelStatus(d)
			}
		}
		out = append(out, AdminNotification{AdminNotificationRow: row, Channels: channels, Status: adminStatus(channels)})
	}
	return out, total, nil
}

// adminStatus rolls channel statuses up the way AdminNotificationFilter.Status
// filters them.
func adminStatus(channels map[string]string) string {
	var failed, sent, skipped bool
	for channel, status := range channels {
		if channel == db.NotificationChannelResponseWebhook {
			continue
		}
		switch status {
		case StatusPending:
			return repositories.AdminStatusPending
		case StatusFailed:
			failed = true
		case StatusSent:
			sent = true
		default:
			skipped = true
		}
	}
	switch {
	case failed:
		return repositories.AdminStatusFailed
	case sent:
		return repositories.AdminStatusSent
	case skipped:
		return repositories.AdminStatusSkipped
	}
	return repositories.AdminStatusNone
}

// NotificationTypes returns every distinct notification type, for filters.
func (s *Service) NotificationTypes() ([]string, error) {
	return s.repo.ListNotificationTypes()
}
