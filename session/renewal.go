package session

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// maxRenewInterval is the most a session goes without being renewed while it
// is used, so active sessions are not rewritten (and their cookie re-sent) on
// every request.
const maxRenewInterval = time.Hour

// renewInterval is how long after its last renewal a session with lifetime
// is renewed again: an hour, or half the lifetime if that is shorter.
func renewInterval(lifetime time.Duration) time.Duration {
	if lifetime/2 < maxRenewInterval {
		return lifetime / 2
	}
	return maxRenewInterval
}

// needsRenewal reports whether a session used at now should get a new
// expiry of now+lifetime.
func needsRenewal(validUntil, now time.Time, lifetime time.Duration) bool {
	return lifetime > 0 && validUntil.Before(now.Add(lifetime-renewInterval(lifetime)))
}

type renewalKey struct{}

// cookieRenewal carries a renewed session from ValidateSession, which has no
// ResponseWriter, to RenewCookies, which re-sends the cookie.
type cookieRenewal struct {
	mu     sync.Mutex
	token  string
	maxAge int
}

func (c *cookieRenewal) set(token string, lifetime time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	c.maxAge = int(lifetime.Seconds())
}

func (c *cookieRenewal) get() (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token, c.maxAge
}

func renewalFrom(ctx context.Context) *cookieRenewal {
	renewal, _ := ctx.Value(renewalKey{}).(*cookieRenewal)
	return renewal
}

// RenewCookies re-sends the session cookie with a fresh Max-Age when
// ValidateSession renewed the session during the request, so the browser
// keeps it as long as the server does. It must wrap every handler that
// validates sessions; the gateway installs it outermost.
func RenewCookies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renewal := &cookieRenewal{}
		r = r.WithContext(context.WithValue(r.Context(), renewalKey{}, renewal))
		next.ServeHTTP(&renewingWriter{ResponseWriter: w, r: r, renewal: renewal}, r)
	})
}

// renewingWriter adds the renewed cookie just before the headers go out.
type renewingWriter struct {
	http.ResponseWriter
	r       *http.Request
	renewal *cookieRenewal
	once    sync.Once
}

func (w *renewingWriter) addCookie() {
	w.once.Do(func() {
		token, maxAge := w.renewal.get()
		if token == "" {
			return
		}
		// Unless the handler is replacing or clearing the cookie itself
		// (login, logout).
		for _, existing := range w.Header().Values("Set-Cookie") {
			if c, err := http.ParseSetCookie(existing); err == nil && c.Name == SessionCookieName {
				return
			}
		}
		http.SetCookie(w.ResponseWriter, &http.Cookie{
			Name:     SessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   RequestIsSecure(w.r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   maxAge,
		})
	})
}

func (w *renewingWriter) WriteHeader(status int) {
	w.addCookie()
	w.ResponseWriter.WriteHeader(status)
}

func (w *renewingWriter) Write(b []byte) (int, error) {
	w.addCookie()
	return w.ResponseWriter.Write(b)
}

// Flush and Hijack keep streaming responses and WebSocket upgrades working
// through the wrapper.
func (w *renewingWriter) Flush() {
	w.addCookie()
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *renewingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *renewingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
