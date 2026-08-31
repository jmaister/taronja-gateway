package middleware

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/lum8rjack/go-ja4h"
)

// JA4HCache provides caching for JA4H fingerprints. GetOrCalculate runs
// concurrently for every in-flight request (it's the whole cache, shared
// process-wide via getJA4HCache's singleton), so hits/misses must be
// atomic — plain int64 with c.hits++ is a data race under any concurrent
// load, confirmed by `go test -race` on TestConcurrentRequests.
type JA4HCache struct {
	cache  *ristretto.Cache
	hits   atomic.Int64
	misses atomic.Int64
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
		c.hits.Add(1)
		if fp, ok := val.(string); ok {
			return fp
		}
	}
	c.misses.Add(1)
	fingerprint := ja4h.JA4H(r)
	// Set with expiration (e.g., 5 minutes)
	c.cache.SetWithTTL(key, fingerprint, 1, 5*time.Minute)
	return fingerprint
}

// GetStats returns cache statistics
func (c *JA4HCache) GetStats() (hits, misses int64, size int64) {
	return c.hits.Load(), c.misses.Load(), int64(c.cache.Metrics.KeysAdded() - c.cache.Metrics.KeysEvicted())
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
