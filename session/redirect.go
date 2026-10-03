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
	const fallback = "/"
	if raw == "" {
		return fallback
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return fallback
	}
	// Defense in depth beyond the prefix checks above: reject anything
	// url.Parse itself resolves to an absolute URL (a scheme or host of its
	// own) or can't parse at all — control characters, stray whitespace,
	// and similar oddities a browser might interpret more liberally than
	// this simple prefix check accounts for.
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return fallback
	}
	return raw
}
