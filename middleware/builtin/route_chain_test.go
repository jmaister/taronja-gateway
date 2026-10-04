package builtin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/stretchr/testify/assert"
)

func TestRouteSpecs_OrderAndRedirect(t *testing.T) {
	plain := routeSpecs(config.RouteConfig{})
	assert.Equal(t, []string{routeMiddlewareCache}, specNames(plain))

	api := routeSpecs(config.RouteConfig{Authentication: config.AuthenticationConfig{Enabled: true}})
	assert.Equal(t, []string{routeMiddlewareAuth, routeMiddlewareCache}, specNames(api))
	assert.False(t, api[0].Config.(RouteAuthConfig).Redirect)

	spa := routeSpecs(config.RouteConfig{IsSPA: true, Authentication: config.AuthenticationConfig{Enabled: true}})
	assert.True(t, spa[0].Config.(RouteAuthConfig).Redirect)
}

func TestBuildRouteChain_AppliesCacheControl(t *testing.T) {
	builder := NewRouteChainBuilder(NewAuthMiddleware(nil, nil, "/_"), NewHttpCacheMiddleware())
	route := config.RouteConfig{}
	handler := builder.BuildRouteChain(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, route)

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func specNames(specs []middleware.MiddlewareSpec) []string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}
