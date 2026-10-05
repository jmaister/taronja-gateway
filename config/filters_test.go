package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestFilterConfig_IsEnabled(t *testing.T) {
	assert.True(t, FilterConfig{Name: "logging"}.IsEnabled(), "absent enabled means true")
	assert.True(t, FilterConfig{Name: "logging", Enabled: boolPtr(true)}.IsEnabled())
	assert.False(t, FilterConfig{Name: "logging", Enabled: boolPtr(false)}.IsEnabled())
}

// writeTestConfig writes a minimal valid gateway config, augmented with extraYAML
// appended at the end, to a temp file and returns its path.
func writeTestConfig(t *testing.T, extraYAML string) string {
	t.Helper()
	base := `
version: 1
name: Test Gateway
server:
  host: 127.0.0.1
  port: 8080
management:
  admin:
    enabled: false
routes:
  - name: root
    from: /
    to: http://localhost:9999
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base+extraYAML), 0o644))
	return path
}

func TestLoadConfig_NoFilters(t *testing.T) {
	cfg, err := LoadConfig(writeTestConfig(t, ""))
	require.NoError(t, err)
	assert.Empty(t, cfg.Filters)
}

func TestLoadConfig_FiltersParsedInOrder(t *testing.T) {
	cfg, err := LoadConfig(writeTestConfig(t, `
filters:
  - name: rate_limiter
    config:
      requestsPerMinute: 42
  - name: logging
    enabled: false
`))
	require.NoError(t, err)
	require.Len(t, cfg.Filters, 2)

	rl := cfg.Filters[0]
	assert.Equal(t, "rate_limiter", rl.Name)
	assert.True(t, rl.IsEnabled())
	var opts struct {
		RequestsPerMinute int `yaml:"requestsPerMinute"`
	}
	require.NotNil(t, rl.Config)
	require.NoError(t, rl.Config.Decode(&opts))
	assert.Equal(t, 42, opts.RequestsPerMinute)

	assert.Equal(t, "logging", cfg.Filters[1].Name)
	assert.False(t, cfg.Filters[1].IsEnabled())
	assert.Nil(t, cfg.Filters[1].Config)
}

func TestLoadConfig_FilterWithoutNameRejected(t *testing.T) {
	_, err := LoadConfig(writeTestConfig(t, "filters:\n  - enabled: true\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'name'")
}

func TestLoadConfig_DuplicateFilterRejected(t *testing.T) {
	_, err := LoadConfig(writeTestConfig(t, "filters:\n  - name: logging\n  - name: logging\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}

func TestLoadConfig_UnknownKeysRejected(t *testing.T) {
	_, err := LoadConfig(writeTestConfig(t, "tracing:\n  endpoint: localhost:4318\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field tracing not found")
}

func TestNewFilterConfig_EncodesOptions(t *testing.T) {
	f := NewFilterConfig("cors", map[string]any{"allowedOrigins": []string{"https://a.example"}})
	assert.Equal(t, "cors", f.Name)
	var opts struct {
		AllowedOrigins []string `yaml:"allowedOrigins"`
	}
	require.NoError(t, f.Config.Decode(&opts))
	assert.Equal(t, []string{"https://a.example"}, opts.AllowedOrigins)

	assert.Nil(t, NewFilterConfig("logging", nil).Config)
}
