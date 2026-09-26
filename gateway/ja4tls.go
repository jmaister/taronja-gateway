package gateway

import (
	"crypto/tls"
	"net"
	"net/http"

	"github.com/exaring/ja4plus"
	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
)

// tlsJA4 wires TLS-level JA4 fingerprinting (github.com/exaring/ja4plus)
// into the gateway. It only exists — and only gets constructed — when TLS
// is enabled with a certificate this gateway itself possesses (a static
// certFile/keyFile, or one obtained via ACME): computing JA4 needs the raw
// ClientHello, which is only ever visible at the TLS layer this gateway
// terminates, never if TLS is terminated by something else in front of it.
//
// Unlike JA4H (middleware/ja4.go — computed per HTTP request from header
// count/order/presence, which varies constantly between a page load and
// its own subresource/API requests; see doc/middleware/ja4-fingerprint.md),
// TLS JA4 is computed once per TLS connection, from the client's actual
// TLS stack (cipher suites, extensions, ALPN, TLS version) — a property of
// the client's OS/browser/TLS library, not of any individual request, so
// it doesn't change between requests on the same connection the way JA4H
// does.
type tlsJA4 struct {
	mw ja4plus.JA4Middleware
}

func newTLSJA4() *tlsJA4 {
	return &tlsJA4{}
}

// configureTLSConfig layers JA4 capture onto an already-built *tls.Config
// (from newTLSConfig or acmeTLSConfig) without disturbing its existing
// GetCertificate/NextProtos/MinVersion. GetConfigForClient returning
// (nil, nil) tells crypto/tls to keep using the Config already selected
// for this handshake — see crypto/tls's documented behavior for
// GetConfigForClient — so this is purely additive over what gateway.go
// already sets.
func (j *tlsJA4) configureTLSConfig(tlsConfig *tls.Config) {
	tlsConfig.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		j.mw.StoreFingerprintFromClientHello(hello)
		return nil, nil
	}
}

// connStateCallback must be set as the http.Server's ConnState so
// ja4plus.JA4Middleware can evict a closed connection's stored fingerprint
// instead of leaking one entry per connection for the life of the process.
func (j *tlsJA4) connStateCallback(conn net.Conn, state http.ConnState) {
	j.mw.ConnStateCallback(conn, state)
}

// middleware wraps next so a downstream handler sees the fingerprint
// captured during the TLS handshake as a request header
// (fingerprint.JA4TLSHeaderName) — the same "read it back off the request"
// pattern JA4H already establishes (see middleware/ja4.go), so
// session.NewClientInfo and any other consumer don't need a second way to
// access a fingerprint. Must wrap the entire chain (added outside
// buildRuntime's handler in applyConfig) so the header is set before
// session_extraction/traffic_metrics run.
func (j *tlsJA4) middleware(next http.Handler) http.Handler {
	return j.mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fp := ja4plus.JA4FromContext(r.Context()); fp != "" {
			r.Header.Set(fingerprint.JA4TLSHeaderName, fp)
		}
		next.ServeHTTP(w, r)
	}))
}
