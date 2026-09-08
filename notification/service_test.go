package notification

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T, cfg config.NotificationConfig) (*Service, db.NotificationRepository, db.UserRepository) {
	t.Helper()
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := db.NewNotificationRepositoryDB(conn)
	userRepo := db.NewDBUserRepository(conn)
	service := NewService(cfg, repo, userRepo, "https://gw.example.com/_/notifications/respond")
	return service, repo, userRepo
}

func TestService_Create(t *testing.T) {
	t.Run("with no external channels configured, still creates the in-app record", func(t *testing.T) {
		service, _, userRepo := newTestService(t, config.NotificationConfig{})
		user := &db.User{Username: "u1", Email: "u1@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{
			UserID: user.ID, Type: "track_added", Title: "New track", Body: "body",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, n.ID)
		assert.Equal(t, "track_added", n.Type)
	})

	t.Run("skips a requested channel the user has no recipient for, without failing Create", func(t *testing.T) {
		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			Telegram: config.TelegramNotificationConfig{Enabled: true, BotToken: "unused-in-this-test"},
		})
		user := &db.User{Username: "u2", Email: "u2@example.com"} // no telegram link
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{
			UserID: user.ID, Type: "t", Title: "T", Body: "B",
			Channels: []string{db.NotificationChannelTelegram},
		})
		require.NoError(t, err)

		delivery, err := repo.FindLatestDelivery(n.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.Equal(t, db.NotificationDeliveryStatusSkipped, delivery.Status)
	})

	t.Run("skips a requested channel that isn't configured at all", func(t *testing.T) {
		service, repo, userRepo := newTestService(t, config.NotificationConfig{})
		user := &db.User{Username: "u3", Email: "u3@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{
			UserID: user.ID, Type: "t", Title: "T", Body: "B",
			Channels: []string{"whatsapp"}, // not implemented yet, but must not error
		})
		require.NoError(t, err)

		delivery, err := repo.FindLatestDelivery(n.ID, "whatsapp")
		require.NoError(t, err)
		assert.Equal(t, db.NotificationDeliveryStatusSkipped, delivery.Status)
		assert.Contains(t, delivery.Error, "not configured")
	})

	t.Run("with no Channels specified, attempts every configured channel", func(t *testing.T) {
		emailServer := newFakeSMTPServer(t)
		defer emailServer.close()
		host, port := emailServer.addr()

		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			Email: config.EmailNotificationConfig{Enabled: true, Host: host, Port: port, From: "gw@example.com"},
		})
		user := &db.User{Username: "u4", Email: "u4@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{UserID: user.ID, Type: "t", Title: "T", Body: "B"})
		require.NoError(t, err)

		delivery, err := repo.FindLatestDelivery(n.ID, db.NotificationChannelEmail)
		require.NoError(t, err)
		assert.Equal(t, db.NotificationDeliveryStatusSent, delivery.Status)
	})

	t.Run("email delivery with actions issues a respond token", func(t *testing.T) {
		emailServer := newFakeSMTPServer(t)
		defer emailServer.close()
		host, port := emailServer.addr()

		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			Email: config.EmailNotificationConfig{Enabled: true, Host: host, Port: port, From: "gw@example.com"},
		})
		user := &db.User{Username: "u5", Email: "u5@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{
			UserID: user.ID, Type: "t", Title: "Approve?", Body: "B",
			Actions:  []Action{{ID: "approve", Label: "Approve"}},
			Channels: []string{db.NotificationChannelEmail},
		})
		require.NoError(t, err)

		fetched, err := repo.GetNotification(n.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.RespondTokenHash)
		require.NotNil(t, fetched.RespondTokenExpiresAt)
	})
}

