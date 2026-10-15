package accounts

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
)

// Mailer sends one plain-text email. notification.EmailProvider implements it.
type Mailer interface {
	SendMessage(ctx context.Context, to, subject, textBody string) error
}

// Paths of the pages the emailed links open, under config.ManagementPrefix.
const (
	VerifyEmailPath    = "/_/verify-email"
	ResetPasswordPath  = "/_/reset-password"
	EmailLoginLinkPath = "/_/auth/email/verify"
	LoginPath          = "/_/login"
	ForgotPasswordPath = "/_/forgot-password"
)

func (s *Service) link(path, token string) string {
	return strings.TrimSuffix(s.opts.BaseURL, "/") + path + "?token=" + url.QueryEscape(token)
}

func (s *Service) send(ctx context.Context, to, subject, body string) error {
	if s.mailer == nil {
		return ErrEmailNotConfigured
	}
	err := s.mailer.SendMessage(ctx, to, subject, body)
	if err != nil {
		log.Printf("accounts: sending %q to %s failed: %v", subject, to, err)
	}
	return err
}

func (s *Service) sendSignupConfirmation(ctx context.Context, to, token string) error {
	body := fmt.Sprintf(`Someone (hopefully you) asked to create a %[1]s account with this email address.

Confirm it and log in:
%[2]s

The link works once and expires in %[3]s.

If this wasn't you, ignore this email: no account is created until the link is used.
`, s.opts.AppName, s.link(VerifyEmailPath, token), humanDuration(verificationTTL))
	return s.send(ctx, to, "Confirm your email for "+s.opts.AppName, body)
}

func (s *Service) sendEmailConfirmation(ctx context.Context, to, token string) error {
	body := fmt.Sprintf(`Confirm that this email address belongs to your %[1]s account:
%[2]s

The link works once and expires in %[3]s.
`, s.opts.AppName, s.link(VerifyEmailPath, token), humanDuration(verificationTTL))
	return s.send(ctx, to, "Confirm your email for "+s.opts.AppName, body)
}

func (s *Service) sendAlreadyRegistered(ctx context.Context, to string) error {
	var reset string
	if s.opts.PasswordResetEnabled {
		reset = "\nForgot your password? Reset it here:\n" + strings.TrimSuffix(s.opts.BaseURL, "/") + ForgotPasswordPath + "\n"
	}
	body := fmt.Sprintf(`Someone tried to create a %[1]s account with this email address, but you already have one.

Log in here:
%[2]s
%[3]s
If this wasn't you, you can ignore this email.
`, s.opts.AppName, strings.TrimSuffix(s.opts.BaseURL, "/")+LoginPath, reset)
	return s.send(ctx, to, "You already have a "+s.opts.AppName+" account", body)
}

func (s *Service) sendPasswordReset(ctx context.Context, to, token string) error {
	body := fmt.Sprintf(`Someone (hopefully you) asked to reset the password of your %[1]s account.

Choose a new password:
%[2]s

The link works once and expires in %[3]s. Resetting your password signs you out everywhere else.

If this wasn't you, ignore this email: your password stays the same.
`, s.opts.AppName, s.link(ResetPasswordPath, token), humanDuration(passwordResetTTL))
	return s.send(ctx, to, "Reset your "+s.opts.AppName+" password", body)
}

func (s *Service) sendEmailLogin(ctx context.Context, to, token, code string) error {
	body := fmt.Sprintf(`Your %[1]s login code is:

    %[2]s

Type it in the browser where you asked for it, or open this link:
%[3]s

The code and link work once and expire in %[4]s.

If this wasn't you, ignore this email.
`, s.opts.AppName, code, s.link(EmailLoginLinkPath, token), humanDuration(emailLoginTTL))
	return s.send(ctx, to, code+" is your "+s.opts.AppName+" login code", body)
}

func humanDuration(d interface{ Minutes() float64 }) string {
	minutes := int(d.Minutes())
	if minutes%60 == 0 {
		hours := minutes / 60
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d minutes", minutes)
}
