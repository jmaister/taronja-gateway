package db

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// Provider constants
const AdminProvider = "tg_admin_provider"

// ClientInfo contains common client and geographical information
type ClientInfo struct {
	// Indexed: BlockedClient's registry is routinely queried/filtered by
	// IP (db.BlockedClientRepositoryDB.List), and this is the field that
	// query filters on across every table ClientInfo is embedded in.
	IPAddress string `gorm:"type:varchar(45);index" json:"ipAddress"` // IP address of the client
	UserAgent string `gorm:"type:text" json:"userAgent"`              // User agent string
	Referrer  string `gorm:"type:varchar(500)" json:"referrer"`       // HTTP referrer

	// Device (UserAgent) information
	BrowserFamily  string `gorm:"type:varchar(100)" json:"browserFamily"`  // Browser family (Chrome, Firefox, etc.)
	BrowserVersion string `gorm:"type:varchar(100)" json:"browserVersion"` // Browser version
	OSFamily       string `gorm:"type:varchar(100)" json:"osFamily"`       // Operating system
	OSVersion      string `gorm:"type:varchar(100)" json:"osVersion"`      // Operating system version
	DeviceFamily   string `gorm:"type:varchar(100)" json:"deviceFamily"`   // Device type (mobile, desktop, tablet)
	DeviceBrand    string `gorm:"type:varchar(100)" json:"deviceBrand"`    // Device brand (if applicable)
	DeviceModel    string `gorm:"type:varchar(100)" json:"deviceModel"`    // Device model (if applicable)

	// Detailed geographical information (might be the address, city, etc.)
	GeoLocation string  `gorm:"type:varchar(200)" json:"geoLocation"` // General geo location string
	Latitude    float64 `gorm:"type:decimal(10,8)" json:"latitude"`   // GPS latitude
	Longitude   float64 `gorm:"type:decimal(11,8)" json:"longitude"`  // GPS longitude
	City        string  `gorm:"type:varchar(100)" json:"city"`        // City name
	ZipCode     string  `gorm:"type:varchar(20)" json:"zipCode"`      // Postal/ZIP code
	Country     string  `gorm:"type:varchar(100)" json:"country"`     // Country name
	CountryCode string  `gorm:"type:varchar(3)" json:"countryCode"`   // ISO country code
	Region      string  `gorm:"type:varchar(100)" json:"region"`      // State/Province/Region
	Continent   string  `gorm:"type:varchar(50)" json:"continent"`    // Continent name

	// Fingerprint is the client fingerprint chosen by fingerprint.SelectFingerprint
	// (stable over JA4H); FingerprintType names the algorithm. Indexed for the
	// new-vs-returning query in db/timeseries.go.
	Fingerprint string `gorm:"type:varchar(100);index" json:"fingerprint"`
	// FingerprintType names which algorithm produced Fingerprint —
	// fingerprint.TypeStable ("stable") or fingerprint.TypeJA4H ("ja4h")
	// (rows stored by earlier versions may also say "ja4_tls"). Empty exactly when
	// Fingerprint is empty too.
	FingerprintType string `gorm:"type:varchar(20)" json:"fingerprintType"`
}

