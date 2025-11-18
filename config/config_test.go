package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteOptions_GetCacheControlHeader(t *testing.T) {
	tests := []struct {
		name     string
		opts     *RouteOptions
		expected string
	}{
		{
			name:     "nil options should return empty string",
			opts:     nil,
			expected: "",
		},
		{
			name:     "nil CacheControlSeconds should return empty string",
			opts:     &RouteOptions{CacheControlSeconds: nil},
			expected: "",
		},
		{
			name:     "zero CacheControlSeconds should return no-cache",
			opts:     &RouteOptions{CacheControlSeconds: intPtr(0)},
			expected: "no-cache",
		},
		{
			name:     "positive CacheControlSeconds should return max-age",
			opts:     &RouteOptions{CacheControlSeconds: intPtr(3600)},
			expected: "max-age=3600",
		},
		{
			name:     "negative CacheControlSeconds should return empty string",
			opts:     &RouteOptions{CacheControlSeconds: intPtr(-1)},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.getCacheControlHeader()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouteConfig_ShouldSetCacheHeader(t *testing.T) {
	tests := []struct {
		name     string
		route    *RouteConfig
		expected bool
	}{
		{
			name:     "nil options should return false",
			route:    &RouteConfig{},
			expected: false,
		},
		{
			name:     "nil CacheControlSeconds should return false",
			route:    &RouteConfig{Options: &RouteOptions{CacheControlSeconds: nil}},
			expected: false,
		},
		{
			name:     "zero CacheControlSeconds should return true",
			route:    &RouteConfig{Options: &RouteOptions{CacheControlSeconds: intPtr(0)}},
			expected: true,
		},
		{
			name:     "positive CacheControlSeconds should return true",
			route:    &RouteConfig{Options: &RouteOptions{CacheControlSeconds: intPtr(3600)}},
			expected: true,
		},
		{
			name:     "negative CacheControlSeconds should return false",
			route:    &RouteConfig{Options: &RouteOptions{CacheControlSeconds: intPtr(-1)}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.route.ShouldSetCacheHeader()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouteConfig_CacheControlMethods(t *testing.T) {
	// Test route with no options
	route1 := &RouteConfig{
		Name: "test-route-1",
		From: "/test1",
		To:   "http://example.com",
	}

	if route1.Options == nil {
		assert.Nil(t, route1.Options)
	} else {
		assert.Nil(t, route1.Options.CacheControlSeconds)
	}
	assert.Equal(t, "", route1.GetCacheControlHeader())
	assert.False(t, route1.ShouldSetCacheHeader())

	// Test route with cache control options
	route2 := &RouteConfig{
		Name: "test-route-2",
		From: "/test2",
		To:   "http://example.com",
		Options: &RouteOptions{
			CacheControlSeconds: intPtr(7200),
		},
	}

	if route2.Options == nil {
		t.Fatal("route2.Options should not be nil")
	}
	assert.NotNil(t, route2.Options.CacheControlSeconds)
	assert.Equal(t, 7200, *route2.Options.CacheControlSeconds)
	assert.Equal(t, "max-age=7200", route2.GetCacheControlHeader())
	assert.True(t, route2.ShouldSetCacheHeader())

	// Test route with no-cache
	route3 := &RouteConfig{
		Name: "test-route-3",
		From: "/test3",
		To:   "http://example.com",
		Options: &RouteOptions{
			CacheControlSeconds: intPtr(0),
		},
	}

	if route3.Options == nil {
		t.Fatal("route3.Options should not be nil")
	}
	assert.NotNil(t, route3.Options.CacheControlSeconds)
	assert.Equal(t, 0, *route3.Options.CacheControlSeconds)
	assert.Equal(t, "no-cache", route3.GetCacheControlHeader())
	assert.True(t, route3.ShouldSetCacheHeader())
}

// Helper function to create int pointers
func intPtr(i int) *int {
	return &i
}
