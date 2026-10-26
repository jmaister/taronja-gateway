package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLoginTestGateway(t *testing.T, providers config.AuthenticationProviders) (*Gateway, *deps.Dependencies) {
	t.Helper()
	d := deps.NewTestWithName(t.Name())
	gw, err := NewGatewayWithDependencies(&config.GatewayConfig{
		Server:                  config.ServerConfig{Host: "localhost", URL: "http://gateway.test"},
		Management:              config.ManagementConfig{Session: config.SessionConfig{DurationSeconds: 3600}},
		AuthenticationProviders: providers,
	}, nil, d)
	require.NoError(t, err)
	return gw, d
}

func get(gw *Gateway, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	gw.Server.Handler.ServeHTTP(w, req)
	return w
}

func TestLoginPage_RedirectsALoggedInUserOnTheServer(t *testing.T) {
	gw, d := newLoginTestGateway(t, config.AuthenticationProviders{Basic: config.BasicAuthenticationConfig{Enabled: true}})
	user := &db.User{Email: "in@example.com", Username: "in", EmailConfirmed: true}
	require.NoError(t, d.UserRepo.CreateUser(user))
	sess, err := d.SessionStore.NewSession(nil, user, "basic", time.Hour)
	require.NoError(t, err)
	cookie := &http.Cookie{Name: session.SessionCookieName, Value: sess.Token}

	w := get(gw, "/_/login?redirect=/app/settings", cookie)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/app/settings", w.Header().Get("Location"))

	w = get(gw, "/_/login?redirect=https://evil.example", cookie)
	assert.Equal(t, "/", w.Header().Get("Location"), "the redirect is sanitized")
	w = get(gw, "/_/login?redirect=/_/login", cookie)
	assert.Equal(t, "/", w.Header().Get("Location"), "never back to the login page")

	w = get(gw, "/_/login?redirect=/app/settings")
	assert.Equal(t, http.StatusOK, w.Code, "no session: the page is shown")
	assert.Contains(t, w.Body.String(), `name="password"`)
}
