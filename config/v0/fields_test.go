package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestRenameFields(t *testing.T) {
	out := string(UpgradeToV1([]byte(`name: Test
management:
  prefix: _
  session:
    secondsDuration: 3600 # one hour
routes:
  - name: Site
    from: /
    toFolder: ./site
    static: true
    authentication:
      enabled: true
  - name: API
    from: /api/*
    to: http://localhost:9000
    authentication:
      enabled: false
`)))

	var cfg struct {
		Management struct {
			Prefix  *string        `yaml:"prefix"`
			Session map[string]int `yaml:"session"`
		} `yaml:"management"`
		Routes []map[string]any `yaml:"routes"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &cfg), out)
	assert.Nil(t, cfg.Management.Prefix)
	assert.Equal(t, map[string]int{"durationSeconds": 3600}, cfg.Management.Session)
	assert.Contains(t, out, "durationSeconds: 3600 # one hour", "comments are kept")

	require.Len(t, cfg.Routes, 2)
	assert.NotContains(t, cfg.Routes[0], "static")
	assert.NotContains(t, cfg.Routes[0], "authentication")
	assert.Equal(t, true, cfg.Routes[0]["authenticated"])
	assert.NotContains(t, cfg.Routes[1], "authentication")
	assert.NotContains(t, cfg.Routes[1], "authenticated", "a disabled requirement is just dropped")
}
