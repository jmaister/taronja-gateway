package middleware

import "net/http"

// ChainBuilder provides a fluent interface for building middleware chains
type ChainBuilder struct {
	middlewares []Middleware
}

// Middleware represents a middleware function
type Middleware func(http.Handler) http.Handler

// NewChainBuilder creates a new middleware chain builder
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{
		middlewares: make([]Middleware, 0),
	}
}

// Add adds a middleware to the chain
func (c *ChainBuilder) Add(middleware Middleware) *ChainBuilder {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Build creates the final middleware chain by wrapping all middlewares around the given handler
func (c *ChainBuilder) Build(handler http.Handler) http.Handler {
	// Apply middlewares in reverse order so they execute in the order they were added
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}
	return handler
}

// Chain is a simple utility function to chain middlewares without using a builder
// Usage: Chain(handler, middleware1, middleware2, ...)
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
