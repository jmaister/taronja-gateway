# Refactoring 01: Middleware Architecture - Declarative & Pluggable Design

**Status**: Complete — all 4 phases implemented (see per-phase status below and [`doc/refactor01-release-notes.md`](./refactor01-release-notes.md) for a summary)
**Priority**: Medium  
**Effort**: 4 weeks (phased)  
**Owner**: TBD  
**Last Updated**: 2026-08-25

---

## Executive Summary

### Problem Statement
Middleware configuration in the gateway is **hardcoded in Go code** within the `BuildGlobalChain()` function. To understand what middleware is active, developers must:
1. Open `middleware/chain.go`
2. Read the `BuildGlobalChain()` function
3. Check configuration flags and understand conditionals
4. Manually piece together the execution flow

This makes it difficult to:
- Discover available middleware
- Add new middleware without code changes
- Configure middleware per-environment
- Monitor middleware health in production
- Test middleware combinations easily

### Solution Overview
Transform middleware configuration from **hardcoded** to **declarative** using a Factory + Registry pattern:

**Before:**
```go
// Hardcoded in BuildGlobalChain()
if gatewayConfig.Management.Analytics {
    chain.Add(JA4Middleware)
    chain.Add(SessionExtraction)
}
```

**After:**
```yaml
# Visible in config file
middleware:
  global:
    - name: ja4_fingerprint
      enabled: true
    - name: session_extraction
      enabled: true
```

---

## Current Architecture Analysis

### Request Flow
```
HTTP Request
    ↓
Global Middleware Chain (BuildGlobalChain - line 43-78 in middleware/chain.go)
    ├─ Rate Limiter (if enabled)
    ├─ JA4H Fingerprinting (if analytics enabled)
    ├─ Session Extraction (if analytics enabled)
    ├─ Traffic Metrics (if analytics enabled)
    └─ Logging (if logging enabled)
    ↓
Route Matching (http.ServeMux)
    ↓
Route-Specific Middleware Chain (BuildRouteChain - line 95-110 in middleware/chain.go)
    ├─ Authentication (if enabled for this route)
    └─ Cache Control (always applied)
    ↓
Handler Execution
    ↓
HTTP Response
```

### Configuration Entry Points
- `config.Management.Analytics` → Enables JA4H + SessionExtraction + TrafficMetrics
- `config.Management.Logging` → Enables Logging
- `config.Management.RateLimiter` → Enables RateLimiter with settings
- `routeConfig.Authentication.Enabled` → Enables Auth per route
- `routeConfig.Options.CacheControlSeconds` → Cache control value

### Current Strengths ✅
1. **ChainBuilder Pattern** - Clean, fluent API for composing middleware
2. **Centralized Configuration** - All decisions in one function
3. **Validation System** - Comprehensive validation in `validation.go`
4. **Consistent Signature** - All middleware use `func(http.Handler) http.Handler`
5. **Type Safety** - Full Go type system support
6. **Conditional Execution** - Easy `if config then add` pattern

### Current Weaknesses ❌
1. **Hardcoded in Go** - Not visible in config file
2. **No Registry** - Existing `MiddlewareRegistry` in `types.go` is unused
3. **Implicit Ordering** - Dependencies between middleware not documented
4. **No Per-Middleware Config** - Middleware get dependencies only, no options
5. **Limited Monitoring** - Can't introspect active middleware at runtime
6. **Brittle Extensibility** - Adding middleware requires code changes

---

## Detailed Improvements

### Improvement 1: Factory Pattern
**What**: Each middleware gets a factory that knows how to create instances

**Why**: Enables runtime creation, validation, and composition of middleware

**Example**:
```go
type MiddlewareFactory interface {
    Create(config interface{}) (Middleware, error)
    GetName() string
    GetDescription() string
    GetDependencies() []string
}

type JA4Factory struct { ... }
func (f *JA4Factory) Create(cfg interface{}) (Middleware, error) {
    return OptimizedJA4Middleware(true), nil
}
```

**Files**: Create `middleware/factory.go`

---

### Improvement 2: Middleware Registry
**What**: Central registry that knows about all available middleware

