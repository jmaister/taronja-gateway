package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
)

const (
	whatsAppDefaultURL     = "https://graph.facebook.com"
	whatsAppDefaultVersion = "v23.0"

	// Template body variables must fit in the template's 1024-character body
	// together with its fixed text.
	whatsAppMaxTitle = 200
	whatsAppMaxBody  = 700
)

// whatsAppRetryableCodes are Cloud API error codes that may pass later:
// throttling (4, 80007, 130429, 131048, 131056) and temporary errors (131000,
// 131016, 133004). Any other error (number not on WhatsApp, template
// mismatch, expired token) is permanent.
var whatsAppRetryableCodes = map[int]bool{4: true, 80007: true, 130429: true, 131000: true, 131016: true, 131048: true, 131056: true, 133004: true}

// WhatsAppProvider delivers notifications through Meta's WhatsApp Cloud API.
// Meta only allows a business to start a conversation with an approved
// template, so every notification is sent as the configured template, with
// the title and body as its two body variables.
type WhatsAppProvider struct {
	endpoint   string
	token      string
	template   config.WhatsAppTemplateConfig
	limiter    *minuteLimiter
	httpClient *http.Client
}

// NewWhatsAppProvider returns nil when WhatsApp is not configured.
func NewWhatsAppProvider(cfg config.WhatsAppNotificationConfig) *WhatsAppProvider {
	if !cfg.IsConfigured() {
		return nil
	}
	base := cfg.APIURL
	if base == "" {
		base = whatsAppDefaultURL
	}
	version := cfg.APIVersion
	if version == "" {
		version = whatsAppDefaultVersion
	}
	perMinute := cfg.MaxPerMinute
	if perMinute == 0 {
		perMinute = config.DefaultMaxMessagesPerMinute
	}
	return &WhatsAppProvider{
		endpoint:   strings.TrimSuffix(base, "/") + "/" + version + "/" + cfg.PhoneNumberID + "/messages",
		token:      cfg.AccessToken,
		template:   cfg.Template,
		limiter:    newMinuteLimiter(perMinute),
		httpClient: &http.Client{Timeout: paidChannelHTTPTimeout},
	}
}

func (p *WhatsAppProvider) Channel() string { return db.NotificationChannelWhatsApp }

// UsesPhoneNumber marks WhatsApp as addressed by the user's phone number.
func (p *WhatsAppProvider) UsesPhoneNumber() bool { return true }

// WantsLink is true whenever the template has a link button: its URL
// variable is required on every message.
func (p *WhatsAppProvider) WantsLink(n *db.Notification, actions []Action) bool {
	return p.template.LinkButton
}

var repeatedSpaces = regexp.MustCompile(` {2,}`)

// templateText makes text acceptable as a template variable: Meta rejects
// newlines, tabs, more than four spaces in a row (error 132018) and empty
// values.
func templateText(text string, max int) string {
	text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(text)
	text = strings.TrimSpace(repeatedSpaces.ReplaceAllString(text, " "))
	if runes := []rune(text); len(runes) > max {
		text = strings.TrimSpace(string(runes[:max-1])) + "…"
	}
	if text == "" {
		return "-"
	}
	return text
}

type whatsAppResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error *struct {
		Message   string `json:"message"`
		Code      int    `json:"code"`
		ErrorData struct {
			Details string `json:"details"`
		} `json:"error_data"`
	} `json:"error"`
}

func (p *WhatsAppProvider) Send(ctx context.Context, req SendRequest) (string, error) {
	var components []map[string]any
	switch p.template.BodyVariableCount() {
	case 2:
		components = append(components, map[string]any{"type": "body", "parameters": []map[string]any{
			{"type": "text", "text": templateText(req.Notification.Title, whatsAppMaxTitle)},
			{"type": "text", "text": templateText(req.Notification.Body, whatsAppMaxBody)},
		}})
	case 1:
		combined := strings.TrimSpace(req.Notification.Title)
		if body := strings.TrimSpace(req.Notification.Body); body != "" {
			combined += ": " + body
		}
		components = append(components, map[string]any{"type": "body", "parameters": []map[string]any{
			{"type": "text", "text": templateText(combined, whatsAppMaxTitle+whatsAppMaxBody)},
		}})
	}
	if p.template.LinkButton {
		if req.LinkToken == "" {
			// The button's URL variable is required; without it Meta rejects
			// the message, so do not pay for the attempt.
			return "", permanent(fmt.Errorf("the WhatsApp template has a link button but no notification page link could be made (is server.url set?)"))
		}
		components = append(components, map[string]any{
			"type":       "button",
			"sub_type":   "url",
			"index":      "0",
			"parameters": []map[string]any{{"type": "text", "text": req.LinkToken}},
		})
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                strings.TrimPrefix(req.Recipient, "+"),
		"type":              "template",
		"template": map[string]any{
			"name":     p.template.Name,
			"language": map[string]string{"code": p.template.Language},
		},
	}
	if len(components) > 0 {
		payload["template"].(map[string]any)["components"] = components
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	err = p.limiter.allow()
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.token)
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading whatsapp response: %w", err)
	}

	var result whatsAppResponse
	_ = json.Unmarshal(raw, &result)
	if resp.StatusCode == http.StatusOK && len(result.Messages) > 0 {
		return result.Messages[0].ID, nil
	}
	if result.Error == nil {
		err = fmt.Errorf("whatsapp rejected the message (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return "", permanent(err)
		}
		return "", err
	}
	err = fmt.Errorf("whatsapp rejected the message (HTTP %d, code %d: %s %s)", resp.StatusCode, result.Error.Code, result.Error.Message, result.Error.ErrorData.Details)
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests || whatsAppRetryableCodes[result.Error.Code] {
		return "", err
	}
	return "", permanent(err)
}
