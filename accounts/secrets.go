package accounts

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func equalHashes(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// CSRFToken derives the anti-CSRF value for a session's forms from its
// session token: stable for the session, useless without it, nothing to store.
func CSRFToken(sessionToken string) string {
	return hashSecret("tg-csrf:" + sessionToken)
}

// ValidCSRF reports whether got is the CSRF token of sessionToken.
func ValidCSRF(sessionToken, got string) bool {
	return sessionToken != "" && got != "" && equalHashes(CSRFToken(sessionToken), got)
}
