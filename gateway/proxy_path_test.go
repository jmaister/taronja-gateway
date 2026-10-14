package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripPathPrefix(t *testing.T) {
	cases := []struct{ path, prefix, want string }{
		{"/api/users", "/api/", "/users"},
		{"/api/users", "/api", "/users"},
		{"/api", "/api", "/"},
		{"/api/", "/api/", "/"},
		{"/apiary/x", "/api", "/apiary/x"},
		{"/other", "/api/", "/other"},
		{"/x", "", "/x"},
		{"/x", "/", "/x"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, stripPathPrefix(c.path, c.prefix), "%s minus %s", c.path, c.prefix)
	}
}

func TestProxyPath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.URL.RequestURI())
	}))
	defer backend.Close()

	cases := []struct {
		name, to, remove, request, want string
	}{
		{"no base path, prefix stripped", backend.URL, "/api/", "/api/users", "/users"},
		{"base path joined once, prefix stripped", backend.URL + "/v2", "/api/", "/api/users", "/v2/users"},
		{"base path joined once, no removeFromPath", backend.URL + "/v2", "", "/api/users", "/v2/api/users"},
		{"base path with trailing slash", backend.URL + "/v2/", "/api", "/api/users", "/v2/users"},
		{"prefix only on a segment boundary", backend.URL, "/api", "/apiary/x", "/apiary/x"},
		{"exact prefix becomes root", backend.URL + "/v2", "/api", "/api", "/v2/"},
		{"query string kept", backend.URL, "/api/", "/api/users?page=2", "/users?page=2"},
		{"escaped characters kept", backend.URL, "/api/", "/api/a%20b", "/a%20b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, err := url.Parse(c.to)
			require.NoError(t, err)
			targets := []*url.URL{target}
			gw, err := NewGatewayWithDependencies(&config.GatewayConfig{}, nil, deps.NewTest())
			require.NoError(t, err)
			handler := gw.createProxyHandlerFunc(config.RouteConfig{Name: c.name, From: "/", To: config.RouteTargets{c.to}, RemoveFromPath: c.remove}, targets)

			rr := httptest.NewRecorder()
			handler(rr, httptest.NewRequest(http.MethodGet, c.request, nil))
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, c.want, rr.Body.String())
		})
	}
}
