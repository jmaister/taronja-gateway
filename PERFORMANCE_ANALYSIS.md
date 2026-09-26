# Taronja Gateway Performance Analysis Report

## Executive Summary

Based on extensive performance testing and code analysis, I've identified several performance bottlenecks in the Taronja Gateway application. The primary issues are in the middleware chain, particularly when analytics are enabled, which significantly impacts request processing time and memory usage.

## Key Performance Findings

### Benchmark Results

1. **No Middleware**: 703.9 ns/op, 210 B/op, 5 allocs/op
2. **Static Request (Analytics Enabled)**: 10,319 ns/op, 1,720 B/op, 21 allocs/op  
3. **Static Request (Analytics Disabled)**: 7,484 ns/op, 1,720 B/op, 21 allocs/op
4. **JA4H with Caching**: 1,651 ns/op, 272 B/op, 8 allocs/op (99.99% hit rate)
5. **JA4H without Caching**: 4,249 ns/op, 704 B/op, 24 allocs/op
6. **Memory Usage**: 7,043 bytes average per request with analytics enabled

### Performance Impact Analysis

The analytics middleware introduces significant overhead:
- **27% performance degradation**: Analytics disabled vs enabled (7,484 ns vs 10,319 ns)
- **8x memory overhead**: 1,720 bytes vs 210 bytes per request
- **4x more allocations**: 21 vs 5 allocations per request

**JA4H Caching provides dramatic improvements**:
- **61% performance improvement**: 1,651 ns vs 4,249 ns per request
- **61% memory reduction**: 272 bytes vs 704 bytes per request  
- **67% allocation reduction**: 8 vs 24 allocations per request
- **Near-perfect hit rate**: 99.99% after initial cache warming

## Performance Bottlenecks Identified

### 1. JA4H Fingerprinting (Primary Bottleneck) ✅ SOLVED

**Location**: `middleware/ja4.go`

**Issues**:
- External library `go-ja4h` processes every HTTP request
- Complex fingerprint calculation on each request  
- No caching of fingerprints for similar requests

**Solution Implemented**:
- JA4H caching with 99.99% hit rate
- 61% performance improvement
- 67% reduction in memory allocations

### 2. Middleware Chain Overhead

**Location**: `middleware/chain.go`, `BuildGlobalChain()`

**Issues**:
- Multiple middleware layers executed for every request
- Each middleware wraps the next handler, creating call stack overhead
- Conditional middleware execution still requires chain traversal

**Current Flow**:
```
Request → RateLimiterMiddleware → JA4Middleware → SessionExtractionMiddleware → TrafficMetricMiddleware → LoggingMiddleware → Handler
```

### 3. Session Validation Overhead

**Location**: `middleware/session_extraction.go`, `middleware/session_utils.go`

**Issues**:
- Database/memory store lookup on every request (even non-authenticated routes)
- Duplicate session validation logic
- Cookie parsing and validation overhead

**Code Hot Spots**:
```go
// Called on every request, even when session not needed
result := ValidateSessionFromRequest(r, store, tokenService)
```

### 4. Traffic Metrics Collection

**Location**: `middleware/trafficmetric.go`

**Issues**:
- Response body buffering for error tracking
- Database write operations (even though async)
- Memory allocation for statistics objects
- JSON marshaling of session data

**Memory Hot Spots**:
```go
// Creates new buffer for every request
body := &bytes.Buffer{}
// Async but still creates goroutine overhead
go func() {
    statsRepo.Create(stat)
}()
```

### 5. Database Operations

**Location**: `db/sessionrepository_db.go`

**Issues**:
- GORM query overhead for session validation
- No connection pooling optimization visible
- Potential N+1 query issues for user data

### 6. Gateway Initialization Overhead

**Location**: `gateway/gateway.go`, `NewGateway()`

**Issues**:
- Gateway instance created for each benchmark run
- Template parsing and middleware validation on each test
- Database initialization overhead

## Memory Usage Analysis

