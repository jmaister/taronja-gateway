package middleware

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
)

// MiddlewareFunc represents a standard middleware function type
// It takes an http.Handler and returns an http.Handler
type MiddlewareFunc func(http.Handler) http.Handler

// HandlerFunc represents a standard handler function type
type HandlerFunc func(http.ResponseWriter, *http.Request)

// MiddlewareConfig represents configuration for middleware
type MiddlewareConfig struct {
	Enabled bool
	Options map[string]interface{}
}

// RouteMiddleware represents middleware that can be applied to individual routes
type RouteMiddleware interface {
	// ApplyToRoute applies the middleware to a handler for a specific route
	ApplyToRoute(handler http.HandlerFunc, routeConfig config.RouteConfig) http.HandlerFunc
}

// GlobalMiddleware represents middleware that is applied globally
type GlobalMiddleware interface {
	// GetMiddlewareFunc returns the middleware function
	GetMiddlewareFunc() MiddlewareFunc

	// IsEnabled returns whether this middleware should be applied
	IsEnabled() bool
}

// ConfigurableMiddleware represents middleware that can be configured
type ConfigurableMiddleware interface {
	// Configure sets up the middleware with the given configuration
	Configure(config MiddlewareConfig) error

	// GetConfig returns the current configuration
	GetConfig() MiddlewareConfig
}

// NamedMiddleware represents middleware with a name for identification
type NamedMiddleware interface {
	// GetName returns the name of the middleware
	GetName() string

	// GetDescription returns a description of what the middleware does
	GetDescription() string
}

// FullMiddleware combines all middleware interfaces for complete middleware implementations
type FullMiddleware interface {
	GlobalMiddleware
	RouteMiddleware
	ConfigurableMiddleware
	NamedMiddleware
}

// StandardGlobalMiddleware provides a base implementation for global middleware
type StandardGlobalMiddleware struct {
	name        string
	description string
	enabled     bool
	config      MiddlewareConfig
}

// NewStandardGlobalMiddleware creates a new standard global middleware
func NewStandardGlobalMiddleware(name, description string, enabled bool) *StandardGlobalMiddleware {
	return &StandardGlobalMiddleware{
		name:        name,
		description: description,
		enabled:     enabled,
		config: MiddlewareConfig{
			Enabled: enabled,
			Options: make(map[string]interface{}),
		},
	}
}

// GetName returns the name of the middleware
func (s *StandardGlobalMiddleware) GetName() string {
	return s.name
}

// GetDescription returns the description of the middleware
func (s *StandardGlobalMiddleware) GetDescription() string {
	return s.description
}

// IsEnabled returns whether the middleware is enabled
func (s *StandardGlobalMiddleware) IsEnabled() bool {
	return s.enabled && s.config.Enabled
}

// Configure sets up the middleware configuration
func (s *StandardGlobalMiddleware) Configure(config MiddlewareConfig) error {
	s.config = config
	s.enabled = config.Enabled
	return nil
}

// GetConfig returns the current configuration
func (s *StandardGlobalMiddleware) GetConfig() MiddlewareConfig {
	return s.config
}

// MiddlewareRegistry maintains a registry of all available middleware
type MiddlewareRegistry struct {
	globalMiddleware map[string]GlobalMiddleware
	routeMiddleware  map[string]RouteMiddleware
}

// NewMiddlewareRegistry creates a new middleware registry
func NewMiddlewareRegistry() *MiddlewareRegistry {
	return &MiddlewareRegistry{
		globalMiddleware: make(map[string]GlobalMiddleware),
		routeMiddleware:  make(map[string]RouteMiddleware),
	}
}

// RegisterGlobal registers a global middleware
func (r *MiddlewareRegistry) RegisterGlobal(name string, middleware GlobalMiddleware) {
	r.globalMiddleware[name] = middleware
}

// RegisterRoute registers a route middleware
func (r *MiddlewareRegistry) RegisterRoute(name string, middleware RouteMiddleware) {
	r.routeMiddleware[name] = middleware
}

// GetGlobal returns a global middleware by name
func (r *MiddlewareRegistry) GetGlobal(name string) (GlobalMiddleware, bool) {
	middleware, exists := r.globalMiddleware[name]
	return middleware, exists
}

// GetRoute returns a route middleware by name
func (r *MiddlewareRegistry) GetRoute(name string) (RouteMiddleware, bool) {
	middleware, exists := r.routeMiddleware[name]
	return middleware, exists
}

// ListGlobal returns all registered global middleware names
func (r *MiddlewareRegistry) ListGlobal() []string {
	names := make([]string, 0, len(r.globalMiddleware))
	for name := range r.globalMiddleware {
		names = append(names, name)
	}
	return names
}

// ListRoute returns all registered route middleware names
func (r *MiddlewareRegistry) ListRoute() []string {
	names := make([]string, 0, len(r.routeMiddleware))
	for name := range r.routeMiddleware {
		names = append(names, name)
	}
	return names
}
