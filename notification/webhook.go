package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

// ResponseWebhookProvider POSTs a small JSON event to a single configured
// URL whenever a user responds to a notification — see
// config.ResponseWebhookConfig's doc comment for why this is a one-time
// event fired after the fact, not a channel a notification is delivered
// *to* the way email/Telegram are. It implements the same Provider
// interface as those two anyway, deliberately: doing so gets it
// Service.deliver/recordDelivery's automatic retry-with-backoff and its
// place in GET /api/notifications/{id}/deliveries for free, with no new
// machinery of its own — see Service.recordValidatedResponse for the one
// place this actually gets invoked (never as part of a notification's own
// initial delivery).
type ResponseWebhookProvider struct {
	url        string
	secret     string
	httpClient *http.Client
}

// NewResponseWebhookProvider returns nil if cfg isn't configured (see
// ResponseWebhookConfig.IsConfigured), the same convention every other
// provider constructor here follows.
func NewResponseWebhookProvider(cfg config.ResponseWebhookConfig) *ResponseWebhookProvider {
	if !cfg.IsConfigured() {
		return nil
	}
	return &ResponseWebhookProvider{
		url:        cfg.URL,
		secret:     cfg.Secret,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *ResponseWebhookProvider) Channel() string { return "response_webhook" } // matches db.NotificationChannelResponseWebhook

// responseWebhookPayload is the JSON body POSTed to ResponseWebhookConfig.URL.
// Deliberately minimal: only what the caller couldn't already know from
// having created the notification itself (which action, via which
// channel, when) plus enough identifiers (notificationId, userId, type,
// metadata) to correlate it back to whatever triggered the original
// notification, without a caller needing a second lookup.
type responseWebhookPayload struct {
	NotificationID    string                 `json:"notificationId"`
	UserID            string                 `json:"userId"`
	Type              string                 `json:"type"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	RespondedActionID string                 `json:"respondedActionId"`
	RespondedVia      string                 `json:"respondedVia"`
	RespondedAt       time.Time              `json:"respondedAt"`
}

// Send POSTs the response event for req.Notification, which must already
// have RespondedActionID/RespondedVia/RespondedAt populated (Service sets
// these in memory immediately after recording the response, before
// calling deliver for this channel — see recordValidatedResponse).
func (p *ResponseWebhookProvider) Send(ctx context.Context, req SendRequest) (string, error) {
	n := req.Notification
	if n.RespondedActionID == nil || n.RespondedVia == nil || n.RespondedAt == nil {
		return "", fmt.Errorf("response webhook: notification %s has no recorded response to report", n.ID)
	}

	metadata, err := DecodeMetadata(n.Metadata)
	if err != nil {
		return "", fmt.Errorf("response webhook: decoding metadata: %w", err)
	}

	body, err := json.Marshal(responseWebhookPayload{
		NotificationID:    n.ID,
		UserID:            n.UserID,
		Type:              n.Type,
		Metadata:          metadata,
		RespondedActionID: *n.RespondedActionID,
		RespondedVia:      *n.RespondedVia,
		RespondedAt:       *n.RespondedAt,
	})
	if err != nil {
		return "", fmt.Errorf("response webhook: encoding payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("response webhook: building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.secret != "" {
		mac := hmac.New(sha256.New, []byte(p.secret))
		mac.Write(body)
		httpReq.Header.Set("X-Taronja-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("response webhook: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("response webhook: receiver returned %s", resp.Status)
	}
	return "", nil
}
