package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
)

// Default values used when a CORSConfig field is left unset — see
// config.CORSConfig's field docs for what each one means.
var (
	defaultCORSAllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	defaultCORSAllowedHeaders = []string{"Content-Type", "Authorization"}
)

const defaultCORSMaxAgeSeconds = 600

// CORSMiddleware adds CORS (Cross-Origin Resource Sharing) response headers
// per cfg. If cfg.IsEnabled() is false (no allowed origins configured), it
// returns a pure pass-through — no headers are added, identical to the
// gateway's behavior before CORS support existed.
//
// This handles both actual cross-origin requests (adding
// Access-Control-Allow-Origin, and Access-Control-Allow-Credentials if
// configured) and preflight OPTIONS requests (additionally responding with
// Access-Control-Allow-Methods/Headers/Max-Age and a 204, short-circuiting
// before the wrapped handler runs — a preflight is never meant to reach real
// application logic).
func CORSMiddleware(cfg config.CORSConfig) func(http.Handler) http.Handler {
	if !cfg.IsEnabled() {
		return func(next http.Handler) http.Handler { return next }
	}

	allowedOrigins := make(map[string]bool, len(cfg.AllowedOrigins))
	allowAny := false
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAny = true
			continue
		}
		allowedOrigins[o] = true
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = defaultCORSAllowedMethods
	}
	methodsHeader := strings.Join(methods, ", ")

	headers := cfg.AllowedHeaders
	if len(headers) == 0 {
		headers = defaultCORSAllowedHeaders
	}
	headersHeader := strings.Join(headers, ", ")

	maxAge := cfg.MaxAgeSeconds
	if maxAge == 0 {
		maxAge = defaultCORSMaxAgeSeconds
	}
	maxAgeHeader := strconv.Itoa(maxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !(allowAny || allowedOrigins[origin]) {
				// Not a cross-origin request, or an origin we don't allow —
				// nothing to add, and definitely nothing to short-circuit
				// (an unrecognized Origin on a non-preflight request is just
				// a normal same-origin-policy-enforced-by-the-browser
				// request as far as this middleware is concerned).
				next.ServeHTTP(w, r)
				return
			}

			header := w.Header()
			if allowAny && !cfg.AllowCredentials {
				header.Set("Access-Control-Allow-Origin", "*")
			} else {
				// Echo the specific origin back (never "*") whenever
				// credentials are involved, or when the origin list is an
				// explicit set rather than a wildcard — both correct per the
				// CORS spec, and Vary: Origin tells caches/CDNs this
				// response depends on the request's Origin header.
				header.Set("Access-Control-Allow-Origin", origin)
				header.Add("Vary", "Origin")
			}
			if cfg.AllowCredentials {
				header.Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == http.MethodOptions {
				header.Set("Access-Control-Allow-Methods", methodsHeader)
				header.Set("Access-Control-Allow-Headers", headersHeader)
				header.Set("Access-Control-Max-Age", maxAgeHeader)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
