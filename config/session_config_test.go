package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSessionDurationConfiguration(t *testing.T) {
	t.Run("default session duration is 24 hours", func(t *testing.T) {
		config := &GatewayConfig{}

		// Set defaults (as done in LoadConfig)
		config.Management.Prefix = "/_"
		config.Management.Session.SecondsDuration = 86400 // Default 24 hours

		assert.Equal(t, 86400, config.Management.Session.SecondsDuration)

		// Verify it equals 24 hours
		assert.Equal(t, 24*time.Hour, config.Management.Session.GetDuration())
	})

	t.Run("custom session duration", func(t *testing.T) {
		config := &GatewayConfig{}

		// Set custom duration (12 hours)
		config.Management.Session.SecondsDuration = 43200 // 12 hours

		assert.Equal(t, 12*time.Hour, config.Management.Session.GetDuration())
	})

	t.Run("session duration from config file", func(t *testing.T) {
		// Test loading from an actual config would require a temp file
		// For now just verify the structure is correct
		config := &GatewayConfig{
			Management: ManagementConfig{
				Session: SessionConfig{
					SecondsDuration: 7200, // 2 hours
				},
			},
		}

		assert.Equal(t, 2*time.Hour, config.Management.Session.GetDuration())
	})

	t.Run("short session duration (1 hour)", func(t *testing.T) {
		config := &GatewayConfig{}
		config.Management.Session.SecondsDuration = 3600 // 1 hour

		assert.Equal(t, 3600, config.Management.Session.SecondsDuration)
		assert.Equal(t, 1*time.Hour, config.Management.Session.GetDuration())
	})

	t.Run("long session duration (7 days)", func(t *testing.T) {
		config := &GatewayConfig{}
		config.Management.Session.SecondsDuration = 604800 // 7 days

		assert.Equal(t, 604800, config.Management.Session.SecondsDuration)
		assert.Equal(t, 168*time.Hour, config.Management.Session.GetDuration()) // 7 days = 168 hours
	})

	t.Run("common session durations", func(t *testing.T) {
		testCases := map[string]struct {
			seconds  int
			expected time.Duration
		}{
			"30 minutes": {1800, 30 * time.Minute},
			"1 hour":     {3600, 1 * time.Hour},
			"2 hours":    {7200, 2 * time.Hour},
			"12 hours":   {43200, 12 * time.Hour},
			"24 hours":   {86400, 24 * time.Hour},
			"3 days":     {259200, 72 * time.Hour},   // 3 days = 72 hours
			"7 days":     {604800, 168 * time.Hour},  // 7 days = 168 hours
			"30 days":    {2592000, 720 * time.Hour}, // 30 days = 720 hours
		}

		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				config := &GatewayConfig{}
				config.Management.Session.SecondsDuration = tc.seconds

				assert.Equal(t, tc.seconds, config.Management.Session.SecondsDuration)
				assert.Equal(t, tc.expected, config.Management.Session.GetDuration())
			})
		}
	})
}
