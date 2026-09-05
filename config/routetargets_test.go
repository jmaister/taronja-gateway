package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeRouteTestConfig writes a minimal, otherwise-valid config file with
// the given routes section spliced in verbatim, and returns its path.
// Distinct from middleware_test.go's writeTestConfig, which appends after a
// fixed base route rather than letting the caller fully control routes:.
func writeRouteTestConfig(t *testing.T, routesYAML string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := "name: Test Gateway\nserver:\n  host: 127.0.0.1\n  port: 8080\nmanagement:\n  admin:\n    enabled: false\n" + routesYAML
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))
	return path
}

func TestRouteConfig_To_AcceptsScalarString(t *testing.T) {
	path := writeRouteTestConfig(t, `routes:
  - name: single
    from: /api/*
    to: http://backend:8080
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.Equal(t, RouteTargets{"http://backend:8080"}, cfg.Routes[0].To)
}

func TestRouteConfig_To_AcceptsListOfStrings(t *testing.T) {
	path := writeRouteTestConfig(t, `routes:
  - name: balanced
    from: /api/*
    to:
      - http://backend1:8080
      - http://backend2:8080
      - http://backend3:8080
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.Equal(t, RouteTargets{"http://backend1:8080", "http://backend2:8080", "http://backend3:8080"}, cfg.Routes[0].To)
}

func TestRouteConfig_To_SingleElementList(t *testing.T) {
	// A one-element list is a valid, if unusual, way to write the same
	// thing as the bare-scalar form — both must produce an identical
	// RouteTargets value.
	path := writeRouteTestConfig(t, `routes:
  - name: single-list
    from: /api/*
    to:
      - http://backend:8080
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.Equal(t, RouteTargets{"http://backend:8080"}, cfg.Routes[0].To)
}

func TestRouteConfig_To_AbsentIsNil(t *testing.T) {
	path := writeRouteTestConfig(t, `routes:
  - name: static-route
    from: /static/*
    static: true
    toFolder: .
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.Nil(t, cfg.Routes[0].To)
}

func TestRouteConfig_To_EmptyStringIsNil(t *testing.T) {
	path := writeRouteTestConfig(t, `routes:
  - name: explicit-empty
    from: /static/*
    static: true
    toFolder: .
    to: ""
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.Nil(t, cfg.Routes[0].To)
}

func TestRouteConfig_To_EmptyListIsEmptyNotNil(t *testing.T) {
	// An explicit `to: []` is a deliberately empty list (distinct from
	// never having written `to:` at all), same distinction this project
	// draws for `middleware.global: []` vs an absent `middleware:` section
	// — both still fail proxy-route validation the same way (len == 0),
	// but it's worth confirming decoding itself doesn't collapse the two.
	path := writeRouteTestConfig(t, `routes:
  - name: explicit-empty-list
    from: /static/*
    static: true
    toFolder: .
    to: []
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.NotNil(t, cfg.Routes[0].To)
	assert.Empty(t, cfg.Routes[0].To)
}

func TestRouteConfig_To_InvalidShapeRejected(t *testing.T) {
	path := writeRouteTestConfig(t, `routes:
  - name: bad-shape
    from: /api/*
    to:
      host: backend:8080
`)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'to' must be a string or a list of strings")
}

func TestRouteConfig_To_AbsentForProxyRoute_LeavesNilAtLoadTime(t *testing.T) {
	// LoadConfig itself doesn't reject a proxy route with no `to:`
	// configured at all (that's middleware.ValidateRouteConfiguration's job
	// — see middleware/validation_test.go for that coverage); decoding one
	// must still leave To nil, not error out or default to something.
	path := writeRouteTestConfig(t, `routes:
  - name: no-target
    from: /api/*
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Routes, 1)
	assert.Nil(t, cfg.Routes[0].To)
}
