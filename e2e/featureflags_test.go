//go:build e2e

package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An admin flips a flag in the API; anonymous frontends see it on /_/flags and
// proxied backends see it in X-Feature-Flags, with no restart.
func TestFeatureFlags_AdminToggleReachesFrontendsAndBackends(t *testing.T) {
	backendSaw := make(chan string, 10)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendSaw <- r.Header.Get("X-Feature-Flags")
	}))
	defer backend.Close()

	gw := startGateway(t, baseConfig("")+`  - name: api
    from: /api/*
    to: `+backend.URL+`
`)
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	anonymous := newClient(t, gw.URL)

	callBackend := func() string {
		req, err := http.NewRequest(http.MethodGet, gw.URL+"/api/orders", nil)
		require.NoError(t, err)
		req.Header.Set("X-Feature-Flags", "forged")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return <-backendSaw
	}

	assert.Equal(t, map[string]any{}, anonymous.getJSON("/_/flags", http.StatusOK)["flags"])
	assert.Equal(t, "", callBackend())

	anonymous.sendJSON(http.MethodPost, "/_/api/feature-flags", map[string]any{"key": "sneaky"}, http.StatusUnauthorized)
	admin.sendJSON(http.MethodPost, "/_/api/feature-flags", map[string]any{
		"key": "new-checkout", "description": "Checkout v2", "enabled": true,
	}, http.StatusCreated)
	admin.sendJSON(http.MethodPost, "/_/api/feature-flags", map[string]any{"key": "dark-mode"}, http.StatusCreated)

	assert.Equal(t, map[string]any{"new-checkout": true, "dark-mode": false}, anonymous.getJSON("/_/flags", http.StatusOK)["flags"])
	assert.Equal(t, "new-checkout", callBackend())

	admin.sendJSON(http.MethodPatch, "/_/api/feature-flags/dark-mode", map[string]any{"enabled": true}, http.StatusOK)
	assert.Equal(t, "dark-mode,new-checkout", callBackend())

	admin.sendJSON(http.MethodDelete, "/_/api/feature-flags/new-checkout", nil, http.StatusNoContent)
	assert.Equal(t, map[string]any{"dark-mode": true}, anonymous.getJSON("/_/flags", http.StatusOK)["flags"])
	assert.Equal(t, "dark-mode", callBackend())
}
