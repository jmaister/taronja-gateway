package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsRenewal(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	lifetime := 15 * 24 * time.Hour

	assert.False(t, needsRenewal(now.Add(lifetime), now, lifetime), "just renewed")
	assert.False(t, needsRenewal(now.Add(lifetime-59*time.Minute), now, lifetime), "renewed less than an hour ago")
	assert.True(t, needsRenewal(now.Add(lifetime-61*time.Minute), now, lifetime))
	assert.True(t, needsRenewal(now.Add(time.Hour), now, lifetime), "about to expire")
	assert.False(t, needsRenewal(now.Add(time.Hour), now, 0), "renewal off")
	assert.True(t, needsRenewal(now.Add(29*time.Minute), now, time.Hour), "short lifetimes renew after half of it")
	assert.False(t, needsRenewal(now.Add(40*time.Minute), now, time.Hour))
}

func newRenewingStore(t *testing.T, renewFor time.Duration) (*SessionStoreDB, *db.Session) {
	t.Helper()
	db.SetupTestDB(t.Name())
	store := NewSessionStore(repositories.NewSessionRepositoryDB(db.GetConnection()), time.Hour)
	store.RenewFor = renewFor
	sess, err := store.NewSession(nil, &db.User{ID: "u1", Username: "u1"}, "basic", 24*time.Hour)
	require.NoError(t, err)
	return store, sess
}

// serve runs one request carrying the session cookie through RenewCookies.
func serve(store *SessionStoreDB, token string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	RenewCookies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store.ValidateSession(r)
		if handler != nil {
			handler(w, r)
			return
		}
		w.Write([]byte("ok"))
	})).ServeHTTP(w, req)
	return w
}

func sessionCookies(w *httptest.ResponseRecorder) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName {
			out = append(out, c)
		}
	}
	return out
}

func TestValidateSession_RenewsAndResendsTheCookie(t *testing.T) {
	store, sess := newRenewingStore(t, 15*24*time.Hour)

	w := serve(store, sess.Token, nil)
	cookies := sessionCookies(w)
	require.Len(t, cookies, 1, "the renewed cookie is sent")
	assert.Equal(t, sess.Token, cookies[0].Value)
	assert.Equal(t, 15*24*60*60, cookies[0].MaxAge)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)

	stored, err := store.Repo.FindSessionByToken(sess.Token)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(15*24*time.Hour), stored.ValidUntil, time.Minute, "renewed to now + 15 days")

	w = serve(store, sess.Token, nil)
	assert.Empty(t, sessionCookies(w), "not renewed again within the hour")
}

func TestValidateSession_NoRenewalWhenOff(t *testing.T) {
	store, sess := newRenewingStore(t, 0)
	w := serve(store, sess.Token, nil)
	assert.Empty(t, sessionCookies(w))
	stored, err := store.Repo.FindSessionByToken(sess.Token)
	require.NoError(t, err)
	assert.WithinDuration(t, sess.ValidUntil, stored.ValidUntil, time.Second, "the expiry is unchanged")
}

func TestRenewCookies_LeavesAHandlersOwnSessionCookie(t *testing.T) {
	store, sess := newRenewingStore(t, 15*24*time.Hour)
	w := serve(store, sess.Token, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1}) // e.g. logout
		w.WriteHeader(http.StatusFound)
	})
	cookies := sessionCookies(w)
	require.Len(t, cookies, 1)
	assert.Equal(t, "", cookies[0].Value, "logout's clearing cookie wins")
}

func TestRenewCookies_KeepsFlushing(t *testing.T) {
	store, sess := newRenewingStore(t, 15*24*time.Hour)
	w := serve(store, sess.Token, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("chunk"))
		require.NoError(t, http.NewResponseController(w).Flush())
	})
	assert.True(t, w.Flushed)
	assert.Len(t, sessionCookies(w), 1)
}
