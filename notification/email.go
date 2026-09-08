package notification

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
)

// EmailProvider delivers notifications by SMTP, using net/smtp directly —
// no queue, no retry, no template engine. See
// config.EmailNotificationConfig's doc comment for why that's an
// appropriate amount of machinery for this.
type EmailProvider struct {
	cfg config.EmailNotificationConfig
}

// NewEmailProvider returns nil if cfg isn't configured (see
// EmailNotificationConfig.IsConfigured) — callers register a Provider in
// Service's registry only when it's actually usable, so a disabled or
// incomplete email config simply means the "email" channel doesn't exist
// for this gateway instance, the same way an unconfigured OAuth2 provider
// never gets registered in providers.RegisterProviders.
func NewEmailProvider(cfg config.EmailNotificationConfig) *EmailProvider {
	if !cfg.IsConfigured() {
		return nil
	}
	return &EmailProvider{cfg: cfg}
}

func (p *EmailProvider) Channel() string { return "email" }

func (p *EmailProvider) Send(ctx context.Context, req SendRequest) (string, error) {
	from := p.cfg.From
	if p.cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", p.cfg.FromName, p.cfg.From)
	}

	var body strings.Builder
	fmt.Fprintf(&body, "%s\r\n\r\n", req.Notification.Body)
	if req.Notification.URL != nil && *req.Notification.URL != "" {
		fmt.Fprintf(&body, "%s\r\n\r\n", *req.Notification.URL)
	}
	if len(req.Actions) > 0 && req.RespondBaseURL != "" && req.RespondToken != "" {
		for _, action := range req.Actions {
			fmt.Fprintf(&body, "%s: %s?token=%s&action=%s\r\n",
				action.Label, req.RespondBaseURL, req.RespondToken, action.ID)
		}
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, req.Recipient, req.Notification.Title, body.String())

	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	var auth smtp.Auth
	if p.cfg.Username != "" {
		auth = smtp.PlainAuth("", p.cfg.Username, p.cfg.Password, p.cfg.Host)
	}
	if err := smtp.SendMail(addr, auth, p.cfg.From, []string{req.Recipient}, []byte(msg)); err != nil {
		return "", fmt.Errorf("smtp send failed: %w", err)
	}
	return "", nil
}
