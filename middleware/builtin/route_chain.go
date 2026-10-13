package builtin

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
)

// WrapRoute wraps a route's handler in its middleware: authentication when the
// route sets `authenticated: true`, then Cache-Control, which always applies.
// Browser-facing routes (static, SPA) redirect to login; plain proxy/API
// routes answer 401.
func WrapRoute(handler http.HandlerFunc, routeConfig config.RouteConfig, auth *AuthMiddleware) http.HandlerFunc {
	wrapped := CacheControlMiddleware(routeConfig)(handler)
	if routeConfig.Authenticated {
		wrapped = auth.AuthMiddlewareFunc(routeConfig.IsStatic() || routeConfig.IsSPA)(wrapped)
	}
	return wrapped.(http.HandlerFunc)
}