### Current Memory Profile
- **Per Request**: 7,043 bytes average
- **1000 Requests**: 6,878 KB total allocation
- **Garbage Collection**: 3 GC cycles for 1000 requests

### Memory Hotspots
1. **Session objects**: JSON marshaling and unmarshaling
2. **Traffic metrics**: Struct creation and buffering
3. **Response wrappers**: Custom response writers for each request
4. **String operations**: URL path manipulation and header setting

## Execution Path Analysis

### Critical Path for Typical Request

1. **Global Middleware Chain** (10-15μs overhead)
   - JA4H calculation: ~60% of middleware time ✅ **OPTIMIZED**
   - Session extraction: ~25% of middleware time
   - Traffic metrics: ~10% of middleware time
   - Logging: ~5% of middleware time

2. **Route Matching** (minimal overhead)

3. **Route-Specific Middleware** (2-5μs overhead)
   - Cache control
   - Authentication (if required)

4. **Handler Execution** (variable)

### Request Types Impact
- **Static files**: High middleware overhead relative to simple file serving
- **API endpoints**: Middleware overhead more reasonable relative to business logic
- **Authenticated requests**: Additional session validation overhead

## Performance Optimization Recommendations

### 1. ✅ JA4H Fingerprint Caching (High Impact) - IMPLEMENTED

**Implementation**:
```go
type JA4HCache struct {
    cache   map[string]string
    mutex   sync.RWMutex
    maxSize int
}

func (c *JA4HCache) GetOrCalculate(r *http.Request) string {
    key := generateRequestKey(r) // Hash of relevant headers
    
    c.mutex.RLock()
    if fingerprint, exists := c.cache[key]; exists {
        c.mutex.RUnlock()
        return fingerprint
    }
    c.mutex.RUnlock()
    
    // Calculate and cache
    fingerprint := ja4h.JA4H(r)
    c.mutex.Lock()
    c.cache[key] = fingerprint
    c.mutex.Unlock()
    
    return fingerprint
}
```

**Results**: 61% performance improvement, 99.99% hit rate

### 2. ✅ Static Asset Detection (High Impact) - IMPLEMENTED

**Implementation**:
```go
func isStaticAsset(path string) bool {
    staticExtensions := []string{
        ".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg",
        ".woff", ".woff2", ".ttf", ".eot", ".webp", ".mp4", ".pdf",
        ".zip", ".tar", ".gz", ".json", ".xml", ".txt",
    }
    
    for _, ext := range staticExtensions {
        if strings.HasSuffix(strings.ToLower(path), ext) {
            return true
        }
    }
    
    // Check for static paths
    staticPaths := []string{"/static/", "/_/static/", "/assets/", "/public/"}
    for _, staticPath := range staticPaths {
        if strings.Contains(strings.ToLower(path), staticPath) {
            return true
        }
    }
    
    return false
}
```

### 3. Conditional Middleware Execution (High Impact) - READY TO IMPLEMENT

**Implementation**:
```go
// Skip analytics middleware for static assets
if isStaticAsset(req.URL.Path) {
    // Use minimal middleware chain
    chain := NewChainBuilder()
    chain.Add(LoggingMiddleware) // Only logging for static assets
    return chain.Build(handler)
}
```

### 4. Session Validation Optimization (Medium Impact)

**Implementation**:
```go
// Only validate sessions for routes that need them
func ConditionalSessionMiddleware(needsAuth bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if needsAuth {
                // Only do expensive session validation when needed
                result := ValidateSessionFromRequest(r, store, tokenService)
                if result.IsAuthenticated {
                    r = AddSessionToContext(r, result.Session)
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### 5. Traffic Metrics Batching (Medium Impact) - ✅ IMPLEMENTED

**Implemented as**: `db.BatchingTrafficMetricRepository` — see "August 2026,
part 3" below for the real implementation and measured numbers; the sketch
below is the original proposal, kept for history (it correctly anticipated
the `CreateBatch` method name, though not the SQLite bound-parameter limit
that made naive unchunked batching unsafe).

**Original sketch**:
```go
type MetricsBatch struct {
    metrics []TrafficMetric
    mutex   sync.Mutex
    size    int
}

