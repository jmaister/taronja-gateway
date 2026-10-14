# Writing a Middleware

Global middleware is configured in the root `middleware:` list (see
[doc/middleware/](middleware/README.md)). This guide covers adding a built-in
one. Middleware is compiled into the gateway; there is no way to load one
from outside.

## 1. One file per middleware

A built-in middleware is one file in `middleware/builtin/` holding everything
about it: its name, its options type and a `Definition`. `cors.go` is a good
model:

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

var corsDefinition = Definition{
	Name:        NameCORS,
	Description: "Adds CORS response headers for cross-origin requests",
	Decode:      decodeOptions(CORSConfig{}),
	New: func(opts any, _ GlobalDeps) (middleware.Middleware, error) {
		return CORSMiddleware(opts.(CORSConfig)), nil
	},
}
```

Then add `&corsDefinition` to `definitions` (`middleware/builtin/definition.go`)
and write its page in `doc/middleware/`. Nothing else needs to change: the
config package does not know middleware names or options, and `tg validate`,
`tg middleware list` and the admin API pick the middleware up from that list.

### Definition fields

- **`Name`**: the `middleware[].name` value, lowercase `snake_case`
  (`rate_limiter`, not `RateLimiter`).
- **`Description`**: one line, shown by `tg middleware list` and the admin API.
- **`DependsOn`**: middleware that must appear earlier in the list. Only
  declare a dependency your middleware really reads (for example
  `traffic_metrics` reads the session `session_extraction` attaches).
- **`Decode`**: turns the entry's `config:` block (nil when absent) into the
  options `New` receives. `decodeOptions(defaults)` decodes over your defaults,
  rejects unknown keys and calls the type's `Validate()` method if it has one.
  A middleware without options uses `noOptions`, which rejects any `config:`
  block.
- **`New`**: builds the middleware once, at startup. Runtime services
  (repositories, stores) come in `GlobalDeps`; return an error when one you
  need is missing. Validation only runs `Decode`, never `New`. `stateless(mw)`
  covers middleware that need neither options nor services.
- **`Health`** (optional): only for middleware with real runtime state (the
  rate limiter reports tracked and blocked IPs). Without it the admin API
  shows no health, which is more honest than a hardcoded `"healthy"`.

### Options other code needs

When code outside the chain depends on a middleware's options (the shared rate
limiter, tracing setup, the statistics API), read them with
`builtin.GlobalOptions[C](chain, name)` from the resolved `*GlobalChain`. Do
not add a second config field for the same setting.

## Conventions

- To observe status or size, wrap the `http.ResponseWriter` the way
  `trafficmetric.go`, `logging.go` and `middleware/metrics.go` do.
- Test through `ResolveGlobalChain` and `httptest`, as
  `middleware/builtin/global_chain_test.go` does, and cover option decoding
  there too.
