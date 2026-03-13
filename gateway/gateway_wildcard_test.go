package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasMiddleWildcard verifies the detection of non-trailing wildcards.
func TestHasMiddleWildcard(t *testing.T) {
	tests := []struct {
		from string
		want bool
	}{
		{"/api/*", false},
		{"/api/boxes/*/certs", true},
		{"/api/*/b/*/c", true},
		{"/api/boxes/*/certs/*", true},
		{"/api/plain", false},
		{"/no/wildcards/here", false},
	}

	for _, tt := range tests {
		t.Run(tt.from, func(t *testing.T) {
			got := hasMiddleWildcard(tt.from)
			assert.Equal(t, tt.want, got)
		})
	}
}
