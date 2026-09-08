package notification

import (
	"context"
	"testing"

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
