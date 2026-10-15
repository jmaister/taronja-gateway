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
		"proxy":                      {RouteConfig{Name: "r", From: "/", To: RouteTargets{"http://x"}}, ""},
		"folder":                     {RouteConfig{Name: "r", From: "/", ToFolder: "."}, ""},
		"file":                       {RouteConfig{Name: "r", From: "/", ToFile: "index.html"}, ""},
		"no target":                  {RouteConfig{Name: "r", From: "/"}, "needs one of"},
		"to and folder":              {RouteConfig{Name: "r", From: "/", To: RouteTargets{"http://x"}, ToFolder: "."}, "more than one"},
		"folder and file":            {RouteConfig{Name: "r", From: "/", ToFolder: ".", ToFile: "a"}, "more than one"},
		"static with removeFromPath": {RouteConfig{Name: "r", From: "/", ToFolder: ".", RemoveFromPath: "/dist"}, "removeFromPath only applies"},
		"negative caching":           {RouteConfig{Name: "r", From: "/", ToFolder: ".", Options: &RouteOptions{CacheControlSeconds: &neg}}, "cacheControlSeconds"},
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

func TestValidate_AccountEmailFeaturesNeedEmailAndURL(t *testing.T) {
	email := NotificationConfig{Email: EmailNotificationConfig{Enabled: true, Host: "smtp", Port: 25, From: "a@b.c"}}
	server := ServerConfig{URL: "https://example.com"}

	cases := []struct {
		name    string
		cfg     GatewayConfig
		wantErr string
	}{
		{"signup without basic", GatewayConfig{Server: server, Notification: email, AuthenticationProviders: AuthenticationProviders{Basic: BasicAuthenticationConfig{Signup: true}}}, "requires authenticationProviders.basic.enabled"},
		{"signup without email", GatewayConfig{Server: server, AuthenticationProviders: AuthenticationProviders{Basic: BasicAuthenticationConfig{Enabled: true, Signup: true}}}, "requires notification.email"},
		{"email login without email", GatewayConfig{Server: server, AuthenticationProviders: AuthenticationProviders{EmailLogin: EmailLoginConfig{Enabled: true}}}, "requires notification.email"},
		{"email login without url", GatewayConfig{Notification: email, AuthenticationProviders: AuthenticationProviders{EmailLogin: EmailLoginConfig{Enabled: true}}}, "require server.url"},
		{"all set", GatewayConfig{Server: server, Notification: email, AuthenticationProviders: AuthenticationProviders{Basic: BasicAuthenticationConfig{Enabled: true, Signup: true}, EmailLogin: EmailLoginConfig{Enabled: true}}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestPasswordResetEnabled(t *testing.T) {
	cfg := GatewayConfig{
		Server:                  ServerConfig{URL: "https://example.com"},
		AuthenticationProviders: AuthenticationProviders{Basic: BasicAuthenticationConfig{Enabled: true}},
	}
	assert.False(t, cfg.PasswordResetEnabled(), "no SMTP server: reset must be off")
	cfg.Notification.Email = EmailNotificationConfig{Enabled: true, Host: "smtp", Port: 25, From: "a@b.c"}
	assert.True(t, cfg.PasswordResetEnabled(), "basic login + SMTP + url: reset must be on")
}

func TestValidate_OIDCProviders(t *testing.T) {
	valid := OIDCProviderConfig{Name: "zitadel", Issuer: "https://auth.example.com", ClientId: "id"}
	with := func(edit func(*OIDCProviderConfig)) GatewayConfig {
		p := valid
		edit(&p)
		return GatewayConfig{AuthenticationProviders: AuthenticationProviders{OIDC: []OIDCProviderConfig{p}}}
	}

	ok := with(func(*OIDCProviderConfig) {})
	require.NoError(t, ok.Validate())
	assert.True(t, ok.HasAnyAuthentication())
	assert.Equal(t, "zitadel", valid.DisplayLabel())

	cases := map[string]struct {
		cfg     GatewayConfig
		wantErr string
	}{
		"bad name":      {with(func(p *OIDCProviderConfig) { p.Name = "Zitadel IdP" }), "lowercase letters"},
		"reserved name": {with(func(p *OIDCProviderConfig) { p.Name = "github" }), "reserved"},
		"no issuer":     {with(func(p *OIDCProviderConfig) { p.Issuer = "" }), "issuer must be"},
		"bad issuer":    {with(func(p *OIDCProviderConfig) { p.Issuer = "auth.example.com" }), "issuer must be"},
		"no client id":  {with(func(p *OIDCProviderConfig) { p.ClientId = "" }), "clientId is required"},
		"duplicate":     {GatewayConfig{AuthenticationProviders: AuthenticationProviders{OIDC: []OIDCProviderConfig{valid, valid}}}, "used twice"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, tc.cfg.Validate(), tc.wantErr)
		})
	}
}