**Why**: Enables discovery, validation, building chains programmatically

**Example**:
```go
registry := NewMiddlewareRegistryV2()
registry.RegisterFactory(NewJA4Factory())
registry.RegisterFactory(NewLoggingFactory())

status := registry.GetStatus()
// Returns: {"ja4_fingerprint": {...}, "logging": {...}}
```

**Files**: Create `middleware/registry_v2.go`

---

### Improvement 3: Dependency Graph
**What**: Explicit dependencies between middleware with automatic ordering

**Why**: Makes implicit dependencies explicit, catches errors at startup

**Example**:
```yaml
middleware:
  global:
    - ja4_fingerprint        # No dependencies
    - session_extraction     # Depends on: ja4_fingerprint
    - traffic_metrics        # Depends on: session_extraction
```

**Validation**: If `session_extraction` is enabled without `ja4_fingerprint`, startup fails with clear error

**Files**: Create `middleware/dependency_graph.go` (future)

---

### Improvement 4: Per-Middleware Configuration
**What**: Each middleware can have its own strongly-typed config

**Why**: Allow environment-specific configuration without code changes

**Example**:
```yaml
middleware:
  global:
    - name: logging
      enabled: true
      logging:
        level: info              # "debug", "info", "warn", "error"
        format: json             # "json", "text"
        includeBody: false
        includeHeaders:
          - authorization
          - x-request-id

    - name: rate_limiter
      enabled: true
      rateLimiter:
        requestsPerMinute: 1000
        blockMinutes: 5
```

**Files**: Create `config/middleware.go`, Update `config/config.go`

---

### Improvement 5: Runtime Monitoring
**What**: Each middleware reports health and metrics

**Why**: Monitor middleware performance in production

**API Endpoints**:
```
GET /management/middleware
→ List all available and active middleware

GET /management/metrics/middleware/ja4_fingerprint
→ Get performance metrics for specific middleware
```

**Files**: Create `handlers/middleware_status.go`, Update `middleware/types.go`

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1) ✅ DONE
**Goal**: Create factory and registry system, fully backward compatible

