# Compression

**Middleware name:** `compression`
**Global chain position:** first — outermost in the chain, so it wraps the
response writer that every other middleware and the final route handler
write through, letting it compress whatever actually reaches the socket.
**Depends on:** nothing.
**Enabled by default:** no.

## What it does

Transparently compresses response bodies with brotli, zstd, gzip, or
deflate — negotiated per request from the client's `Accept-Encoding` header,
per standard HTTP content negotiation (RFC 9110 §12.5.3) — for every route:
reverse-proxied backends, static files, the SPA fallback, and the
management API/dashboard alike.

It has **no configuration options at all**: no algorithm choice, no
compression level, no size threshold, no per-route opt-out. It is either
enabled or it isn't. The only decisions it makes are fixed correctness
rules, not tunables:

- Skips a response under 1024 bytes *when the handler declared
  `Content-Length` up front* — below that, a compressor's own framing
  overhead can eat the savings. A response with no declared length
  (streaming/chunked) is compressed regardless, since its eventual size
  isn't known in advance.
- Skips Content-Types that are already compressed or gain nothing from
  compression: `image/*`, `video/*`, `audio/*`, `font/*`, and a short list
  of already-compressed formats (`application/zip`, `application/gzip`,
  `application/pdf`, `application/wasm`, `application/octet-stream`, ...).
- Skips a response that already set its own `Content-Encoding` — never
  double-compresses.
- Skips byte-range requests (`Range` header) — compressing would change
  what "byte 0" refers to, breaking the range semantics.
- Skips `HEAD` requests (no body to compress) and `Connection: Upgrade`
  requests (WebSocket and similar — the bytes after a `101` response aren't
  HTTP payload, and wrapping the response writer would break the
  `http.Hijacker` access the upgrade needs).
- When a client's `Accept-Encoding` accepts more than one coding equally
  (no explicit preference between them), picks in this order: **brotli >
  zstd > gzip > deflate**. Brotli and zstd generally compress typical HTTP
  payloads (HTML/CSS/JS/JSON) more tightly than gzip/deflate for comparable
  CPU cost, so they're preferred once a client has actually advertised
  support for one — universality isn't a concern at that point, since the
  client already said it understands it. gzip keeps its long-standing edge
  over the older, weaker deflate for the same reason it always did: more
  battle-tested client/intermediary support.

When it does compress, it removes the original `Content-Length` (the
compressed size isn't known up front, so the response falls back to chunked
transfer encoding), sets `Content-Encoding` to whichever coding was
negotiated (`br`, `zstd`, `gzip`, or `deflate`), and adds
`Vary: Accept-Encoding` so caches/CDNs know the response depends on that
request header.

## Enabling it

Disabled by default — compressing every response has a real (if usually
small) CPU cost, so it's opt-in rather than silently changing behavior for
existing deployments.

```yaml
management:
  compression: true
```

Or, in an explicit [`middleware:` section](../../README.md#middleware-optional-advanced):

```yaml
middleware:
  global:
    - name: compression
```

See [doc/middleware/README.md#two-ways-to-enable-any-of-these](README.md#two-ways-to-enable-any-of-these)
for which form to use, and the important gotcha: an explicit
`middleware:` section replaces *every* legacy flag at once, not just this
one.

## Config options

None. Listing it (via either form above) just enables it.

## Notes

- Because it's the outermost middleware in the chain, other middlewares
  that record response size or status (`traffic_metrics`, `logging`) still
  see and report the **uncompressed** byte count the handler actually
  produced — compression only affects the bytes forwarded past them to the
  real socket, not what they observe.
- Server-Sent Events and other flush-driven streaming responses are still
  delivered incrementally: the middleware forwards `Flush()` through the
  active compressor (all four — `brotli.Writer`, `zstd.Encoder`,
  `gzip.Writer`, `flate.Writer` — support flushing without closing the
  stream) rather than buffering until the handler finishes.
- Brotli and zstd are provided by `github.com/andybalholm/brotli` and
  `github.com/klauspost/compress/zstd` respectively — both pure Go, no
  CGO, matching this project's `modernc.org/sqlite`-driven no-CGO build.
  zstd's encoder is created with concurrency forced to 1
  (`zstd.WithEncoderConcurrency(1)`): its default is `GOMAXPROCS`
  background goroutines *per Writer*, which is reasonable for compressing
  one large file but wasteful for the one-writer-per-HTTP-response pattern
  this middleware uses on a busy gateway.

## See also

- [Middleware Architecture](../../README.md#middleware-architecture) — the
  factory/registry system all of these middlewares are built on, and how to
  inspect the running chain.
- [doc/middleware/cors.md](cors.md) — runs immediately after this one in
  the default chain.
