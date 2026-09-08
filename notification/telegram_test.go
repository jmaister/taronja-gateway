package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTelegramAPI is a real HTTP server implementing just enough of
// Telegram's Bot API (sendMessage, getMe, getUpdates, answerCallbackQuery,
// editMessageText) to drive TelegramProvider and TelegramPoller against
// real JSON over real HTTP, the same "fake but real protocol" style
// email_test.go's fakeSMTPServer uses.
type fakeTelegramAPI struct {
	*httptest.Server
	mu            sync.Mutex
	sentMessages  []map[string]interface{}
	editedTexts   []map[string]interface{}
	answeredCbs   []map[string]interface{}
	nextMessageID int
	updates       []telegramUpdate // queued updates returned once each, in order, by getUpdates
}

func newFakeTelegramAPI(t *testing.T) *fakeTelegramAPI {
	t.Helper()
	api := &fakeTelegramAPI{nextMessageID: 1}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)

		switch {
		case matchMethod(r.URL.Path, "sendMessage"):
			api.mu.Lock()
			api.sentMessages = append(api.sentMessages, body)
			msgID := api.nextMessageID
			api.nextMessageID++
			api.mu.Unlock()
			fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, msgID)
		case matchMethod(r.URL.Path, "getMe"):
			fmt.Fprint(w, `{"ok":true,"result":{"username":"taronja_test_bot"}}`)
		case matchMethod(r.URL.Path, "getUpdates"):
			api.mu.Lock()
			pending := api.updates
			api.updates = nil
			api.mu.Unlock()
			resp := telegramGetUpdatesResult{OK: true, Result: pending}
			_ = json.NewEncoder(w).Encode(resp)
		case matchMethod(r.URL.Path, "answerCallbackQuery"):
			api.mu.Lock()
			api.answeredCbs = append(api.answeredCbs, body)
			api.mu.Unlock()
			fmt.Fprint(w, `{"ok":true}`)
		case matchMethod(r.URL.Path, "editMessageText"):
			api.mu.Lock()
			api.editedTexts = append(api.editedTexts, body)
			api.mu.Unlock()
			fmt.Fprint(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	})
	api.Server = httptest.NewServer(mux)
	t.Cleanup(api.Server.Close)
	return api
}

func matchMethod(path, method string) bool {
	// Real paths look like /bot<token>/<method>.
	return len(path) >= len(method) && path[len(path)-len(method):] == method
}

func (api *fakeTelegramAPI) queueUpdate(u telegramUpdate) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.updates = append(api.updates, u)
}

// mustDecodeUpdate builds a telegramUpdate from a JSON literal — simpler
// and less fragile than constructing telegramUpdate's anonymous nested
// struct fields by hand, since Go requires those literals to match the
// unexported struct's exact shape.
func mustDecodeUpdate(t *testing.T, jsonStr string) telegramUpdate {
	t.Helper()
	var u telegramUpdate
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &u))
	return u
}

func withFakeTelegramAPI(t *testing.T, fn func(api *fakeTelegramAPI)) {
	t.Helper()
	api := newFakeTelegramAPI(t)
	original := telegramAPIBaseURL
	telegramAPIBaseURL = api.Server.URL
	t.Cleanup(func() { telegramAPIBaseURL = original })
	fn(api)
}

func TestTelegramProvider(t *testing.T) {
	t.Run("NewTelegramProvider returns nil when not configured", func(t *testing.T) {
		assert.Nil(t, NewTelegramProvider(config.TelegramNotificationConfig{}))
		assert.Nil(t, NewTelegramProvider(config.TelegramNotificationConfig{Enabled: true}))
	})

	withFakeTelegramAPI(t, func(api *fakeTelegramAPI) {
		provider := NewTelegramProvider(config.TelegramNotificationConfig{Enabled: true, BotToken: "test-token"})
		require.NotNil(t, provider)
		assert.Equal(t, "telegram", provider.Channel())

		t.Run("Send delivers a plain message and returns chat:messageID as externalRef", func(t *testing.T) {
			notif := &db.Notification{ID: "n1", Title: "Track added", Body: "Details here."}
			ref, err := provider.Send(context.Background(), SendRequest{Notification: notif, Recipient: "12345"})
			require.NoError(t, err)
			assert.Equal(t, "12345:1", ref)

			api.mu.Lock()
			defer api.mu.Unlock()
			require.Len(t, api.sentMessages, 1)
			assert.Equal(t, "12345", api.sentMessages[0]["chat_id"])
			assert.Contains(t, api.sentMessages[0]["text"], "Track added")
			assert.NotContains(t, api.sentMessages[0], "reply_markup")
		})

		t.Run("Send with actions attaches an inline keyboard keyed by notification|action", func(t *testing.T) {
			notif := &db.Notification{ID: "n2", Title: "Approve?", Body: "Please decide."}
			_, err := provider.Send(context.Background(), SendRequest{
				Notification: notif, Recipient: "999",
				Actions: []Action{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}},
			})
			require.NoError(t, err)

			api.mu.Lock()
			last := api.sentMessages[len(api.sentMessages)-1]
			api.mu.Unlock()
			markupJSON, err := json.Marshal(last["reply_markup"])
			require.NoError(t, err)
			var markup telegramInlineKeyboardMarkup
			require.NoError(t, json.Unmarshal(markupJSON, &markup))
			require.Len(t, markup.InlineKeyboard, 1)
			require.Len(t, markup.InlineKeyboard[0], 2)
			assert.Equal(t, "n2|yes", markup.InlineKeyboard[0][0].CallbackData)
			assert.Equal(t, "n2|no", markup.InlineKeyboard[0][1].CallbackData)
		})

		t.Run("Username fetches and caches via getMe", func(t *testing.T) {
			username, err := provider.Username(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "taronja_test_bot", username)
		})
	})

	t.Run("parseCallbackData round-trips and rejects malformed input", func(t *testing.T) {
		data := callbackData("notif-123", "approve")
		assert.Equal(t, "notif-123|approve", data)
		id, action, ok := parseCallbackData(data)
		require.True(t, ok)
		assert.Equal(t, "notif-123", id)
		assert.Equal(t, "approve", action)

		_, _, ok = parseCallbackData("no-pipe-here")
		assert.False(t, ok)
		_, _, ok = parseCallbackData("|missing-id")
		assert.False(t, ok)
	})
}

