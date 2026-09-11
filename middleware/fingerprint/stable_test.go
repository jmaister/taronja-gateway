package fingerprint

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newReqWithHeaders(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestStableFingerprint_EmptyWhenNoStableHeadersPresent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Empty(t, StableFingerprint(req))
}

func TestStableFingerprint_SameHeadersProduceSameFingerprint(t *testing.T) {
	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0",
		"Accept-Encoding": "gzip, deflate, br",
		"Accept-Language": "en-US,en;q=0.9",
	}
	fp1 := StableFingerprint(newReqWithHeaders(t, headers))
	fp2 := StableFingerprint(newReqWithHeaders(t, headers))
	assert.NotEmpty(t, fp1)
	assert.Equal(t, fp1, fp2)
}

func TestStableFingerprint_UnaffectedByRequestTypeVariation(t *testing.T) {
	// This is the whole point of this fingerprint: the fields that vary
	// wildly between a page navigation and a subresource/XHR request from
	// the identical browser (Accept, HTTP method, and everything JA4H's
	// header-count/header-name-set hash depends on) must not change the
	// result, as long as the browser-identity headers stay the same.
	base := map[string]string{
		"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0",
		"Accept-Encoding": "gzip, deflate, br",
		"Accept-Language": "en-US,en;q=0.9",
	}

	navigation := httptest.NewRequest(http.MethodGet, "/page", nil)
	for k, v := range base {
		navigation.Header.Set(k, v)
	}
	navigation.Header.Set("Accept", "text/html,application/xhtml+xml")
	navigation.Header.Set("Sec-Fetch-Dest", "document")
	navigation.Header.Set("Sec-Fetch-Mode", "navigate")
	navigation.Header.Set("Sec-Fetch-User", "?1")
	navigation.Header.Set("Upgrade-Insecure-Requests", "1")

	xhr := httptest.NewRequest(http.MethodPost, "/api/data", nil)
	for k, v := range base {
		xhr.Header.Set(k, v)
	}
	xhr.Header.Set("Accept", "application/json")
	xhr.Header.Set("Sec-Fetch-Dest", "empty")
	xhr.Header.Set("Sec-Fetch-Mode", "cors")
	xhr.Header.Set("X-Requested-With", "XMLHttpRequest")

	assert.Equal(t, StableFingerprint(navigation), StableFingerprint(xhr),
		"a navigation and an XHR call from the identical browser must produce the same stable fingerprint despite very different header sets/methods/Accept values")
}

func TestStableFingerprint_DiffersForDifferentUserAgent(t *testing.T) {
	fp1 := StableFingerprint(newReqWithHeaders(t, map[string]string{"User-Agent": "Chrome/120.0"}))
	fp2 := StableFingerprint(newReqWithHeaders(t, map[string]string{"User-Agent": "Firefox/121.0"}))
	assert.NotEqual(t, fp1, fp2)
}

func TestStableFingerprint_ClientHintsAffectResult(t *testing.T) {
	base := map[string]string{"User-Agent": "Chrome/120.0"}
	withHints := map[string]string{
		"User-Agent":         "Chrome/120.0",
		"Sec-Ch-Ua":          `"Chromium";v="120"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Linux"`,
	}
	fp1 := StableFingerprint(newReqWithHeaders(t, base))
	fp2 := StableFingerprint(newReqWithHeaders(t, withHints))
	assert.NotEqual(t, fp1, fp2, "presence of client hints is part of the fingerprint")
}
