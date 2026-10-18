package notification

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/db"
)

// e164Pattern is an E.164 phone number: "+", a country code that does not
// start with 0, at most 15 digits in all.
var e164Pattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// ErrInvalidPhone is returned for a phone number that is not E.164.
var ErrInvalidPhone = errors.New(`phone numbers must be in E.164 format, e.g. "+34600123456"`)

// NormalizePhone strips spaces, dashes, dots and parentheses and checks the
// result is E.164.
func NormalizePhone(raw string) (string, error) {
	phone := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if !e164Pattern.MatchString(phone) {
		return "", ErrInvalidPhone
	}
	return phone, nil
}

// PermanentError marks a delivery failure that retrying cannot fix (an
// invalid number, a rejected template, bad credentials). Paid channels return
// it so a hopeless message is not paid for again on every retry.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func permanent(err error) error { return &PermanentError{Err: err} }

// IsPermanent reports whether err is a PermanentError.
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// minuteLimiter allows at most limit events per calendar minute. Paid
// channels use it to bound the bill; a message over the limit fails as
// retryable and the retry worker sends it in a later minute.
type minuteLimiter struct {
	limit int
	now   func() time.Time

	mu     sync.Mutex
	minute time.Time
	count  int
}

func newMinuteLimiter(limit int) *minuteLimiter {
	return &minuteLimiter{limit: limit, now: time.Now}
}

func (l *minuteLimiter) allow() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	minute := l.now().Truncate(time.Minute)
	if !minute.Equal(l.minute) {
		l.minute = minute
		l.count = 0
	}
	if l.count >= l.limit {
		return fmt.Errorf("rate limit: more than %d messages this minute, will retry", l.limit)
	}
	l.count++
	return nil
}

// DeliveryChannels are the channel names a notification can be delivered on
// (and a user can prefer), whether or not this gateway configures them.
var DeliveryChannels = []string{db.NotificationChannelEmail, db.NotificationChannelTelegram, db.NotificationChannelSMS, db.NotificationChannelWhatsApp}

// IsDeliveryChannel reports whether channel is one of DeliveryChannels.
func IsDeliveryChannel(channel string) bool {
	return slices.Contains(DeliveryChannels, channel)
}