// TestTelegramPoller builds a real Service against a real SQLite test
// database (db.SetupTestDB) rather than a mock repository, consistent with
// this codebase's existing preference for a real repository over a
// hand-rolled mock wherever the real one is cheap to stand up (see
// db/notificationrepository_test.go).
func TestTelegramPoller(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := db.NewNotificationRepositoryDB(db.GetConnection())
	userRepo := db.NewDBUserRepository(db.GetConnection())

	user := &db.User{Username: "poller-user", Email: "poller@example.com"}
	require.NoError(t, db.GetConnection().Create(user).Error)

	withFakeTelegramAPI(t, func(api *fakeTelegramAPI) {
		service := NewService(config.NotificationConfig{
			Telegram: config.TelegramNotificationConfig{Enabled: true, BotToken: "test-token"},
		}, repo, userRepo, "https://gw.example.com/_/notifications/respond")
		poller := service.TelegramPoller()
		require.NotNil(t, poller)

		ctx, cancel := context.WithCancel(context.Background())
		go poller.Run(ctx)
		t.Cleanup(cancel)

		t.Run("a /start <code> message links the chat to the user", func(t *testing.T) {
			deepLink, _, err := service.GetTelegramLinkCode(context.Background(), user.ID)
			require.NoError(t, err)
			require.Contains(t, deepLink, "https://t.me/taronja_test_bot?start=")
			code := deepLink[len("https://t.me/taronja_test_bot?start="):]

			update := mustDecodeUpdate(t, fmt.Sprintf(`{"update_id":1,"message":{"chat":{"id":555},"text":"/start %s"}}`, code))
			api.queueUpdate(update)

			require.Eventually(t, func() bool {
				link, err := repo.FindChannelLink(user.ID, db.NotificationChannelTelegram)
				return err == nil && link.ExternalID == "555"
			}, 3*time.Second, 20*time.Millisecond, "chat 555 should become linked to the user")
		})

		t.Run("a callback query records the response and edits the message", func(t *testing.T) {
			notif, err := service.Create(context.Background(), CreateInput{
				UserID: user.ID, Type: "t", Title: "Approve?", Body: "b",
				Actions:  []Action{{ID: "approve", Label: "Approve"}},
				Channels: []string{db.NotificationChannelTelegram},
			})
			require.NoError(t, err)

			update := mustDecodeUpdate(t, fmt.Sprintf(
				`{"update_id":2,"callback_query":{"id":"cbq-1","data":%q,"from":{"id":555},"message":{"message_id":42,"chat":{"id":555}}}}`,
				callbackData(notif.ID, "approve")))
			api.queueUpdate(update)

			require.Eventually(t, func() bool {
				n, err := repo.GetNotification(notif.ID)
				return err == nil && n.RespondedAt != nil
			}, 3*time.Second, 20*time.Millisecond, "the response should be recorded")

			n, err := repo.GetNotification(notif.ID)
			require.NoError(t, err)
			require.NotNil(t, n.RespondedActionID)
			assert.Equal(t, "approve", *n.RespondedActionID)
			require.NotNil(t, n.RespondedVia)
			assert.Equal(t, db.NotificationChannelTelegram, *n.RespondedVia)

			require.Eventually(t, func() bool {
				api.mu.Lock()
				defer api.mu.Unlock()
				return len(api.editedTexts) > 0
			}, 3*time.Second, 20*time.Millisecond, "the message should be edited after response")
		})

		t.Run("a callback query from a chat that isn't the notification's owner is rejected", func(t *testing.T) {
			other := &db.User{Username: "other-user", Email: "other@example.com"}
			require.NoError(t, db.GetConnection().Create(other).Error)
			notif, err := service.Create(context.Background(), CreateInput{
				UserID: other.ID, Type: "t", Title: "Approve?", Body: "b",
				Actions:  []Action{{ID: "approve", Label: "Approve"}},
				Channels: []string{db.NotificationChannelTelegram},
			})
			require.NoError(t, err)

			// Chat 555 is linked to `user`, not `other` — tapping "approve"
			// from it must not answer other's notification.
			update := mustDecodeUpdate(t, fmt.Sprintf(
				`{"update_id":3,"callback_query":{"id":"cbq-2","data":%q,"from":{"id":555},"message":{"message_id":1,"chat":{"id":555}}}}`,
				callbackData(notif.ID, "approve")))
			api.queueUpdate(update)

			require.Eventually(t, func() bool {
				api.mu.Lock()
				defer api.mu.Unlock()
				for _, a := range api.answeredCbs {
					if a["callback_query_id"] == "cbq-2" {
						return true
					}
				}
				return false
			}, 3*time.Second, 20*time.Millisecond, "the callback should be answered (with a rejection toast)")

			n, err := repo.GetNotification(notif.ID)
			require.NoError(t, err)
			assert.Nil(t, n.RespondedAt, "an unauthorized chat must not be able to record a response")
		})
	})
}
