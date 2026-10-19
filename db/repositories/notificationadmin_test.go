package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type adminFixture struct {
	repo  *NotificationRepositoryDB
	ana   *db.User
	bruno *db.User
	base  time.Time
	ids   map[string]string // name -> notification ID
}

// newAdminFixture stores notifications whose delivery histories cover every
// status, two days apart:
//
//	sent      ana   sms sent (after a failed try)       day 0, answered, read
//	failed    ana   email failed for good               day 1
//	retrying  bruno sms failed, retry scheduled         day 2
//	inflight  bruno whatsapp pending + email sent       day 3
//	skipped   bruno sms skipped                         day 4
//	inapp     ana   no external delivery                day 5
//	mixed     ana   sms sent + email failed for good    day 6
func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	users := NewDBUserRepository(conn)
	f := &adminFixture{
		repo:  NewNotificationRepositoryDB(conn),
		ana:   &db.User{Username: "ana", Email: "ana@school.example"},
		bruno: &db.User{Username: "bruno", Email: "bruno@parents.example"},
		base:  time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		ids:   map[string]string{},
	}
	require.NoError(t, users.CreateUser(f.ana))
	require.NoError(t, users.CreateUser(f.bruno))

	retryAt := f.base.Add(30 * 24 * time.Hour)
	type attempt struct{ channel, status string }
	seed := []struct {
		name, kind string
		user       *db.User
		attempts   []attempt // oldest first
		retryLast  bool
	}{
		{"sent", "trip", f.ana, []attempt{{"sms", "failed"}, {"sms", "pending"}, {"sms", "sent"}}, false},
		{"failed", "trip", f.ana, []attempt{{"email", "pending"}, {"email", "failed"}}, false},
		{"retrying", "fees", f.bruno, []attempt{{"sms", "pending"}, {"sms", "failed"}}, true},
		{"inflight", "fees", f.bruno, []attempt{{"email", "sent"}, {"whatsapp", "pending"}}, false},
		{"skipped", "trip", f.bruno, []attempt{{"sms", "skipped"}}, false},
		{"inapp", "news", f.ana, nil, false},
		{"mixed", "news", f.ana, []attempt{{"sms", "sent"}, {"email", "failed"}}, false},
	}
	for i, s := range seed {
		n := &db.Notification{UserID: s.user.ID, BatchID: "batch-" + s.name, Type: s.kind, Title: "Title " + s.name, Body: "Body of " + s.name, CreatedAt: f.base.Add(time.Duration(i) * 24 * time.Hour)}
		require.NoError(t, f.repo.CreateNotification(n))
		f.ids[s.name] = n.ID
		for j, a := range s.attempts {
			d := &db.NotificationDelivery{NotificationID: n.ID, Channel: a.channel, Status: a.status, AttemptNumber: j + 1}
			if s.retryLast && j == len(s.attempts)-1 {
				d.NextRetryAt = &retryAt
			}
			require.NoError(t, f.repo.CreateDelivery(d))
		}
	}
	now := f.base
	require.NoError(t, f.repo.RecordResponse(f.ids["sent"], "ok", "sms", now))
	require.NoError(t, conn.Model(&db.Notification{}).Where("id = ?", f.ids["sent"]).Update("read_at", now).Error)
	return f
}

func (f *adminFixture) names(t *testing.T, filter AdminNotificationFilter) []string {
	t.Helper()
	rows, total, err := f.repo.ListForAdmin(filter, 100, 0)
	require.NoError(t, err)
	assert.EqualValues(t, len(rows), total)
	byID := map[string]string{}
	for name, id := range f.ids {
		byID[id] = name
	}
	names := []string{}
	for _, r := range rows {
		names = append(names, byID[r.ID])
	}
	return names
}

