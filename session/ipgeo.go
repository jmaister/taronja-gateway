package session

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
)

// Global geolocation configuration
var globalGeoConfig *config.GeolocationConfig

// SetGeolocationConfig sets the global geolocation configuration
func SetGeolocationConfig(geoConfig *config.GeolocationConfig) {
	globalGeoConfig = geoConfig
}

// geoSuccessTTL and geoFailureTTL bound how long a cached lookup is trusted.
// Failures are cached briefly so an API outage does not cost every request the
// full 5s timeout.
const (
	geoSuccessTTL = 7 * 24 * time.Hour
	geoFailureTTL = time.Minute
)

// geoCacheEntry holds one cached GetGeoDataFromIP result — either outcome,
// not just success (see geoSuccessTTL/geoFailureTTL above).
type geoCacheEntry struct {
	data GeoData
	err  error
	at   time.Time
}

// expired reports whether e should be treated as a cache miss as of t —
// geoFailureTTL after it was recorded if it was a failure, geoSuccessTTL
// after if it was a success.
func (e geoCacheEntry) expired(t time.Time) bool {
	ttl := geoSuccessTTL
	if e.err != nil {
		ttl = geoFailureTTL
	}
	return t.Sub(e.at) >= ttl
}

// IPGeoCache provides caching to avoid excessive API calls for the same IP
type IPGeoCache struct {
	cache map[string]geoCacheEntry
	mutex sync.RWMutex
}

// ipCacheMaxEntries bounds the cache so a client presenting many distinct IPs
// cannot grow it without limit. When full, new values are not cached; the
// periodic sweep reclaims expired entries.
const ipCacheMaxEntries = 50_000

// ipCacheCleanupInterval is how often the background sweep (see
// startIPCacheCleanup) removes entries geoCacheEntry.expired considers
// stale — independent of geoSuccessTTL/geoFailureTTL, which only ever
// governed whether a *read* treats a cached entry as a miss, never whether
// it gets removed. Paired with ipCacheMaxEntries: the cap bounds growth
// within one interval, the sweep bounds it thereafter.
const ipCacheCleanupInterval = 10 * time.Minute

var ipCacheCleanupOnce sync.Once

// startIPCacheCleanup lazily starts ipCache's background sweep on first
// use, not at package init — a process that imports this package without
// ever calling GetGeoDataFromIP (most tests) never spins up a goroutine it
// has no way to stop. This is deliberately a forever-running,
// process-lifetime goroutine rather than one with a Close() method like
// middleware.RateLimiter's: ipCache is a package-level singleton.
func startIPCacheCleanup() {
	ipCacheCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(ipCacheCleanupInterval)
			defer ticker.Stop()
			for now := range ticker.C {
				sweepExpiredGeoCacheEntries(now)
			}
		}()
	})
}

// sweepExpiredGeoCacheEntries removes every ipCache entry geoCacheEntry.expired
// considers stale as of now. Split out from startIPCacheCleanup's ticker loop
// so a test can exercise one sweep pass directly, without waiting on
// ipCacheCleanupInterval or depending on the background goroutine at all.
func sweepExpiredGeoCacheEntries(now time.Time) {
	ipCache.mutex.Lock()
	defer ipCache.mutex.Unlock()
	for ip, entry := range ipCache.cache {
		if entry.expired(now) {
			delete(ipCache.cache, ip)
		}
	}
}

// cacheGeoResult records a lookup result unless the cache is full and ip is new
// (it is still returned to the caller). Split out so tests can exercise the cap.
func cacheGeoResult(ip string, geoData GeoData, err error) {
	ipCache.mutex.Lock()
	defer ipCache.mutex.Unlock()
	if _, exists := ipCache.cache[ip]; exists || len(ipCache.cache) < ipCacheMaxEntries {
		ipCache.cache[ip] = geoCacheEntry{data: geoData, err: err, at: time.Now()}
	}
}

// GeoData holds the geolocation data for an IP
type GeoData struct {
	Latitude     float64
	Longitude    float64
	City         string
	Country      string
	CountryCode  string
	Region       string
	Continent    string
	ZipCode      string
	FormattedLoc string // Formatted location string for display
}

// Global cache instance
var ipCache = &IPGeoCache{
	cache: make(map[string]geoCacheEntry),
}

// isNonRoutable reports whether ip is loopback, private, link-local or
// unspecified, none of which a public geolocation API can answer. A malformed
// value is not treated as non-routable.
func isNonRoutable(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() || parsed.IsUnspecified()
}

