package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

// randomSecret returns 32 random bytes, base64url-encoded: unguessable values
// for email links and browser bindings.
func randomSecret() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// randomCode returns a 6-digit code for typing in by hand. It is only
// accepted from the browser that asked for it, and only maxCodeAttempts times.
func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashSecret is what gets stored for a secret: a database dump never holds a
// usable link, code or binding.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// hashCode salts a code with its token ID, since 6 digits alone are cheap to
// reverse from a plain hash.
func hashCode(tokenID, code string) string {
	return hashSecret(tokenID + ":" + code)
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
