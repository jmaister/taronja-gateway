package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"github.com/jmaister/taronja-gateway/session"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// createProxyHandlerFunc builds the handler for proxy routes (without auth).
// targetURLs has several entries for a load-balanced route; targetURLs[0] gives
// the base proxy and path composition, as replicas share a path structure.
// Backend selection and failover happen per request in proxy.Transport.
func (g *Gateway) createProxyHandlerFunc(routeConfig config.RouteConfig, targetURLs []*url.URL) http.HandlerFunc {
	targetURL := targetURLs[0]

	// Create the proxy once when the handler is created
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	// otelhttp.NewTransport, not a bare http.DefaultTransport, when tracing
	// is enabled: it starts a child span per backend call and injects the
	// current trace context into the outbound request's "traceparent"
	// header, which is what actually makes this a *distributed* trace
	// instead of one isolated span per hop.
	var transport http.RoundTripper = http.DefaultTransport
	if builtin.FilterEnabled(g.GatewayConfig, builtin.FilterTracing) {
		transport = otelhttp.NewTransport(transport)
	}
	proxy.Transport = newRoundRobinTransport(targetURLs, routeConfig.Name, transport)

	// Read once here, same as transport above, at route-registration time.
	// See trustedForwardedHost's doc comment for what this actually gates.
	allowedHosts := allowedRedirectHosts(g.GatewayConfig)

	// Store the original director
	originalDirector := proxy.Director

	// Create a custom director that modifies the request
	proxy.Director = func(req *http.Request) {
		// Call the original director first
		originalDirector(req)

		// Apply path stripping and join logic
		if routeConfig.RemoveFromPath != "" {
			if strings.HasPrefix(req.URL.Path, routeConfig.RemoveFromPath) {
				req.URL.Path = strings.TrimPrefix(req.URL.Path, routeConfig.RemoveFromPath)

				if len(req.URL.Path) > 0 && !strings.HasPrefix(req.URL.Path, "/") {
					req.URL.Path = "/" + req.URL.Path
				} else if len(req.URL.Path) == 0 {
					req.URL.Path = "/"
				}

				// When using RemoveFromPath, we should NOT join with targetURL.Path
				// as that would reintroduce the prefix we just removed
				req.URL.RawPath = req.URL.EscapedPath()
			} else {
				req.URL.Path = singleJoiningSlash(targetURL.Path, req.URL.Path)
				req.URL.RawPath = req.URL.EscapedPath()
			}
		} else {
			// No RemoveFromPath, just join as before
			req.URL.Path = singleJoiningSlash(targetURL.Path, req.URL.Path)
			req.URL.RawPath = req.URL.EscapedPath()
		}

		// trustedForwardedHost only forwards hosts this gateway is configured for;
		// forwarding a client-supplied Host would allow Host-header poisoning of
		// backends that build absolute URLs from it.
		if forwardedHost := trustedForwardedHost(req, allowedHosts); forwardedHost != "" {
			req.Header.Set("X-Forwarded-Host", forwardedHost)
		} else {
			req.Header.Del("X-Forwarded-Host")
		}
		// session.RequestIsSecure applies the same trusted-proxy boundary as GetClientIP:
		// neither a bare req.TLS check nor a client-supplied X-Forwarded-Proto suffices.
		scheme := "http"
		if session.RequestIsSecure(req) {
			scheme = "https"
		}
		req.Header.Set("X-Forwarded-Proto", scheme)
		if clientIP := req.RemoteAddr; clientIP != "" {
			if prior, ok := req.Header["X-Forwarded-For"]; ok {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
			req.Header.Set("X-Forwarded-For", clientIP)
		}

		req.Host = targetURL.Host
	}

	// Set up SPA fallback via ModifyResponse when isSPA is enabled
	if routeConfig.IsSPA {
		proxy.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode != http.StatusNotFound {
				return nil
			}

			// resp.Request is what the transport actually sent, so the SPA fallback
			// re-fetches from the backend that returned the 404.
			fallbackBaseURL := &url.URL{
				Scheme: resp.Request.URL.Scheme,
				Host:   resp.Request.URL.Host,
				Path:   targetURL.Path,
			}

			log.Printf("Proxy Route [%s]: SPA fallback - upstream returned 404 for %s, fetching base URL: %s",
				routeConfig.Name, resp.Request.URL.Path, fallbackBaseURL.String())

			fallbackReq, err := http.NewRequestWithContext(resp.Request.Context(), http.MethodGet, fallbackBaseURL.String(), nil)
			if err != nil {
				log.Printf("Proxy Route [%s]: SPA fallback failed - could not create request: %v", routeConfig.Name, err)
				return nil
			}

			client := &http.Client{Timeout: 10 * time.Second}
			fallbackResp, err := client.Do(fallbackReq)
			if err != nil {
				log.Printf("Proxy Route [%s]: SPA fallback failed - could not fetch base URL: %v", routeConfig.Name, err)
				return nil
			}

			// Drain and close the original 404 response body
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			// Replace the 404 response with the fallback response
			resp.StatusCode = fallbackResp.StatusCode
			resp.Status = fallbackResp.Status
			for key := range resp.Header {
				delete(resp.Header, key)
			}
			for key, values := range fallbackResp.Header {
				resp.Header[key] = values
			}
			resp.Body = fallbackResp.Body
			resp.ContentLength = fallbackResp.ContentLength

			return nil
		}
	}

	// Fires when the upstream round trip fails. Do not Hijack the connection to
	// check whether the header was written: nothing closes it afterwards, so clients
	// would hang instead of getting a 502. http.Error is safe after a partial write.
	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error for route '%s' (From: %s) to %s: %v", routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), err)
		http.Error(rw, "Bad Gateway", http.StatusBadGateway)
	}

	// Return the handler function
	return func(w http.ResponseWriter, r *http.Request) {
		// Always drop client-supplied copies of these headers, including on routes with
		// authentication disabled, so a client cannot forge a gateway-asserted identity.
		// Validated sessions re-set them below.
		r.Header.Del(session.UserIdHeader)
		r.Header.Del(session.UserDataHeader)

		// For authenticated routes, extract user ID and set header
		if routeConfig.Authentication.Enabled {

			// Try to get session from request
			var sessionObject *db.Session

			// First check if session is in context
			if r.Context() != nil {
				if ctxSession, ok := r.Context().Value(session.SessionKey).(*db.Session); ok && ctxSession != nil {
					sessionObject = ctxSession
				}
			}

			// If not in context, try to get from cookie
			if sessionObject == nil {
				cookie, err := r.Cookie(session.SessionCookieName)
				if err == nil && cookie != nil {

					// Try direct lookup in memory repository for test scenarios
					if memStore, ok := g.Dependencies.SessionStore.(*session.SessionStoreDB); ok {
						session, err := memStore.Repo.FindSessionByToken(cookie.Value)
						if err == nil && session != nil {
							sessionObject = session
						} else {
							log.Printf("[auth] Session not found in repository: %v", err)
						}
					} else {
						log.Printf("[auth] SessionStore is not a SessionStoreDB, type: %T", g.Dependencies.SessionStore)
					}

					// If still not found, try normal validation
					if sessionObject == nil {
						validSession, exists := g.Dependencies.SessionStore.ValidateSession(r)
						if exists && validSession != nil {
							sessionObject = validSession
						}
					}
				} else {
					log.Printf("[auth] No session cookie found: %v", err)
				}
			}

			if sessionObject != nil && sessionObject.UserID != "" {
				// Modify the original request directly using header constants
				r.Header.Set(session.UserIdHeader, sessionObject.UserID)
				// Set X-User-Data header with serialized session object (JSON)
				sessionJson, err := json.Marshal(sessionObject)
				if err == nil {
					r.Header.Set(session.UserDataHeader, string(sessionJson))
				}

				// Serve the request with the modified headers
				proxy.ServeHTTP(w, r)
				return
			}
			log.Printf("[auth] No valid session found for authenticated route %s", routeConfig.Name)
		}

		// For non-authenticated routes or if no session found
		proxy.ServeHTTP(w, r)
	}
}

// singleJoiningSlash remains unchanged
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		if b == "" {
			return a
		}
		if len(b) > 0 && !strings.HasPrefix(b, "/") {
			b = "/" + b
		}
		return a + b
	case aslash && !bslash:
		return a + b
	case !aslash && bslash:
		return a + b
	}
	return a + b
}
