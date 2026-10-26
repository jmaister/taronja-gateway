package providers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeOIDCServer is a minimal OpenID Connect provider: discovery, a token
// endpoint that checks the PKCE verifier and client authentication, and a
// userinfo endpoint.
type fakeOIDCServer struct {
	*httptest.Server
	t *testing.T

	mu         sync.Mutex
	issuer     string // overrides the advertised issuer when set
	down       bool
	challenges map[string]string // code -> PKCE challenge
	claims     map[string]any
	// lastTokenForm and lastBasicAuth record how the client authenticated.
	lastTokenForm url.Values
	lastBasicUser string
}

func newFakeOIDCServer(t *testing.T) *fakeOIDCServer {
	f := &fakeOIDCServer{t: t, challenges: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeOIDCServer) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		issuer := f.URL
		if f.issuer != "" {
			issuer = f.issuer
		}
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 issuer,
			"authorization_endpoint": f.URL + "/authorize",
			"token_endpoint":         f.URL + "/token",
			"userinfo_endpoint":      f.URL + "/userinfo",
		})
	case "/token":
		require.NoError(f.t, r.ParseForm())
		f.lastTokenForm = r.PostForm
		f.lastBasicUser, _, _ = r.BasicAuth()
		challenge, ok := f.challenges[r.PostForm.Get("code")]
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		delete(f.challenges, r.PostForm.Get("code"))
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at-" + r.PostForm.Get("code"), "token_type": "Bearer", "expires_in": 3600})
	case "/userinfo":
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(f.claims)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeOIDCServer) set(edit func(*fakeOIDCServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	edit(f)
}

type oidcHarness struct {
	t      *testing.T
	mux    *http.ServeMux
	server *fakeOIDCServer
}

func newOIDCHarness(t *testing.T, edit func(*config.OIDCProviderConfig)) *oidcHarness {
	server := newFakeOIDCServer(t)
	cfg := config.OIDCProviderConfig{Name: "zid", Label: "Zitadel", Issuer: server.URL, ClientId: "client-1", ClientSecret: "secret-1"}
	if edit != nil {
		edit(&cfg)
	}
	gatewayConfig := &config.GatewayConfig{
		Server:     config.ServerConfig{URL: "http://gateway.test"},
		Management: config.ManagementConfig{Session: config.SessionConfig{DurationSeconds: 3600}},
	}
	mux := http.NewServeMux()
	NewOIDCAuthenticationProvider(cfg, testSessionStore, gatewayConfig, testUserRepo, testAccountsFor(testUserRepo)).RegisterEndpoints(mux)
	return &oidcHarness{t: t, mux: mux, server: server}
}

// login runs the whole flow: start, "authorize" at the fake provider, callback.
func (h *oidcHarness) login(claims map[string]any) *httptest.ResponseRecorder {
	h.t.Helper()
	h.server.set(func(f *fakeOIDCServer) { f.claims = claims })

	start := httptest.NewRecorder()
	h.mux.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/_/auth/zid/login?redirect=/app", nil))
	require.Equal(h.t, http.StatusTemporaryRedirect, start.Code, start.Body.String())
	authorize, err := url.Parse(start.Header().Get("Location"))
	require.NoError(h.t, err)
	require.Equal(h.t, h.server.URL+"/authorize", authorize.Scheme+"://"+authorize.Host+authorize.Path)
	q := authorize.Query()
	assert.Equal(h.t, "S256", q.Get("code_challenge_method"))
	assert.Equal(h.t, "http://gateway.test/_/auth/zid/callback", q.Get("redirect_uri"))

	code := "code-" + q.Get("state")
	h.server.set(func(f *fakeOIDCServer) { f.challenges[code] = q.Get("code_challenge") })

	callback := httptest.NewRequest(http.MethodGet, "/_/auth/zid/callback?state="+url.QueryEscape(q.Get("state"))+"&code="+url.QueryEscape(code), nil)
	for _, c := range start.Result().Cookies() {
		callback.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, callback)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == session.SessionCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestOIDCLogin_CreatesUserWithPKCE(t *testing.T) {
	h := newOIDCHarness(t, nil)
	w := h.login(map[string]any{
		"sub": "zid-user-1", "email": "oidc-new@example.com", "email_verified": true,
		"preferred_username": "oidc-new", "name": "Oidc New", "given_name": "Oidc", "family_name": "New",
	})
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.Equal(t, "/app", w.Header().Get("Location"))
	require.NotNil(t, sessionCookie(w))

	user, err := testUserRepo.FindUserByEmail("oidc-new@example.com")
	require.NoError(t, err)
	assert.Equal(t, "zid", user.Provider)
	assert.Equal(t, "oidc-new", user.Username)
	assert.Equal(t, "Oidc New", user.Name)
	identity, err := repositories.NewUserIdentityRepositoryDB(testDBConn()).FindByProviderSubject("zid", "zid-user-1")
	require.NoError(t, err)
	assert.Equal(t, user.ID, identity.UserID)

	assert.Equal(t, "client-1", h.server.lastBasicUser, "a confidential client authenticates with its secret")
	assert.NotEmpty(t, h.server.lastTokenForm.Get("code_verifier"))
}

func TestOIDCLogin_PublicClientSendsClientIDInBody(t *testing.T) {
	h := newOIDCHarness(t, func(c *config.OIDCProviderConfig) { c.ClientSecret = "" })
	w := h.login(map[string]any{"sub": "zid-public", "email": "oidc-public@example.com", "email_verified": "true"})
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	require.NotNil(t, sessionCookie(w), `email_verified as the string "true" counts`)
	assert.Equal(t, "", h.server.lastBasicUser)
	assert.Equal(t, "client-1", h.server.lastTokenForm.Get("client_id"))
}

func TestOIDCLogin_UnverifiedEmail(t *testing.T) {
	h := newOIDCHarness(t, nil)
	w := h.login(map[string]any{"sub": "zid-unverified", "email": "oidc-unverified@example.com"})
	assert.Equal(t, "/_/login?error=email_not_verified", w.Header().Get("Location"))
	assert.Nil(t, sessionCookie(w))

	trusting := newOIDCHarness(t, func(c *config.OIDCProviderConfig) { c.TrustEmail = true })
	w = trusting.login(map[string]any{"sub": "zid-trusted", "email": "oidc-trusted@example.com"})
	assert.Equal(t, "/app", w.Header().Get("Location"), "trustEmail accepts emails without the claim")
	assert.NotNil(t, sessionCookie(w))
}

func TestOIDCLogin_RejectsWrongPKCEVerifier(t *testing.T) {
	h := newOIDCHarness(t, nil)
	h.server.set(func(f *fakeOIDCServer) {
		f.claims = map[string]any{"sub": "x", "email": "x@example.com", "email_verified": true}
	})
	start := httptest.NewRecorder()
	h.mux.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/_/auth/zid/login", nil))
	q, _ := url.Parse(start.Header().Get("Location"))
	state := q.Query().Get("state")
	h.server.set(func(f *fakeOIDCServer) { f.challenges["c"] = q.Query().Get("code_challenge") })

	callback := httptest.NewRequest(http.MethodGet, "/_/auth/zid/callback?state="+url.QueryEscape(state)+"&code=c", nil)
	for _, c := range start.Result().Cookies() {
		if c.Name == PKCEVerifierCookieName {
			c.Value = "a-different-verifier-than-the-one-the-challenge-was-made-from"
		}
		callback.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, callback)
	assert.Equal(t, http.StatusInternalServerError, w.Code, "the provider refuses the exchange")
	assert.Nil(t, sessionCookie(w))

	noVerifier := httptest.NewRequest(http.MethodGet, "/_/auth/zid/callback?state="+url.QueryEscape(state)+"&code=c", nil)
	noVerifier.AddCookie(&http.Cookie{Name: StateCookieName, Value: state})
	w = httptest.NewRecorder()
	h.mux.ServeHTTP(w, noVerifier)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOIDCDiscovery_RetriesUntilTheProviderIsUp(t *testing.T) {
	h := newOIDCHarness(t, nil)
	h.server.set(func(f *fakeOIDCServer) { f.down = true })

	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/auth/zid/login", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Zitadel login is unavailable")

	h.server.set(func(f *fakeOIDCServer) { f.down = false })
	w = h.login(map[string]any{"sub": "zid-later", "email": "oidc-later@example.com", "email_verified": true})
	assert.Equal(t, "/app", w.Header().Get("Location"))
}

func TestOIDCDiscovery_RejectsIssuerMismatch(t *testing.T) {
	h := newOIDCHarness(t, nil)
	h.server.set(func(f *fakeOIDCServer) { f.issuer = "https://someone-else.example" })
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/auth/zid/login", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func testDBConn() *gorm.DB { return db.GetConnection() }
