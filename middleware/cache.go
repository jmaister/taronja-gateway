package middleware

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
)

// HttpCacheMiddleware provides cache control middleware functionality
type HttpCacheMiddleware struct{}

// NewHttpCacheMiddleware creates a new cache control middleware
func NewHttpCacheMiddleware() *HttpCacheMiddleware {
	return &HttpCacheMiddleware{}
}

// CacheControlMiddlewareFunc creates a middleware function for cache control
func (c *HttpCacheMiddleware) CacheControlMiddlewareFunc(routeConfig config.RouteConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set Cache-Control header if configured for this route
			if routeConfig.ShouldSetCacheHeader() {
				w.Header().Set("Cache-Control", routeConfig.GetCacheControlHeader())
			}
			next.ServeHTTP(w, r)
		})
	}
}
