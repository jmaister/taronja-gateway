package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupNotificationTestServer(t *testing.T) (*StrictApiServer, *deps.Dependencies) {
	t.Helper()
	dependencies := deps.NewTestWithName(t.Name())
	server := &StrictApiServer{
		userRepo:            dependencies.UserRepo,
		notificationService: dependencies.NotificationService,
	}
	return server, dependencies
}

func sessionContext(userID string, isAdmin bool) context.Context {
	sess := &db.Session{Token: "x", UserID: userID, IsAuthenticated: true, IsAdmin: isAdmin, ValidUntil: time.Now().Add(time.Hour)}
	return context.WithValue(context.Background(), session.SessionKey, sess)
}

func TestCreateNotification(t *testing.T) {
	server, dependencies := setupNotificationTestServer(t)
	user := &db.User{Username: "recipient", Email: "recipient@example.com"}
	require.NoError(t, dependencies.UserRepo.CreateUser(user))

	t.Run("rejects a non-admin caller", func(t *testing.T) {
		resp, err := server.CreateNotification(sessionContext(user.ID, false), api.CreateNotificationRequestObject{
			Body: &api.CreateNotificationJSONRequestBody{UserId: user.ID, Type: "t", Title: "T", Body: "B"},
		})
		require.NoError(t, err)
		_, ok := resp.(api.CreateNotification401JSONResponse)
		assert.True(t, ok)
	})

	t.Run("rejects an unauthenticated caller", func(t *testing.T) {
		resp, err := server.CreateNotification(context.Background(), api.CreateNotificationRequestObject{
			Body: &api.CreateNotificationJSONRequestBody{UserId: user.ID, Type: "t", Title: "T", Body: "B"},
		})
		require.NoError(t, err)
		_, ok := resp.(api.CreateNotification401JSONResponse)
		assert.True(t, ok)
	})

	t.Run("an admin caller creates a notification with actions", func(t *testing.T) {
		style := "primary"
		resp, err := server.CreateNotification(sessionContext("admin-id", true), api.CreateNotificationRequestObject{
			Body: &api.CreateNotificationJSONRequestBody{
				UserId: user.ID, Type: "track_added", Title: "New track", Body: "body",
				Actions: &[]api.NotificationAction{{Id: "approve", Label: "Approve", Style: &style}},
			},
		})
		require.NoError(t, err)
		created, ok := resp.(api.CreateNotification201JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "track_added", created.Type)
		require.NotNil(t, created.Actions)
		require.Len(t, *created.Actions, 1)
		assert.Equal(t, "approve", (*created.Actions)[0].Id)
	})
}

func TestListAndReadNotifications(t *testing.T) {
	server, dependencies := setupNotificationTestServer(t)
	user := &db.User{Username: "u", Email: "u@example.com"}
	require.NoError(t, dependencies.UserRepo.CreateUser(user))

	adminCtx := sessionContext("admin-id", true)
	for i := 0; i < 2; i++ {
		_, err := server.CreateNotification(adminCtx, api.CreateNotificationRequestObject{
			Body: &api.CreateNotificationJSONRequestBody{UserId: user.ID, Type: "t", Title: "T", Body: "B"},
		})
		require.NoError(t, err)
	}

	userCtx := sessionContext(user.ID, false)

	t.Run("ListNotifications returns only the caller's own notifications", func(t *testing.T) {
		resp, err := server.ListNotifications(userCtx, api.ListNotificationsRequestObject{})
		require.NoError(t, err)
		list, ok := resp.(api.ListNotifications200JSONResponse)
		require.True(t, ok)
		assert.Len(t, list.Notifications, 2)
	})

	t.Run("GetUnreadNotificationCount reflects unread state", func(t *testing.T) {
		resp, err := server.GetUnreadNotificationCount(userCtx, api.GetUnreadNotificationCountRequestObject{})
		require.NoError(t, err)
		count, ok := resp.(api.GetUnreadNotificationCount200JSONResponse)
		require.True(t, ok)
		assert.Equal(t, 2, count.Count)
	})

	t.Run("MarkAllNotificationsRead zeroes the unread count", func(t *testing.T) {
		_, err := server.MarkAllNotificationsRead(userCtx, api.MarkAllNotificationsReadRequestObject{})
		require.NoError(t, err)
		resp, err := server.GetUnreadNotificationCount(userCtx, api.GetUnreadNotificationCountRequestObject{})
		require.NoError(t, err)
		count := resp.(api.GetUnreadNotificationCount200JSONResponse)
		assert.Equal(t, 0, count.Count)
	})
}

