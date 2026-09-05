package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORSConfig_IsEnabled(t *testing.T) {
	assert.False(t, CORSConfig{}.IsEnabled(), "no allowed origins means disabled")
	assert.True(t, CORSConfig{AllowedOrigins: []string{"https://example.com"}}.IsEnabled())
	assert.True(t, CORSConfig{AllowedOrigins: []string{"*"}}.IsEnabled())
}

func TestCORSConfig_AllowsAnyOrigin(t *testing.T) {
	assert.False(t, CORSConfig{}.AllowsAnyOrigin())
	assert.False(t, CORSConfig{AllowedOrigins: []string{"https://example.com"}}.AllowsAnyOrigin())
	assert.True(t, CORSConfig{AllowedOrigins: []string{"https://example.com", "*"}}.AllowsAnyOrigin())
}
