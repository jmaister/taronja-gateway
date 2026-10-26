//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func accountsConfig() string {
	return baseConfig(`authenticationProviders:
  basic:
    enabled: true
`)
}

func uniqueEmail(name string) string {
	return fmt.Sprintf("%s-%d@example.com", name, time.Now().UnixNano())
}

// postForm submits a form and returns the final response body (after redirects).
func (c *client) postForm(path string, values url.Values) (*http.Response, string) {
	c.t.Helper()
	resp, err := c.http.PostForm(c.base+path, values)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return resp, string(body)
}

func (c *client) getPage(path string) (*http.Response, string) {
	c.t.Helper()
	resp, err := c.http.Get(c.base + path)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return resp, string(body)
}

func (c *client) me() map[string]any {
	c.t.Helper()
	resp, body := c.do(http.MethodGet, "/_/me", nil)
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return mustJSON(c.t, body)
}

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func (c *client) accountCSRF() string {
	c.t.Helper()
	_, page := c.getPage("/_/account")
	m := csrfField.FindStringSubmatch(page)
	require.NotNil(c.t, m, "account page has no CSRF token:\n%s", page)
	return m[1]
}

func TestAccountPage_PasswordSessionsAndActivity(t *testing.T) {
	gw := startGateway(t, accountsConfig())
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	email := uniqueEmail("account")
	admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": "acct-" + email, "email": email, "password": "first-password"}, http.StatusCreated)

	anonymous := newClient(t, gw.URL)
	resp, _ := anonymous.getPage("/_/account")
	assert.Equal(t, "/_/login", resp.Request.URL.Path, "the account page needs a login")

	laptop := newClient(t, gw.URL)
	laptop.login(email, "first-password")
	phone := newClient(t, gw.URL)
	phone.login(email, "first-password")
	resp, _ = newClient(t, gw.URL).postForm("/_/auth/basic/login", url.Values{"username": {email}, "password": {"wrong-password"}})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	_, page := laptop.getPage("/_/account")
	assert.Contains(t, page, html.EscapeString(email))
	assert.Contains(t, page, "not confirmed")
	assert.Contains(t, page, "this device")
	assert.Equal(t, 2, strings.Count(page, `name="session"`), "both sessions are listed")
	assert.Contains(t, page, "Failed login (wrong password)")

	// Forms without the session's CSRF token are refused.
	resp, _ = laptop.postForm("/_/account/sessions/revoke-others", url.Values{"csrf": {"forged"}})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NotNil(t, phone.me())

	csrf := laptop.accountCSRF()
	resp, page = laptop.postForm("/_/account/password", url.Values{"csrf": {csrf}, "current_password": {"nope"}, "new_password": {"second-password"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, page, "current password is wrong")

	_, page = laptop.postForm("/_/account/password", url.Values{"csrf": {csrf}, "current_password": {"first-password"}, "new_password": {"second-password"}})
	assert.Contains(t, page, "Your password was changed")
	assert.Nil(t, phone.me(), "changing the password signs out the other sessions")
	require.NotNil(t, laptop.me(), "but keeps this one")
	newClient(t, gw.URL).login(email, "second-password")

	tablet := newClient(t, gw.URL)
	tablet.login(email, "second-password")
	csrf = laptop.accountCSRF()
	_, page = laptop.postForm("/_/account/sessions/revoke-others", url.Values{"csrf": {csrf}})
	assert.Contains(t, page, "All other sessions were signed out")
	assert.Nil(t, tablet.me())

	_, page = laptop.getPage("/_/account")
	for _, activity := range []string{"Logged in · Password", "Password changed", "Session signed out"} {
		assert.Contains(t, page, activity)
	}
}

func mustJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}