func TestRespondToNotification(t *testing.T) {
	server, dependencies := setupNotificationTestServer(t)
	owner := &db.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, dependencies.UserRepo.CreateUser(owner))
	stranger := &db.User{Username: "stranger", Email: "stranger@example.com"}
	require.NoError(t, dependencies.UserRepo.CreateUser(stranger))

	createResp, err := server.CreateNotification(sessionContext("admin-id", true), api.CreateNotificationRequestObject{
		Body: &api.CreateNotificationJSONRequestBody{
			UserId: owner.ID, Type: "t", Title: "Approve?", Body: "B",
			Actions: &[]api.NotificationAction{{Id: "approve", Label: "Approve"}},
		},
	})
	require.NoError(t, err)
	created := createResp.(api.CreateNotification201JSONResponse)

	t.Run("a stranger gets 403", func(t *testing.T) {
		resp, err := server.RespondToNotification(sessionContext(stranger.ID, false), api.RespondToNotificationRequestObject{
			NotificationId: created.Id,
			Body:           &api.RespondToNotificationJSONRequestBody{ActionId: "approve"},
		})
		require.NoError(t, err)
		_, ok := resp.(api.RespondToNotification403JSONResponse)
		assert.True(t, ok)
	})

	t.Run("an invalid action ID gets 422", func(t *testing.T) {
		resp, err := server.RespondToNotification(sessionContext(owner.ID, false), api.RespondToNotificationRequestObject{
			NotificationId: created.Id,
			Body:           &api.RespondToNotificationJSONRequestBody{ActionId: "nope"},
		})
		require.NoError(t, err)
		_, ok := resp.(api.RespondToNotification422JSONResponse)
		assert.True(t, ok)
	})

	t.Run("the owner can respond once, and a second attempt gets 409", func(t *testing.T) {
		resp, err := server.RespondToNotification(sessionContext(owner.ID, false), api.RespondToNotificationRequestObject{
			NotificationId: created.Id,
			Body:           &api.RespondToNotificationJSONRequestBody{ActionId: "approve"},
		})
		require.NoError(t, err)
		updated, ok := resp.(api.RespondToNotification200JSONResponse)
		require.True(t, ok)
		require.NotNil(t, updated.RespondedActionId)
		assert.Equal(t, "approve", *updated.RespondedActionId)

		resp, err = server.RespondToNotification(sessionContext(owner.ID, false), api.RespondToNotificationRequestObject{
			NotificationId: created.Id,
			Body:           &api.RespondToNotificationJSONRequestBody{ActionId: "approve"},
		})
		require.NoError(t, err)
		_, ok = resp.(api.RespondToNotification409JSONResponse)
		assert.True(t, ok)
	})
}

func TestGetTelegramLinkCode(t *testing.T) {
	server, _ := setupNotificationTestServer(t)

	t.Run("returns 503 when telegram isn't configured", func(t *testing.T) {
		resp, err := server.GetTelegramLinkCode(sessionContext("u1", false), api.GetTelegramLinkCodeRequestObject{})
		require.NoError(t, err)
		_, ok := resp.(api.GetTelegramLinkCode503JSONResponse)
		assert.True(t, ok)
	})

	t.Run("returns 401 when unauthenticated", func(t *testing.T) {
		resp, err := server.GetTelegramLinkCode(context.Background(), api.GetTelegramLinkCodeRequestObject{})
		require.NoError(t, err)
		_, ok := resp.(api.GetTelegramLinkCode401JSONResponse)
		assert.True(t, ok)
	})
}

func TestListNotificationDeliveries(t *testing.T) {
	server, dependencies := setupNotificationTestServer(t)
	owner := &db.User{Username: "deliveries-owner", Email: "deliveries-owner@example.com"}
	require.NoError(t, dependencies.UserRepo.CreateUser(owner))
	stranger := &db.User{Username: "deliveries-stranger", Email: "deliveries-stranger@example.com"}
	require.NoError(t, dependencies.UserRepo.CreateUser(stranger))

	createResp, err := server.CreateNotification(sessionContext("admin-id", true), api.CreateNotificationRequestObject{
		Body: &api.CreateNotificationJSONRequestBody{UserId: owner.ID, Type: "t", Title: "T", Body: "B"},
	})
	require.NoError(t, err)
	created := createResp.(api.CreateNotification201JSONResponse)

	// No external channels are configured in the test dependencies, so
	// this notification has no delivery attempts at all — the owner
	// should still get a 200 with an empty list, not an error.
	t.Run("the owner sees an empty delivery list when nothing was attempted", func(t *testing.T) {
		resp, err := server.ListNotificationDeliveries(sessionContext(owner.ID, false), api.ListNotificationDeliveriesRequestObject{
			NotificationId: created.Id,
		})
		require.NoError(t, err)
		list, ok := resp.(api.ListNotificationDeliveries200JSONResponse)
		require.True(t, ok)
		assert.Empty(t, list.Deliveries)
	})

	t.Run("a stranger gets 403", func(t *testing.T) {
		resp, err := server.ListNotificationDeliveries(sessionContext(stranger.ID, false), api.ListNotificationDeliveriesRequestObject{
			NotificationId: created.Id,
		})
		require.NoError(t, err)
		_, ok := resp.(api.ListNotificationDeliveries403JSONResponse)
		assert.True(t, ok)
	})

	t.Run("a nonexistent notification gets 404", func(t *testing.T) {
		resp, err := server.ListNotificationDeliveries(sessionContext(owner.ID, false), api.ListNotificationDeliveriesRequestObject{
			NotificationId: "nonexistent-id",
		})
		require.NoError(t, err)
		_, ok := resp.(api.ListNotificationDeliveries404JSONResponse)
		assert.True(t, ok)
	})

	t.Run("unauthenticated gets 401", func(t *testing.T) {
		resp, err := server.ListNotificationDeliveries(context.Background(), api.ListNotificationDeliveriesRequestObject{
			NotificationId: created.Id,
		})
		require.NoError(t, err)
		_, ok := resp.(api.ListNotificationDeliveries401JSONResponse)
		assert.True(t, ok)
	})

	t.Run("an admin can see a stranger's delivery history", func(t *testing.T) {
		resp, err := server.ListNotificationDeliveries(sessionContext("admin-id", true), api.ListNotificationDeliveriesRequestObject{
			NotificationId: created.Id,
		})
		require.NoError(t, err)
		_, ok := resp.(api.ListNotificationDeliveries200JSONResponse)
		assert.True(t, ok)
	})
}
