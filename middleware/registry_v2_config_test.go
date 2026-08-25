package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
)

func enabledPtr(b bool) *bool { return &b }

// --- ResolveGlobalChainSpecs: legacy flags (Phase 1 behavior preserved) ---

func TestResolveGlobalChainSpecs_LegacyFlagsWhenNoMiddlewareSection(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Analytics = true
	gatewayConfig.Management.Logging = true
	gatewayConfig.Management.RateLimiter = config.RateLimiterConfig{RequestsPerMinute: 100, MaxErrors: 5, BlockMinutes: 5}

	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantOrder := []string{
		config.MiddlewareNameRateLimiter,
		config.MiddlewareNameJA4Fingerprint,
		config.MiddlewareNameSessionExtraction,
		config.MiddlewareNameTrafficMetrics,
		config.MiddlewareNameLogging,
	}
	if len(specs) != len(wantOrder) {
		t.Fatalf("expected %d specs, got %d: %+v", len(wantOrder), len(specs), specs)
	}
	for i, want := range wantOrder {
		if specs[i].Name != want {
			t.Fatalf("spec[%d]: expected %s, got %s", i, want, specs[i].Name)
		}
	}
}

func TestResolveGlobalChainSpecs_EmptyByDefault(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected no specs for a default config, got %+v", specs)
	}
}

// --- ResolveGlobalChainSpecs: explicit `middleware:` section (Phase 2) ---

func TestResolveGlobalChainSpecs_ExplicitSectionOverridesLegacyFlags(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	// Legacy flags say "analytics off", but the explicit section should win entirely.
	gatewayConfig.Management.Analytics = false
	gatewayConfig.Management.Logging = false
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameJA4Fingerprint},
		{Name: config.MiddlewareNameSessionExtraction},
	}

	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 || specs[0].Name != config.MiddlewareNameJA4Fingerprint || specs[1].Name != config.MiddlewareNameSessionExtraction {
		t.Fatalf("expected explicit section to fully replace legacy derivation, got %+v", specs)
	}
}

func TestResolveGlobalChainSpecs_ExplicitSectionSkipsDisabledEntries(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameLogging, Enabled: enabledPtr(false)},
		{Name: config.MiddlewareNameJA4Fingerprint},
	}

	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != config.MiddlewareNameJA4Fingerprint {
		t.Fatalf("expected disabled logging entry to be skipped, got %+v", specs)
	}
}

func TestResolveGlobalChainSpecs_ExplicitRateLimiterOverride(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.RateLimiter = config.RateLimiterConfig{RequestsPerMinute: 100}
	override := config.RateLimiterConfig{RequestsPerMinute: 5, MaxErrors: 1, BlockMinutes: 1}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameRateLimiter, RateLimiter: &override},
	}

	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := specs[0].Config.(config.RateLimiterConfig)
	if !ok {
		t.Fatalf("expected config.RateLimiterConfig, got %T", specs[0].Config)
	}
	if got.RequestsPerMinute != 5 {
		t.Fatalf("expected per-entry override (5 rpm) to win over management.rateLimiter (100 rpm), got %+v", got)
	}
}

func TestResolveGlobalChainSpecs_RateLimiterFallsBackToManagementConfig(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.RateLimiter = config.RateLimiterConfig{RequestsPerMinute: 100}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameRateLimiter}, // no per-entry override
	}

	specs, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := specs[0].Config.(config.RateLimiterConfig)
	if !ok {
		t.Fatalf("expected config.RateLimiterConfig, got %T", specs[0].Config)
	}
	if got.RequestsPerMinute != 100 {
		t.Fatalf("expected fallback to management.rateLimiter (100 rpm), got %+v", got)
	}
}

func TestResolveGlobalChainSpecs_UnknownNameFromRawConfigErrors(t *testing.T) {
	// GatewayConfig built programmatically, bypassing config.LoadConfig's own
	// name validation — ResolveGlobalChainSpecs should still catch it.
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: "not_a_real_middleware"},
	}

	_, err := ResolveGlobalChainSpecs(gatewayConfig)
	if err == nil {
		t.Fatal("expected an error for an unknown middleware name")
	}
}