**Tasks**:
- [x] Create `middleware/factory.go` with MiddlewareFactory interface
- [x] Implement factories for all existing middleware:
  - [x] RateLimiterFactory (reuses the gateway's existing `*RateLimiter` instance when supplied, so stats/config stay consistent with the management API)
  - [x] JA4Factory
  - [x] SessionExtractionFactory
  - [x] TrafficMetricsFactory
  - [x] LoggingFactory
- [x] Create `middleware/registry_v2.go` with MiddlewareRegistryV2
  - [x] RegisterFactory(factory MiddlewareFactory) error
  - [x] BuildChain(specs []MiddlewareSpec) (*ChainBuilder, error) (also creates middleware from specs internally, so a separate BuildFromSpec wasn't needed)
  - [x] GetStatus() map[string]MiddlewareStatus
- [x] Add BuildGlobalChainV2() to `middleware/chain.go`
- [x] Create `middleware/registry_v2_test.go` with comprehensive tests
  - [x] Test factory creation
  - [x] Test dependency validation
  - [x] Test chain building
  - [x] Test status reporting
- [x] Update `gateway/gateway.go` createHTTPServer() to call BuildGlobalChainV2()
- [x] Verify all existing tests still pass
- [x] Document new patterns in AGENTS.md (CLAUDE.md just points to AGENTS.md for project context)

**Success Criteria**:
- ✅ All existing middleware work via factories
- ✅ Registry builds complete chains from specs
- ✅ Dependencies validated at startup
- ✅ All tests passing (old and new)
- ✅ Zero behavior change (identical to before)
- ✅ Fully backward compatible

**Estimated Effort**: 2-3 days development + 1 day testing/review

---

### Phase 2: Config Integration (Week 2) ✅ DONE
**Goal**: Parse middleware from config file instead of hardcoded

**Tasks**:
- [x] Create typed config structs for each middleware (`config/middleware.go`: `MiddlewareEntryConfig`, `MiddlewareSection`, shared name constants — only `rate_limiter` gets real per-entry options today, since it's the only middleware with tunable runtime config; see Key Design Decisions below)
- [x] Update config loader to parse `middleware:` YAML section (`config.LoadConfig` in `config.go`, with fail-fast validation of unknown/missing/duplicate names)
- [x] Migrate logic from BuildGlobalChain() to use registry (`ResolveGlobalChainSpecs` in `registry_v2.go`: explicit `middleware.global` section when present, else the legacy flags translated exactly as before)
- [x] Make BuildGlobalChain() delegate to BuildGlobalChainV2() (`chain.go`; since `BuildGlobalChain` has no error return, a build failure is logged and falls back to an empty chain rather than panicking)
- [x] Update validation system to use dependency graph (`middleware.ValidateMiddlewareChainConfig`, wired into `ValidateAllMiddleware`, validates names + `GetDependencies()` via `MiddlewareRegistryV2.ValidateSpecs` before real dependencies exist)
- [x] Add integration tests (`config/middleware_test.go` for YAML parsing/validation, `middleware/registry_v2_config_test.go` for spec resolution, dependency validation, and end-to-end chain building)

**Success Criteria**:
- ✅ Middleware config read from YAML file
- ✅ Same behavior as Phase 1 (legacy flags still produce identical specs when no `middleware:` section is present — covered by `TestResolveGlobalChainSpecs_LegacyFlagsWhenNoMiddlewareSection`)
- ✅ Developers can now modify middleware without code changes

**Estimated Effort**: 2-3 days development + 1 day testing

**Note on scope**: the doc's original example showed per-middleware `logging:` options (level, format, includeBody, includeHeaders). Those aren't implemented — `LoggingMiddleware` itself has no such knobs today, and adding them would be a behavior change, not just a wiring change. Only `rate_limiter` (which already had a typed, wired-up `config.RateLimiterConfig`) gets per-entry YAML config in Phase 2. Extending other middlewares similarly is future work once they actually support the options.

**Bug found and fixed after initial implementation**: the per-entry `rateLimiter:` override was resolved correctly by `ResolveGlobalChainSpecs` and covered by a unit test at that layer, but never actually took effect when running the real gateway — `createHTTPServer` built the shared `*RateLimiter` instance from `management.rateLimiter` *before* the registry existed, and `RateLimiterFactory.Create` always prefers that shared instance over the `cfg` it's given whenever one is supplied. `middleware.EffectiveRateLimiterConfig` now resolves the same override *before* that instance is constructed, and `gateway/middleware_wiring_test.go` covers the previously-broken end-to-end path (spec resolution alone wasn't enough to catch this — the gap was in what the resolved config was actually used *for*). Also fixed in the same pass: `MiddlewareRegistryV2.BuildChain` was accumulating `built`/`metrics` state across repeated calls on the same registry instead of resetting it, so `GetStatus`/`GetMetrics` could report stale "active" middleware from an earlier call — not reachable from any current caller (registries are only ever built once), but corrected to match the documented "most recent build" contract; and an explicit `middleware.global: []` was indistinguishable from no `middleware:` section at all and silently fell back to the legacy flags — `ResolveGlobalChainSpecs` now checks `Global != nil` rather than `len(Global) > 0`, since YAML preserves that distinction.

---

### Phase 3: Monitoring & Observability (Week 3) ✅ DONE
**Goal**: Add runtime inspection and health checks

**Tasks**:
- [x] Add health check interface to middleware (`middleware/health.go`: `HealthChecker`/`MiddlewareHealth`, optionally implemented by a `MiddlewareFactory`)
- [x] Implement health checks in existing middleware (`RateLimiterFactory.HealthCheck()` reports tracked/blocked IP counts from `RateLimiter.Stats()`; the other four factories don't implement it — see Note below — so `GetHealth`/`GetStatus` report `"unknown"` for them rather than a fabricated `"healthy"`)
- [x] Create middleware status API endpoint (`GET <prefix>/api/middleware`, admin-only — `handlers/api_middleware.go` + `api/taronja-gateway-api.yaml`)
- [x] Create middleware metrics API endpoint (`GET <prefix>/api/middleware/{name}/metrics`, admin-only, 404 for an unknown name)
- [ ] Add middleware status dashboard — skipped; "(if applicable)" in the original plan, and no webapp page consumes these endpoints yet. The API is usable directly; a dashboard page is future work if/when needed.
- [x] Document health check patterns (`AGENTS.md`, this file)

**Tasks not in the original list, added during implementation**:
- [x] Per-middleware request metrics (`middleware/metrics.go`): `BuildChain` wraps every middleware with `instrumentMiddleware`, recording request count, error count (status ≥ 500), and elapsed time in-memory. This was necessary to have real data for the metrics endpoint — without it, "performance data" would have to be fabricated.
- [x] `NewGlobalMiddlewareRegistry` split out of `BuildGlobalChainV2` (`middleware/chain.go`) so `gateway.go` can keep the built `*MiddlewareRegistryV2` (on `Gateway.MiddlewareRegistry`) instead of it being discarded once the chain is built — needed for the status/metrics endpoints to have anything to query at request time.

**Success Criteria**:
- ✅ `GET /management/middleware` returns status of all middleware — implemented as `GET <prefix>/api/middleware` (e.g. `/_/api/middleware`), matching this codebase's existing endpoint convention (`/api/statistics/rate-limiter`, `/api/config/rate-limiter`, etc.) rather than a literal `/management/` path.
- ✅ `GET /management/metrics/middleware/:name` returns performance data — implemented as `GET <prefix>/api/middleware/{name}/metrics`, same convention.
- ✅ Can monitor middleware health in production

**Estimated Effort**: 2-3 days development + 1 day testing

**Note on scope**: only `rate_limiter` implements `HealthChecker`, since it's the only built-in middleware with real runtime state to report (tracked/blocked IPs). `ja4_fingerprint`, `session_extraction`, `traffic_metrics`, and `logging` are stateless per request — there's nothing genuine to check — so they report health `"unknown"` rather than a hardcoded `"healthy"` that would just be decoration. A middleware's `averageDurationMs` metric is cumulative from entering that middleware to the response being written (middlewares nest, so this includes every downstream middleware and the final handler), not an isolated per-middleware cost; this is documented on the type and in the OpenAPI schema.

---

### Phase 4: Documentation & Tooling (Week 4) ✅ DONE
**Goal**: Make it easy for developers to add custom middleware

**Tasks**:
- [x] Create middleware development guide (`doc/middleware_development.md`)
- [x] Create middleware plugin template — this codebase has no dynamic-loading Go `plugin` mechanism (middleware is always compiled in and registered via `RegisterFactory`), so "plugin template" means a real, compiling, tested example of that registration pattern rather than a `.so`: [`examples/middleware-plugin/`](../examples/middleware-plugin/) (an `X-Request-Id` tracing middleware, `request_id`)
- [x] Create CLI tool for middleware introspection (`tg middleware list --config <path>`, in `main.go`)
- [x] Update CLAUDE.md with new patterns — CLAUDE.md is intentionally a one-line pointer to `AGENTS.md` (per this repo's convention, established before this refactor); all patterns are documented there instead, kept current every phase
- [x] Update repository README with architecture overview (`README.md`: new "Middleware Architecture" section + `middleware:` config subsection + `tg middleware list` in Commands)
- [x] Create release notes (`doc/refactor01-release-notes.md` — GoReleaser generates its own changelog from commit messages at actual release time; this is a human-readable summary of the whole refactor for a PR description or manual release notes)

**Success Criteria**:
- ✅ New developers can add custom middleware
- ✅ Clear documentation of patterns
- ✅ Example of 3rd-party middleware — compiled and tested, not just prose: `go test ./examples/middleware-plugin/...`

**Estimated Effort**: 1-2 days

---

## Phase 1: Concrete Implementation Details

### File 1: `middleware/factory.go` (New)

```go
package middleware

import (
	"fmt"
	"net/http"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// MiddlewareFactory creates middleware instances from configuration
type MiddlewareFactory interface {
	Create(config interface{}) (Middleware, error)
	GetName() string
	GetDescription() string
	GetDependencies() []string
	GetDefaultConfig() interface{}
}

// Base implementation
type ConcreteFactory struct {
	name         string
	description  string
	dependencies []string
}

func (f *ConcreteFactory) GetName() string { return f.name }
func (f *ConcreteFactory) GetDescription() string { return f.description }
func (f *ConcreteFactory) GetDependencies() []string { return f.dependencies }

// RateLimiterFactory
type RateLimiterFactory struct{ ConcreteFactory }

func NewRateLimiterFactory() *RateLimiterFactory {
	return &RateLimiterFactory{
		ConcreteFactory: ConcreteFactory{
			name:        "rate_limiter",
			description: "Limits request rate and detects vulnerability scans",
		},
	}
}

func (f *RateLimiterFactory) Create(cfg interface{}) (Middleware, error) {
	rateLimiterCfg := cfg.(config.RateLimiterConfig)
	return RateLimiterMiddleware(rateLimiterCfg), nil
}

func (f *RateLimiterFactory) GetDefaultConfig() interface{} {
	return config.RateLimiterConfig{
		RequestsPerMinute: 1000,
		MaxErrors:         10,
		BlockMinutes:      5,
	}
}

// ... Similar implementations for JA4, SessionExtraction, TrafficMetrics, Logging

// Global registry
var MiddlewareFactoryMap = map[string]MiddlewareFactory{}

func RegisterFactory(factory MiddlewareFactory) {
	MiddlewareFactoryMap[factory.GetName()] = factory
}

func GetFactory(name string) (MiddlewareFactory, bool) {
	factory, exists := MiddlewareFactoryMap[name]
	return factory, exists
}

func ListFactories() []string {
	names := make([]string, 0, len(MiddlewareFactoryMap))
	for name := range MiddlewareFactoryMap {
		names = append(names, name)
	}
	return names
}
```

**Lines of Code**: ~250  
**Complexity**: Low  
**Dependencies**: config, auth, db, session packages  

---

### File 2: `middleware/registry_v2.go` (New)

```go
package middleware

import (
	"fmt"
	"log"

	"github.com/jmaister/taronja-gateway/config"
)

// MiddlewareSpec describes a middleware to build
type MiddlewareSpec struct {
	Name   string      `json:"name" yaml:"name"`
	Config interface{} `json:"config" yaml:"config"`
}

// MiddlewareRegistryV2 builds middleware chains from specifications
type MiddlewareRegistryV2 struct {
	factories map[string]MiddlewareFactory
	built     map[string]Middleware
}

func NewMiddlewareRegistryV2() *MiddlewareRegistryV2 {
	return &MiddlewareRegistryV2{
		factories: make(map[string]MiddlewareFactory),
		built:     make(map[string]Middleware),
	}
}

// RegisterFactory registers a middleware factory
func (r *MiddlewareRegistryV2) RegisterFactory(factory MiddlewareFactory) error {
	name := factory.GetName()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("middleware factory '%s' already registered", name)
	}
	r.factories[name] = factory
	log.Printf("Registered middleware factory: %s", name)
	return nil
}

// BuildChain builds a chain of middleware from specifications
// Validates that dependencies are available and returns them in correct order
func (r *MiddlewareRegistryV2) BuildChain(specs []MiddlewareSpec) (*ChainBuilder, error) {
	chain := NewChainBuilder()
	built := make(map[string]bool)

	for _, spec := range specs {
		factory, exists := r.factories[spec.Name]
		if !exists {
			return nil, fmt.Errorf("unknown middleware: %s", spec.Name)
		}

		// Validate dependencies
		for _, dep := range factory.GetDependencies() {
			if !built[dep] {
				return nil, fmt.Errorf(
					"middleware '%s' depends on '%s' which is not enabled",
					spec.Name, dep,
				)
			}
		}

		// Create middleware
		middleware, err := factory.Create(spec.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to create middleware '%s': %w", spec.Name, err)
		}

		chain.Add(middleware)
		built[spec.Name] = true
		log.Printf("Added middleware to chain: %s", spec.Name)
	}

	return chain, nil
}

// GetStatus returns the status of all registered middleware
func (r *MiddlewareRegistryV2) GetStatus() map[string]MiddlewareStatus {
	status := make(map[string]MiddlewareStatus)

	for name, factory := range r.factories {
		s := MiddlewareStatus{
			Name:        name,
			Description: factory.GetDescription(),
			Enabled:     r.built[name] != nil,
			Dependencies: factory.GetDependencies(),
		}

		if r.built[name] != nil {
			s.Status = "active"
		} else {
			s.Status = "available"
		}

		status[name] = s
	}

	return status
}

// MiddlewareStatus describes the status of a middleware
type MiddlewareStatus struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Status       string   `json:"status"` // "active", "available"
	Enabled      bool     `json:"enabled"`
	Dependencies []string `json:"dependencies"`
}

// BuildGlobalChainFromConfig builds global chain from gateway config
// This is a migration helper - eventually config will declare middleware directly
func BuildGlobalChainFromConfigV2(
	registry *MiddlewareRegistryV2,
	gatewayConfig *config.GatewayConfig,
) (*ChainBuilder, error) {
	specs := []MiddlewareSpec{}

	// Rate limiter
	if gatewayConfig.Management.RateLimiter.IsEnabled() {
		specs = append(specs, MiddlewareSpec{
			Name: "rate_limiter",
			Config: gatewayConfig.Management.RateLimiter,
		})
	}

	// Analytics group
	if gatewayConfig.Management.Analytics {
		specs = append(specs, MiddlewareSpec{Name: "ja4_fingerprint", Config: struct{}{}})
		specs = append(specs, MiddlewareSpec{Name: "session_extraction", Config: struct{}{}})
		specs = append(specs, MiddlewareSpec{Name: "traffic_metrics", Config: struct{}{}})
	}

	// Logging
	if gatewayConfig.Management.Logging {
		specs = append(specs, MiddlewareSpec{Name: "logging", Config: struct{}{}})
	}

	return registry.BuildChain(specs)
}
```

**Lines of Code**: ~150  
**Complexity**: Low  
**Dependencies**: config package

---

### File 3: Update `middleware/chain.go`

Add new function at end of file:

```go
// BuildGlobalChainV2 uses the new registry system
// Once complete migration happens, this becomes the default BuildGlobalChain
func BuildGlobalChainV2(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	rateLimiter *RateLimiter,
) (*ChainBuilder, error) {
	// Initialize registry
	registry := NewMiddlewareRegistryV2()

	// Register all factories
	registry.RegisterFactory(NewRateLimiterFactory())
	registry.RegisterFactory(NewJA4Factory())
	registry.RegisterFactory(NewSessionExtractionFactory(sessionStore, tokenService))
	registry.RegisterFactory(NewTrafficMetricsFactory(trafficMetricRepo))
	registry.RegisterFactory(NewLoggingFactory())

	// Build chain using config
	return BuildGlobalChainFromConfigV2(registry, gatewayConfig)
}
```

**Lines of Code**: ~20  
**Complexity**: Very Low

---

### File 4: Update `gateway/gateway.go` createHTTPServer()

Replace existing `BuildGlobalChain` call:

```go
// OLD:
globalChain := middleware.BuildGlobalChain(config, deps.SessionStore, deps.TokenService, deps.TrafficMetricRepo, rl)

// NEW:
globalChain, err := middleware.BuildGlobalChainV2(
	config,
	deps.SessionStore,
	deps.TokenService,
	deps.TrafficMetricRepo,
	rl,
)
if err != nil {
	return nil, nil, nil, fmt.Errorf("failed to build middleware chain: %w", err)
}
```

**Changes**: ~5 lines  
**Complexity**: Very Low

---

### File 5: Create `middleware/registry_v2_test.go` (New)

```go
package middleware

import (
	"testing"

	"github.com/jmaister/taronja-gateway/config"
)

func TestRegistryBuildsMiddleware(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewRateLimiterFactory())
	registry.RegisterFactory(NewLoggingFactory())

	specs := []MiddlewareSpec{
		{Name: "rate_limiter", Config: config.RateLimiterConfig{RequestsPerMinute: 100}},
		{Name: "logging", Config: struct{}{}},
	}

	chain, err := registry.BuildChain(specs)
	if err != nil {
		t.Fatalf("Failed to build chain: %v", err)
	}

	if chain == nil {
		t.Fatal("Chain should not be nil")
	}
}

func TestRegistryValidatesDependencies(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewSessionExtractionFactory(nil, nil))
	
	// Try to build session extraction without ja4_fingerprint
	specs := []MiddlewareSpec{
		{Name: "session_extraction", Config: struct{}{}},
	}

	_, err := registry.BuildChain(specs)
	if err == nil {
		t.Fatal("Should fail when dependency is missing")
	}
}

func TestRegistryReportsStatus(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewRateLimiterFactory())
	registry.RegisterFactory(NewLoggingFactory())

	status := registry.GetStatus()
	
	if len(status) != 2 {
		t.Fatalf("Expected 2 middleware, got %d", len(status))
	}

	if _, ok := status["rate_limiter"]; !ok {
		t.Fatal("rate_limiter should be in status")
	}
}

// ... More tests for error cases, ordering, etc.
```

**Lines of Code**: ~100  
**Complexity**: Low  
**Test Coverage**: Factory creation, chain building, dependency validation, error handling

---

## Testing Strategy

### Unit Tests (registry_v2_test.go)
```
✓ Factory creation and registration
✓ BuildChain with valid specs
✓ BuildChain with missing dependencies (error)
✓ BuildChain with unknown middleware (error)
✓ GetStatus returns all middleware
✓ GetStatus differentiates active vs available
```

### Integration Tests
```
✓ BuildGlobalChainV2 with real dependencies
✓ Chain executes middleware in correct order
✓ Middleware receive request successfully
✓ Response flows back through middleware
✓ Conditional middleware inclusion works
```

### Backward Compatibility Tests
```
✓ Old BuildGlobalChain still works
✓ Gateway initializes with both systems
✓ Request flow unchanged
✓ All existing middleware tests pass
```

---

## Implementation Checklist

### Phase 1 Tasks
- [x] Create `middleware/factory.go`
  - [x] MiddlewareFactory interface
  - [x] RateLimiterFactory
  - [x] JA4Factory
  - [x] SessionExtractionFactory
  - [x] TrafficMetricsFactory
  - [x] LoggingFactory
  - [x] Registry functions (RegisterFactory, GetFactory, ListFactories)

- [x] Create `middleware/registry_v2.go`
  - [x] MiddlewareRegistryV2 struct and methods
  - [x] MiddlewareSpec struct
  - [x] BuildChain() with dependency validation
  - [x] GetStatus() for introspection
  - [x] BuildGlobalChainFromConfigV2() helper

- [x] Update `middleware/chain.go`
  - [x] Add BuildGlobalChainV2() function

- [x] Update `gateway/gateway.go`
  - [x] Change createHTTPServer() to call BuildGlobalChainV2()
  - [x] Add error handling for chain building

- [x] Create `middleware/registry_v2_test.go`
  - [x] Unit tests for all scenarios
  - [x] Tests for error conditions
  - [x] Tests for dependency validation

- [x] Verify all existing tests pass
  - [x] Run full test suite
  - [x] Check for any regressions

- [x] Update `AGENTS.md` (CLAUDE.md defers to it for project context)
  - [x] Document new factory pattern
  - [x] Document registry usage
  - [x] Update architecture section

### Acceptance Criteria
- [x] All unit tests passing
- [x] All integration tests passing (existing gateway/middleware suites)
- [x] All existing tests still passing
- [x] Zero behavior change from before
- [ ] Code review approved
- [x] Documentation updated

---

## Key Design Decisions

### 1. Why Factory Pattern?
- **Allows runtime composition** of middleware
- **Enables testing** individual middleware in isolation
- **Supports 3rd-party middleware** without modifying core
- **Clear responsibility** - factory knows how to create its middleware

### 2. Why Registry?
- **Centralized discovery** of available middleware
- **Dependency validation** at startup
- **Status reporting** for operations
- **Foundation for dynamic configuration** in future phases

### 3. Why Keep BuildGlobalChain()?
- **Backward compatibility** - existing code still works
- **Allows phased migration** - can run both systems simultaneously
- **Reduces risk** - old fallback if something breaks
- **Gradual deprecation** - can warn developers before removal

### 4. Why MiddlewareSpec Instead of Map?
- **Ordered list** - preserves middleware order
- **Type safe** - Name and Config fields are explicit
- **Extensible** - can add more fields later (priority, tags, etc.)
- **Serializable** - can be loaded from YAML/JSON

---

## Success Metrics

After Phase 1, you should be able to:

✅ **Discover** - See all available middleware without reading code  
✅ **Extend** - Add new middleware with just a factory registration  
✅ **Test** - Build middleware chains programmatically for testing  
✅ **Monitor** - Call registry.GetStatus() to see what's active  
✅ **Maintain** - Understand dependencies explicitly  
✅ **Scale** - Support multiple middleware combinations easily  

---

## Files Changed/Created

### Created (New Files)
```
middleware/factory.go              (~250 lines)
middleware/registry_v2.go          (~150 lines)
middleware/registry_v2_test.go     (~100 lines)
```

### Modified (Existing Files)
```
middleware/chain.go                (+20 lines)
gateway/gateway.go                 (5 line change)
CLAUDE.md                          (update architecture section)
```

### Total Impact
- **New Code**: ~500 lines
- **Modified Code**: ~25 lines
- **Breaking Changes**: None (fully backward compatible)
- **Test Coverage**: Comprehensive for new code

---

## Risk Assessment

### Low Risk Areas
- ✅ Factory pattern is well-known
- ✅ Registry is additive, doesn't replace anything
- ✅ Can run both old and new systems side-by-side
- ✅ No changes to middleware behavior
- ✅ No changes to request processing

### Mitigation Strategies
- ✅ Keep old BuildGlobalChain() working
- ✅ Comprehensive tests for new code
- ✅ Gradual migration path across phases
- ✅ Clear rollback plan (remove BuildGlobalChainV2, keep old code)

---

## Future Phases Overview

### Phase 2: Config-Driven
- YAML middleware section in config file
- Typed config structs per middleware
- No code changes needed to enable/disable middleware

### Phase 3: Monitoring
- Middleware health checks
- Performance metrics API
- Status dashboard

### Phase 4: Extensibility
- Middleware development guide
- Plugin system
- 3rd-party middleware support

---

## References

### Related Files
- `middleware/chain.go` - Current ChainBuilder implementation
- `middleware/types.go` - Existing middleware interfaces
- `middleware/validation.go` - Current validation system
- `gateway/gateway.go` - Gateway initialization
- `config/config.go` - Configuration structure

### Documentation
- See `doc/middlewares.md` for current middleware analysis
- See separate implementation documents for detailed code

---

## Questions & Answers

**Q: Will this impact performance?**  
A: No. Registry building happens once at startup (~1-2ms). Middleware execution per-request is identical to before.

**Q: What if middleware depends on startup side effects?**  
A: That's fine. Factory.Create() is called once, so side effects happen during initialization same as before.

**Q: Can we add this without breaking changes?**  
A: Yes. BuildGlobalChainV2() exists alongside BuildGlobalChain(). Can migrate gradually.

**Q: What about middleware configuration?**  
A: Phase 1 supports basic config passing. Phase 2 adds full config file integration.

**Q: How do we test this?**  
A: MiddlewareRegistryV2_test.go has comprehensive tests. Can also build test chains from specs.

---

## Next Steps

All four planned phases are implemented (see the per-phase sections above).
Possible future work, not currently planned or scheduled:

1. **Per-middleware YAML config beyond `rate_limiter`** — e.g. the `logging:`
   level/format/includeBody options sketched in Improvement 4 above, once
   `LoggingMiddleware` itself actually supports them.
2. **A middleware status/metrics page in the admin dashboard** — the API
   (`GET <prefix>/api/middleware`, `GET <prefix>/api/middleware/{name}/metrics`)
   exists; no webapp UI consumes it yet (Phase 3 marked this "if applicable").
3. **A real dependency graph structure** (`middleware/dependency_graph.go`,
   Improvement 3) if dependencies ever become more complex than the simple
   "all direct deps satisfied by an earlier spec" check `MiddlewareRegistryV2`
   does today.

