package db

import (
	"crypto/rand"
	"time"

	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

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
	ValidUntil      time.Time
	Provider        string
	ClosedOn        *time.Time
}
