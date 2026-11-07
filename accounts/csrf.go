package accounts

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// CSRFToken derives the anti-CSRF value for a session's forms from its
// session token: stable for the session, useless without it, nothing to store.
func CSRFToken(sessionToken string) string {
	sum := sha256.Sum256([]byte("tg-csrf:" + sessionToken))
	return hex.EncodeToString(sum[:])
}

// ValidCSRF reports whether got is the CSRF token of sessionToken.
func ValidCSRF(sessionToken, got string) bool {
	return sessionToken != "" && got != "" && subtle.ConstantTimeCompare([]byte(CSRFToken(sessionToken)), []byte(got)) == 1
}