// User struct definition
type User struct {
	gorm.Model
	ID string `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	// TODO: does this unique really work?
	Email                    string `gorm:"unique"`
	Username                 string `gorm:"unique"`
	Picture                  string
	Name                     string
	GivenName                string
	FamilyName               string
	Locale                   string
	Provider                 string
	ProviderId               string
	Password                 string
	PasswordReset            bool
	PasswordResetCode        string
	PasswordResetExpires     *time.Time
	EmailConfirmed           bool
	EmailConfirmationCode    string
	EmailConfirmationExpires *time.Time
}

// BeforeCreate will set a CUID rather than numeric ID.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}
	u.ID = newId
	return nil
}

// BeforeSave will handle password encryption if the password field is set,
// and normalizes PasswordResetExpires/EmailConfirmationExpires to UTC —
// see TrafficMetric.BeforeCreate's comment for why this needs doing
// defensively here rather than trusting every call site that sets these
// (e.g. a password-reset handler building the expiry from time.Now(), which
// carries the server's local zone) to remember .UTC() itself.
func (u *User) BeforeSave(tx *gorm.DB) error {
	if u.Password != "" && !encryption.IsPasswordHashed(u.Password) {
		hashedPassword, err := encryption.GeneratePasswordHash(u.Password)
		if err != nil {
			return err
		}
		u.Password = hashedPassword
	}
	if u.PasswordResetExpires != nil {
		utcExpires := u.PasswordResetExpires.UTC()
		u.PasswordResetExpires = &utcExpires
	}
	if u.EmailConfirmationExpires != nil {
		utcExpires := u.EmailConfirmationExpires.UTC()
		u.EmailConfirmationExpires = &utcExpires
	}
	return nil
}

// Session is a persistent session. The JSON tags (and ClientInfo's) shape the
// X-User-Data header sent to backends; gorm.Model's own fields stay PascalCase.
type Session struct {
	gorm.Model
	// Token is the raw bearer value from the tg_session_token cookie. It is never
	// persisted (only TokenHash is); session repository methods set it themselves.
	Token string `gorm:"-" json:"token"`
	// TokenHash is the hex SHA-256 of Token and the primary key, so a database
	// dump does not expose usable session tokens.
	TokenHash       string     `gorm:"primaryKey;column:token_hash;type:varchar(64);not null;default:''" json:"-"`
	UserID          string     `gorm:"column:user_id;type:varchar(255)" json:"userId"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	IsAuthenticated bool       `json:"isAuthenticated"`
	IsAdmin         bool       `json:"isAdmin"`
	ValidUntil      time.Time  `json:"validUntil"`
	Provider        string     `json:"provider"`
	ClosedOn        *time.Time `json:"closedOn"`
	LastActivity    time.Time  `json:"lastActivity"`
	SessionName     string     `gorm:"type:varchar(100)" json:"sessionName"`
	CreatedFrom     string     `gorm:"type:varchar(100)" json:"createdFrom"` // How the session was created

	// Embed common client information
	ClientInfo
}

// hashSessionToken returns the hex-encoded SHA-256 hash of a raw session
// token — the value actually stored in and looked up from TokenHash. Used
// by BeforeSave (whenever a full *Session goes through Create/Save) and
// directly by SessionRepository's lookup/mutation methods (which only ever
// see the raw token, never a *Session to hook into).
func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}

// BeforeSave hashes Token and normalizes the time fields to UTC. Raw-column
// updates (CloseSession) normalize at the call site. An empty Token leaves
// TokenHash untouched so a Save that only bumps LastActivity keeps it.
func (s *Session) BeforeSave(tx *gorm.DB) error {
	if s.Token != "" {
		s.TokenHash = hashSessionToken(s.Token)
	}
	if !s.ValidUntil.IsZero() {
		s.ValidUntil = s.ValidUntil.UTC()
	}
	if !s.LastActivity.IsZero() {
		s.LastActivity = s.LastActivity.UTC()
	}
	if s.ClosedOn != nil {
		utcClosedOn := s.ClosedOn.UTC()
		s.ClosedOn = &utcClosedOn
	}
	return nil
}

