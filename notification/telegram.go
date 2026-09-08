package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

// telegramAPIBaseURL is a var, not a const, so tests can point the
// provider and poller at a fake HTTP server instead of Telegram's real
// API — the same reasoning providers/apple.go's appleJWKSURL documents.
var telegramAPIBaseURL = "https://api.telegram.org"

// TelegramProvider delivers notifications as Telegram bot messages, with
// Actions rendered as an inline keyboard. Recipients are chat IDs (see
// db.NotificationChannelLink), not usernames or phone numbers — Telegram's
// Bot API has no way to message a user who hasn't first started a chat
// with the bot, which is exactly what the linking flow in Poller
// establishes.
type TelegramProvider struct {
	botToken   string
	httpClient *http.Client

	usernameMu sync.Mutex
	username   string // cached result of Username, fetched at most once
}

// NewTelegramProvider returns nil if cfg isn't configured (see
// TelegramNotificationConfig.IsConfigured), the same convention
// NewEmailProvider follows.
func NewTelegramProvider(cfg config.TelegramNotificationConfig) *TelegramProvider {
	if !cfg.IsConfigured() {
		return nil
	}
	return &TelegramProvider{botToken: cfg.BotToken, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (p *TelegramProvider) Channel() string { return "telegram" }

// telegramInlineKeyboardButton and the wrapping types mirror just enough
// of Telegram's Bot API JSON shape for sendMessage's reply_markup — see
// https://core.telegram.org/bots/api#inlinekeyboardmarkup.
type telegramInlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type telegramInlineKeyboardMarkup struct {
	InlineKeyboard [][]telegramInlineKeyboardButton `json:"inline_keyboard"`
}

type telegramSendMessageResult struct {
	OK     bool `json:"ok"`
	Result struct {
		MessageID int `json:"message_id"`
	} `json:"result"`
	Description string `json:"description"`
}

// callbackData encodes which notification and which action a button tap
// answers. Kept as a plain delimited string (not JSON) since Telegram
// limits callback_data to 64 bytes — a cuid-based notification ID plus a
// short action ID comfortably fits; JSON's extra punctuation might not for
// a caller with a longer action ID.
func callbackData(notificationID, actionID string) string {
	return notificationID + "|" + actionID
}

func parseCallbackData(data string) (notificationID, actionID string, ok bool) {
	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (p *TelegramProvider) apiURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", telegramAPIBaseURL, p.botToken, method)
}

func (p *TelegramProvider) call(ctx context.Context, method string, payload interface{}, out interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL(method), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding telegram response: %w (body: %s)", err, respBody)
		}
	}
	return nil
}

func (p *TelegramProvider) Send(ctx context.Context, req SendRequest) (string, error) {
	text := fmt.Sprintf("%s\n\n%s", req.Notification.Title, req.Notification.Body)
	if req.Notification.URL != nil && *req.Notification.URL != "" {
		text += "\n" + *req.Notification.URL
	}

	payload := map[string]interface{}{
		"chat_id": req.Recipient,
		"text":    text,
	}
	if len(req.Actions) > 0 {
		var row []telegramInlineKeyboardButton
		for _, action := range req.Actions {
			row = append(row, telegramInlineKeyboardButton{
				Text:         action.Label,
				CallbackData: callbackData(req.Notification.ID, action.ID),
			})
		}
		payload["reply_markup"] = telegramInlineKeyboardMarkup{InlineKeyboard: [][]telegramInlineKeyboardButton{row}}
	}

	var result telegramSendMessageResult
	if err := p.call(ctx, "sendMessage", payload, &result); err != nil {
		return "", fmt.Errorf("telegram sendMessage request failed: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("telegram sendMessage rejected: %s", result.Description)
	}
	return fmt.Sprintf("%s:%d", req.Recipient, result.Result.MessageID), nil
}

// editMessageAfterResponse replaces a message's inline keyboard with plain
// confirmation text once one of its buttons has been answered — mostly to
// stop a second tap on a now-stale button rather than let it error out
// confusingly against an already-answered notification. Best-effort: a
// failure here doesn't undo the response that was already recorded, it
// just means the message in Telegram still shows its old buttons.
func (p *TelegramProvider) editMessageAfterResponse(ctx context.Context, chatID string, messageID int, chosenLabel string) {
	err := p.call(ctx, "editMessageText", map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       fmt.Sprintf("✓ %s", chosenLabel),
	}, nil)
	if err != nil {
		log.Printf("notification: failed to edit telegram message %s:%d after response: %v", chatID, messageID, err)
	}
}

func (p *TelegramProvider) answerCallbackQuery(ctx context.Context, callbackQueryID, text string) {
	err := p.call(ctx, "answerCallbackQuery", map[string]interface{}{
		"callback_query_id": callbackQueryID,
		"text":              text,
	}, nil)
	if err != nil {
		log.Printf("notification: failed to answer telegram callback query %s: %v", callbackQueryID, err)
	}
}

type telegramGetMeResult struct {
	OK     bool `json:"ok"`
	Result struct {
		Username string `json:"username"`
	} `json:"result"`
}

