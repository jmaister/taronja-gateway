package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
)

const bulkGateDefaultURL = "https://portal.bulkgate.com/api/1.0/simple/transactional"

// paidChannelHTTPTimeout bounds one call to an SMS or WhatsApp API.
const paidChannelHTTPTimeout = 15 * time.Second

// smsBackend sends one already-composed SMS. Only BulkGate exists today; a
// new provider is one more implementation.
type smsBackend interface {
	send(ctx context.Context, to, text string, unicode bool) (externalRef string, err error)
}

// SMSProvider delivers notifications by SMS to the user's phone number.
type SMSProvider struct {
	backend     smsBackend
	maxSegments int
	limiter     *minuteLimiter
}

// NewSMSProvider returns nil when SMS is not configured.
func NewSMSProvider(cfg config.SMSNotificationConfig) *SMSProvider {
	if !cfg.IsConfigured() {
		return nil
	}
	maxSegments := cfg.MaxSegments
	if maxSegments == 0 {
		maxSegments = config.DefaultSMSMaxSegments
	}
	perMinute := cfg.MaxPerMinute
	if perMinute == 0 {
		perMinute = config.DefaultMaxMessagesPerMinute
	}
	url := cfg.BulkGate.APIURL
	if url == "" {
		url = bulkGateDefaultURL
	}
	return &SMSProvider{
		backend: &bulkGateBackend{
			url:           url,
			applicationID: cfg.BulkGate.ApplicationID,
			token:         cfg.BulkGate.ApplicationToken,
			senderID:      cfg.BulkGate.SenderID,
			senderIDValue: cfg.BulkGate.SenderIDValue,
			client:        &http.Client{Timeout: paidChannelHTTPTimeout},
		},
		maxSegments: maxSegments,
		limiter:     newMinuteLimiter(perMinute),
	}
}

func (p *SMSProvider) Channel() string { return db.NotificationChannelSMS }

// UsesPhoneNumber marks SMS as addressed by the user's phone number.
func (p *SMSProvider) UsesPhoneNumber() bool { return true }

// WantsLink asks for a notification page link when there is something to
// open: actions to answer or a URL.
func (p *SMSProvider) WantsLink(n *db.Notification, actions []Action) bool {
	return len(actions) > 0 || (n.URL != nil && *n.URL != "")
}

func (p *SMSProvider) Send(ctx context.Context, req SendRequest) (string, error) {
	text, unicode := composeSMS(req.Notification.Title, req.Notification.Body, req.LinkURL, p.maxSegments)
	err := p.limiter.allow()
	if err != nil {
		return "", err
	}
	return p.backend.send(ctx, req.Recipient, text, unicode)
}

// bulkGateBackend uses BulkGate's Simple API "transactional SMS" endpoint.
type bulkGateBackend struct {
	url, applicationID, token string
	senderID, senderIDValue   string
	client                    *http.Client
}

type bulkGateResponse struct {
	Data *struct {
		Status string `json:"status"`
		SMSID  string `json:"sms_id"`
	} `json:"data"`
	Type  string `json:"type"`
	Code  int    `json:"code"`
	Error string `json:"error"`
}

func (b *bulkGateBackend) send(ctx context.Context, to, text string, unicode bool) (string, error) {
	payload := map[string]any{
		"application_id":    b.applicationID,
		"application_token": b.token,
		"number":            strings.TrimPrefix(to, "+"),
		"text":              text,
		"unicode":           unicode,
	}
	if b.senderID != "" {
		payload["sender_id"] = b.senderID
	}
	if b.senderIDValue != "" {
		payload["sender_id_value"] = b.senderIDValue
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("bulkgate request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading bulkgate response: %w", err)
	}

	var result bulkGateResponse
	_ = json.Unmarshal(raw, &result)
	if resp.StatusCode == http.StatusOK && result.Data != nil && (result.Data.Status == "accepted" || result.Data.Status == "scheduled") {
		return result.Data.SMSID, nil
	}

	err = fmt.Errorf("bulkgate rejected the SMS (HTTP %d, %s: %s)", resp.StatusCode, result.Type, strings.TrimSpace(result.Error))
	if result.Error == "" && result.Type == "" {
		err = fmt.Errorf("bulkgate rejected the SMS (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	// Server errors and rate limits may pass; so may low credit once topped
	// up. Any other client error (invalid number, bad credentials) will not.
	retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests || strings.Contains(result.Type, "credit")
	if !retryable && resp.StatusCode >= 400 {
		return "", permanent(err)
	}
	return "", err
}