// TrafficMetric struct definition
// This struct is used to store HTTP traffic metrics and analytics data
type TrafficMetric struct {
	gorm.Model
	HttpMethod     string    `gorm:"type:varchar(10);not null"`  // HTTP method (GET, POST, etc.)
	Path           string    `gorm:"type:varchar(500);not null"` // URL path of the request
	HttpStatus     int       `gorm:"not null"`                   // HTTP status code of the response
	ResponseTimeNs int64     `gorm:"not null"`                   // Time taken to process the request in nanoseconds
	Timestamp      time.Time `gorm:"not null"`                   // Time when the request was received
	ResponseSize   int64     `gorm:"default:0"`                  // Size of the response in bytes
	Error          string    `gorm:"type:text"`                  // Any error message if the request failed
	UserID         string    `gorm:"type:varchar(255)"`          // ID of the user making the request, if authenticated
	SessionID      string    `gorm:"type:varchar(255)"`          // ID of the session, if applicable
	// default:false: SQLite cannot ADD COLUMN NOT NULL without a default on a
	// non-empty table. Old rows predate static-asset tracking, so false is correct.
	IsStaticAsset bool `gorm:"not null;default:false;index"` // Whether Path looks like a static asset (see session.IsStaticAssetPath). Set even when management.excludeStaticAssets skips recording most such requests, so the rows that do exist stay filterable.
	// Embed common client and geographical information
	ClientInfo
}

// BeforeCreate normalizes Timestamp to UTC (also on CreateBatch):
// db/timeseries.go buckets the stored wall-clock prefix as UTC, so a non-UTC
// value would land in the wrong hour.
func (t *TrafficMetric) BeforeCreate(tx *gorm.DB) error {
	t.Timestamp = t.Timestamp.UTC()
	return nil
}

// TrafficMetricWithUser combines TrafficMetric with User information for detailed reports
type TrafficMetricWithUser struct {
	TrafficMetric
	User *User `gorm:"foreignKey:UserID;references:ID"`
}

// Token struct definition for API tokens
type Token struct {
	ID         string     `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	UserID     string     `gorm:"column:user_id;type:varchar(255);not null"`
	TokenHash  string     `gorm:"type:varchar(255);not null;index"` // Hashed version of the token
	Name       string     `gorm:"type:varchar(100);not null"`       // User-defined name for the token
	IsActive   bool       `gorm:"default:true"`                     // Whether the token is active
	ExpiresAt  *time.Time // When the token expires (nullable for no expiration)
	UsageCount int64      `gorm:"default:0"` // How many times the token has been used
	LastUsedAt *time.Time // When the token was last used
	// Scopes is a JSON array of caller-supplied labels for the token's
	// intended purpose, stored and returned as-is. Not read or enforced
	// anywhere else in this codebase — a token grants everything its
	// owning user's session would, regardless of what's listed here. See
	// api/taronja-gateway-api.yaml's TokenResponse.scopes description,
	// which carries the same caveat for API consumers.
	Scopes      string     `gorm:"type:text"`
	CreatedFrom string     `gorm:"type:varchar(100)"` // How the token was created
	RevokedAt   *time.Time // When the token was revoked
	RevokedBy   string     `gorm:"type:varchar(255)"` // Who revoked the token
	CreatedAt   time.Time  `gorm:"autoCreateTime"`    // When the token was created
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`    // When the token was last updated

	// Embed common client information from when the token was created
	ClientInfo
}

// BeforeCreate will set a CUID rather than numeric ID.
func (t *Token) BeforeCreate(tx *gorm.DB) error {
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}
	t.ID = newId
	return nil
}

// BeforeSave normalizes the time fields to UTC; ExpiresAt comes from API
// callers with arbitrary offsets. Raw-column updates normalize at the call site.
func (t *Token) BeforeSave(tx *gorm.DB) error {
	if t.ExpiresAt != nil {
		utcExpiresAt := t.ExpiresAt.UTC()
		t.ExpiresAt = &utcExpiresAt
	}
	if t.LastUsedAt != nil {
		utcLastUsedAt := t.LastUsedAt.UTC()
		t.LastUsedAt = &utcLastUsedAt
	}
	if t.RevokedAt != nil {
		utcRevokedAt := t.RevokedAt.UTC()
		t.RevokedAt = &utcRevokedAt
	}
	return nil
}

// Block reason constants — the value BlockedClient.Reason takes,
// matching exactly which counter middleware/builtin/ratelimiter.go's Handler
// found tripped a configured threshold.
const (
	BlockReasonRateLimit         = "rate_limit"
	BlockReasonMaxErrors         = "max_errors"
	BlockReasonVulnerabilityScan = "vulnerability_scan"
)

