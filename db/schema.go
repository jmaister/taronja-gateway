package db

import (
	"crypto/rand"
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

	// Fingerprint is the single client fingerprint value for this
	// request/session — see FingerprintType for which algorithm produced
	// it. Only one is ever stored, chosen by priority among the available
	// signals (most reliable wins) via fingerprint.SelectFingerprint:
	// TLS-level JA4 (fingerprint.TypeJA4TLS, only available when the
	// gateway terminates TLS itself — see gateway/ja4tls.go) over the
	// reduced-entropy "stable" fingerprint (fingerprint.TypeStable, works
	// without TLS but still request-type-independent — see
	// middleware/fingerprint.StableFingerprint) over JA4H
	// (fingerprint.TypeJA4H, always available but the noisiest of the
	// three — see doc/middleware/ja4-fingerprint.md). Empty if none of the
	// three produced anything at all. Indexed (on whichever table embeds
	// ClientInfo) since db/timeseries.go's new-vs-returning-visitor
	// calculation does a MIN(timestamp) GROUP BY fingerprint over the
	// entire TrafficMetric table — an unindexed scan of that would get
	// more expensive as the table grows, the opposite of that feature's
	// entire point.
	Fingerprint string `gorm:"type:varchar(100);index" json:"fingerprint"`
	// FingerprintType names which algorithm produced Fingerprint —
	// fingerprint.TypeJA4TLS ("ja4_tls"), fingerprint.TypeStable
	// ("stable"), or fingerprint.TypeJA4H ("ja4h"). Empty exactly when
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

// Session struct definition for persistent sessions
//
// JSON tags on this struct (and the embedded ClientInfo's) are what
// actually shape the X-User-Data header a backend route receives — see
// gateway.go's json.Marshal(sessionObject) and README.md's "X-User-Data
// JSON Structure" reference, which documents the exact camelCase shape
// these tags produce. Note gorm.Model's own embedded fields (ID,
// CreatedAt, UpdatedAt, DeletedAt) have no tags of their own and so still
// serialize in Go's default PascalCase — that's a pre-existing gap in the
// X-User-Data contract this change doesn't attempt to close, since
// suppressing an embedded struct's fields from JSON needs shadowing it
// entirely, a larger change than what was asked for here.
type Session struct {
	gorm.Model
	Token           string     `gorm:"primaryKey;column:token;type:varchar(255);not null" json:"token"`
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

// BeforeSave normalizes ValidUntil, LastActivity, and ClosedOn to UTC
// before they're persisted — see TrafficMetric.BeforeCreate's comment for
// the same reasoning applied here: every one of these is set from a plain
// time.Now() somewhere (session/session.go), which carries the server
// process's local zone, and this makes the schema's UTC convention hold
// regardless of what any given call site remembers to do. Only reaches the
// paths that pass a full *Session through GORM's Create/Save — the one
// raw-column update in this codebase (SessionRepository.CloseSession's
// Update("closed_on", ...)) normalizes its own value at the call site
// instead, since a hook on an empty *Session{} model can't see it.
func (s *Session) BeforeSave(tx *gorm.DB) error {
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
	// default:false, not just not null: this column was added after
	// TrafficMetric already existed in the wild (see AGENTS.md's "Data
	// migrations" section), and AutoMigrate's ALTER TABLE ADD COLUMN for a
	// NOT NULL column with no default fails outright on SQLite the moment
	// the table already has one row — not a soft failure, a startup panic
	// (confirmed against a real database from before this column existed:
	// "SQL logic error: Cannot add a NOT NULL column with default value
	// NULL"). false is the correct backfill value regardless: every
	// pre-existing row predates this column's introduction, at which point
	// every request was recorded, static or not (excludeStaticAssets
	// skipping most static-asset rows came later still), so an old row
	// having no way to say "this one was static" defaulting to "not
	// static" undercounts static traffic for that old data, never fabricates
	// it.
	IsStaticAsset bool `gorm:"not null;default:false;index"` // Whether Path looks like a static asset (see session.IsStaticAssetPath). Set even when management.excludeStaticAssets skips recording most such requests, so the rows that do exist stay filterable.
	// Embed common client and geographical information
	ClientInfo
}

// BeforeCreate normalizes Timestamp to UTC before it's persisted.
// db/timeseries.go's SQL-side bucketing (GetTimeSeries) extracts just the
// "YYYY-MM-DD HH:MM:SS" prefix of the stored value — the pure-Go SQLite
// driver this project uses stores time.Time as Go's default .String()
// representation ("2026-06-10 08:00:00 +0200 CEST"), which none of
// SQLite's date/time functions parse, but stripping the trailing
// offset/zone leaves a format they do — and interprets that prefix
// directly as UTC wall-clock time, with no offset conversion applied. A
// non-UTC Timestamp would silently bucket at the wrong hour (its own local
// wall-clock hour, not the equivalent UTC one) without this normalization,
// which is why it happens here — once, defensively, on every insert path
// (including CreateBatch, since GORM invokes model hooks per-record in a
// batch create) — rather than trusting every call site that constructs a
// TrafficMetric to remember `.UTC()` itself.
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
	ID          string     `gorm:"primaryKey;column:id;type:varchar(255);not null"`
	UserID      string     `gorm:"column:user_id;type:varchar(255);not null"`
	TokenHash   string     `gorm:"type:varchar(255);not null;index"` // Hashed version of the token
	Name        string     `gorm:"type:varchar(100);not null"`       // User-defined name for the token
	IsActive    bool       `gorm:"default:true"`                     // Whether the token is active
	ExpiresAt   *time.Time // When the token expires (nullable for no expiration)
	UsageCount  int64      `gorm:"default:0"` // How many times the token has been used
	LastUsedAt  *time.Time // When the token was last used
	Scopes      string     `gorm:"type:text"`         // JSON array of scopes/permissions
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

// BeforeSave normalizes ExpiresAt, LastUsedAt, and RevokedAt to UTC before
// they're persisted — see TrafficMetric.BeforeCreate's comment for the same
// reasoning applied here. ExpiresAt in particular is API-caller-supplied
// (handlers/api_tokens.go, from request JSON) rather than always built from
// time.Now() server-side, so this is the one place that can normalize it
// regardless of what offset a client happened to send. Only reaches the
// paths that pass a full *Token through GORM's Create/Save — the two
// raw-column updates in this codebase (TokenRepository's RevokeToken and
// IncrementUsageCount) normalize their own values at the call site instead,
// since a hook on an empty *Token{} model can't see them.
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
// matching exactly which counter middleware/ratelimiter.go's Handler
// found tripped a configured threshold.
const (
	BlockReasonRateLimit         = "rate_limit"
	BlockReasonMaxErrors         = "max_errors"
	BlockReasonVulnerabilityScan = "vulnerability_scan"
)

// BlockedClient records one rate-limiter block event: an IP was blocked,
// for how long, and what triggered it. This is a persistent history —
// the in-memory RateLimiter itself keeps no such record past the block's
// own expiry: middleware/ratelimiter.go's cleanupLoop deletes an IP's
// entire tracked state (including its block) the moment the block window
// ends and the IP has gone quiet, so without this, "was this IP blocked
// last week, and why" had no answer once that happened — only
// RateLimiter.Stats()'s live snapshot of whatever's still tracked right
// now. See doc/TODO.md's "Rate limiter" section, which asked for exactly
// this ("store persistent info about attackers... show blocked IPs with
// start and end date of the block").
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
// (middleware/ratelimiter.go builds both from a plain time.Now(), which
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
