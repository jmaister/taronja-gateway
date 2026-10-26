package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/featureflag"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxySendsFeatureFlagsHeader(t *testing.T) {
	var received []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get(featureflag.HeaderName))
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	d := deps.NewTestWithName(t.Name())
	gw, err := NewGatewayWithDependencies(&config.GatewayConfig{
		Server: config.ServerConfig{Host: "localhost"},
		Routes: []config.RouteConfig{{Name: "api", From: "/api/*", To: []string{backend.URL}}},
	}, nil, d)
	require.NoError(t, err)
	server := httptest.NewServer(gw.Server.Handler)
	defer server.Close()

	get := func() {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/api/x", nil)
		require.NoError(t, err)
		req.Header.Set(featureflag.HeaderName, "forged-by-client")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	get()
	_, err = d.FeatureFlags.Create(featureflag.CreateInput{Key: "new-checkout", Enabled: true})
	require.NoError(t, err)
	_, err = d.FeatureFlags.Create(featureflag.CreateInput{Key: "beta", Enabled: true})
	require.NoError(t, err)
	_, err = d.FeatureFlags.Create(featureflag.CreateInput{Key: "off"})
	require.NoError(t, err)
	get()

	assert.Equal(t, []string{"", "beta,new-checkout"}, received, "client copies are dropped; enabled keys are sent sorted")
}
