# Writing a Middleware

Global middleware is configured in the root `middleware:` list (see [doc/middleware/](middleware/README.md)). This guide
covers adding one, either built into the gateway or as a separate module.

A complete, tested third-party example lives in
[`examples/middleware-plugin/`](../examples/middleware-plugin/).

## The moving parts

All in the `middleware` package:

- **`Middleware`** (`chain.go`): `func(http.Handler) http.Handler`.
- **`MiddlewareFactory`** (`factory.go`): what a middleware implements: its name,
  description, dependencies, how to decode its options, and how to build it.
- **`Registry`** (`registry.go`): holds factories and builds a chain from an
  ordered list of `MiddlewareSpec`, checking that each dependency appears
  earlier in the list.
- **`HealthChecker`** (`health.go`): optional, for middleware with runtime state.

## 1. One file per middleware

A built-in middleware is one file in `middleware/builtin/` holding everything
about it. `cors.go` is a good model:

```go
// NameCORS answers CORS preflights and adds CORS headers.
const NameCORS = "cors"

// CORSConfig holds the cors middleware options.
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowedOrigins"`
	// ...
}

// Validate rejects combinations browsers refuse.
func (c CORSConfig) Validate() error { ... }

type corsFactory struct{ middleware.ConcreteFactory }

func newCORSFactory() *corsFactory {
	return &corsFactory{ConcreteFactory: middleware.NewConcreteFactory(NameCORS, "Adds CORS response headers")}
}

func (f *corsFactory) DecodeConfig(raw *yaml.Node) (interface{}, error) {
	return middleware.DecodeConfigInto(raw, CORSConfig{})
}

func (f *corsFactory) Create(cfg interface{}) (middleware.Middleware, error) {
	c, err := middleware.ConfigAs[CORSConfig](NameCORS, cfg)
	if err != nil {
		return nil, err
	}
	return CORSMiddleware(c), nil
}
```

Then add `newCORSFactory()` to the list in `NewGlobalRegistry`
(`middleware/builtin/global.go`) and write its page in `doc/middleware/`.
Nothing else needs to change: the config package does not know middleware names
or options, and `tg validate`, `tg middleware list` and the admin API pick the
middleware up from the registry.

### The factory methods

- **`GetName()`**: the `middleware[].name` value, lowercase `snake_case`
  (`rate_limiter`, not `RateLimiter`).
- **`GetDescription()`**: one line, shown by `tg middleware list` and the admin
  API.
- **`GetDependencies()`**: middleware that must appear earlier in the list. Only
  declare a dependency your middleware really reads (for example
  `traffic_metrics` reads the session `session_extraction` attaches); the
  registry rejects a chain that does not satisfy it.
- **`DecodeConfig(raw)`**: turns the entry's `config:` block (nil when
  absent) into the value `Create` receives. `middleware.DecodeConfigInto`
  decodes over your defaults, rejects unknown keys and calls the type's
  `Validate()` method if it has one. A middleware without options embeds
  `middleware.NoConfig`, which rejects any `config:` block.
- **`Create(cfg)`**: builds the middleware once, at startup. Take runtime
  services (repositories, stores) in the factory constructor and return an
  error when one is missing; validation builds factories without them and
  never calls `Create`.

### Options other code needs

When code outside the chain depends on a middleware's options (the shared rate
limiter, tracing setup, the statistics API), read them with
`builtin.MiddlewareSettings[C](cfg, name)`, which decodes the same entry the
chain uses. Do not add a second config field for the same setting.

## 2. Health (optional)

```go
type HealthChecker interface {
	HealthCheck() MiddlewareHealth
}
```

Implement it only when the middleware has real runtime state (the rate
limiter reports tracked and blocked IPs). Without it the status reads
`"unknown"`, which is more honest than a hardcoded `"healthy"`.

## 3. A middleware in its own module

A module outside the gateway implements the same interface; see
`examples/middleware-plugin/`. To run it, the embedding program registers its
factory next to the built-ins and builds the chain:

```go
registry := builtin.NewGlobalRegistry(deps)
registry.RegisterFactory(requestid.NewFactory())
specs, err := builtin.ResolveGlobalMiddleware(registry, cfg.Middleware)
chain, err := registry.BuildChain(specs)
handler := chain.Build(mux)
```

## Conventions

- One factory type per middleware; do not share a `ConcreteFactory` value.
- `Create` may run more than once (validation, tests): keep it idempotent.
- To observe status or size, wrap the `http.ResponseWriter` the way
  `trafficmetric.go`, `logging.go` and `middleware/metrics.go` do.
- Test through the real `Registry` and `httptest`, as
  `middleware/builtin/global_test.go` does, and cover option decoding in a
  `ResolveGlobalMiddleware` test.
