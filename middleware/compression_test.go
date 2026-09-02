package middleware

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- negotiateEncoding -------------------------------------------------

func TestNegotiateEncoding(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"no header", "", ""},
		{"plain gzip", "gzip", "gzip"},
		{"plain deflate", "deflate", "deflate"},
		{"both, gzip preferred on tie", "gzip, deflate", "gzip"},
		{"both, explicit equal q", "gzip;q=0.8, deflate;q=0.8", "gzip"},
		{"deflate preferred with higher q", "gzip;q=0.5, deflate;q=0.9", "deflate"},
		{"gzip excluded with q=0", "gzip;q=0, deflate", "deflate"},
		{"only unsupported coding", "br", ""},
		{"wildcard accepts gzip", "*", "gzip"},
		{"wildcard excluded, no explicit gzip", "*;q=0, gzip;q=0.5", "gzip"},
		{"wildcard excludes everything", "*;q=0", ""},
		// deflate has no explicit q, so it defaults to 1.0 and outranks
		// gzip's explicit 0.9 — this case is really about tolerating the
		// stray whitespace around the ';' and ',' separators, not about q.
		{"extra whitespace", "  gzip ; q=0.9 , deflate", "deflate"},
		{"unsupported plus gzip", "br, gzip", "gzip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, negotiateEncoding(tt.header))
		})
	}
}

// --- isIncompressibleContentType -----------------------------------------

func TestIsIncompressibleContentType(t *testing.T) {
	assert.True(t, isIncompressibleContentType("image/png"))
	assert.True(t, isIncompressibleContentType("video/mp4"))
	assert.True(t, isIncompressibleContentType("application/zip"))
	assert.True(t, isIncompressibleContentType("application/pdf; charset=binary"))
	assert.False(t, isIncompressibleContentType("text/html"))
	assert.False(t, isIncompressibleContentType("application/json"))
	assert.False(t, isIncompressibleContentType("text/html; charset=utf-8"))
	assert.False(t, isIncompressibleContentType(""))
}

// --- CompressionMiddleware: end-to-end behavior --------------------------

// bigBody is comfortably larger than compressionMinBytes and repetitive
// enough that gzip/deflate visibly shrink it, so a test asserting the
// response actually got smaller isn't relying on incompressible noise.
var bigBody = strings.Repeat("the quick brown fox jumps over the lazy dog. ", 100)

func handlerWritingBody(body string, contentType string, setContentLength bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if setContentLength {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		w.Write([]byte(body))
	})
}

func decompress(t *testing.T, encoding string, data []byte) []byte {
	t.Helper()
	switch encoding {
	case "gzip":
		r, err := gzip.NewReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer r.Close()
		out, err := io.ReadAll(r)
		require.NoError(t, err)
		return out
	case "deflate":
		r := flate.NewReader(bytes.NewReader(data))
		defer r.Close()
		out, err := io.ReadAll(r)
		require.NoError(t, err)
		return out
	default:
		t.Fatalf("unknown encoding %q", encoding)
		return nil
	}
}

func TestCompressionMiddleware_CompressesWithGzip(t *testing.T) {
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "text/plain", true))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "gzip", rw.Header().Get("Content-Encoding"))
	assert.Contains(t, rw.Header().Values("Vary"), "Accept-Encoding")
	assert.Empty(t, rw.Header().Get("Content-Length"), "the declared length no longer matches the compressed body")
	assert.Less(t, rw.Body.Len(), len(bigBody), "a repetitive body must actually shrink")
	assert.Equal(t, bigBody, string(decompress(t, "gzip", rw.Body.Bytes())))
}

func TestCompressionMiddleware_CompressesWithDeflate(t *testing.T) {
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "text/plain", true))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "deflate")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "deflate", rw.Header().Get("Content-Encoding"))
	assert.Less(t, rw.Body.Len(), len(bigBody))
	assert.Equal(t, bigBody, string(decompress(t, "deflate", rw.Body.Bytes())))
}

func TestCompressionMiddleware_NoAcceptEncodingIsPassthrough(t *testing.T) {
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "text/plain", true))

	req := httptest.NewRequest(http.MethodGet, "/", nil) // no Accept-Encoding at all
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Empty(t, rw.Header().Get("Content-Encoding"))
	assert.Equal(t, bigBody, rw.Body.String())
}

func TestCompressionMiddleware_AlreadyEncodedResponseIsLeftAlone(t *testing.T) {
	handler := CompressionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br") // handler compressed it itself
		w.Write([]byte(bigBody))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "br", rw.Header().Get("Content-Encoding"), "must not overwrite the handler's own encoding")
	assert.Equal(t, bigBody, rw.Body.String(), "must not double-compress an already-encoded body")
}

func TestCompressionMiddleware_TinyResponseIsPassthrough(t *testing.T) {
	tiny := "ok"
	handler := CompressionMiddleware(handlerWritingBody(tiny, "text/plain", true))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Empty(t, rw.Header().Get("Content-Encoding"), "a declared-tiny body isn't worth compressing")
	assert.Equal(t, tiny, rw.Body.String())
}

func TestCompressionMiddleware_UnknownLengthStreamIsStillCompressed(t *testing.T) {
	// No Content-Length declared (the streaming/chunked case) — compression
	// must not skip it just because the size threshold check has nothing to
	// compare against.
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "text/plain", false))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "gzip", rw.Header().Get("Content-Encoding"))
	assert.Equal(t, bigBody, string(decompress(t, "gzip", rw.Body.Bytes())))
}

func TestCompressionMiddleware_IncompressibleContentTypeIsPassthrough(t *testing.T) {
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "image/png", true))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Empty(t, rw.Header().Get("Content-Encoding"))
	assert.Equal(t, bigBody, rw.Body.String())
}

func TestCompressionMiddleware_RangeRequestIsPassthrough(t *testing.T) {
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "text/plain", true))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-99")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Empty(t, rw.Header().Get("Content-Encoding"), "compressing would change what the requested byte range means")
	assert.Equal(t, bigBody, rw.Body.String())
}

func TestCompressionMiddleware_HeadRequestIsPassthrough(t *testing.T) {
	handler := CompressionMiddleware(handlerWritingBody(bigBody, "text/plain", true))

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Empty(t, rw.Header().Get("Content-Encoding"))
}

func TestCompressionMiddleware_NoContentStatusIsUntouched(t *testing.T) {
	handler := CompressionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNoContent, rw.Code)
	assert.Empty(t, rw.Header().Get("Content-Encoding"))
	assert.Empty(t, rw.Body.Bytes())
}

// hijackableRecorder is an httptest.ResponseRecorder that also implements
// http.Hijacker, to prove CompressionMiddleware leaves Connection: Upgrade
// requests' ResponseWriter untouched (real Hijacker access, not just
// Unwrap()) rather than wrapping it — httputil.ReverseProxy's own upgrade
// handling does a direct `w.(http.Hijacker)` type assertion, which a wrapper
// that only exposes Unwrap() would fail.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func TestCompressionMiddleware_UpgradeRequestLeavesHijackerIntact(t *testing.T) {
	var sawHijacker bool
	handler := CompressionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawHijacker = w.(http.Hijacker)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rw := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(rw, req)

	assert.True(t, sawHijacker, "an Upgrade request must reach the handler with its Hijacker capability intact")
}
