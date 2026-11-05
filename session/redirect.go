package session

import (
	"net/url"
	"strings"
)

// SanitizeRedirectPath returns raw if it is a safe same-origin redirect target,
// else "/". Safe means exactly one leading "/" and no scheme or host: no absolute
// URLs and no protocol-relative ones ("//" or "/\", as browsers treat a backslash
// like a slash). This stops `?redirect=https://evil.example` open redirects after
// login or logout.
func SanitizeRedirectPath(raw string) string {
	if !IsSafeRedirectPath(raw) {
		return "/"
	}
	return raw
}

// IsSafeRedirectPath reports whether raw is a same-origin path: one leading
// "/", no backslash anywhere, no control characters (browsers drop tabs and
// newlines, so "/\t/evil.example" would become "//evil.example"), and
// nothing url.Parse reads as having a scheme or host.
func IsSafeRedirectPath(raw string) bool {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || len(raw) > 2048 {
		return false
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return false
		}
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "" && parsed.Host == ""
}