// GetGeoDataFromIP attempts to get comprehensive geolocation data for an IP address
// Uses iplocate.io if config has API key set, otherwise falls back to freeipapi.com
func GetGeoDataFromIP(ip string) (GeoData, error) {
	// Clean up the IP address
	ip = strings.TrimSpace(ip)
	// Check if IP is empty
	if ip == "" {
		return GeoData{}, fmt.Errorf("IP address is empty")
	}
	// Both providers embed ip in a URL, so reject anything that is not a real IP
	// address (e.g. containing "/", "?" or "#").
	if net.ParseIP(ip) == nil {
		return GeoData{}, fmt.Errorf("invalid IP address: %q", ip)
	}
	if isNonRoutable(ip) {
		return GeoData{}, nil // nothing a public geo API could ever answer for
	}

	startIPCacheCleanup()

	// First check the cache — a hit, success or failure, is returned as-is
	// without calling the API again. See geoSuccessTTL/geoFailureTTL for
	// why a failure is cached too, just briefly.
	ipCache.mutex.RLock()
	entry, found := ipCache.cache[ip]
	ipCache.mutex.RUnlock()

	if found && !entry.expired(time.Now()) {
		return entry.data, entry.err
	}

	// Check if we have an API key for iplocate.io
	var geoData GeoData
	var err error

	if globalGeoConfig != nil && globalGeoConfig.IPLocateAPIKey != "" {
		geoData, err = getGeoDataFromIPLocate(ip, globalGeoConfig.IPLocateAPIKey)
	} else {
		geoData, err = getGeoDataFromFreeIPAPI(ip)
	}

	cacheGeoResult(ip, geoData, err)

	if err != nil {
		return GeoData{}, err
	}
	return geoData, nil
}

// getGeoDataFromFreeIPAPI calls the free freeipapi.com service
func getGeoDataFromFreeIPAPI(ip string) (GeoData, error) {
	url := fmt.Sprintf("https://freeipapi.com/api/json/%s", ip)
	client := http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return GeoData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GeoData{}, fmt.Errorf("FreeIPAPI returned status code %d", resp.StatusCode)
	}

	// Parse the response
	var result struct {
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		CityName    string  `json:"cityName"`
		CountryName string  `json:"countryName"`
		CountryCode string  `json:"countryCode"`
		RegionName  string  `json:"regionName"`
		Continent   string  `json:"continent"`
		ZipCode     string  `json:"zipCode"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return GeoData{}, err
	}

	// Create the GeoData object
	geoData := GeoData{
		Latitude:    result.Latitude,
		Longitude:   result.Longitude,
		City:        result.CityName,
		Country:     result.CountryName,
		CountryCode: result.CountryCode,
		Region:      result.RegionName,
		Continent:   result.Continent,
		ZipCode:     result.ZipCode,
	}

	formatGeoLocation(&geoData)
	return geoData, nil
}

// getGeoDataFromIPLocate calls the iplocate.io service with API key
func getGeoDataFromIPLocate(ip, apiKey string) (GeoData, error) {
	url := fmt.Sprintf("https://www.iplocate.io/api/lookup/%s?format=json", ip)
	client := http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return GeoData{}, err
	}

	// Add API key as header
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return GeoData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GeoData{}, fmt.Errorf("IPLocate returned status code %d", resp.StatusCode)
	}

	// Parse the response
	var result struct {
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		City        string  `json:"city"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Subdivision string  `json:"subdivision"`
		Continent   string  `json:"continent"`
		PostalCode  string  `json:"postal_code"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return GeoData{}, err
	}

	// Create the GeoData object
	geoData := GeoData{
		Latitude:    result.Latitude,
		Longitude:   result.Longitude,
		City:        result.City,
		Country:     result.Country,
		CountryCode: result.CountryCode,
		Region:      result.Subdivision,
		Continent:   result.Continent,
		ZipCode:     result.PostalCode,
	}

	formatGeoLocation(&geoData)
	return geoData, nil
}

// formatGeoLocation formats the location string for display
func formatGeoLocation(geoData *GeoData) {
	if geoData.City != "" && geoData.Region != "" && geoData.Country != "" {
		geoData.FormattedLoc = fmt.Sprintf("%s, %s, %s", geoData.Country, geoData.Region, geoData.City)
	} else if geoData.City != "" && geoData.Country != "" {
		geoData.FormattedLoc = fmt.Sprintf("%s, %s", geoData.Country, geoData.City)
	} else if geoData.Country != "" {
		geoData.FormattedLoc = geoData.Country
	} else {
		geoData.FormattedLoc = "Unknown"
	}
}

// Copy GeoData into an instance of TrafficMetric
func (g GeoData) ToTrafficMetric(target *db.TrafficMetric) {
	target.GeoLocation = g.FormattedLoc
	target.Latitude = g.Latitude
	target.Longitude = g.Longitude
	target.City = g.City
	target.ZipCode = g.ZipCode
	target.Country = g.Country
	target.CountryCode = g.CountryCode
	target.Region = g.Region
	target.Continent = g.Continent
}
