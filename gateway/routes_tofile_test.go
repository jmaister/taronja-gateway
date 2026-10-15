package gateway

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A toFile route whose From already ends in "/" used to register the same
// ServeMux pattern twice, which panics at startup.
func TestToFileRouteFromSlashRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "index.html")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0o600))

	gw, err := NewGatewayWithDependencies(&config.GatewayConfig{
		Server: config.ServerConfig{Host: "localhost"},
		Routes: []config.RouteConfig{{Name: "root", From: "/", ToFile: file}},
	}, nil, deps.NewTestWithName(t.Name()))
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	gw.Server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
}
