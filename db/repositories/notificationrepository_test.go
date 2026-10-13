package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationRepository(t *testing.T) {
	db.SetupTestDB(t.Name())
	dbConn := db.GetConnection()
	repo := NewNotificationRepositoryDB(dbConn)

	user1 := db.User{Username: "notif-user1", Email: "user1@example.com"}
	require.NoError(t, dbConn.Create(&user1).Error)
	user2 := db.User{Username: "notif-user2", Email: "user2@example.com"}
	require.NoError(t, dbConn.Create(&user2).Error)

	t.Run("CreateNotification and GetNotification", func(t *testing.T) {
		n := &db.Notification{UserID: user1.ID, Type: "track_added", Title: "New track", Body: "A new track was added"}
		require.NoError(t, repo.CreateNotification(n))
		assert.NotEmpty(t, n.ID)

		fetched, err := repo.GetNotification(n.ID)
		require.NoError(t, err)
		assert.Equal(t, "New track", fetched.Title)
		assert.Nil(t, fetched.ReadAt)
		assert.Nil(t, fetched.RespondedAt)
	})

	t.Run("ListByBatchID returns every notification sharing a batch, and nothing else", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u1 := db.User{Username: "batch-user-1", Email: "batch1@example.com"}
		require.NoError(t, dbConn.Create(&u1).Error)
		u2 := db.User{Username: "batch-user-2", Email: "batch2@example.com"}
		require.NoError(t, dbConn.Create(&u2).Error)

		n1 := &db.Notification{UserID: u1.ID, BatchID: "batch-a", Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n1))
		n2 := &db.Notification{UserID: u2.ID, BatchID: "batch-a", Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n2))
		other := &db.Notification{UserID: u1.ID, BatchID: "batch-b", Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(other))

		batch, err := repo.ListByBatchID("batch-a")
		require.NoError(t, err)
		require.Len(t, batch, 2)
		assert.ElementsMatch(t, []string{n1.ID, n2.ID}, []string{batch[0].ID, batch[1].ID})

		empty, err := repo.ListByBatchID("nonexistent-batch")
		require.NoError(t, err)
		assert.Empty(t, empty)
	})

	t.Run("ListNotifications ordering, unreadOnly, and cursor pagination", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "list-user", Email: "list@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)

		var ids []string
		for i := 0; i < 5; i++ {
			n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
			require.NoError(t, repo.CreateNotification(n))
			ids = append(ids, n.ID)
			time.Sleep(2 * time.Millisecond) // ensure distinct CreatedAt ordering
		}
		// ids[4] is newest, ids[0] is oldest.

		page1, err := repo.ListNotifications(u.ID, false, 2, nil)
		require.NoError(t, err)
		require.Len(t, page1, 2)
		assert.Equal(t, ids[4], page1[0].ID)
		assert.Equal(t, ids[3], page1[1].ID)

		cursor := page1[1].ID
		page2, err := repo.ListNotifications(u.ID, false, 2, &cursor)
		require.NoError(t, err)
		require.Len(t, page2, 2)
		assert.Equal(t, ids[2], page2[0].ID)
		assert.Equal(t, ids[1], page2[1].ID)

		// Mark the newest one read, then unreadOnly should exclude it.
		require.NoError(t, repo.MarkRead(ids[4], u.ID, time.Now()))
		unread, err := repo.ListNotifications(u.ID, true, 10, nil)
		require.NoError(t, err)
		assert.Len(t, unread, 4)
		for _, n := range unread {
			assert.NotEqual(t, ids[4], n.ID)
		}
	})

	t.Run("CountUnread, MarkRead, MarkAllRead", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "unread-user", Email: "unread@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)

		var ids []string
		for i := 0; i < 3; i++ {
			n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
			require.NoError(t, repo.CreateNotification(n))
			ids = append(ids, n.ID)
		}

		count, err := repo.CountUnread(u.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)

		require.NoError(t, repo.MarkRead(ids[0], u.ID, time.Now()))
		count, err = repo.CountUnread(u.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)

		// MarkRead is idempotent and ownership-scoped: a wrong user is a no-op, not an error.
		require.NoError(t, repo.MarkRead(ids[1], "someone-else", time.Now()))
		count, err = repo.CountUnread(u.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)

		require.NoError(t, repo.MarkAllRead(u.ID, time.Now()))
		count, err = repo.CountUnread(u.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})

	t.Run("RecordResponse succeeds once and rejects a second response", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "respond-user", Email: "respond@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)
		n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n))

		require.NoError(t, repo.RecordResponse(n.ID, "approve", db.NotificationChannelWeb, time.Now()))
		fetched, err := repo.GetNotification(n.ID)
		require.NoError(t, err)
		require.NotNil(t, fetched.RespondedActionID)
		assert.Equal(t, "approve", *fetched.RespondedActionID)
		require.NotNil(t, fetched.RespondedVia)
		assert.Equal(t, db.NotificationChannelWeb, *fetched.RespondedVia)
		require.NotNil(t, fetched.RespondedAt)

		err = repo.RecordResponse(n.ID, "deny", db.NotificationChannelEmail, time.Now())
		assert.ErrorIs(t, err, ErrNotificationAlreadyResponded)

		// The first response must be untouched by the rejected second attempt.
		fetched, err = repo.GetNotification(n.ID)
		require.NoError(t, err)
		assert.Equal(t, "approve", *fetched.RespondedActionID)
	})

	t.Run("SetRespondToken and FindNotificationByRespondTokenHash", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "token-user", Email: "token@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)
		n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n))

		require.NoError(t, repo.SetRespondToken(n.ID, "abc123hash", time.Now().Add(24*time.Hour)))

		found, err := repo.FindNotificationByRespondTokenHash("abc123hash")
		require.NoError(t, err)
		assert.Equal(t, n.ID, found.ID)

		_, err = repo.FindNotificationByRespondTokenHash("nonexistent")
		assert.Error(t, err)

		// Every notification starts with an empty hash; that empty string
		// must never itself be a valid lookup key (or every un-tokened
		// notification would collide on one lookup).
		_, err = repo.FindNotificationByRespondTokenHash("")
		assert.Error(t, err)
	})

	t.Run("CreateDelivery and FindLatestDelivery", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "delivery-user", Email: "delivery@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)
		n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n))

		require.NoError(t, repo.CreateDelivery(&db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelTelegram,
			Status: db.NotificationDeliveryStatusSent, ExternalRef: "111:222",
		}))
		time.Sleep(2 * time.Millisecond)
		require.NoError(t, repo.CreateDelivery(&db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelTelegram,
			Status: db.NotificationDeliveryStatusSent, ExternalRef: "111:333",
		}))

		latest, err := repo.FindLatestDelivery(n.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.Equal(t, "111:333", latest.ExternalRef)

		_, err = repo.FindLatestDelivery(n.ID, db.NotificationChannelEmail)
		assert.Error(t, err)
	})

	t.Run("ListDeliveries returns every attempt across channels, newest first", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "list-delivery-user", Email: "list-delivery@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)
		n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n))

		require.NoError(t, repo.CreateDelivery(&db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelEmail, Status: db.NotificationDeliveryStatusSent,
		}))
		time.Sleep(2 * time.Millisecond)
		require.NoError(t, repo.CreateDelivery(&db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelTelegram, Status: db.NotificationDeliveryStatusFailed,
		}))

		deliveries, err := repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		require.Len(t, deliveries, 2)
		assert.Equal(t, db.NotificationChannelTelegram, deliveries[0].Channel, "newest first")
		assert.Equal(t, db.NotificationChannelEmail, deliveries[1].Channel)

		empty, err := repo.ListDeliveries("nonexistent-notification")
		require.NoError(t, err)
		assert.Empty(t, empty)
	})

	t.Run("FindDeliveriesDueForRetry and ClearNextRetry", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "retry-user", Email: "retry@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)
		n := &db.Notification{UserID: u.ID, Type: "t", Title: "n", Body: "b"}
		require.NoError(t, repo.CreateNotification(n))

		past := time.Now().Add(-1 * time.Minute)
		future := time.Now().Add(1 * time.Hour)

		due := &db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelEmail,
			Status: db.NotificationDeliveryStatusFailed, AttemptNumber: 1, NextRetryAt: &past,
		}
		require.NoError(t, repo.CreateDelivery(due))
		notYetDue := &db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelTelegram,
			Status: db.NotificationDeliveryStatusFailed, AttemptNumber: 1, NextRetryAt: &future,
		}
		require.NoError(t, repo.CreateDelivery(notYetDue))
		neverRetried := &db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelEmail,
			Status: db.NotificationDeliveryStatusSent, AttemptNumber: 1,
		}
		require.NoError(t, repo.CreateDelivery(neverRetried))

		dueList, err := repo.FindDeliveriesDueForRetry(time.Now())
		require.NoError(t, err)
		require.Len(t, dueList, 1)
		assert.Equal(t, due.ID, dueList[0].ID)

		require.NoError(t, repo.ClearNextRetry(due.ID))
		dueList, err = repo.FindDeliveriesDueForRetry(time.Now())
		require.NoError(t, err)
		assert.Empty(t, dueList, "cleared row must no longer be due")
	})

	t.Run("Channel link upsert and lookups", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "link-user", Email: "link@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)

		require.NoError(t, repo.UpsertChannelLink(u.ID, db.NotificationChannelTelegram, "chat-1"))
		link, err := repo.FindChannelLink(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.Equal(t, "chat-1", link.ExternalID)

		byExternal, err := repo.FindChannelLinkByExternalID(db.NotificationChannelTelegram, "chat-1")
		require.NoError(t, err)
		assert.Equal(t, u.ID, byExternal.UserID)

		// Re-linking (e.g. a fresh Telegram chat) overwrites, not duplicates.
		require.NoError(t, repo.UpsertChannelLink(u.ID, db.NotificationChannelTelegram, "chat-2"))
		link, err = repo.FindChannelLink(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.Equal(t, "chat-2", link.ExternalID)

		_, err = repo.FindChannelLinkByExternalID(db.NotificationChannelTelegram, "chat-1")
		assert.Error(t, err, "the old external ID must no longer resolve after re-linking")
	})

	t.Run("Channel link status and deletion", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "status-user", Email: "status@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)

		linked, linkedAt, err := repo.GetChannelLinkStatus(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.False(t, linked, "no gorm.ErrRecordNotFound leaking out — just false")
		assert.True(t, linkedAt.IsZero())

		deleted, err := repo.DeleteChannelLink(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.False(t, deleted, "nothing to delete yet")

		before := time.Now()
		require.NoError(t, repo.UpsertChannelLink(u.ID, db.NotificationChannelTelegram, "chat-status"))

		linked, linkedAt, err = repo.GetChannelLinkStatus(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.True(t, linked)
		assert.WithinDuration(t, before, linkedAt, 5*time.Second)

		deleted, err = repo.DeleteChannelLink(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.True(t, deleted)

		linked, _, err = repo.GetChannelLinkStatus(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.False(t, linked, "must actually be gone after deletion, not just marked")

		deletedAgain, err := repo.DeleteChannelLink(u.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.False(t, deletedAgain, "deleting an already-gone link is a no-op, not an error")
	})

	t.Run("Link code is single-use and expiry is enforced", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "code-user", Email: "code@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)

		require.NoError(t, repo.CreateLinkCode(&db.NotificationLinkCode{
			Code: "good-code", UserID: u.ID, Channel: db.NotificationChannelTelegram,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}))

		consumed, err := repo.ConsumeLinkCode("good-code", time.Now())
		require.NoError(t, err)
		assert.Equal(t, u.ID, consumed.UserID)

		// Second consumption of the same code fails — it was deleted by the first.
		_, err = repo.ConsumeLinkCode("good-code", time.Now())
		assert.ErrorIs(t, err, ErrNotificationLinkCodeInvalid)

		require.NoError(t, repo.CreateLinkCode(&db.NotificationLinkCode{
			Code: "expired-code", UserID: u.ID, Channel: db.NotificationChannelTelegram,
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		}))
		_, err = repo.ConsumeLinkCode("expired-code", time.Now())
		assert.ErrorIs(t, err, ErrNotificationLinkCodeInvalid)

		// An expired code is deleted on the attempt above too — confirm it
		// doesn't linger for a second, equally-failing attempt to find.
		_, err = repo.ConsumeLinkCode("expired-code", time.Now())
		assert.ErrorIs(t, err, ErrNotificationLinkCodeInvalid)
	})

	t.Run("preferred channel: unset, set, changed, and cleared", func(t *testing.T) {
		db.SetupTestDB(t.Name())
		u := db.User{Username: "pref-user", Email: "pref@example.com"}
		require.NoError(t, dbConn.Create(&u).Error)

		channel, err := repo.GetPreferredChannel(u.ID)
		require.NoError(t, err)
		assert.Empty(t, channel, "no preference set yet")

		require.NoError(t, repo.SetPreferredChannel(u.ID, db.NotificationChannelTelegram))
		channel, err = repo.GetPreferredChannel(u.ID)
		require.NoError(t, err)
		assert.Equal(t, db.NotificationChannelTelegram, channel)

		// Setting again overwrites, it doesn't add a second row.
		require.NoError(t, repo.SetPreferredChannel(u.ID, db.NotificationChannelEmail))
		channel, err = repo.GetPreferredChannel(u.ID)
		require.NoError(t, err)
		assert.Equal(t, db.NotificationChannelEmail, channel)

		var count int64
		require.NoError(t, dbConn.Model(&db.NotificationPreference{}).Where("user_id = ?", u.ID).Count(&count).Error)
		assert.Equal(t, int64(1), count)

		// An empty channel clears the preference back to "none set".
		require.NoError(t, repo.SetPreferredChannel(u.ID, ""))
		channel, err = repo.GetPreferredChannel(u.ID)
		require.NoError(t, err)
		assert.Empty(t, channel)

		// A user who's never touched their preference at all gets the
		// same "no preference" answer, not an error.
		other := db.User{Username: "no-pref-user", Email: "no-pref@example.com"}
		require.NoError(t, dbConn.Create(&other).Error)
		channel, err = repo.GetPreferredChannel(other.ID)
		require.NoError(t, err)
		assert.Empty(t, channel)
	})
}