func (m *MetricsBatch) Add(metric TrafficMetric) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    m.metrics = append(m.metrics, metric)
    if len(m.metrics) >= m.size {
        go m.flush()
    }
}

func (m *MetricsBatch) flush() {
    // Batch insert to database
    statsRepo.CreateBatch(m.metrics)
    m.metrics = m.metrics[:0]
}
```

### 6. Memory Pool for Response Writers (Medium Impact)

**Implementation**:
```go
var responseWriterPool = sync.Pool{
    New: func() interface{} {
        return &responseWriterWithStats{
            body: &bytes.Buffer{},
        }
    },
}

func getResponseWriter(w http.ResponseWriter) *responseWriterWithStats {
    rw := responseWriterPool.Get().(*responseWriterWithStats)
    rw.ResponseWriter = w
    rw.statusCode = http.StatusOK
    rw.responseSize = 0
    rw.body.Reset()
    return rw
}
```

## Configuration-Based Performance Tuning

### 1. Analytics Levels

```yaml
management:
  analytics:
    level: "basic" # none, basic, full
    excludeStatic: true
    fingerprinting: 
      enabled: true
      cacheSize: 1000 # Number of fingerprints to cache
    sessionTracking: true
    metrics: true
```

### 2. Performance Profiles

```yaml
performance:
  profile: "production" # development, production, high-performance
  caching:
    sessionCache: true
    fingerprintCache: true
    staticAssetCache: true
  optimization:
    skipAnalyticsForStatic: true
    batchMetrics: true
    poolResponseWriters: true