// BlockedClient is the persistent history of one rate-limiter block event; the
// in-memory limiter forgets a block once it expires.
type BlockedClient struct {
	gorm.Model
	// Reason names which counter tripped the block — one of the
	// BlockReason* constants above.
	Reason string `gorm:"type:varchar(30);not null;index" json:"reason"`
	// Path is the request path that triggered the block. Only ever set
	// for BlockReasonVulnerabilityScan (the specific configured URL
	// pattern that matched) — BlockReasonRateLimit and
	// BlockReasonMaxErrors trip on an aggregate count accumulated across
	// many requests/paths, not any one specific path, so there's no
	// single "the" path to record for those.
	Path string `gorm:"type:varchar(500)" json:"path"`
	// TriggerCount is the relevant counter's value at the moment it
	// crossed the configured threshold — requests-in-the-last-minute for
	// BlockReasonRateLimit, errors-in-the-block-window for
	// BlockReasonMaxErrors, or scan-404s-in-the-block-window for
	// BlockReasonVulnerabilityScan.
	TriggerCount int `gorm:"not null" json:"triggerCount"`
	// BlockedAt and BlockedUntil bound the block window. Always UTC —
	// see BeforeCreate below, and TrafficMetric.BeforeCreate's comment
	// for the same reasoning applied here.
	BlockedAt    time.Time `gorm:"not null;index" json:"blockedAt"`
	BlockedUntil time.Time `gorm:"not null" json:"blockedUntil"`
	// Embed common client and geographical information — the same
	// IP/User-Agent/geolocation/fingerprint fields every other traffic
	// record carries, captured once at block time via
	// session.NewClientInfo, the same helper TrafficMetric/Session use.
	ClientInfo
}

// BeforeCreate normalizes BlockedAt/BlockedUntil to UTC before they're
// persisted — see TrafficMetric.BeforeCreate's comment for why this
// happens defensively here rather than trusting every call site
// (middleware/builtin/ratelimiter.go builds both from a plain time.Now(), which
// carries the server's local zone) to remember .UTC() itself.
func (b *BlockedClient) BeforeCreate(tx *gorm.DB) error {
	b.BlockedAt = b.BlockedAt.UTC()
	b.BlockedUntil = b.BlockedUntil.UTC()
	return nil
}

