package gateway

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync/atomic"
)

// roundRobinTransport distributes requests across config.RouteTargets (assumed
// interchangeable replicas) and fails over to the next target when RoundTrip
// returns an error. It never retries on a response status: repeating a request a
// backend already executed is riskier. Built once per route at registration.
type roundRobinTransport struct {
	targets   []*url.URL
	next      atomic.Uint64
	base      http.RoundTripper
	routeName string
}

// newRoundRobinTransport builds a roundRobinTransport for targets, using
// base to actually perform each attempt — http.DefaultTransport for a
// plain proxy route, or an otelhttp.NewTransport-wrapped one (see
// createProxyHandlerFunc) when tracing is enabled, so every proxied
// backend call gets its own child span and carries the trace context
// forward via the standard "traceparent" header. targets must be
// non-empty.
func newRoundRobinTransport(targets []*url.URL, routeName string, base http.RoundTripper) *roundRobinTransport {
	return &roundRobinTransport{
		targets:   targets,
		base:      base,
		routeName: routeName,
	}
}

// RoundTrip implements http.RoundTripper. For a single target, this is a
// direct, zero-overhead call to the base transport — byte-for-byte the
// same behavior as before this type existed (proxy.Transport was
// previously left nil, which http.Client/httputil.ReverseProxy treat as
// http.DefaultTransport). Only routes with more than one target pay for
// the round-robin selection and retry-on-failure logic below.
func (t *roundRobinTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.targets) == 1 {
		return t.base.RoundTrip(req)
	}

	// req.Body is single-use and a failed attempt may have consumed it, so buffer it
	// once and replay per attempt. Only multi-target routes pay this cost.
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("buffering request body for load-balanced retry: %w", err)
		}
	}

	start := t.next.Add(1) - 1
	var lastErr error
	for i := 0; i < len(t.targets); i++ {
		target := t.targets[(int(start)+i)%len(t.targets)]

		attempt := req.Clone(req.Context())
		attempt.URL.Scheme = target.Scheme
		attempt.URL.Host = target.Host
		attempt.Host = target.Host
		if bodyBytes != nil {
			attempt.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			attempt.ContentLength = int64(len(bodyBytes))
		}

		resp, err := t.base.RoundTrip(attempt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		log.Printf("Proxy Route [%s]: backend %s failed (%v)%s", t.routeName, target, err,
			nextAttemptSuffix(i, len(t.targets)))
	}
	return nil, lastErr
}

// nextAttemptSuffix returns ", trying next backend" unless attempt i was
// the last one available, purely to make roundRobinTransport's log line
// read correctly whether or not there's another target left to try.
func nextAttemptSuffix(i, total int) string {
	if i < total-1 {
		return ", trying next backend"
	}
	return " — no backends left to try"
}
