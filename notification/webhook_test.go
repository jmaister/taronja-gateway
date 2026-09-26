package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseWebhookProvider(t *testing.T) {
	t.Run("NewResponseWebhookProvider returns nil when not configured", func(t *testing.T) {
		assert.Nil(t, NewResponseWebhookProvider(config.ResponseWebhookConfig{}))
		assert.Nil(t, NewResponseWebhookProvider(config.ResponseWebhookConfig{Enabled: true})) // no URL
	})

	t.Run("Send POSTs the response payload, signed, when a secret is configured", func(t *testing.T) {
		var receivedBody []byte
		var receivedSignature string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedSignature = r.Header.Get("X-Taronja-Signature")
			receivedBody, _ = io.ReadAll(r.Body)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := NewResponseWebhookProvider(config.ResponseWebhookConfig{Enabled: true, URL: server.URL, Secret: "shh"})
		require.NotNil(t, provider)
		assert.Equal(t, "response_webhook", provider.Channel())

		actionID := "approve"
		via := "email"
		respondedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		notif := &db.Notification{
			ID: "notif-1", UserID: "user-1", Type: "music_track_added",
			Metadata:          `{"folderId":"f1"}`,
			RespondedActionID: &actionID,
			RespondedVia:      &via,
			RespondedAt:       &respondedAt,
		}

		externalRef, err := provider.Send(context.Background(), SendRequest{Notification: notif})
		require.NoError(t, err)
		assert.Empty(t, externalRef)

		var payload responseWebhookPayload
		require.NoError(t, json.Unmarshal(receivedBody, &payload))
		assert.Equal(t, "notif-1", payload.NotificationID)
		assert.Equal(t, "user-1", payload.UserID)
		assert.Equal(t, "music_track_added", payload.Type)
		assert.Equal(t, "approve", payload.RespondedActionID)
		assert.Equal(t, "email", payload.RespondedVia)
		assert.True(t, respondedAt.Equal(payload.RespondedAt))
		require.NotNil(t, payload.Metadata)
		assert.Equal(t, "f1", payload.Metadata["folderId"])

		mac := hmac.New(sha256.New, []byte("shh"))
		mac.Write(receivedBody)
		wantSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		assert.Equal(t, wantSignature, receivedSignature)
	})

	t.Run("Send omits the signature header when no secret is configured", func(t *testing.T) {
		var sawSignatureHeader bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, sawSignatureHeader = r.Header["X-Taronja-Signature"]
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := NewResponseWebhookProvider(config.ResponseWebhookConfig{Enabled: true, URL: server.URL})
		require.NotNil(t, provider)

		actionID, via := "approve", "web"
		now := time.Now()
		_, err := provider.Send(context.Background(), SendRequest{Notification: &db.Notification{
			ID: "n", UserID: "u", RespondedActionID: &actionID, RespondedVia: &via, RespondedAt: &now,
		}})
		require.NoError(t, err)
		assert.False(t, sawSignatureHeader)
	})

	t.Run("Send fails when the notification has no recorded response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("must not be called")
		}))
		defer server.Close()

		provider := NewResponseWebhookProvider(config.ResponseWebhookConfig{Enabled: true, URL: server.URL})
		require.NotNil(t, provider)

		_, err := provider.Send(context.Background(), SendRequest{Notification: &db.Notification{ID: "n", UserID: "u"}})
		assert.Error(t, err)
	})

	t.Run("Send fails when the receiver returns a non-2xx status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		provider := NewResponseWebhookProvider(config.ResponseWebhookConfig{Enabled: true, URL: server.URL})
		require.NotNil(t, provider)

		actionID, via := "approve", "web"
		now := time.Now()
		_, err := provider.Send(context.Background(), SendRequest{Notification: &db.Notification{
			ID: "n", UserID: "u", RespondedActionID: &actionID, RespondedVia: &via, RespondedAt: &now,
		}})
		assert.Error(t, err)
	})
}

