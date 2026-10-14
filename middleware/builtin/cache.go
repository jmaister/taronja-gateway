package builtin

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
)

// CacheControlMiddleware sets the route's Cache-Control header, when it has one.
func CacheControlMiddleware(routeConfig config.RouteConfig) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if routeConfig.ShouldSetCacheHeader() {
				w.Header().Set("Cache-Control", routeConfig.GetCacheControlHeader())
			}
			next.ServeHTTP(w, r)
		})
	}
}