func TestService_ListAndReadState(t *testing.T) {
	service, _, userRepo := newTestService(t, config.NotificationConfig{})
	user := &db.User{Username: "list-u", Email: "list-u@example.com"}
	require.NoError(t, userRepo.CreateUser(user))

	for i := 0; i < 3; i++ {
		_, err := service.Create(context.Background(), CreateInput{UserID: user.ID, Type: "t", Title: "T", Body: "B"})
		require.NoError(t, err)
	}

	count, err := service.UnreadCount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	list, err := service.List(user.ID, false, 0, nil) // limit 0 -> DefaultListLimit
	require.NoError(t, err)
	require.Len(t, list, 3)

	require.NoError(t, service.MarkRead(list[0].ID, user.ID))
	count, err = service.UnreadCount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	require.NoError(t, service.MarkAllRead(user.ID))
	count, err = service.UnreadCount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	t.Run("List clamps an excessive limit", func(t *testing.T) {
		list, err := service.List(user.ID, false, 10000, nil)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(list), MaxListLimit)
	})
}

func TestService_RespondViaWeb(t *testing.T) {
	service, _, userRepo := newTestService(t, config.NotificationConfig{})
	owner := &db.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, userRepo.CreateUser(owner))
	stranger := &db.User{Username: "stranger", Email: "stranger@example.com"}
	require.NoError(t, userRepo.CreateUser(stranger))

	n, err := service.Create(context.Background(), CreateInput{
		UserID: owner.ID, Type: "t", Title: "Approve?", Body: "B",
		Actions: []Action{{ID: "approve", Label: "Approve"}},
	})
	require.NoError(t, err)

	t.Run("rejects a response from a user who doesn't own the notification", func(t *testing.T) {
		_, err := service.RespondViaWeb(n.ID, stranger.ID, "approve")
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("rejects an action ID that isn't one of the notification's actions", func(t *testing.T) {
		_, err := service.RespondViaWeb(n.ID, owner.ID, "not-a-real-action")
		assert.ErrorIs(t, err, ErrInvalidAction)
	})

	t.Run("rejects a notification ID that doesn't exist", func(t *testing.T) {
		_, err := service.RespondViaWeb("nonexistent-id", owner.ID, "approve")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("records the response once and rejects a second attempt", func(t *testing.T) {
		label, err := service.RespondViaWeb(n.ID, owner.ID, "approve")
		require.NoError(t, err)
		assert.Equal(t, "Approve", label)

		_, err = service.RespondViaWeb(n.ID, owner.ID, "approve")
		assert.ErrorIs(t, err, ErrAlreadyResponded)
	})
}

func TestService_RespondViaToken(t *testing.T) {
	emailServer := newFakeSMTPServer(t)
	defer emailServer.close()
	host, port := emailServer.addr()

	service, repo, userRepo := newTestService(t, config.NotificationConfig{
		Email: config.EmailNotificationConfig{Enabled: true, Host: host, Port: port, From: "gw@example.com"},
	})
	user := &db.User{Username: "token-u", Email: "token-u@example.com"}
	require.NoError(t, userRepo.CreateUser(user))

	n, err := service.Create(context.Background(), CreateInput{
		UserID: user.ID, Type: "t", Title: "Approve?", Body: "B",
		Actions:  []Action{{ID: "approve", Label: "Approve"}},
		Channels: []string{db.NotificationChannelEmail},
	})
	require.NoError(t, err)

	t.Run("an unknown token is rejected", func(t *testing.T) {
		_, err := service.RespondViaToken("not-a-real-token", "approve")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	// Recover the raw token the same way an email recipient would have it:
	// there's no API for this (by design — only the email itself carries
	// it), so the test reaches into the repository layer just enough to
	// prove the hash-matching contract, without ever exposing the raw
	// value back out of Create/Service.
	fetched, err := repo.GetNotification(n.ID)
	require.NoError(t, err)
	require.NotEmpty(t, fetched.RespondTokenHash)

	t.Run("the correct raw token responds successfully", func(t *testing.T) {
		// We don't have the raw token (only its hash is stored) — instead,
		// confirm end to end that RespondViaToken(hashToken(raw)) would
		// match by generating our own token, hashing it the same way
		// SetRespondToken's caller did, and wiring it in directly. This
		// keeps the test honest about what's actually verified: hashing
		// symmetry, not a hardcoded value.
		raw := "test-raw-token-value"
		require.NoError(t, repo.SetRespondToken(n.ID, hashToken(raw), fetched.RespondTokenExpiresAt.Add(0)))

		label, err := service.RespondViaToken(raw, "approve")
		require.NoError(t, err)
		assert.Equal(t, "Approve", label)
	})
}

func TestNextRetryAt(t *testing.T) {
	now := time.Now()
	for i := 1; i <= len(retryBackoffSchedule); i++ {
		got := nextRetryAt(i, now)
		require.NotNil(t, got, "attempt %d should still have a scheduled retry", i)
		assert.Equal(t, now.Add(retryBackoffSchedule[i-1]), *got)
	}
	assert.Nil(t, nextRetryAt(len(retryBackoffSchedule)+1, now), "the schedule must be exhausted past its last entry")
}

// backdateNextRetry directly rewrites a delivery's NextRetryAt to a moment
// in the past, standing in for "real time has passed" — the shortest real
// backoff interval is a minute, far too slow for a unit test to actually
// wait out. Reaches into the DB directly (there's no repository method for
// this — production code only ever sets NextRetryAt forward, via
// recordDelivery) since this is purely a test-setup concern.
//
// .UTC(): this is a raw single-column Update against an empty
// &db.NotificationDelivery{} model, so NotificationDelivery.BeforeSave
// never sees this value — same reasoning as
// TokenRepositoryDB.IncrementUsageCount's identical .UTC() call, needed so
// the stored value compares correctly against FindDeliveriesDueForRetry's
// now.UTC().
func backdateNextRetry(t *testing.T, deliveryID string) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Minute)
	err := db.GetConnection().Model(&db.NotificationDelivery{}).
		Where("id = ?", deliveryID).Update("next_retry_at", past).Error
	require.NoError(t, err)
}

func TestService_RetryFailedDeliveries(t *testing.T) {
	t.Run("resends a due failed delivery and records a new successful attempt", func(t *testing.T) {
		port := freeTCPPort(t) // nothing listens yet, so the first attempt fails
		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			Email: config.EmailNotificationConfig{Enabled: true, Host: "127.0.0.1", Port: port, From: "gw@example.com"},
		})
		user := &db.User{Username: "retry-ok-u", Email: "retry-ok@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{
			UserID: user.ID, Type: "t", Title: "T", Body: "B",
			Channels: []string{db.NotificationChannelEmail},
		})
		require.NoError(t, err)

		deliveries, err := repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		require.Len(t, deliveries, 1)
		assert.Equal(t, db.NotificationDeliveryStatusFailed, deliveries[0].Status)
		assert.Equal(t, 1, deliveries[0].AttemptNumber)
		require.NotNil(t, deliveries[0].NextRetryAt, "a failed first attempt must schedule a retry")

		// Not due yet — RetryFailedDeliveries right now must be a no-op.
		attempted, err := service.RetryFailedDeliveries(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, attempted)

		backdateNextRetry(t, deliveries[0].ID)
		smtpServer := newFakeSMTPServerOnPort(t, port) // now something's listening
		defer smtpServer.close()

		attempted, err = service.RetryFailedDeliveries(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, attempted)

		deliveries, err = repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		require.Len(t, deliveries, 2, "a retry appends a new row, it doesn't overwrite the failed one")
		assert.Equal(t, db.NotificationDeliveryStatusSent, deliveries[0].Status, "newest first")
		assert.Equal(t, 2, deliveries[0].AttemptNumber)
		assert.Nil(t, deliveries[0].NextRetryAt)
		assert.Equal(t, db.NotificationDeliveryStatusFailed, deliveries[1].Status, "the original failed attempt is preserved as history")
	})

	t.Run("stops retrying once the schedule is exhausted", func(t *testing.T) {
		port := freeTCPPort(t) // nothing ever listens — every attempt fails
		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			Email: config.EmailNotificationConfig{Enabled: true, Host: "127.0.0.1", Port: port, From: "gw@example.com"},
		})
		user := &db.User{Username: "retry-exhaust-u", Email: "retry-exhaust@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{
			UserID: user.ID, Type: "t", Title: "T", Body: "B",
			Channels: []string{db.NotificationChannelEmail},
		})
		require.NoError(t, err)

		totalAttempts := len(retryBackoffSchedule) + 1 // the initial attempt plus every scheduled retry
		for i := 1; i < totalAttempts; i++ {
			latest, err := repo.FindLatestDelivery(n.ID, db.NotificationChannelEmail)
			require.NoError(t, err)
			require.NotNilf(t, latest.NextRetryAt, "attempt %d should still have a pending retry", i)

			backdateNextRetry(t, latest.ID)
			attempted, err := service.RetryFailedDeliveries(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 1, attempted)
		}

		latest, err := repo.FindLatestDelivery(n.ID, db.NotificationChannelEmail)
		require.NoError(t, err)
		assert.Equal(t, totalAttempts, latest.AttemptNumber)
		assert.Equal(t, db.NotificationDeliveryStatusFailed, latest.Status)
		assert.Nil(t, latest.NextRetryAt, "no further retry should be scheduled once the backoff schedule is exhausted")

		due, err := repo.FindDeliveriesDueForRetry(time.Now().Add(24 * time.Hour))
		require.NoError(t, err)
		assert.Empty(t, due, "an exhausted delivery must never become due again")
	})

	t.Run("records skipped, not failed, when the channel is no longer usable by retry time", func(t *testing.T) {
		service, repo, userRepo := newTestService(t, config.NotificationConfig{}) // nothing configured
		user := &db.User{Username: "retry-skip-u", Email: "retry-skip@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n, err := service.Create(context.Background(), CreateInput{UserID: user.ID, Type: "t", Title: "T", Body: "B"})
		require.NoError(t, err)

		// Simulate a channel that was configured when it first failed but
		// no longer is by the time its retry comes due.
		past := time.Now().Add(-time.Minute)
		require.NoError(t, repo.CreateDelivery(&db.NotificationDelivery{
			NotificationID: n.ID, Channel: db.NotificationChannelTelegram,
			Status: db.NotificationDeliveryStatusFailed, AttemptNumber: 1, NextRetryAt: &past,
		}))

		attempted, err := service.RetryFailedDeliveries(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, attempted)

		latest, err := repo.FindLatestDelivery(n.ID, db.NotificationChannelTelegram)
		require.NoError(t, err)
		assert.Equal(t, db.NotificationDeliveryStatusSkipped, latest.Status)
		assert.Equal(t, 2, latest.AttemptNumber)
		assert.Nil(t, latest.NextRetryAt, "a skip must not itself schedule a further retry")
	})
}

