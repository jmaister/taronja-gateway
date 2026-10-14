# compression

Compresses responses with brotli, zstd, gzip or deflate, picked from the
client's `Accept-Encoding` (preference on ties: `br > zstd > gzip > deflate`).

```yaml
middleware:
  - name: compression
```

**Options:** none. **Depends on:** nothing. Place it near the top: middleware
after it still sees the uncompressed status and size.

Skipped automatically for:

- bodies under 1 KB (when `Content-Length` is set)
- already compressed types (`image/*`, `video/*`, `audio/*`, `font/*`, zip, gzip, pdf, wasm, octet-stream)
- responses that already set `Content-Encoding`
- `Range`, `HEAD` and `Connection: Upgrade` (WebSocket) requests

Compressed responses get `Vary: Accept-Encoding` and no `Content-Length`.
Streaming responses are flushed through as they are written.
