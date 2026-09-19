package session

import (
	"net/url"
	"strings"
)

// SanitizeRedirectPath returns raw if it's safe to use as a same-origin
// post-login/logout redirect target, or "/" otherwise. "Safe" means a path
// starting with exactly one "/" and carrying no scheme or host of its own —
// never an absolute URL, and never a protocol-relative one (a leading "//"
// or "/\", the latter because several browsers treat a backslash the same
// as a forward slash when resolving a URL, so "/\evil.example" can resolve
// exactly like "//evil.example" even though it starts with one literal
// "/").
//
// Every one of this gateway's post-login/logout redirects (handlers/
// api_logout.go, providers/basicAuthentication.go, providers/providers.go's
// OAuth flow, and the equivalent client-side JS in static/login.html) reads
// this same "redirect" value from an unauthenticated request and, before
// this existed, passed it straight to http.Redirect (or, client-side,
// window.location) with no validation at all: a link to this gateway's own
// trusted domain with `?redirect=https://evil.example` completed a real
// login or logout against the real gateway, then bounced the browser to an
// attacker-controlled page — a convincing phishing pretext, since the
// victim's browser had just been talking to the legitimate service.
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