func TestService_ListDeliveries(t *testing.T) {
	service, _, userRepo := newTestService(t, config.NotificationConfig{})
	owner := &db.User{Username: "deliveries-owner", Email: "deliveries-owner@example.com"}
	require.NoError(t, userRepo.CreateUser(owner))
	stranger := &db.User{Username: "deliveries-stranger", Email: "deliveries-stranger@example.com"}
	require.NoError(t, userRepo.CreateUser(stranger))

	n, err := service.Create(context.Background(), CreateInput{UserID: owner.ID, Type: "t", Title: "T", Body: "B"})
	require.NoError(t, err)

	t.Run("the owner can list their own notification's deliveries", func(t *testing.T) {
		_, err := service.ListDeliveries(n.ID, owner.ID, false)
		assert.NoError(t, err)
	})

	t.Run("a stranger is forbidden", func(t *testing.T) {
		_, err := service.ListDeliveries(n.ID, stranger.ID, false)
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("an admin can, regardless of ownership", func(t *testing.T) {
		_, err := service.ListDeliveries(n.ID, "admin-id", true)
		assert.NoError(t, err)
	})

	t.Run("a nonexistent notification is not found", func(t *testing.T) {
		_, err := service.ListDeliveries("nonexistent-id", owner.ID, false)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}
