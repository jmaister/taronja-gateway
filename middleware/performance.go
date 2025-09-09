package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/lum8rjack/go-ja4h"
)

// PerformanceConfig holds performance optimization settings
type PerformanceConfig struct {
	EnableJA4HCaching         bool
	EnableStaticAssetSkipping bool
	EnableMetricsBatching     bool
	EnableResponseWriterPool  bool
	JA4HCacheSize             int
	MetricsBatchSize          int
}

// DefaultPerformanceConfig returns sensible defaults for performance optimization
func DefaultPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		EnableJA4HCaching:         true,
		EnableStaticAssetSkipping: true,
		EnableMetricsBatching:     true,
		EnableResponseWriterPool:  true,
		JA4HCacheSize:             1000,
		MetricsBatchSize:          100,
	}
}

// JA4HCache provides caching for JA4H fingerprints
type JA4HCache struct {
	cache  *ristretto.Cache
	hits   int64
	misses int64
}

// NewJA4HCache creates a new JA4H cache
func NewJA4HCache(maxSize int) *JA4HCache {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(maxSize) * 10, // recommended: 10x maxSize
		MaxCost:     int64(maxSize),
		BufferItems: 64,
	})
	if err != nil {
		panic(err)
	}
	return &JA4HCache{
		cache: c,
	}
}

// generateRequestKey creates a cache key from request characteristics
func (c *JA4HCache) generateRequestKey(r *http.Request) string {
	// Create a key based on relevant headers that affect JA4H fingerprint
	var keyParts []string

	// Add relevant headers
	if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
		keyParts = append(keyParts, "ua:"+userAgent)
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		keyParts = append(keyParts, "acc:"+accept)
	}
	if acceptEncoding := r.Header.Get("Accept-Encoding"); acceptEncoding != "" {
		keyParts = append(keyParts, "ae:"+acceptEncoding)
	}
	if acceptLanguage := r.Header.Get("Accept-Language"); acceptLanguage != "" {
		keyParts = append(keyParts, "al:"+acceptLanguage)
	}

	// Add remote IP address
	if r.RemoteAddr != "" {
		keyParts = append(keyParts, "ip:"+r.RemoteAddr)
	}

	// Add custom headers (X-Forwarded-For, X-Real-IP)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		keyParts = append(keyParts, "xff:"+xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		keyParts = append(keyParts, "xri:"+xri)
	}

	// Add connection info
	keyParts = append(keyParts, "method:"+r.Method)
	keyParts = append(keyParts, "proto:"+r.Proto)

	return strings.Join(keyParts, "|")
}

// GetOrCalculate retrieves a cached fingerprint or calculates a new one
func (c *JA4HCache) GetOrCalculate(r *http.Request) string {
	key := c.generateRequestKey(r)
	if val, found := c.cache.Get(key); found {
		c.hits++
		if fp, ok := val.(string); ok {
			return fp
		}
	}
	c.misses++
	fingerprint := ja4h.JA4H(r)
	// Set with expiration (e.g., 5 minutes)
	c.cache.SetWithTTL(key, fingerprint, 1, 5*time.Minute)
	return fingerprint
}

// GetStats returns cache statistics
func (c *JA4HCache) GetStats() (hits, misses int64, size int64) {
	return c.hits, c.misses, int64(c.cache.Metrics.KeysAdded() - c.cache.Metrics.KeysEvicted())
}

// Global cache instance
var ja4hCache *JA4HCache
var cacheOnce sync.Once

// getJA4HCache returns the singleton cache instance
func getJA4HCache() *JA4HCache {
	cacheOnce.Do(func() {
		ja4hCache = NewJA4HCache(1000) // Default cache size
	})
	return ja4hCache
}

// OptimizedJA4Middleware is an optimized version of JA4Middleware with caching
func OptimizedJA4Middleware(enableCaching bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ja4hFingerprint string

			if enableCaching {
				ja4hFingerprint = getJA4HCache().GetOrCalculate(r)
			} else {
				ja4hFingerprint = ja4h.JA4H(r)
			}

			if ja4hFingerprint == "" {
				// Don't log in production to avoid spam
				// log.Printf("Warning: JA4H fingerprint is empty for request %s %s", r.Method, r.URL.Path)
			}

			// Store the fingerprint in a custom header
			r.Header.Set(fingerprint.JA4HHeaderName, ja4hFingerprint)

			next.ServeHTTP(w, r)
		})
	}
}

// isStaticAsset determines if a request is for a static asset
func isStaticAsset(path string) bool {
	staticExtensions := []string{
		".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg",
		".woff", ".woff2", ".ttf", ".eot", ".webp", ".mp4", ".pdf",
		".zip", ".tar", ".gz", ".json", ".xml", ".txt",
	}

	pathLower := strings.ToLower(path)
	for _, ext := range staticExtensions {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}

	// Check for static paths
	staticPaths := []string{"/static/", "/_/static/", "/assets/", "/public/"}
	for _, staticPath := range staticPaths {
		if strings.Contains(pathLower, staticPath) {
			return true
		}
	}

	return false
}

