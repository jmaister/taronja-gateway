package notification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/jmaister/taronja-gateway/db"
)

// SendRequest is everything a Provider needs to deliver one notification
// to one recipient on its channel.
type SendRequest struct {
	Notification *db.Notification
	Actions      []Action
	// Recipient is the channel-specific address to deliver to: an email
	// address for the email provider, a chat ID (as a string) for the
	// Telegram provider. Resolved by Service before calling Send — a
	// Provider never looks up recipients itself, so adding a channel never
	// means teaching Service about a new kind of user identity beyond
	// "however this channel's provider says users are addressed."
	Recipient string
	// RespondBaseURL, when non-empty, is the base URL a Provider should
	// build clickable answer links from (e.g. an email "Approve" link).
	// Empty when the gateway has no ServerConfig.URL/URL to build an
	// absolute link from, or when Notification has no Actions — a
	// Provider must degrade to a plain, unanswerable message in that case
	// rather than send a broken link.
	RespondBaseURL string
	// RespondToken is the opaque, single-use token identifying this
	// specific notification for the public respond link (see
	// db.Notification.RespondTokenHash) — "" under the same conditions as
	// RespondBaseURL.
	RespondToken string
}

// Provider delivers notifications over one external channel. Implementing
// a new channel (WhatsApp, Slack, SMS, ...) means writing one of these and
// registering it in Service's provider map — no change to the data model,
// the API, or any other provider.
type Provider interface {
	// Channel returns this provider's channel name — one of the
	// db.NotificationChannel* constants (or a new one, for a channel this
	// codebase doesn't have yet).
	Channel() string
	// Send delivers req and returns an opaque externalRef to remember for
	// this delivery (see db.NotificationDelivery.ExternalRef), or an error
	// if delivery failed. Returning "" for externalRef is fine for a
	// channel with nothing to remember (e.g. email).
	Send(ctx context.Context, req SendRequest) (externalRef string, err error)
}

// generateOpaqueToken returns a random, URL-safe token and the sha256 hex
// hash of it, following the same random-bytes-then-hash pattern
// auth.TokenService.GenerateToken uses for API tokens: the raw token is
// handed to whoever needs to prove they have it (an email link, a Telegram
// deep link), only the hash is ever persisted, so a database read alone
// never yields something usable as a valid token.
func generateOpaqueToken(numBytes int) (raw string, hash string, err error) {
	randomBytes := make([]byte, numBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random token: %w", err)
	}
	raw = base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(randomBytes)
	return raw, hashToken(raw), nil
}

// hashToken is generateOpaqueToken's hash step, exposed on its own so a
// raw token handed back to the gateway later (in an email link's ?token=)
// can be hashed the same way to look it up, without regenerating it.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}
