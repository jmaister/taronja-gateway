package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

// smtpTimeout bounds the whole SMTP exchange. A var so tests can shrink it.
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

// Send delivers req by SMTP. net/smtp.SendMail cannot be bounded, and a hung relay
// would tie up the limited delivery slots, so sendMailWithTimeout reimplements it
// with one deadline on the connection.
func (p *EmailProvider) Send(ctx context.Context, req SendRequest) (string, error) {
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

	if err := p.SendMessage(ctx, req.Recipient, req.Notification.Title, body.String()); err != nil {
		return "", err
	}
	return "", nil
}

// SendMessage sends one plain-text email.
func (p *EmailProvider) SendMessage(ctx context.Context, to, subject, textBody string) error {
	// Header values are RFC 2047-encoded so non-ASCII names and titles
	// survive strict mail servers; ASCII passes through unchanged.
	from := (&mail.Address{Name: p.cfg.FromName, Address: p.cfg.From}).String()
	encodedSubject := mime.QEncoding.Encode("utf-8", subject)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, encodedSubject, textBody)

	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	var auth smtp.Auth
	if p.cfg.Username != "" {
		auth = smtp.PlainAuth("", p.cfg.Username, p.cfg.Password, p.cfg.Host)
	}
	if err := sendMailWithTimeout(addr, auth, p.cfg.From, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	return nil
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
		// Upstream only checks when EHLO returned an extension list; Extension("AUTH")
		// also fails when there is none. That only happens with legacy servers where Auth
		// would fail anyway.
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