// Counter struct definition for counter transactions
type Counter struct {
	gorm.Model

	ID           string `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	UserID       string `gorm:"column:user_id;type:varchar(255);not null;index"`    // Foreign key to User
	CounterID    string `gorm:"column:counter_id;type:varchar(255);not null;index"` // Type of counter (credits, coins, points, etc.)
	Amount       int    `gorm:"not null"`                                           // Amount added (positive) or deducted (negative)
	BalanceAfter int    `gorm:"not null"`                                           // Balance after this transaction
	Description  string `gorm:"type:text;not null"`                                 // Description of the counter transaction

	// Reference to the user
	User User `gorm:"foreignKey:UserID;references:ID"`
}

// BeforeCreate will set a CUID rather than numeric ID.
func (c *Counter) BeforeCreate(tx *gorm.DB) error {
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}
	c.ID = newId
	return nil
}

// Notification channel names — the value NotificationDelivery.Channel and
// Notification.RespondedVia take. Kept as plain strings rather than an enum
// type so a future channel (WhatsApp, Slack, SMS, ...) is just a new
// constant here plus a new notification.Provider implementation, never a
// schema or migration change.
const (
	NotificationChannelWeb      = "web" // not an external delivery — used for RespondedVia when the user answered from the in-app list, not a link/button
	NotificationChannelEmail    = "email"
	NotificationChannelTelegram = "telegram"
	// NotificationChannelResponseWebhook is a NotificationDelivery.Channel
	// value only — it never appears as a Notification.RespondedVia (the
	// webhook doesn't answer a notification, it reports that one was
	// already answered) and is deliberately excluded from
	// notification.Service.resolveChannelsForUser's "every configured
	// channel" default, since it isn't something a notification is ever
	// requested to be delivered *by* — see config.ResponseWebhookConfig.
	NotificationChannelResponseWebhook = "response_webhook"
)

// Notification delivery statuses — the value NotificationDelivery.Status
// takes. A pending delivery is one that has been accepted and kicked off
// (the provider's Send call is in-flight or queued) but has not completed
// yet; it transitions to sent, failed, or skipped once the provider responds.
const (
	NotificationDeliveryStatusSent    = "sent"
	NotificationDeliveryStatusFailed  = "failed"
	NotificationDeliveryStatusSkipped = "skipped" // channel requested but the user has no recipient for it (no linked Telegram chat, provider disabled, ...)
	NotificationDeliveryStatusPending = "pending" // delivery accepted and in-flight; the provider's Send call has not yet returned
)

// Notification is one in-app notification for one user. External delivery is
// recorded per attempt in NotificationDelivery. Metadata is returned as-is;
// Actions are only parsed to render buttons and validate responses.
type Notification struct {
	ID     string `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	UserID string `gorm:"column:user_id;type:varchar(255);not null;index"`
	// BatchID groups the rows from one Create call (a single recipient is a batch
	// of one). It has no table of its own.
	BatchID  string `gorm:"type:varchar(255);not null;index"`
	Type     string `gorm:"type:varchar(255);not null"` // caller-defined, opaque to the gateway, e.g. "music_track_added"
	Title    string `gorm:"type:text;not null"`
	Body     string `gorm:"type:text;not null"`
	URL      *string
	Metadata string `gorm:"type:text"` // JSON object, caller-defined, opaque to the gateway. "" means none.
	Actions  string `gorm:"type:text"` // JSON array of notification.Action. "" means none (a plain, unanswerable notification).

	CreatedAt time.Time `gorm:"autoCreateTime"`
	ReadAt    *time.Time

	// Set once the user responds to one of Actions, from any channel —
	// clicking an email link, tapping a Telegram button, or answering from
	// the in-app list. A notification with no Actions can never have these
	// set. Re-answering is rejected (first response wins) rather than
	// silently overwritten, since a caller reacting to "the user chose X"
	// needs that to be a one-time event, not a value that can change under
	// it after the fact.
	RespondedActionID *string
	RespondedAt       *time.Time
	RespondedVia      *string // one of the NotificationChannel* constants

	// RespondTokenHash is the sha256 hex of the single-use token in emailed answer
	// links; RespondTokenExpiresAt bounds how long those links work.
	RespondTokenHash      string
	RespondTokenExpiresAt *time.Time
}

// BeforeCreate will set a CUID rather than numeric ID.
func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}
	n.ID = newId
	return nil
}

// BeforeSave normalizes ReadAt, RespondedAt, and RespondTokenExpiresAt to
// UTC before they're persisted — none of them are set via GORM's own
// autoCreateTime/autoUpdateTime clock (NowFunc), so each is normalized here
// instead, the same reasoning as Token.BeforeSave.
func (n *Notification) BeforeSave(tx *gorm.DB) error {
	if n.ReadAt != nil {
		utcReadAt := n.ReadAt.UTC()
		n.ReadAt = &utcReadAt
	}
	if n.RespondedAt != nil {
		utcRespondedAt := n.RespondedAt.UTC()
		n.RespondedAt = &utcRespondedAt
	}
	if n.RespondTokenExpiresAt != nil {
		utcExpiresAt := n.RespondTokenExpiresAt.UTC()
		n.RespondTokenExpiresAt = &utcExpiresAt
	}
	return nil
}