func TestListForAdmin_Filters(t *testing.T) {
	f := newAdminFixture(t)
	day := func(n int) *time.Time { d := f.base.Add(time.Duration(n) * 24 * time.Hour); return &d }
	yes, no := true, false

	assert.Equal(t, []string{"mixed", "inapp", "skipped", "inflight", "retrying", "failed", "sent"}, f.names(t, AdminNotificationFilter{}), "newest first")

	cases := map[string]struct {
		filter AdminNotificationFilter
		want   []string
	}{
		"date range":          {AdminNotificationFilter{From: day(1), To: day(3)}, []string{"inflight", "retrying", "failed"}},
		"type":                {AdminNotificationFilter{Type: "fees"}, []string{"inflight", "retrying"}},
		"user id":             {AdminNotificationFilter{UserID: f.bruno.ID}, []string{"skipped", "inflight", "retrying"}},
		"user by email part":  {AdminNotificationFilter{User: "PARENTS.example"}, []string{"skipped", "inflight", "retrying"}},
		"user by name part":   {AdminNotificationFilter{User: "an"}, []string{"mixed", "inapp", "failed", "sent"}},
		"batch":               {AdminNotificationFilter{BatchID: "batch-failed"}, []string{"failed"}},
		"text":                {AdminNotificationFilter{Search: "body OF in"}, []string{"inapp", "inflight"}},
		"answered":            {AdminNotificationFilter{Responded: &yes}, []string{"sent"}},
		"unanswered and read": {AdminNotificationFilter{Responded: &no, Read: &yes}, []string{}},
		"unread":              {AdminNotificationFilter{Read: &no, Type: "trip"}, []string{"skipped", "failed"}},
		"attempted on sms":    {AdminNotificationFilter{Channel: "sms"}, []string{"mixed", "skipped", "retrying", "sent"}},
		"combined":            {AdminNotificationFilter{Type: "trip", User: "bruno"}, []string{"skipped"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, f.names(t, tc.filter))
		})
	}
}

func TestListForAdmin_StatusFilters(t *testing.T) {
	f := newAdminFixture(t)
	cases := map[string]struct {
		filter AdminNotificationFilter
		want   []string
	}{
		// Overall: pending wins, then failed, then sent, then skipped.
		"pending: in flight or retry scheduled": {AdminNotificationFilter{Status: AdminStatusPending}, []string{"inflight", "retrying"}},
		"failed for good":                       {AdminNotificationFilter{Status: AdminStatusFailed}, []string{"mixed", "failed"}},
		"sent: an earlier failure was retried":  {AdminNotificationFilter{Status: AdminStatusSent}, []string{"sent"}},
		"skipped":                               {AdminNotificationFilter{Status: AdminStatusSkipped}, []string{"skipped"}},
		"none: in-app only":                     {AdminNotificationFilter{Status: AdminStatusNone}, []string{"inapp"}},
		// Per channel: only that channel's latest attempt counts.
		"sms sent":             {AdminNotificationFilter{Channel: "sms", Status: AdminStatusSent}, []string{"mixed", "sent"}},
		"email failed":         {AdminNotificationFilter{Channel: "email", Status: AdminStatusFailed}, []string{"mixed", "failed"}},
		"email sent":           {AdminNotificationFilter{Channel: "email", Status: AdminStatusSent}, []string{"inflight"}},
		"never tried whatsapp": {AdminNotificationFilter{Channel: "whatsapp", Status: AdminStatusNone, Type: "fees"}, []string{"retrying"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, f.names(t, tc.filter))
		})
	}

	_, _, err := f.repo.ListForAdmin(AdminNotificationFilter{Status: "lost"}, 10, 0)
	assert.Error(t, err)
}

func TestListForAdmin_PaginationAndRecipient(t *testing.T) {
	f := newAdminFixture(t)
	page1, total, err := f.repo.ListForAdmin(AdminNotificationFilter{}, 3, 0)
	require.NoError(t, err)
	page3, _, err := f.repo.ListForAdmin(AdminNotificationFilter{}, 3, 6)
	require.NoError(t, err)
	assert.EqualValues(t, 7, total, "the total ignores the page")
	require.Len(t, page1, 3)
	require.Len(t, page3, 1)
	assert.Equal(t, f.ids["mixed"], page1[0].ID)
	assert.Equal(t, f.ids["sent"], page3[0].ID)
	assert.Equal(t, "ana", page1[0].Username)
	assert.Equal(t, "ana@school.example", page1[0].Email)

	_, total, err = f.repo.ListForAdmin(AdminNotificationFilter{Type: "trip"}, 1, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total, "the total counts the filtered set")

	deliveries, err := f.repo.ListDeliveriesForNotifications([]string{f.ids["sent"], f.ids["inapp"]})
	require.NoError(t, err)
	require.Len(t, deliveries[f.ids["sent"]], 3)
	assert.Equal(t, "sent", deliveries[f.ids["sent"]][0].Status, "newest first")
	assert.Empty(t, deliveries[f.ids["inapp"]])

	types, err := f.repo.ListNotificationTypes()
	require.NoError(t, err)
	assert.Equal(t, []string{"fees", "news", "trip"}, types)
}
