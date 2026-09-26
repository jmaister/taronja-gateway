package session

import "strings"

// staticAssetExtensions lists file extensions treated as static assets:
// stylesheets, scripts, images, fonts, and other files a browser fetches
// alongside a page but that carry no per-user application state of their
// own.
var staticAssetExtensions = []string{
	".css", ".js", ".mjs", ".map",
	".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".webp",
	".woff", ".woff2", ".ttf", ".eot",
	".mp4", ".webm", ".pdf",
	".zip", ".tar", ".gz",
}

// staticAssetPathPrefixes lists URL path substrings conventionally used to
// serve static assets, for paths that don't carry one of
// staticAssetExtensions (e.g. an extensionless font or a path with a query
// string already stripped by the time it gets here).
var staticAssetPathPrefixes = []string{
	"/static/", "/_/static/", "/assets/", "/public/",
}

// IsStaticAssetPath reports whether path looks like a request for a static
// asset (CSS, JS, images, fonts, ...) rather than a page, API call, or other
// dynamic request. It's a path-only heuristic — extension and conventional
// directory names — because it has to run in global middleware, before the
// mux has matched a route and before anything is known about whether that
// route was configured with `static: true`.
//
// Used both to tag TrafficMetric rows with IsStaticAsset (so reports can
// filter by it after the fact) and, when
// config.ManagementConfig.ExcludeStaticAssets is set, to decide whether
// TrafficMetricMiddleware should record a row for this request at all.
func IsStaticAssetPath(path string) bool {
	pathLower := strings.ToLower(path)

	for _, ext := range staticAssetExtensions {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}

	for _, prefix := range staticAssetPathPrefixes {
		if strings.Contains(pathLower, prefix) {
			return true
		}
	}

	return false
}
