package middleware

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// compressionMinBytes is the smallest declared response size worth
// compressing. Below it, gzip/deflate's own framing overhead can eat the
// savings, and it's not worth the CPU. This only applies when the handler
// declares Content-Length up front (e.g. a static file or a small JSON
// response) — a response with no declared length (streaming/chunked) is
// compressed regardless, since its eventual size isn't known in advance.
const compressionMinBytes = 1024

// incompressibleContentTypePrefixes are response Content-Type prefixes for
// formats that are already compressed (or otherwise gain nothing from
// gzip/deflate): re-compressing them wastes CPU for no size benefit. This is
// a fixed, internal list, not a configuration surface — see
// CompressionMiddleware's doc comment for why this middleware has no options.
var incompressibleContentTypePrefixes = []string{
	"image/",
	"video/",
	"audio/",
	"font/",
}

// incompressibleContentTypes are exact Content-Type matches (ignoring any
// ";charset=..." suffix) for the same reason as incompressibleContentTypePrefixes.
var incompressibleContentTypes = map[string]bool{
	"application/zip":              true,
	"application/gzip":             true,
	"application/x-gzip":           true,
	"application/x-7z-compressed":  true,
	"application/x-rar-compressed": true,
	"application/pdf":              true,
	"application/wasm":             true,
	"application/octet-stream":     true,
}

// CompressionMiddleware transparently compresses response bodies with gzip
// or deflate — the two algorithms every HTTP client has understood for
// decades — following standard content negotiation (RFC 9110 §12.5.3)
// against the request's Accept-Encoding header.
//
// It is deliberately built with no configuration at all: no algorithm
// choice, no compression level, no size threshold, no per-route opt-out.
// Every response is compressed whenever the client accepts gzip or deflate,
// except for cases where compressing would be actively wrong or pointless —
// a body under compressionMinBytes (when its size is known up front), a
// Content-Type that's already compressed (incompressibleContentTypePrefixes/
// incompressibleContentTypes), a response that already set its own
// Content-Encoding, a byte-range request (compressing would change what
// "byte 0" refers to), a HEAD request (no body to compress), or a
// Connection: Upgrade request (WebSocket and similar — the bytes after the
// 101 response aren't HTTP payload at all). Those are fixed correctness
// rules, not options: this middleware is either enabled or it isn't.
func CompressionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		if isUpgradeRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		encoding := negotiateEncoding(r.Header.Get("Accept-Encoding"))
		if encoding == "" {
			next.ServeHTTP(w, r)
			return
		}

		cw := &compressingResponseWriter{ResponseWriter: w, encoding: encoding}
		next.ServeHTTP(cw, r)
		cw.Close()
	})
}

func isUpgradeRequest(r *http.Request) bool {
	for _, v := range r.Header.Values("Connection") {
		for _, token := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "Upgrade") {
				return true
			}
		}
	}
	return false
}

// negotiateEncoding parses an Accept-Encoding header (RFC 9110 §12.5.3) and
// returns "gzip" or "deflate" if the client accepts one of them, or "" if it
// accepts neither (no header, only unsupported codings, or every coding this
// middleware supports is excluded with q=0). gzip is preferred when both are
// equally acceptable — it has the more universally battle-tested client and
// intermediary support of the two.
func negotiateEncoding(header string) string {
	if header == "" {
		return ""
	}

	const notListed = -1.0
	gzipQ, deflateQ, starQ := notListed, notListed, notListed

	for _, part := range strings.Split(header, ",") {
		name, q := parseCoding(part)
		switch name {
		case "gzip":
			gzipQ = q
		case "deflate":
			deflateQ = q
		case "*":
			starQ = q
		}
	}

	// A coding not explicitly listed falls back to "*"'s q-value, if present.
	if gzipQ == notListed {
		gzipQ = starQ
	}
	if deflateQ == notListed {
		deflateQ = starQ
	}

	gzipOK := gzipQ > 0
	deflateOK := deflateQ > 0

	switch {
	case gzipOK && deflateOK:
		if deflateQ > gzipQ {
			return "deflate"
		}
		return "gzip"
	case gzipOK:
		return "gzip"
	case deflateOK:
		return "deflate"
	default:
		return ""
	}
}