// --- ValidateGlobalChainSpecs / ValidateMiddlewareChainConfig ---

func TestValidateGlobalChainSpecs_CatchesMissingDependency(t *testing.T) {
	specs := []MiddlewareSpec{{Name: config.MiddlewareNameSessionExtraction}}
	if err := ValidateGlobalChainSpecs(specs); err == nil {
		t.Fatal("expected error: session_extraction requires ja4_fingerprint")
	}
}

func TestValidateGlobalChainSpecs_AcceptsSatisfiedDependencies(t *testing.T) {
	specs := []MiddlewareSpec{
		{Name: config.MiddlewareNameJA4Fingerprint},
		{Name: config.MiddlewareNameSessionExtraction},
		{Name: config.MiddlewareNameTrafficMetrics},
	}
	if err := ValidateGlobalChainSpecs(specs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateGlobalChainSpecs_DoesNotRequireRealDependencies(t *testing.T) {
	// referenceGlobalFactories wires every factory with nil dependencies; this
	// must still succeed since ValidateSpecs never calls factory.Create().
	specs := []MiddlewareSpec{
		{Name: config.MiddlewareNameRateLimiter, Config: config.RateLimiterConfig{}},
		{Name: config.MiddlewareNameJA4Fingerprint},
		{Name: config.MiddlewareNameSessionExtraction},
		{Name: config.MiddlewareNameTrafficMetrics},
		{Name: config.MiddlewareNameLogging},
	}
	if err := ValidateGlobalChainSpecs(specs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMiddlewareChainConfig_CatchesBadExplicitSection(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameTrafficMetrics}, // missing session_extraction dependency
	}

	err := ValidateMiddlewareChainConfig(gatewayConfig)
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
}

func TestValidateMiddlewareChainConfig_AcceptsLegacyFlagsConfig(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Analytics = true
	gatewayConfig.Management.Logging = true

	if err := ValidateMiddlewareChainConfig(gatewayConfig); err != nil {
		t.Fatalf("unexpected error validating legacy-derived config: %v", err)
	}
}

// --- End-to-end: explicit config section produces a working chain ---

func TestBuildGlobalChainV2_ExplicitMiddlewareSection(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameLogging},
	}

	chain, err := BuildGlobalChainV2(gatewayConfig, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("BuildGlobalChainV2 failed: %v", err)
	}

	called := false
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected wrapped handler to be called")
	}
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
}

func TestBuildGlobalChainV2_ExplicitSectionWithMissingDependencyErrors(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameSessionExtraction}, // missing ja4_fingerprint
	}

	_, err := BuildGlobalChainV2(gatewayConfig, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error building a chain with an unmet dependency")
	}
}

// --- BuildGlobalChain (legacy signature) delegates to BuildGlobalChainV2 ---

func TestBuildGlobalChain_DelegatesAndHonorsExplicitSection(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameLogging},
	}

	chain := BuildGlobalChain(gatewayConfig, nil, nil, nil, nil)
	if chain == nil {
		t.Fatal("expected a non-nil chain")
	}

	called := false
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected wrapped handler to be called")
	}
}

func TestBuildGlobalChain_FallsBackToEmptyChainOnError(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Middleware.Global = []config.MiddlewareEntryConfig{
		{Name: config.MiddlewareNameSessionExtraction}, // missing dependency -> BuildGlobalChainV2 errors
	}

	// BuildGlobalChain has no error return; it must not panic, and should
	// fall back to a usable (empty) chain instead.
	chain := BuildGlobalChain(gatewayConfig, nil, nil, nil, nil)
	if chain == nil {
		t.Fatal("expected a non-nil fallback chain")
	}

	called := false
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected the fallback empty chain to still invoke the wrapped handler")
	}
}
