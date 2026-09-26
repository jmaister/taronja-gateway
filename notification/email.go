package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

// smtpTimeout bounds the entire SMTP exchange sendMailWithTimeout drives —
// connect, greeting, optional STARTTLS/AUTH, envelope, body, and QUIT — the
// same 10-second figure webhook.go/telegram.go's http.Client.Timeout
// already use for their own outbound calls. See Send's doc comment for why
// net/smtp.SendMail alone can't provide this. A var, not a const, like
// retryBackoffSchedule elsewhere in this package — so a test exercising an
// actually-unresponsive relay can shrink it rather than genuinely waiting
// out the full production timeout.
var smtpTimeout = 10 * time.Second

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

// Send delivers req by SMTP.
//
// Unlike the webhook and Telegram providers, which both send over
// net/http with an http.Client.Timeout, net/smtp.SendMail has no way to
// bound how long it runs at all — no context parameter, no deadline
// option, nothing. An unresponsive relay (a TCP handshake that never
// completes, a server that accepts the connection and then just never
// speaks) would otherwise hang this delivery goroutine indefinitely — one
// of a limited pool of them (see notification/service.go's
// maxConcurrentDeliveries), so an outage on the operator's own SMTP relay
// could tie up every one of those slots and stall every other channel's
// deliveries too. sendMailWithTimeout below reimplements SendMail's own
// logic (github.com/golang/go's src/net/smtp/smtp.go is the reference)
// with a single deadline applied to the underlying connection, covering
// the whole exchange rather than any one step of it — net/smtp has no
// public API that accepts an existing, already-deadlined net.Conn, so
// there's no way to add this by wrapping SendMail instead of
// reimplementing its body.
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
	if err := sendMailWithTimeout(addr, auth, p.cfg.From, []string{req.Recipient}, []byte(msg)); err != nil {
		return "", fmt.Errorf("smtp send failed: %w", err)
	}
	return "", nil
}

// validateSMTPLine rejects a value containing CR or LF — the same guard
// net/smtp.SendMail applies to from/to before ever dialing, preventing a
// value that reaches here from injecting extra SMTP command lines into the
// envelope. Preserved here since sendMailWithTimeout otherwise reimplements
// SendMail's body from scratch.
func validateSMTPLine(line string) error {
	if strings.ContainsAny(line, "\n\r") {
		return errors.New("smtp: a line must not contain CR or LF")
	}
	return nil
}

// sendMailWithTimeout behaves like net/smtp.SendMail, except the entire
// exchange — connect, greeting, optional STARTTLS/AUTH, envelope, body,
// and QUIT — is bounded by smtpTimeout via a deadline on the underlying
// connection, rather than being able to block indefinitely at any one
// step. See Send's doc comment for why this exists as a reimplementation
// rather than a wrapper around SendMail itself.
func sendMailWithTimeout(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	if err := validateSMTPLine(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := validateSMTPLine(recipient); err != nil {
			return err
		}
	}

	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		return err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if auth != nil {
		// Upstream SendMail only makes this check when the server's EHLO
		// actually returned an extension list at all (c.ext != nil,
		// unexported — not reachable from here) and separately checks that
		// list for "AUTH". Extension("AUTH") collapses those two cases
		// into one: it also returns false when EHLO produced no extension
		// list to begin with, which errors here where upstream would
		// instead silently attempt Auth anyway. That upstream case only
		// arises against a legacy, EHLO-incapable server — Auth would
		// almost certainly fail there regardless, so failing slightly
		// earlier, with a clearer message, isn't a meaningful behavior
		// change in practice.
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
