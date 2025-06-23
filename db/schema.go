package db

import (
	"crypto/rand"
	"time"

	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// ClientInfo contains common client and geographical information
type ClientInfo struct {
	IPAddress string `gorm:"type:varchar(45)"`  // IP address of the client
	UserAgent string `gorm:"type:text"`         // User agent string
	Referrer  string `gorm:"type:varchar(500)"` // HTTP referrer

	// Device (UserAgent) information
	BrowserFamily  string `gorm:"type:varchar(100)"` // Browser family (Chrome, Firefox, etc.)
	BrowserVersion string `gorm:"type:varchar(100)"` // Browser version
	OSFamily       string `gorm:"type:varchar(100)"` // Operating system
	OSVersion      string `gorm:"type:varchar(100)"` // Operating system version
	DeviceFamily   string `gorm:"type:varchar(100)"` // Device type (mobile, desktop, tablet)
	DeviceBrand    string `gorm:"type:varchar(100)"` // Device brand (if applicable)
	DeviceModel    string `gorm:"type:varchar(100)"` // Device model (if applicable)

	// Detailed geographical information (might be the address, city, etc.)
	GeoLocation string  `gorm:"type:varchar(200)"`  // General geo location string
	Latitude    float64 `gorm:"type:decimal(10,8)"` // GPS latitude
	Longitude   float64 `gorm:"type:decimal(11,8)"` // GPS longitude
	City        string  `gorm:"type:varchar(100)"`  // City name
	ZipCode     string  `gorm:"type:varchar(20)"`   // Postal/ZIP code
	Country     string  `gorm:"type:varchar(100)"`  // Country name
	CountryCode string  `gorm:"type:varchar(3)"`    // ISO country code
	Region      string  `gorm:"type:varchar(100)"`  // State/Province/Region
	Continent   string  `gorm:"type:varchar(50)"`   // Continent name
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

// BeforeSave will handle password encryption if the password field is set
func (u *User) BeforeSave(tx *gorm.DB) error {
	if u.Password != "" && !encryption.IsPasswordHashed(u.Password) {
		hashedPassword, err := encryption.GeneratePasswordHash(u.Password)
		if err != nil {
			return err
		}
		u.Password = hashedPassword
	}
	return nil
}

// Session struct definition for persistent sessions
type Session struct {
	gorm.Model
	Token           string `gorm:"primaryKey;column:token;type:varchar(255);not null"`
	UserID          string `gorm:"column:user_id;type:varchar(255)"`
	Username        string
	Email           string
	IsAuthenticated bool
	IsAdmin         bool
	ValidUntil      time.Time
	Provider        string
	ClosedOn        *time.Time
	LastActivity    time.Time
	SessionName     string `gorm:"type:varchar(100)"`
	CreatedFrom     string `gorm:"type:varchar(100)"` // How the session was created

	// Embed common client information
	ClientInfo
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
	// Embed common client and geographical information
	ClientInfo
}