// parseCoding parses one comma-separated Accept-Encoding entry (e.g.
// "gzip;q=0.8") into its lowercased coding name and q-value (default 1.0).
func parseCoding(part string) (name string, q float64) {
	part = strings.TrimSpace(part)
	if part == "" {
		return "", 0
	}
	q = 1.0
	if i := strings.IndexByte(part, ';'); i >= 0 {
		params := part[i+1:]
		part = part[:i]
		for _, p := range strings.Split(params, ";") {
			p = strings.TrimSpace(p)
			if v, ok := strings.CutPrefix(p, "q="); ok {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = parsed
				}
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(part)), q
}

// isIncompressibleContentType reports whether ct (a Content-Type header
// value, possibly with a ";charset=..." suffix) names a format that gains
// nothing from gzip/deflate.
func isIncompressibleContentType(ct string) bool {
	if ct == "" {
		return false
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.ToLower(strings.TrimSpace(ct))
	if incompressibleContentTypes[ct] {
		return true
	}
	for _, prefix := range incompressibleContentTypePrefixes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

// compressingResponseWriter wraps an http.ResponseWriter, deferring the
// decision of whether to actually compress until the first byte is about to
// be written (WriteHeader or Write, whichever comes first) — by then the
// handler has had a chance to set Content-Type/Content-Length/its own
// Content-Encoding, all of which the decision depends on.
type compressingResponseWriter struct {
	http.ResponseWriter
	encoding   string // "gzip" or "deflate", never "" — CompressionMiddleware only constructs this after negotiating one
	status     int
	decided    bool
	compress   bool
	compressor io.WriteCloser
}

func (cw *compressingResponseWriter) WriteHeader(status int) {
	if cw.decided {
		cw.ResponseWriter.WriteHeader(status)
		return
	}
	cw.status = status
	cw.decide()
	cw.ResponseWriter.WriteHeader(status)
}

func (cw *compressingResponseWriter) Write(b []byte) (int, error) {
	if !cw.decided {
		if cw.status == 0 {
			cw.status = http.StatusOK
		}
		cw.decide()
		cw.ResponseWriter.WriteHeader(cw.status)
	}
	if cw.compress {
		return cw.compressor.Write(b)
	}
	return cw.ResponseWriter.Write(b)
}

// decide inspects the headers the handler has set so far and either commits
// to compressing (rewriting Content-Length/Content-Encoding/Vary and
// creating the compressor) or leaves the response untouched. Idempotent —
// only the first call has any effect, since both WriteHeader and Write only
// call it once (see decided).
func (cw *compressingResponseWriter) decide() {
	if cw.decided {
		return
	}
	cw.decided = true

	h := cw.ResponseWriter.Header()

	if h.Get("Content-Encoding") != "" {
		return // handler already encoded the body itself; don't double-compress
	}
	if cw.status != 0 && (cw.status < http.StatusOK || cw.status == http.StatusNoContent || cw.status == http.StatusNotModified) {
		return // no body to compress
	}
	if isIncompressibleContentType(h.Get("Content-Type")) {
		return
	}
	if cl := h.Get("Content-Length"); cl != "" {
		if n, err := strconv.Atoi(cl); err == nil && n < compressionMinBytes {
			return
		}
	}

	h.Del("Content-Length") // the compressed length isn't known up front; let chunked transfer encoding take over
	h.Set("Content-Encoding", cw.encoding)
	h.Add("Vary", "Accept-Encoding")

	switch cw.encoding {
	case "gzip":
		cw.compressor = gzip.NewWriter(cw.ResponseWriter)
	case "deflate":
		fw, err := flate.NewWriter(cw.ResponseWriter, flate.DefaultCompression)
		if err != nil {
			return // extremely unlikely (only invalid levels error); fall back to uncompressed
		}
		cw.compressor = fw
	default:
		return
	}
	cw.compress = true
}

// Close flushes and closes the compressor, if one was created. It's a no-op
// if the response was never written to (decide never ran) or wasn't
// compressed. Must be called after the wrapped handler returns — a
// gzip/flate Writer buffers internally and won't emit its final bytes
// (including the gzip trailer/checksum) until Close.
func (cw *compressingResponseWriter) Close() error {
	if cw.compressor == nil {
		return nil
	}
	return cw.compressor.Close()
}

// Flush implements http.Flusher, forwarding through the compressor (if
// active) so a streamed/chunked response — e.g. Server-Sent Events — still
// reaches the client incrementally instead of sitting in the compressor's
// internal buffer until Close.
func (cw *compressingResponseWriter) Flush() {
	if cw.compress {
		if f, ok := cw.compressor.(interface{ Flush() error }); ok {
			f.Flush()
		}
	}
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap gives net/http's http.ResponseController (and anything else using
// the standard unwrap convention) access to the underlying ResponseWriter,
// so capabilities this wrapper doesn't itself implement (e.g. http.Hijacker,
// used by WebSocket upgrades and by httputil.ReverseProxy) still work
// through it.
func (cw *compressingResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}
