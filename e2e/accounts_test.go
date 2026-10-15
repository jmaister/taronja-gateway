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
    signup: true
  emailLogin:
    enabled: true
notification:
  email:
    enabled: true
    host: localhost
    port: ` + mailpitSMTPPort + `
    from: noreply@taronja.test
    fromName: Taronja E2E
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

// openEmailLink follows a link from an email the way a browser does: GET the
// confirmation page, then press its button (POST with the token).
func (c *client) openEmailLink(link string) (*http.Response, string) {
	c.t.Helper()
	parsed, err := url.Parse(link)
	require.NoError(c.t, err)
	resp, page := c.getPage(parsed.RequestURI())
	require.Equal(c.t, http.StatusOK, resp.StatusCode, page)
	return c.postForm(parsed.Path, url.Values{"token": {parsed.Query().Get("token")}})
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

// waitForEmail returns the newest email to recipient whose subject contains
// subject, waiting for it to arrive.
func waitForEmail(t *testing.T, recipient, subject string) mailpitMessage {
	t.Helper()
	var found mailpitMessage
	eventually(t, 15*time.Second, fmt.Sprintf("an email %q to %s", subject, recipient), func() bool {
		for _, m := range mailpitMessagesTo(t, recipient) {
			if strings.Contains(m.Subject, subject) {
				found = m
				return true
			}
		}
		return false
	})
	return mailpitGetMessage(t, found.ID)
}

var linkInEmail = regexp.MustCompile(`https?://\S+\?token=\S+`)

func emailLink(t *testing.T, msg mailpitMessage) string {
	t.Helper()
	link := linkInEmail.FindString(msg.Text)
	require.NotEmpty(t, link, "no link in email:\n%s", msg.Text)
	return link
}

func TestSignup_ConfirmEmailThenLogIn(t *testing.T) {
	requireMailpit(t)
	gw := startGateway(t, accountsConfig())
	email := uniqueEmail("signup")
	browser := newClient(t, gw.URL)

	resp, page := browser.postForm("/_/signup", url.Values{"email": {email}, "password": {"short"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, page, "at least 8 characters")

	resp, page = browser.postForm("/_/signup", url.Values{"email": {email}, "password": {"sign-up-password"}, "redirect": {"/welcome"}})
	require.Equal(t, http.StatusOK, resp.StatusCode, page)
	assert.Contains(t, page, "Check your inbox")

	// The account does not exist until the link is used.
	stranger := newClient(t, gw.URL)
	resp, _ = stranger.postForm("/_/auth/basic/login", url.Values{"username": {email}, "password": {"sign-up-password"}})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	msg := waitForEmail(t, email, "Confirm your email")
	assert.Contains(t, msg.Text, "no account is created until the link is used")
	resp, _ = browser.openEmailLink(emailLink(t, msg))
	assert.Equal(t, "/welcome", resp.Request.URL.Path, "lands where the sign-up started")
	me := browser.me()
	require.NotNil(t, me, "the link logs the user in")
	assert.Equal(t, email, me["email"])

	// The password chosen at sign-up works from another browser.
	other := newClient(t, gw.URL)
	other.login(email, "sign-up-password")

	// Signing up with an email that already has an account sends a notice
	// instead of a link, and the page says the same as for a new address.
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	existing := uniqueEmail("existing")
	admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": existing, "email": existing, "password": "existing-password"}, http.StatusCreated)
	resp, page = newClient(t, gw.URL).postForm("/_/signup", url.Values{"email": {existing}, "password": {"another-password"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, page, "Check your inbox", "the answer does not reveal that the account exists")
	notice := waitForEmail(t, existing, "already have")
	assert.NotRegexp(t, linkInEmail, notice.Text)
}

func TestPasswordReset_EmailLinkSetsNewPassword(t *testing.T) {
	requireMailpit(t)
	gw := startGateway(t, accountsConfig())
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	email := uniqueEmail("reset")
	admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": "reset-" + email, "email": email, "password": "old-password-1"}, http.StatusCreated)

	oldSession := newClient(t, gw.URL)
	oldSession.login(email, "old-password-1")

	browser := newClient(t, gw.URL)
	_, loginPage := browser.getPage("/_/login")
	assert.Contains(t, loginPage, "Forgot your password?")

	resp, page := browser.postForm("/_/forgot-password", url.Values{"email": {strings.ToUpper(email)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, page, "If "+strings.ToUpper(email)+" has an account")

	_, page = browser.postForm("/_/forgot-password", url.Values{"email": {uniqueEmail("nobody")}})
	assert.Contains(t, page, "has an account, we sent", "unknown emails get the same answer")

	link := emailLink(t, waitForEmail(t, email, "Reset your"))
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	resp, page = browser.getPage(parsed.RequestURI())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, page, "Choose a new password")

	resp, page = browser.postForm("/_/reset-password", url.Values{"token": {parsed.Query().Get("token")}, "password": {"brand-new-password"}})
	require.Equal(t, http.StatusOK, resp.StatusCode, page)
	assert.Equal(t, "/_/account", resp.Request.URL.Path)
	assert.Contains(t, page, "Your password was reset")

	assert.Nil(t, oldSession.me(), "a reset signs out every other session")
	resp, _ = newClient(t, gw.URL).postForm("/_/auth/basic/login", url.Values{"username": {email}, "password": {"old-password-1"}})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	newClient(t, gw.URL).login(email, "brand-new-password")

	resp, page = browser.postForm("/_/reset-password", url.Values{"token": {parsed.Query().Get("token")}, "password": {"third-password"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, page, "already used")
}

func TestEmailLogin_CodeAndLink(t *testing.T) {
	requireMailpit(t)
	gw := startGateway(t, accountsConfig())
	email := uniqueEmail("magic")

	// A new email signs up through its first login (sign-up is on).
	browser := newClient(t, gw.URL)
	resp, page := browser.postForm("/_/auth/email/start", url.Values{"email": {email}, "redirect": {"/after-login"}})
	require.Equal(t, http.StatusOK, resp.StatusCode, page)
	assert.Contains(t, page, "We sent a 6-digit code")

	msg := waitForEmail(t, email, "login code")
	code := regexp.MustCompile(`(?m)^\s+(\d{6})\s*$`).FindStringSubmatch(msg.Text)
	require.NotNil(t, code, msg.Text)
	assert.True(t, strings.HasPrefix(msg.Subject, code[1]), "the code is in the subject for quick reading")

	// The code does not work from a browser that did not ask for it.
	thief := newClient(t, gw.URL)
	resp, _ = thief.postForm("/_/auth/email/code", url.Values{"code": {code[1]}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Nil(t, thief.me())

	resp, _ = browser.postForm("/_/auth/email/code", url.Values{"code": {code[1]}})
	assert.Equal(t, "/after-login", resp.Request.URL.Path)
	me := browser.me()
	require.NotNil(t, me)
	assert.Equal(t, email, me["email"])

	// The emailed link works in any browser, e.g. opened on another device.
	otherEmail := uniqueEmail("magic-link")
	phone := newClient(t, gw.URL)
	_, _ = phone.postForm("/_/auth/email/start", url.Values{"email": {otherEmail}})
	laptop := newClient(t, gw.URL)
	resp, _ = laptop.openEmailLink(emailLink(t, waitForEmail(t, otherEmail, "login code")))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, laptop.me(), "the emailed link works in any browser")
}

func TestAccountPage_PasswordSessionsAndActivity(t *testing.T) {
	requireMailpit(t)
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

	// Confirming the email from the account page.
	_, page = laptop.postForm("/_/account/verify-email", url.Values{"csrf": {csrf}})
	assert.Contains(t, page, "We sent you a confirmation email")
	resp, page = laptop.openEmailLink(emailLink(t, waitForEmail(t, email, "Confirm your email")))
	assert.Equal(t, "/_/account", resp.Request.URL.Path)
	assert.Contains(t, page, "Your email is confirmed")
	assert.Contains(t, page, `class="badge ok">confirmed`)

	tablet := newClient(t, gw.URL)
	tablet.login(email, "second-password")
	csrf = laptop.accountCSRF()
	_, page = laptop.postForm("/_/account/sessions/revoke-others", url.Values{"csrf": {csrf}})
	assert.Contains(t, page, "All other sessions were signed out")
	assert.Nil(t, tablet.me())

	_, page = laptop.getPage("/_/account")
	for _, activity := range []string{"Logged in · Password", "Password changed", "Email confirmed", "Session signed out"} {
		assert.Contains(t, page, activity)
	}
}

func mustJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}