```

## Actual Performance Test Results

### JA4H Fingerprinting Optimization
- **Without Caching**: 4,249 ns/op, 704 B/op, 24 allocs/op
- **With Caching**: 1,651 ns/op, 272 B/op, 8 allocs/op
- **Improvement**: 61% faster, 61% less memory, 67% fewer allocations
- **Cache Hit Rate**: 99.99% after warmup

### Static Asset Detection  
- Successfully detects static assets by extension and path
- Ready for implementation in conditional middleware chains

## Expected Performance Improvements

### With All Recommended Optimizations:

1. **JA4H Caching**: ✅ 61% reduction confirmed
2. **Conditional Analytics**: 80% reduction for static assets (projected)
3. **Session Optimization**: 20-30% reduction in session-related overhead (projected)
4. **Memory Pooling**: 30-50% reduction in allocations (projected)

### Projected Final Results:
- **Static requests**: 2,000-3,000 ns/op (70-80% improvement from 10,319 ns)
- **API requests**: 5,000-7,000 ns/op (40-50% improvement from current)  
- **Memory per request**: 2,000-3,000 bytes (60-70% improvement)
- **Allocations per request**: 8-12 allocs (60% improvement)

## Implementation Priority

### ✅ Phase 1 (High Impact, Low Risk) - COMPLETED
1. ✅ JA4H fingerprint caching - **61% performance improvement achieved**
2. ✅ Static asset detection logic - **Ready for integration**
3. Performance configuration structure

### Phase 2 (High Impact, Medium Risk) - ✅ DONE (August 2026, part 2 — `management.excludeStaticAssets`)
1. ✅ Conditional middleware for static assets — implemented as an opt-in config flag rather than always-on; see "August 2026, part 2" below
2. Configuration-based analytics levels
3. Route-specific middleware optimization

### Phase 3 (Medium Impact, Medium Risk)
1. Session validation optimization
2. ✅ Traffic metrics batching — DONE, see "August 2026, part 3" below
3. Memory pooling for response writers

### Phase 4 (Lower Impact, Higher Risk)
1. Database query optimization
2. Advanced caching strategies
3. Connection pooling enhancements

## Monitoring and Metrics

### Key Performance Indicators
1. **Request latency**: p95, p99 response times
2. **Memory usage**: Allocation rate, GC frequency
3. **Throughput**: Requests per second
4. **Middleware overhead**: Time spent in each middleware
5. **Cache performance**: Hit rates, cache size

### Recommended Monitoring
```go
// Add performance metrics collection
type PerformanceMetrics struct {
    MiddlewareTime    time.Duration
    DatabaseTime      time.Duration
    HandlerTime       time.Duration
    TotalRequestTime  time.Duration
    MemoryAllocated   int64
    CacheHitRate      float64
}
```

## Conclusion

The performance analysis has identified significant bottlenecks and implemented proven optimizations:

**Immediate Impact Achieved**:
- JA4H caching provides 61% performance improvement with 99.99% hit rate
- Static asset detection ready for conditional middleware implementation

**Next Steps**:
1. Integrate conditional middleware chains for static vs dynamic requests
2. Implement configuration-based performance profiles
3. Add metrics batching and memory pooling
4. Monitor and measure improvements in production

The combination of these optimizations should result in 60-80% performance improvements for static assets and 30-50% improvements for API requests, making the Taronja Gateway significantly more efficient at handling high loads.

---

## August 2026 Update: the benchmarks above were measuring the wrong handler

Every benchmark and load test in this repo (`performance_test.go` at the
module root, and `gateway/performance_test.go`,
`gateway/performance_noanalytics_test.go`,
`gateway/performance_optimized_test.go`) called `gw.Mux.ServeHTTP(...)`
directly. `gw.Mux` is only the route table — routing plus whatever
middleware an individual route attaches directly (auth, cache-control). The
global middleware chain (JA4H fingerprinting, session extraction, traffic
metrics, request logging — everything `BuildGlobalChainV2` assembles, see
`gateway/reload.go`'s `buildRuntime`) wraps *around* `gw.Mux` into a separate
handler (`gw.handler`) that only the real HTTP server actually uses.
Calling `gw.Mux.ServeHTTP` in a benchmark silently skips that entire chain.

Every number in this document above this section was measured that way, so
none of them reflect what a real request pays. That gap is also exactly what
let a severe production bug go undetected: **`session.NewClientInfo` called
`uaparser.NewFromSaved()` — which parses the entire embedded user-agent regex
database (several hundred patterns) from scratch — on every single request**
that passed through traffic-metrics or session-extraction middleware, i.e.
every analytics-tracked request in a normal deployment. Benchmarking the real
handler path (`gw.handler`, not `gw.Mux`) surfaced it immediately:

| Static-asset request, full middleware chain | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Before fix | 195,100,423 (~195 ms) | 44,802,953 (~44.8 MB) | 293,989 |
| After fix | 201,009 (~0.2 ms) | 16,487 (~16 KB) | 186 |
| **Improvement** | **~970×** | **~2,720×** | **~1,580×** |

The fix (`session/clientinfo.go`): build the `*uaparser.Parser` once, in a
package-level `sync.Once`, and reuse it for every request instead of
reconstructing it per call. The type is documented as safe for concurrent
use — it holds its own mutex-guarded LRU cache internally — which is exactly
the intended usage pattern (`uaparser.NewFromSaved()` is itself marked
deprecated in favor of `New()`, precisely because callers kept making this
mistake). No other code changed; every existing test still passes.

Fixing the benchmarks to call `gw.handler` instead of `gw.Mux` surfaced one
more real bug as a side effect: the test-only in-memory SQLite database
(`db.SetupTestDB`) hands out up to 10 pooled connections against a
`file::memory:` DSN *without* `cache=shared`. Without that flag, every pooled
connection is an independent, empty in-memory database — only the one
connection `AutoMigrate` happened to run on has any tables. Under enough
concurrent load (e.g. the async traffic-metrics insert racing a session
lookup, both real behavior of the actual request path), a query could land on
a different, unmigrated connection and fail with `no such table:
traffic_metrics`. Capping the test DB to a single pooled connection
(`SetMaxOpenConns(1)`) fixes it; production is unaffected since it points at
a real file-backed SQLite database, not `:memory:`.

### Corrected, honest numbers (full chain, analytics enabled, post-fix)

Measured via `make bench` on this branch, Go 1.26, `-12` == `GOMAXPROCS=12`:

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkWithoutMiddleware` (raw handler, no gateway at all) | 1,709 | 250 | 5 |
| `BenchmarkAPIRequest` (full chain, `/_/health`) | 158,211 | 9,725 | 110 |
| `BenchmarkStaticRequest` (full chain, `/_/login`) | 263,901 | 29,496 | 153 |
| `BenchmarkAuthenticatedRequest` (full chain + route auth) | 8,546,069 | 405,620 | 3,944 |
| `BenchmarkJA4HCaching` | 7,808 | 432 | 10 |
| `BenchmarkJA4HNoCaching` | 14,781 | 704 | 24 |

