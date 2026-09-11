package middleware

import (
	"testing"
)

// scanURLs is a realistic-sized vulnerability-scan URL list — the kind of
// hand-curated set of common scanner-probe paths an operator would
// actually configure (WordPress admin paths, .env, common backup/archive
// names, PHP shells, ...).
var scanURLs = []string{
	"/wp-admin/*", "/wp-login.php", "/.env", "/.git/*", "/*.php",
	"/phpmyadmin/*", "/admin/*.php", "/backup/*.zip", "/config/*.yml",
	"/*.bak", "/vendor/*", "/.aws/*", "/shell.php", "/xmlrpc.php",
	"/*.sql", "/db_backup/*", "/.ssh/*", "/server-status", "/*.tar.gz",
	"/cgi-bin/*",
}

// requestPaths are sample 404 paths a scanner would generate — mostly
// non-matches (a real scan tries far more dead ends than hits), matching
// the loop's actual worst case: scanning every pattern before concluding
// "no match".
var scanRequestPaths = []string{
	"/some/totally/unrelated/path",
	"/api/v1/widgets/123",
	"/favicon.ico",
	"/nonexistent-page",
}

// BenchmarkVulnerabilityScanMatch_PerCallRecompute mirrors what
// RateLimiter.Handler's vulnerability-scan check used to do: for every
// configured pattern, on every 404, re-derive the normalized/expanded form
// from scratch via matchesVulnerabilityScanPath.
func BenchmarkVulnerabilityScanMatch_PerCallRecompute(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		path := scanRequestPaths[i%len(scanRequestPaths)]
		for _, pattern := range scanURLs {
			if matchesVulnerabilityScanPath(pattern, path) {
				break
			}
		}
	}
}

// BenchmarkVulnerabilityScanMatch_Precomputed mirrors the current
// RateLimiter.Handler: patterns preprocessed once (as NewRateLimiter does),
// request path normalized once per request rather than once per pattern.
func BenchmarkVulnerabilityScanMatch_Precomputed(b *testing.B) {
	patterns := make([]scanPattern, len(scanURLs))
	for i, url := range scanURLs {
		patterns[i] = newScanPattern(url)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		path := scanRequestPaths[i%len(scanRequestPaths)]
		normalizedPath := normalizeScanPath(path)
		for _, sp := range patterns {
			if sp.matches(normalizedPath) {
				break
			}
		}
	}
}
