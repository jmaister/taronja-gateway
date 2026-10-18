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

// A used session is pushed back to now + management.session's duration, and
// the browser gets the cookie with the new lifetime.
func TestGatewayRenewsSessionsOnUse(t *testing.T) {
	d := deps.NewTestWithName(t.Name())
	gw, err := NewGatewayWithDependencies(&config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost"},
		Management: config.ManagementConfig{Session: config.SessionConfig{DurationSeconds: config.DefaultSessionDurationSeconds}},
	}, nil, d)
	require.NoError(t, err)

	user := &db.User{Email: "active@example.com", Username: "active", EmailConfirmed: true}
	require.NoError(t, d.UserRepo.CreateUser(user))
	sess, err := d.SessionStore.NewSession(nil, user, "basic", 2*time.Hour) // nearly expired
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/_/me", nil)
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: sess.Token})
	w := httptest.NewRecorder()
	gw.Server.Handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var renewed *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == session.SessionCookieName {
			renewed = c
		}
	}
	require.NotNil(t, renewed, "the cookie is re-sent with the new lifetime")
	assert.Equal(t, sess.Token, renewed.Value)
	assert.Equal(t, 15*24*60*60, renewed.MaxAge)

	stored, err := d.SessionRepo.FindSessionByToken(sess.Token)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(15*24*time.Hour), stored.ValidUntil, time.Minute)
}
