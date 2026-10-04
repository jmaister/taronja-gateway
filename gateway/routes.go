package gateway

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
)

// configureUserRoutes sets up the main proxy and static file routes defined by the user.
// This function continues to serve user-defined static routes from the OS filesystem.
func (g *Gateway) configureUserRoutes() error {
	log.Printf("Registering user-defined routes...")
	for _, routeConfig := range g.GatewayConfig.Routes {
		var handler http.HandlerFunc

		// Create the base handler (proxy or static)
		if routeConfig.Static {
			handler = g.createStaticHandlerFunc(routeConfig)
			if handler == nil {
				// Skip to next route if handler creation failed
				continue
			}
		} else {
			if len(routeConfig.To) == 0 {
				log.Printf("Warning: Empty 'to' URL for proxy route '%s'. Skipping registration.", routeConfig.Name)
				continue
			}
			targetURLs := make([]*url.URL, 0, len(routeConfig.To))
			var parseErr error
			for _, to := range routeConfig.To {
				targetURL, err := url.Parse(to)
				if err != nil {
					parseErr = fmt.Errorf("target %q: %w", to, err)
					break
				}
				targetURLs = append(targetURLs, targetURL)
			}
			if parseErr != nil {
				log.Printf("Warning: Invalid target URL(s) %v for proxy route '%s': %v. Skipping registration.", []string(routeConfig.To), routeConfig.Name, parseErr)
				continue
			}
			handler = g.createProxyHandlerFunc(routeConfig, targetURLs)
			if len(targetURLs) > 1 {
				log.Printf("Proxy Route [%s]: load balancing across %d backends: %s", routeConfig.Name, len(targetURLs), formatTargets(routeConfig.To))
			}
			if routeConfig.IsSPA {
				log.Printf("Proxy Route [%s]: SPA mode enabled - upstream 404s will fall back to base URL: %s", routeConfig.Name, formatTargets(routeConfig.To))
			}
		}

		// Wrap with cache control for all routes
		handler = g.RouteChainBuilder.BuildRouteChain(handler, routeConfig)

		// Register the final handler for routeConfig.From
		// To match /api with /api/hello and /api/foo, the pattern must be "/api/"
		pattern := routeConfig.From
		if strings.HasSuffix(pattern, "/*") {
			// Register both the base and wildcard pattern for ServeMux
			basePattern := strings.TrimSuffix(pattern, "*")
			g.Mux.HandleFunc(basePattern, handler)
			log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (patterns: %s, %s)",
				routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), routeConfig.Authentication.Enabled, basePattern, pattern)
		} else {
			// For static file routes, register both with and without trailing slash to avoid redirects
			if routeConfig.Static && routeConfig.ToFile != "" {
				// Register without trailing slash
				g.Mux.HandleFunc(routeConfig.From, handler)

				// Also register with trailing slash
				patternWithSlash := routeConfig.From
				if !strings.HasSuffix(patternWithSlash, "/") {
					patternWithSlash += "/"
				}
				g.Mux.HandleFunc(patternWithSlash, handler)

				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (patterns: %s, %s)",
					routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), routeConfig.Authentication.Enabled,
					routeConfig.From, patternWithSlash)
			} else {
				// For other routes, ensure the pattern ends with a slash for consistency
				if !strings.HasSuffix(pattern, "/") {
					pattern += "/"
				}
				g.Mux.HandleFunc(pattern, handler)
				log.Printf("Registered User Route  : %-25s | From: %-20s | To: %s | Auth: %t (pattern: %s)",
					routeConfig.Name, routeConfig.From, formatTargets(routeConfig.To), routeConfig.Authentication.Enabled, pattern)
			}
		}
	}
	return nil
}

// configureOAuthCallbackRoute remains the same
func (g *Gateway) configureOAuthCallbackRoute() {
	g.Mux.HandleFunc("/auth/callback/", func(w http.ResponseWriter, r *http.Request) {
		pathSegments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathSegments) != 3 || pathSegments[0] != "auth" || pathSegments[1] != "callback" {
			log.Printf("OAuth Callback: Invalid callback path format: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		//provider := pathSegments[2]

		// TODO: Check if provider is valid

		//g.authManager.HandleOAuthCallback(provider, w, r)
	})
	log.Printf("Registered OAuth Callback Handler: /auth/callback/*")
}

// convertToServeMuxPattern converts a route "from" glob pattern that contains
// wildcard "*" segments into a Go 1.22+ net/http.ServeMux named-wildcard pattern.
//
// Rules:
//   - A "*" in a non-final segment becomes a named wildcard {wN} that matches
//     exactly one path segment.
//   - A "*" as the final segment is removed and a trailing slash is used instead,
//     which makes ServeMux match everything below that prefix (subtree match).
//
// Examples:
//
//	/api/boxes/*/certs        → /api/boxes/{w0}/certs
//	/a/*/b/*/c                → /a/{w0}/b/{w1}/c
//	/api/boxes/*/certs/*      → /api/boxes/{w0}/certs/
func convertToServeMuxPattern(from string) string {
	segments := strings.Split(from, "/")
	wildcardIdx := 0
	for i, seg := range segments {
		if seg == "*" {
			if i == len(segments)-1 {
				// Last segment: strip it; the trailing slash left by the join provides subtree match.
				segments[i] = ""
			} else {
				segments[i] = fmt.Sprintf("{w%d}", wildcardIdx)
				wildcardIdx++
			}
		}
	}
	return strings.Join(segments, "/")
}

// hasMiddleWildcard reports whether from contains a "*" wildcard that is not
// solely a trailing "/*" (i.e. there is at least one wildcard in a non-final
// position, or there are multiple wildcards).
func hasMiddleWildcard(from string) bool {
	if !strings.Contains(from, "*") {
		return false
	}
	// A single trailing /* is handled by the existing subtree-match code path.
	if strings.HasSuffix(from, "/*") && strings.Count(from, "*") == 1 {
		return false
	}
	return true
}

// formatTargets renders a route's config.RouteTargets for a log line — a
// single target prints as itself, unadorned, so existing single-backend
// routes' log output is unchanged; more than one prints comma-joined
// rather than Go's default "[a b]" slice format.
func formatTargets(targets config.RouteTargets) string {
	return strings.Join(targets, ", ")
}
