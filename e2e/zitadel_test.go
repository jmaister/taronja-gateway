//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	zitadelURL     = envOr("E2E_ZITADEL_URL", "http://localhost:18180")
	zitadelKeysDir = envOr("E2E_ZITADEL_KEYS", ".zitadel")
)

// zitadelAPI calls Zitadel's API as one of the service accounts created at
// startup (see e2e/docker-compose.yml).
type zitadelAPI struct {
	t     *testing.T
	token string
}

func zitadelAccount(t *testing.T, name string) *zitadelAPI {
	t.Helper()
	pat, err := os.ReadFile(filepath.Join(zitadelKeysDir, name+".pat"))
	require.NoError(t, err, "Zitadel tokens missing (start the services with `make e2e-up`)")
	return &zitadelAPI{t: t, token: strings.TrimSpace(string(pat))}
}

func (z *zitadelAPI) post(path string, body any) map[string]any {
	z.t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(z.t, err)
	req, err := http.NewRequest(http.MethodPost, zitadelURL+path, bytes.NewReader(encoded))
	require.NoError(z.t, err)
	req.Header.Set("Authorization", "Bearer "+z.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(z.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(z.t, err)
	require.True(z.t, resp.StatusCode >= 200 && resp.StatusCode < 300, "POST %s: %d %s", path, resp.StatusCode, data)
	var out map[string]any
	require.NoError(z.t, json.Unmarshal(data, &out))
	return out
}

func requireZitadel(t *testing.T) {
	t.Helper()
	resp, err := http.Get(zitadelURL + "/debug/ready")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Zitadel is not ready at %s (start it with `make e2e-up`): %v", zitadelURL, err)
	}
	resp.Body.Close()
}

// zitadelApp registers an OIDC web app whose redirect URI is the gateway's
// callback and returns its client ID and secret.
func zitadelApp(t *testing.T, gatewayURL string) (clientID, clientSecret string) {
	t.Helper()
	admin := zitadelAccount(t, "admin")
	project := admin.post("/management/v1/projects", map[string]any{"name": fmt.Sprintf("taronja-e2e-%d", time.Now().UnixNano())})
	app := admin.post("/management/v1/projects/"+project["id"].(string)+"/apps/oidc", map[string]any{
		"name":           "gateway",
		"redirectUris":   []string{gatewayURL + "/_/auth/zitadel/callback"},
		"responseTypes":  []string{"OIDC_RESPONSE_TYPE_CODE"},
		"grantTypes":     []string{"OIDC_GRANT_TYPE_AUTHORIZATION_CODE"},
		"appType":        "OIDC_APP_TYPE_WEB",
		"authMethodType": "OIDC_AUTH_METHOD_TYPE_BASIC",
		"devMode":        true, // allows the http:// redirect URI
	})
	return app["clientId"].(string), app["clientSecret"].(string)
}

type zitadelUser struct {
	username, email, password string
}

func newZitadelUser(t *testing.T, name string, emailVerified bool) zitadelUser {
	t.Helper()
	u := zitadelUser{
		username: fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
		password: "Zitadel-Passw0rd!",
	}
	u.email = u.username + "@example.com"
	zitadelAccount(t, "admin").post("/v2/users/human", map[string]any{
		"username": u.username,
		"profile":  map[string]any{"givenName": "Zed", "familyName": "Test-" + name},
		"email":    map[string]any{"email": u.email, "isVerified": emailVerified},
		"password": map[string]any{"password": u.password, "changeRequired": false},
	})
	return u
}

func zitadelConfig(clientID, clientSecret string) string {
	return baseConfig(`authenticationProviders:
  basic:
    enabled: true
  oidc:
    - name: zitadel
      label: Zitadel
      issuer: ` + zitadelURL + `
      clientId: "` + clientID + `"
      clientSecret: "` + clientSecret + `"
`)
}

// zitadelLogin plays the browser and Zitadel's login UI: it starts the login
// at the gateway, signs the user in through Zitadel's session API, and
// follows the callback back to the gateway. It returns the final response.
func (c *client) zitadelLogin(startPath string, start func() *http.Response, user zitadelUser) *http.Response {
	c.t.Helper()
	noFollow := *c.http
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// Gateway -> Zitadel authorize -> Zitadel's login UI, which carries the
	// auth request ID.
	resp := start()
	resp.Body.Close()
	require.Equal(c.t, http.StatusTemporaryRedirect, resp.StatusCode, "%s should redirect to Zitadel", startPath)
	authorize := resp.Header.Get("Location")
	require.True(c.t, strings.HasPrefix(authorize, zitadelURL+"/oauth/v2/authorize?"), authorize)
	assert.Contains(c.t, authorize, "code_challenge_method=S256")

	resp, err := noFollow.Get(authorize)
	require.NoError(c.t, err)
	resp.Body.Close()
	loginUI, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(c.t, err)
	authRequest := loginUI.Query().Get("authRequest")
	require.NotEmpty(c.t, authRequest, "Zitadel should hand over to its login UI: %s", loginUI)

	// What the login UI would do after the user typed their password.
	loginClient := zitadelAccount(c.t, "login-client")
	sess := loginClient.post("/v2/sessions", map[string]any{"checks": map[string]any{
		"user":     map[string]any{"loginName": user.username},
		"password": map[string]any{"password": user.password},
	}})
	finished := loginClient.post("/v2/oidc/auth_requests/"+authRequest, map[string]any{
		"session": map[string]any{"sessionId": sess["sessionId"], "sessionToken": sess["sessionToken"]},
	})

	resp, err = c.http.Get(finished["callbackUrl"].(string))
	require.NoError(c.t, err)
	resp.Body.Close()
	return resp
}

func (c *client) zitadelLoginFromLoginPage(user zitadelUser) *http.Response {
	c.t.Helper()
	noFollow := *c.http
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c.zitadelLogin("/_/auth/zitadel/login", func() *http.Response {
		resp, err := noFollow.Get(c.base + "/_/auth/zitadel/login?redirect=/after-login")
		require.NoError(c.t, err)
		return resp
	}, user)
}

func TestZitadel_LoginCreatesAndReusesUser(t *testing.T) {
	requireZitadel(t)
	listener, gatewayURL := listenGateway(t)
	clientID, secret := zitadelApp(t, gatewayURL)
	gw := startGatewayOn(t, listener, zitadelConfig(clientID, secret))

	browser := newClient(t, gw.URL)
	_, loginPage := browser.getPage("/_/login")
	assert.Contains(t, loginPage, "Login with Zitadel")

	user := newZitadelUser(t, "carol", true)
	resp := browser.zitadelLoginFromLoginPage(user)
	assert.Equal(t, "/after-login", resp.Request.URL.Path)
	me := browser.me()
	require.NotNil(t, me, "logged in through Zitadel")
	assert.Equal(t, user.email, me["email"])
	assert.Equal(t, "zitadel", me["provider"])
	firstID := mustUserID(t, gw, user.email)

	// A second login, in another browser, is the same gateway user.
	again := newClient(t, gw.URL)
	again.zitadelLoginFromLoginPage(user)
	require.NotNil(t, again.me())
	assert.Equal(t, firstID, mustUserID(t, gw, user.email))
}

func TestZitadel_UnverifiedEmailIsRefused(t *testing.T) {
	requireZitadel(t)
	listener, gatewayURL := listenGateway(t)
	clientID, secret := zitadelApp(t, gatewayURL)
	gw := startGatewayOn(t, listener, zitadelConfig(clientID, secret))

	browser := newClient(t, gw.URL)
	resp := browser.zitadelLoginFromLoginPage(newZitadelUser(t, "dave", false))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Nil(t, browser.me())
}

func mustUserID(t *testing.T, gw *testGateway, email string) string {
	t.Helper()
	user, err := gw.Deps.UserRepo.FindUserByIdOrUsername("", "", email)
	require.NoError(t, err)
	return user.ID
}