func TestService_ResponseWebhook(t *testing.T) {
	t.Run("fires exactly once, with the actual response, when a user answers", func(t *testing.T) {
		var received responseWebhookPayload
		var callCount int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &received)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		service, _, userRepo := newTestService(t, config.NotificationConfig{
			ResponseWebhook: config.ResponseWebhookConfig{Enabled: true, URL: server.URL},
		})
		user := &db.User{Username: "webhook-user", Email: "webhook-user@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n := createOne(t, service, CreateInput{
			UserIDs: []string{user.ID}, Type: "music_track_added", Title: "New track", Body: "B",
			Actions: []Action{{ID: "approve", Label: "Approve"}},
		})

		label, err := service.RespondViaWeb(context.Background(), n.ID, user.ID, "approve")
		require.NoError(t, err)
		assert.Equal(t, "Approve", label)

		require.Equal(t, 1, callCount)
		assert.Equal(t, n.ID, received.NotificationID)
		assert.Equal(t, user.ID, received.UserID)
		assert.Equal(t, "music_track_added", received.Type)
		assert.Equal(t, "approve", received.RespondedActionID)
		assert.Equal(t, db.NotificationChannelWeb, received.RespondedVia)
	})

	t.Run("never fires as part of a notification's own initial delivery", func(t *testing.T) {
		var callCount int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			ResponseWebhook: config.ResponseWebhookConfig{Enabled: true, URL: server.URL},
		})
		user := &db.User{Username: "webhook-no-fire-user", Email: "webhook-no-fire@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n := createOne(t, service, CreateInput{UserIDs: []string{user.ID}, Type: "t", Title: "T", Body: "B"})

		assert.Equal(t, 0, callCount, "creating a notification must never itself trigger the response webhook")
		deliveries, err := repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		assert.Empty(t, deliveries, "response_webhook must not appear as one of the 'every configured channel' defaults")
	})

	t.Run("is invisible (no delivery row at all) when not configured", func(t *testing.T) {
		service, repo, userRepo := newTestService(t, config.NotificationConfig{}) // no webhook configured
		user := &db.User{Username: "webhook-unconfigured-user", Email: "webhook-unconfigured@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n := createOne(t, service, CreateInput{
			UserIDs: []string{user.ID}, Type: "t", Title: "T", Body: "B",
			Actions: []Action{{ID: "approve", Label: "Approve"}},
		})
		_, err := service.RespondViaWeb(context.Background(), n.ID, user.ID, "approve")
		require.NoError(t, err)

		deliveries, err := repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		assert.Empty(t, deliveries, "no webhook configured means no attempt is recorded at all, not a skip")
	})

	t.Run("a failed webhook call retries with the same backoff/history machinery as any other delivery", func(t *testing.T) {
		port := freeTCPPort(t) // nothing listens yet, so the first attempt fails
		service, repo, userRepo := newTestService(t, config.NotificationConfig{
			ResponseWebhook: config.ResponseWebhookConfig{Enabled: true, URL: fmt.Sprintf("http://127.0.0.1:%d/hook", port)},
		})
		user := &db.User{Username: "webhook-retry-user", Email: "webhook-retry@example.com"}
		require.NoError(t, userRepo.CreateUser(user))

		n := createOne(t, service, CreateInput{
			UserIDs: []string{user.ID}, Type: "t", Title: "T", Body: "B",
			Actions: []Action{{ID: "approve", Label: "Approve"}},
		})
		_, err := service.RespondViaWeb(context.Background(), n.ID, user.ID, "approve")
		require.NoError(t, err)

		deliveries, err := repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		require.Len(t, deliveries, 1)
		assert.Equal(t, db.NotificationChannelResponseWebhook, deliveries[0].Channel)
		assert.Equal(t, db.NotificationDeliveryStatusFailed, deliveries[0].Status)
		require.NotNil(t, deliveries[0].NextRetryAt, "a failed webhook call must be scheduled for retry, same as any other channel")

		var received responseWebhookPayload
		receiverUp := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &received)
			w.WriteHeader(http.StatusOK)
		}))
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		receiverUp.Listener = listener
		receiverUp.Start()
		defer receiverUp.Close()

		backdateNextRetry(t, deliveries[0].ID)
		attempted, err := service.RetryFailedDeliveries(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, attempted)

		deliveries, err = repo.ListDeliveries(n.ID)
		require.NoError(t, err)
		require.Len(t, deliveries, 2)
		assert.Equal(t, db.NotificationDeliveryStatusSent, deliveries[0].Status)
		assert.Equal(t, "approve", received.RespondedActionID, "the retry must still report the actual response, not an empty one")
	})
}
