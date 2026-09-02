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

// roundRobinTransport is an http.RoundTripper that distributes requests
// across multiple backend targets (config.RouteTargets — see its doc
// comment for the "interchangeable replicas" assumption this relies on)
// and fails over to the next target if a backend's RoundTrip returns an
// error (connection refused, DNS failure, timeout — the same class of
// error httputil.ReverseProxy's own ErrorHandler reacts to), rather than
// failing the whole request on the first backend that happens to be down.
//
// It never inspects the response status code to decide whether to retry —
// only a transport-level error triggers a failover attempt, the same
// distinction httputil.ReverseProxy itself draws between "the round trip
// failed" (ErrorHandler) and "the backend responded, however unhappily"
// (ModifyResponse, or nothing). Retrying because a backend returned a 5xx
// is a materially different, riskier feature (repeating a request a
// backend already executed) that this deliberately doesn't attempt.
//
// Built once per proxy route at registration time (see
// createProxyHandlerFunc), not per request.
type roundRobinTransport struct {
	targets   []*url.URL
	next      atomic.Uint64
	base      http.RoundTripper
	routeName string
}

// newRoundRobinTransport builds a roundRobinTransport for targets, using
// http.DefaultTransport to actually perform each attempt. targets must be
// non-empty.
func newRoundRobinTransport(targets []*url.URL, routeName string) *roundRobinTransport {
	return &roundRobinTransport{
		targets:   targets,
		base:      http.DefaultTransport,
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

	// req.Body is a single-use io.ReadCloser, and a failed attempt against
	// one backend may have already consumed part or all of it before the
	// connection failure surfaced — so it can't just be reused as-is
	// across attempts. Buffering it once and replaying a fresh reader per
	// attempt is the standard, if memory-costly, way reverse proxies
	// support retrying a request with a body; that cost only applies to
	// load-balanced (multi-target) routes; a single-target route never
	// reaches this branch at all.
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