Two things stand out as the next real targets, found by profiling the
post-fix `BenchmarkStaticRequest` (`go tool pprof` on a fresh CPU profile,
not projection):

1. **The async traffic-metrics DB write is now the single largest cost in
   the chain** (~21% of CPU time in the static-request profile is
   `TrafficMetricRepositoryDB.Create` → `gorm.(*DB).Create`), even though it
   runs in its own goroutine and doesn't block the response. One
   `INSERT`-sized transaction per request is exactly the "Traffic Metrics
   Batching" idea from Improvement 5 above — still unimplemented, and now
   backed by a real profile instead of a guess.
2. **`BenchmarkAuthenticatedRequest` costs ~30× a plain full-chain request**
   (8.5 ms vs 158 µs) with ~36× the allocations (3,944 vs 110). This wasn't
   investigated further this round; it's the top candidate for the next
   profiling pass.

### Dead code found along the way (not fixed, flagging for a decision)

- `middleware/performance.go`'s `BuildOptimizedGlobalChain` /
  `buildConditionalChain` / `OptimizedTrafficMetricMiddleware` and
  `gateway/performance_optimized_test.go`'s `NewGatewayWithPerformanceConfig`
  are unreachable from production code. `isStaticAsset`-based skipping of the
  full analytics chain for static assets (Recommendation 3, "Conditional
  Middleware Execution", above) was fully written years ago but never wired
  into `BuildGlobalChainV2`/the registry system that actually replaced it —
  it's the most-recommended, least-implemented idea in this whole document.
  Worth resurrecting now inside the registry/factory architecture (Phase
  1-5's `MiddlewareRegistryV2`) rather than the old standalone
  `ChainBuilder` functions, which are themselves now dead ends.
  `NewGatewayWithPerformanceConfig` is a placeholder that returns a plain
  gateway regardless of the `PerformanceConfig` passed to it — the
  `BenchmarkOptimized*` benchmarks built on it were never measuring an
  optimized path.
- The module-root `performance_test.go` (removed in this pass) was a stale,
  never-fixed duplicate of `gateway/performance_test.go`: it hit
  `/_/api/health` and `/_/api/me`, routes that don't exist, and had been
  failing under `-bench` for as long as those routes moved. `make bench`
  never ran it (it only targets `./gateway`), which is presumably why nobody
  noticed.

---

## August 2026, part 2: `management.excludeStaticAssets`

Recommendation 3 from the very top of this document ("Conditional Middleware
Execution") had actually been implemented once already — `middleware/performance.go`
had a working `isStaticAsset` + conditional-chain-skip — but as the section
above found, it was never wired into `BuildGlobalChainV2`/the registry
system that replaced the old hardcoded `BuildGlobalChain`. It's now wired in
for real, as a config flag rather than an always-on behavior, since skipping
analytics for static assets is an observability trade-off the operator
should opt into, not a decision this codebase should make for them.

**What changed**:

- `management.excludeStaticAssets: true` (default `false`, so existing
  configs keep their current behavior unchanged) skips
  `TrafficMetricMiddleware`'s work — response-writer wrapping, `TrafficMetric`
  construction, session/JA4H lookups it triggers, and the async DB write —
  for any request whose path looks like a static asset. It has no effect
  unless `analytics` is also enabled. Also available as a per-entry override
  (`middleware.global[].trafficMetrics.excludeStaticAssets`) for configs
  using the explicit `middleware:` section, following the same pattern as
  `rate_limiter` and `cors`'s per-entry overrides.
- The static-asset heuristic itself moved to `session.IsStaticAssetPath`
  (extension list + conventional path prefixes — CSS/JS/images/fonts under
  `/static/`, `/_/static/`, `/assets/`, `/public/`). It's now the one
  canonical implementation; the near-identical but subtly different copy in
  `middleware/performance.go` and the dead, per-request-regex-compiling
  `shouldExcludeFromStats` in `middleware/trafficmetric.go` (health check /
  favicon / robots.txt patterns — never wired to anything either) are both
  gone.
- Every `TrafficMetric` row now records `IsStaticAsset`, regardless of
  whether `excludeStaticAssets` is on — so a row that *was* recorded stays
  filterable by request type even if the flag is flipped on later, or was
  never on at all.
- The request-details report (`GET .../api/statistics/requests/details`,
  the "Request Details" admin dashboard page) gained an `is_static` filter
  parameter — `true` for static-asset requests only, `false` for non-static
  only, omitted for both — surfaced in the UI as a "Request type" dropdown
  next to the existing date-range picker, plus a Static/Dynamic badge column
  in the table so unfiltered results still show which is which.

**Measured improvement** (`BenchmarkStaticRequestAnalyticsIncludingStatic` vs
`...ExcludingStatic`, `gateway/performance_optimized_test.go`, real full
middleware chain, static-asset request, 3 runs):

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `excludeStaticAssets: false` (default, unchanged) | ~16,400 | ~8,900 | ~104 |
| `excludeStaticAssets: true` | ~6,800 | ~3,450 | ~56 |
| **Improvement** | **~2.4×** | **~2.6×** | **~1.9×** |

(Absolute ns/op varies run-to-run on this machine by as much as 10× — compare
the two rows of a single run, not against numbers from a different session's
run, e.g. part 1's numbers above.)

---

## August 2026, part 3: batching traffic-metrics writes

Part 1's post-fix profile of `BenchmarkStaticRequest` named the async
traffic-metrics DB write as the next-largest cost in the chain (~21% of CPU
for one `INSERT`-per-request transaction, even though it doesn't block the
response). This is Improvement 5 ("Traffic Metrics Batching") from the very
top of this document, implemented for real.

**What changed**:

- New `db.BatchingTrafficMetricRepository`, a decorator over the real
  `TrafficMetricRepository`: `Create` now only appends to an in-memory
  buffer and returns — no database access on that call at all — and a
  background goroutine flushes the buffer via a new `CreateBatch` method
  once it reaches 100 records or 500ms elapses, whichever comes first.
  `Close()` does one final flush and stops the goroutine; wired into
  `gateway/deps.Dependencies.Close()`, called from `main.go` right after
  the HTTP server stops accepting requests on both shutdown paths (signal
  and `ListenAndServe` returning on its own).
- `TrafficMetricRepositoryDB.CreateBatch` uses GORM's `CreateInBatches`
  (chunked at 100 rows/transaction), not a single unchunked `Create` on the
  whole slice. This isn't just an optimization detail: `TrafficMetric`
  embeds `ClientInfo` (~30 columns), and SQLite rejects a statement with
  too many bound parameters — a single `Create` on an unchunked slice
  crashed outright with "too many SQL variables" once the buffer backed up
  past roughly 1,000 records in testing, silently losing the entire
  batch. `CreateInBatches` was necessary before this could ship safely, not
  a nice-to-have.
- Only `gateway/deps.NewProduction()` wraps `TrafficMetricRepo` this way;
  `NewTest()`/`NewTestWithName()` don't, deliberately — every existing test
  that asserts on a `TrafficMetric` row shortly after triggering it (a
  handful of milliseconds' `time.Sleep`, sprinkled across
  `middleware/trafficmetric_test.go` and others) keeps working unmodified,
  since test dependencies still write synchronously-ish, one `Create` per
  call, exactly as before. This is a production-only implementation
  change, not a config flag like `excludeStaticAssets` — there's no
  meaningful trade-off for an operator to tune, just "up to 500ms of the
  most recent traffic-metrics rows can be lost on a hard crash instead of a
  graceful shutdown," which is the same trade every access-log shipper and
  APM agent makes for the same kind of data.
- Found and fixed a real, pre-existing data race while verifying this
  under `go test -race`: `JA4HCache`'s `hits`/`misses` counters
  (`middleware/performance.go`) were plain `int64` incremented with `c.hits++`
  from every concurrent request — undefined behavior under the Go memory
  model, confirmed by `-race` failing `TestConcurrentRequests`, and present
  since the JA4H caching work in part 1's predecessor, unrelated to
  anything changed this session until `-race` was actually run against it.
  Fixed with `atomic.Int64`. `go test -race ./...` is clean now; it wasn't
  before this fix and apparently never had been run in CI.

**Measured improvement** (`BenchmarkTrafficMetricCreate_Unbatched` vs
`_Batched`, `db/trafficmetricrepository_batching_bench_test.go`, fixed
`-benchtime=5000x` — see that file's doc comment for why adaptive
benchtime is the wrong tool here — 3 runs):

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Unbatched (one `Create` per record) | ~98,600 | ~16,500 | 153 |
| Batched (`BatchingTrafficMetricRepository`) | ~62,700 | ~5,355 | ~50 |
| **Improvement** | **~1.6×** | **~3.1×** | **~3.1×** |

Smaller than part 1 and part 2's numbers, and deliberately reported from a
saturated, sustained-load benchmark rather than a single request: at that
point the single background flush goroutine is itself the bottleneck, so
the ceiling is "how fast can one goroutine keep issuing 100-row
`CreateInBatches` chunks," not "how much does any individual request save."
Real traffic is far less saturated than a tight benchmark loop with no
per-iteration work, so production headroom is larger than this number
suggests — but this is the honest steady-state figure, not a best case.

---

## August 2026, part 4: filter algorithms — a rate-limiter fix, a dashboard cleanup, and a severe geolocation bug

Asked to specifically audit "the algorithms used when applying filters" —
middleware and other request-filtering logic — three findings, ranging from
a clean micro-optimization to the most severe bug of this whole document.

### 1. Vulnerability-scan pattern matching recompiled every pattern, every 404

`RateLimiter.Handler`'s scanner-detection check looped over every configured
`management.rateLimiter.vulnerabilityScan.urls` pattern on every 404
response, and for each one re-derived its normalized/expanded form from
scratch (`strings.ReplaceAll`, `strings.Contains`, another `ReplaceAll`) via
`matchesVulnerabilityScanPath` — work that depends only on the *pattern*,
never the request, redone from zero every single time regardless. The
request path itself was also renormalized once per pattern rather than once
per request. This is exactly the traffic this feature exists to handle
efficiently — a scanner generating many rapid 404s — so redoing
pattern-only, request-independent work on that specific hot path was
self-defeating.

Fixed by precomputing each pattern's normalized/expanded form once, in
`NewRateLimiter` (a new `scanPattern` type), and normalizing the request
path once per request instead of once per pattern.
`matchesVulnerabilityScanPath` (kept for its existing exhaustive test table)
is now a thin wrapper over the same precomputable logic, so there's one
source of truth and its ~35 test cases now exercise the real code path.

**Measured improvement** (`BenchmarkVulnerabilityScanMatch_PerCallRecompute`
vs `_Precomputed`, `middleware/ratelimiter_scanpattern_bench_test.go`, 20
realistic scanner-probe patterns, 3 runs):

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Per-call recompute (before) | ~2,962 | 240 | 15 |
| Precomputed (after) | ~1,645 | **0** | **0** |
| **Improvement** | **~1.8×** | **∞** | **∞** |

### 2. A third, inconsistent, incomplete "is this a static asset" check

`gateway.go`'s admin-dashboard SPA handler had its own hand-rolled static-
asset detector — 12 chained `strings.HasSuffix` calls — used to decide
whether to even attempt reading a file from the embedded webapp build, plus
a *second*, equally long `HasSuffix` chain to guess its Content-Type. This
was the third independent "is this a static asset" implementation in the
codebase this document has now found (after the one in the old, dead
`middleware/performance.go`, and the canonical `session.IsStaticAssetPath`
part 2 introduced) — inconsistent with both, and with two real bugs of its
own: any asset extension not on its 12-item list (a `.wasm` chunk, a
`.map` source map, anything future tooling emits) silently always fell back
to serving `index.html` instead of the real file, and any served file type
not on the *second* list got `Content-Type: text/html` — actively wrong for
a binary asset, not merely a missing header.

Fixed by replacing the first check with a single `strings.Contains(path,
".")` (the real check is the `embed.FS.ReadFile` attempt immediately after
it; this is only ever a cheap pre-filter for the common case of an
extensionless SPA route) and the second with the standard library's
`mime.TypeByExtension` — a complete, maintained, O(1) lookup — falling back
to `application/octet-stream` (never silently to `text/html`) for anything
it doesn't recognize.

### 3. `GetGeoDataFromIP` never cached failures — the most severe bug in this document

`session.NewClientInfo` (part of the same traffic-metrics/session-extraction
filter pipeline part 1's uaparser fix lives in) calls `GetGeoDataFromIP` for
every analytics-tracked request. Its cache only ever stored *successful*
lookups; a failed one — the free geolocation API unreachable, rate-limiting,
or just slow — was never cached at all, so every single request from an IP
the API couldn't be reached for paid the *full* client timeout (5s) again,
for as long as that IP kept sending requests, indefinitely. Confirmed
directly, not hypothetically: with the geolocation API unreachable from the
sandbox this was found in, `gateway/performance_test.go`'s `TestMemoryUsage`
— 1,000 sequential requests from one IP — went from a sub-second test to an
80+ minute hang.

This is a real production risk for any deployment where the free API is
unreachable (a corporate egress firewall, an air-gapped environment) or
simply rate-limits the gateway's IP under load — not a sandbox artifact.
Every analytics-tracked request would silently gain 5 seconds of latency,
forever, until the outage or rate-limiting ended.

Fixed with negative caching: a failed lookup is now cached too, just
briefly (`geoFailureTTL`, 1 minute, vs. `geoSuccessTTL`'s 7 days) — long
enough that repeated requests from the same client during an outage don't
each pay the timeout, short enough that service recovers within a minute of
the API coming back. `GeoData`'s now-redundant `Timestamp` field (TTL
tracking moved to the cache entry itself, which now needs to track it for
both outcomes) was removed.

Fixing this surfaced three more tests silently depending on real network
reachability to that same free API: `session.TestNewClientInfoWithJA4H` and
two more in `session_test.go` all construct requests via
`httptest.NewRequest`, whose default `RemoteAddr` (`192.0.2.1`) triggered a
real, un-mockable lookup. One of them (`TestNewAndValidateSession`) had a
5-second `assert.WithinDuration` tolerance that the API being unreachable
blew straight through — a latent flakiness that predates this session
(any sufficiently slow response, network-reachable or not, could have
triggered it) newly exposed by consistently-unreachable-in-this-sandbox
conditions rather than caused by them. Fixed by seeding one permanent,
successful cache entry for that well-known test IP in the `session`
package's `TestMain`, so nothing in the package's test suite depends on
real network access any more (verified: the whole package now runs in
~0.1s, down from ~15s of pure network-timeout waiting across three tests).
`main`'s and `providers`' own test suites each still pay one such 5s
network round trip on their first request through the full middleware
chain — not a hang or a failure, and not chased further here, since (unlike
the bug just fixed) it only costs once per test *binary*, not once per
*request*.