// NotificationDelivery records one delivery attempt over one channel; the log
// is append-only.
//
// ExternalRef is a per-channel value (Telegram "chatID:messageID") used to match
// a later button tap to its message.
//
// A retry adds a new row with AttemptNumber+1. NextRetryAt is set only on the
// latest failed row per (NotificationID, Channel), which the retry worker's
// query relies on.
type NotificationDelivery struct {
	ID             string     `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	NotificationID string     `gorm:"column:notification_id;type:varchar(255);not null;index"`
	Channel        string     `gorm:"type:varchar(50);not null"`
	Status         string     `gorm:"type:varchar(50);not null"`
	Error          string     `gorm:"type:text"` // populated when Status is NotificationDeliveryStatusFailed
	ExternalRef    string     `gorm:"type:text"`
	AttemptNumber  int        `gorm:"not null;default:1"`
	NextRetryAt    *time.Time `gorm:"index"`
	// SentAt is set only when Status is sent; CreatedAt exists on every row.
	SentAt    *time.Time
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// BeforeCreate will set a CUID rather than numeric ID.
func (d *NotificationDelivery) BeforeCreate(tx *gorm.DB) error {
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}
	d.ID = newId
	return nil
}

// BeforeSave normalizes NextRetryAt/SentAt to UTC before they're
// persisted — both are set via time.Now() at the call site
// (notification.Service), not GORM's own autoCreateTime/autoUpdateTime
// clock, the same reasoning as Notification.BeforeSave.
func (d *NotificationDelivery) BeforeSave(tx *gorm.DB) error {
	if d.NextRetryAt != nil {
		utcNextRetryAt := d.NextRetryAt.UTC()
		d.NextRetryAt = &utcNextRetryAt
	}
	if d.SentAt != nil {
		utcSentAt := d.SentAt.UTC()
		d.SentAt = &utcSentAt
	}
	return nil
}

// NotificationChannelLink records that a gateway user has connected an
// external channel identity to their account — today, only Telegram (a
// chat ID), established via the /start deep-link flow in
// GET /_/notifications/telegram/link. Email needs no equivalent row: the
// gateway already knows every user's email address from User.Email.
type NotificationChannelLink struct {
	ID         string    `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	UserID     string    `gorm:"column:user_id;type:varchar(255);not null;uniqueIndex:idx_notification_channel_link_user_channel"`
	Channel    string    `gorm:"type:varchar(50);not null;uniqueIndex:idx_notification_channel_link_user_channel"`
	ExternalID string    `gorm:"type:varchar(255);not null;index"` // e.g. the Telegram chat ID, as a string
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

// BeforeCreate will set a CUID rather than numeric ID.
func (l *NotificationChannelLink) BeforeCreate(tx *gorm.DB) error {
	newId, err := cuid.NewCrypto(rand.Reader)
	if err != nil {
		return err
	}
	l.ID = newId
	return nil
}

// NotificationLinkCode connects a gateway user to an external channel identity
// (e.g. a Telegram deep-link code). It is stored unhashed because it lives
// minutes and is deleted on use.
type NotificationLinkCode struct {
	Code      string    `gorm:"primaryKey;column:code;type:varchar(255);not null"`
	UserID    string    `gorm:"column:user_id;type:varchar(255);not null"`
	Channel   string    `gorm:"type:varchar(50);not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	ExpiresAt time.Time
}

// BeforeSave normalizes ExpiresAt to UTC before it's persisted — it's set
// via time.Now().Add(...) at the call site (notification.Service), not
// GORM's own autoCreateTime/autoUpdateTime clock, the same reasoning as
// Token.BeforeSave and Notification.BeforeSave.
func (l *NotificationLinkCode) BeforeSave(tx *gorm.DB) error {
	l.ExpiresAt = l.ExpiresAt.UTC()
	return nil
}

// NotificationPreference is one user's preferred delivery channel (UserID is
// the primary key). A missing row or empty value both mean no preference.
// The channel is not validated against configured ones, so an unknown channel
// is skipped rather than rejected.
type NotificationPreference struct {
	UserID           string    `gorm:"primaryKey;column:user_id;type:varchar(255);not null"`
	PreferredChannel string    `gorm:"type:varchar(50)"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}