// Username returns the bot's own @username (without the @), used to build
// the "https://t.me/<username>?start=<code>" linking deep link — fetched
// via Telegram's getMe once and cached for the process lifetime, since a
// bot's username essentially never changes and this would otherwise be one
// extra round trip on every GetTelegramLinkCode call.
func (p *TelegramProvider) Username(ctx context.Context) (string, error) {
	p.usernameMu.Lock()
	defer p.usernameMu.Unlock()
	if p.username != "" {
		return p.username, nil
	}
	var result telegramGetMeResult
	if err := p.call(ctx, "getMe", map[string]interface{}{}, &result); err != nil {
		return "", fmt.Errorf("telegram getMe failed: %w", err)
	}
	if !result.OK || result.Result.Username == "" {
		return "", fmt.Errorf("telegram getMe returned no username")
	}
	p.username = result.Result.Username
	return p.username, nil
}

func (p *TelegramProvider) sendPlainMessage(ctx context.Context, chatID, text string) {
	err := p.call(ctx, "sendMessage", map[string]interface{}{"chat_id": chatID, "text": text}, nil)
	if err != nil {
		log.Printf("notification: failed to send telegram message to %s: %v", chatID, err)
	}
}

// --- Long-polling update loop ---

type telegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message struct {
			MessageID int `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

type telegramGetUpdatesResult struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

// TelegramPoller long-polls Telegram's getUpdates endpoint for incoming
// "/start <code>" account-linking messages and inline-keyboard button
// taps, and feeds both into a Service. Long-polling rather than a webhook
// deliberately means no inbound network exposure is required — see
// config.TelegramNotificationConfig's doc comment.
type TelegramPoller struct {
	provider *TelegramProvider
	service  *Service
}

// NewTelegramPoller returns nil if provider is nil (Telegram not
// configured) — Run becomes a safe no-op to call unconditionally is not
// how this is used; callers check for nil the same way they'd check
// whether the provider itself is nil before wiring anything up.
func NewTelegramPoller(provider *TelegramProvider, service *Service) *TelegramPoller {
	if provider == nil {
		return nil
	}
	return &TelegramPoller{provider: provider, service: service}
}

// Run polls until ctx is cancelled. Meant to be started in its own
// goroutine at gateway startup (see gateway.InitNotifications) and stopped
// by cancelling ctx during graceful shutdown.
func (p *TelegramPoller) Run(ctx context.Context) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var result telegramGetUpdatesResult
		reqCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		err := p.provider.call(reqCtx, "getUpdates", map[string]interface{}{
			"offset":  offset,
			"timeout": 30, // seconds Telegram itself long-polls for before returning an empty result
		}, &result)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return // shutting down, not a real error
			}
			log.Printf("notification: telegram getUpdates failed, retrying: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, update := range result.Result {
			offset = update.UpdateID + 1
			p.handleUpdate(ctx, update)
		}
	}
}

func (p *TelegramPoller) handleUpdate(ctx context.Context, update telegramUpdate) {
	switch {
	case update.Message != nil && strings.HasPrefix(update.Message.Text, "/start"):
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		code := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/start"))
		if code == "" {
			p.provider.sendPlainMessage(ctx, chatID, "Send the connection link from the app's notification settings to link this chat.")
			return
		}
		if err := p.service.LinkTelegramChat(code, chatID); err != nil {
			p.provider.sendPlainMessage(ctx, chatID, "That connection link is invalid or has expired — generate a new one from the app.")
			return
		}
		p.provider.sendPlainMessage(ctx, chatID, "✓ Connected. You'll receive notifications here.")

	case update.CallbackQuery != nil:
		cq := update.CallbackQuery
		notificationID, actionID, ok := parseCallbackData(cq.Data)
		if !ok {
			p.provider.answerCallbackQuery(ctx, cq.ID, "Sorry, that button couldn't be processed.")
			return
		}
		chatID := strconv.FormatInt(cq.From.ID, 10)
		label, err := p.service.RespondViaTelegram(notificationID, actionID, chatID)
		if err != nil {
			p.provider.answerCallbackQuery(ctx, cq.ID, humanizeRespondError(err))
			return
		}
		p.provider.answerCallbackQuery(ctx, cq.ID, "Recorded, thanks!")
		p.provider.editMessageAfterResponse(ctx, strconv.FormatInt(cq.Message.Chat.ID, 10), cq.Message.MessageID, label)
	}
}

// humanizeRespondError maps a Service response error to a short string
// shown as a toast in the Telegram app (via answerCallbackQuery's text) —
// the tapping user gets some explanation rather than Telegram's generic
// "loading" spinner just timing out.
func humanizeRespondError(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "This notification no longer exists."
	case errors.Is(err, ErrForbidden):
		return "This isn't your notification."
	case errors.Is(err, ErrInvalidAction):
		return "Sorry, that button is no longer valid."
	case errors.Is(err, ErrAlreadyResponded):
		return "You've already responded to this."
	default:
		return "Something went wrong recording your response."
	}
}