// BuildOptimizedGlobalChain builds an optimized global middleware chain
func BuildOptimizedGlobalChain(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	perfConfig *PerformanceConfig,
) *ChainBuilder {
	chain := NewChainBuilder()

	// Skip analytics middleware for static assets if enabled
	if perfConfig.EnableStaticAssetSkipping {
		return buildConditionalChain(gatewayConfig, sessionStore, tokenService, trafficMetricRepo, perfConfig)
	}

	// Add middlewares conditionally based on configuration
	if gatewayConfig.Management.Analytics {
		// Use optimized JA4H middleware if caching is enabled
		if perfConfig.EnableJA4HCaching {
			chain.Add(OptimizedJA4Middleware(true))
		} else {
			chain.Add(JA4Middleware)
		}

		// Session extraction middleware (before traffic metrics to capture user info)
		chain.Add(SessionExtractionMiddleware(sessionStore, tokenService))

		// Traffic metrics middleware (potentially with batching)
		if perfConfig.EnableMetricsBatching {
			chain.Add(OptimizedTrafficMetricMiddleware(trafficMetricRepo, perfConfig))
		} else {
			chain.Add(TrafficMetricMiddleware(trafficMetricRepo))
		}
	}

	// Logging middleware (if enabled)
	if gatewayConfig.Management.Logging {
		chain.Add(LoggingMiddleware)
	}

	return chain
}

// buildConditionalChain creates a middleware chain that conditionally applies analytics
func buildConditionalChain(
	gatewayConfig *config.GatewayConfig,
	sessionStore session.SessionStore,
	tokenService *auth.TokenService,
	trafficMetricRepo db.TrafficMetricRepository,
	perfConfig *PerformanceConfig,
) *ChainBuilder {
	return NewChainBuilder().Add(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this is a static asset
			if isStaticAsset(r.URL.Path) {
				// For static assets, only apply minimal middleware
				if gatewayConfig.Management.Logging {
					LoggingMiddleware(next).ServeHTTP(w, r)
				} else {
					next.ServeHTTP(w, r)
				}
				return
			}

			// For non-static assets, apply full middleware chain
			fullChain := NewChainBuilder()

			if gatewayConfig.Management.Analytics {
				// Use optimized middleware
				if perfConfig.EnableJA4HCaching {
					fullChain.Add(OptimizedJA4Middleware(true))
				} else {
					fullChain.Add(JA4Middleware)
				}

				fullChain.Add(SessionExtractionMiddleware(sessionStore, tokenService))

				if perfConfig.EnableMetricsBatching {
					fullChain.Add(OptimizedTrafficMetricMiddleware(trafficMetricRepo, perfConfig))
				} else {
					fullChain.Add(TrafficMetricMiddleware(trafficMetricRepo))
				}
			}

			if gatewayConfig.Management.Logging {
				fullChain.Add(LoggingMiddleware)
			}

			// Apply the full chain
			fullChain.Build(next).ServeHTTP(w, r)
		})
	})
}

// OptimizedTrafficMetricMiddleware is a placeholder for optimized traffic metrics
// This would implement batching and other optimizations
func OptimizedTrafficMetricMiddleware(statsRepo db.TrafficMetricRepository, perfConfig *PerformanceConfig) func(http.Handler) http.Handler {
	// For now, return the standard middleware
	// In a full implementation, this would include:
	// - Batching of metrics
	// - Response writer pooling
	// - Reduced memory allocations
	return TrafficMetricMiddleware(statsRepo)
}

// PerformanceMiddlewareMetrics holds performance metrics for middleware
type PerformanceMiddlewareMetrics struct {
	JA4HCacheHits       int64
	JA4HCacheMisses     int64
	StaticAssetsSkipped int64
	TotalRequests       int64
	mutex               sync.RWMutex
}

// Global metrics instance
var perfMetrics = &PerformanceMiddlewareMetrics{}

// GetPerformanceMetrics returns the current performance metrics
func GetPerformanceMetrics() *PerformanceMiddlewareMetrics {
	return perfMetrics
}

// IncrementStaticAssetsSkipped increments the counter for skipped static assets
func (p *PerformanceMiddlewareMetrics) IncrementStaticAssetsSkipped() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.StaticAssetsSkipped++
}

// IncrementTotalRequests increments the total request counter
func (p *PerformanceMiddlewareMetrics) IncrementTotalRequests() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.TotalRequests++
}

// GetStats returns current statistics
func (p *PerformanceMiddlewareMetrics) GetStats() (staticSkipped, totalRequests int64) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.StaticAssetsSkipped, p.TotalRequests
}
