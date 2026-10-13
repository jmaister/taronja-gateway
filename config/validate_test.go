package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeRawTestConfig writes a minimal config (version, name, port) plus extraYAML.
func writeRawTestConfig(t *testing.T, extraYAML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: \"1.0\"\nname: Test\nserver:\n  port: 8080\n"+extraYAML), 0o644))
	return path
}

func TestLoadConfig_HostDefaultsToLocalhost(t *testing.T) {
	cfg, err := LoadConfig(writeRawTestConfig(t, ""))
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
}

func TestLoadConfig_VersionQuotedOrNot(t *testing.T) {
	for _, v := range []string{`"1.0"`, `1.0`, `'1.0'`} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("version: "+v+"\nname: Test\nserver:\n  port: 8080\n"), 0o644))
		cfg, err := LoadConfig(path)
		require.NoError(t, err, v)
		assert.Equal(t, "1.0", *cfg.Version, v)
	}
}

func TestLoadConfig_RejectsRemovedFields(t *testing.T) {
	for name, extra := range map[string]string{
		"management.prefix":    "management:\n  prefix: _\n",
		"route static":         "routes:\n  - name: s\n    from: /s\n    toFolder: .\n    static: true\n",
		"route authentication": "routes:\n  - name: s\n    from: /s\n    toFolder: .\n    authentication:\n      enabled: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(writeRawTestConfig(t, extra))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not found")
		})
	}
}

func TestLoadConfig_AuthenticatedRoute(t *testing.T) {
	cfg, err := LoadConfig(writeRawTestConfig(t, "routes:\n  - name: s\n    from: /s\n    toFolder: .\n    authenticated: true\n"))
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.True(t, cfg.Routes[0].Authenticated)
	assert.True(t, cfg.Routes[0].IsStatic())
}

func TestValidate_Routes(t *testing.T) {
	neg := -1
	cases := map[string]struct {
		route RouteConfig
		want  string
	}{
		"proxy":            {RouteConfig{Name: "r", From: "/", To: RouteTargets{"http://x"}}, ""},
		"folder":           {RouteConfig{Name: "r", From: "/", ToFolder: "."}, ""},
		"file":             {RouteConfig{Name: "r", From: "/", ToFile: "index.html"}, ""},
		"no target":        {RouteConfig{Name: "r", From: "/"}, "needs one of"},
		"to and folder":    {RouteConfig{Name: "r", From: "/", To: RouteTargets{"http://x"}, ToFolder: "."}, "more than one"},
		"folder and file":  {RouteConfig{Name: "r", From: "/", ToFolder: ".", ToFile: "a"}, "more than one"},
		"negative caching": {RouteConfig{Name: "r", From: "/", ToFolder: ".", Options: &RouteOptions{CacheControlSeconds: &neg}}, "cacheControlSeconds"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &GatewayConfig{Server: ServerConfig{Port: 8080}, Routes: []RouteConfig{tc.route}}
			err := cfg.Validate()
			if tc.want == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestValidate_AdminNeedsCredentials(t *testing.T) {
	cfg := &GatewayConfig{Server: ServerConfig{Port: 8080}}
	cfg.Management.Admin.Enabled = true
	assert.ErrorContains(t, cfg.Validate(), "username and/or password")
	cfg.Management.Admin.Username, cfg.Management.Admin.Password = "admin", "secret"
	assert.NoError(t, cfg.Validate())
}
