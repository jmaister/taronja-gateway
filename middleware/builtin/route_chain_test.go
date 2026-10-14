package builtin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/stretchr/testify/assert"
)

func TestWrapRoute_AppliesCacheControl(t *testing.T) {
	seconds := 60
	route := config.RouteConfig{Options: &config.RouteOptions{CacheControlSeconds: &seconds}}
	handler := WrapRoute(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, route, NewAuthMiddleware(nil, nil, "/_"))

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, route.GetCacheControlHeader(), rec.Header().Get("Cache-Control"))
}
