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

// geoSuccessTTL and geoFailureTTL are how long GetGeoDataFromIP trusts a
// cached result before calling the geolocation API again. A successful
// lookup is cached for a long time — an IP's location essentially never
// changes. A *failed* lookup — the API unreachable, rate-limiting, timing
// out — is cached too, but only briefly: long enough that repeated
// requests from the same client during an outage don't each pay the full
// network timeout (5s, see getGeoDataFromFreeIPAPI/getGeoDataFromIPLocate),
// short enough that service recovers within a minute of the API coming
// back. Before this, a failure was never cached at all, so every single
// request from an IP the API couldn't be reached for paid that 5s penalty
// — confirmed directly: 1,000 requests from the same test IP, with the
// free API unreachable from the sandbox this was found in, turned
// gateway/performance_test.go's TestMemoryUsage into an 80+ minute hang
// instead of a sub-second test.
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

// ipCacheMaxEntries bounds ipCache.cache's size. Before this, nothing ever
// removed an entry from the map at read or write time — every distinct
// value GetGeoDataFromIP was ever called with stayed cached forever — so a
// remote, unauthenticated client could grow it without limit simply by
// presenting many distinct "client IP" values: trivial over IPv6, or via a
// spoofed X-Forwarded-For entry in the common reverse-proxy topology this
// gateway documents as normal (see GetClientIP's own doc comment). 50,000
// entries is generous for any real deployment's distinct-visitor diversity
// — a few tens of MB at worst, not unbounded — while giving an attacker
// nothing to gain by exceeding it: once full, a genuinely new value simply
// isn't cached rather than displacing something else, and
// ipCacheCleanupInterval's periodic sweep reclaims room from real, expired
// traffic on an ordinary schedule.
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
// middleware.RateLimiter's: ipCache is a package-level singleton rebuilt
// only when the process restarts, with no equivalent per-instance/
// per-reload lifecycle to leak against.
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

// cacheGeoResult records the outcome of looking up ip — success or failure
// alike, see geoSuccessTTL/geoFailureTTL — unless ip is a genuinely new key
// and the cache is already at ipCacheMaxEntries, in which case the result is
// still returned to this one caller but not retained for the next. See that
// constant's doc comment for why a full cache drops new entries instead of
// evicting an old one to make room. Split out from GetGeoDataFromIP so a
// test can exercise the capping behavior directly, without needing a real
// network call to produce a genuine cache miss.
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

// isNonRoutable reports whether ip is loopback, private (RFC 1918 /
// RFC 4193), link-local, or unspecified ("0.0.0.0"/"::") — none of which a
// public geolocation API can meaningfully answer for, and none of which
// should ever be sent to one. Parses the address with net.ParseIP rather
// than matching a string prefix: the "127."/"localhost" check this
// replaced only ever caught IPv4 loopback, so a deployment behind a
// reverse proxy on the same private network — a normal, common shape for
// this gateway — sent every internal client's real RFC 1918 address
// (192.168.x.x, 10.x.x.x, 172.16-31.x.x) straight to the geolocation API,
// which naturally has no idea what to do with a non-routable address and
// returned an error for every single one of them, logged on every request.
// A malformed value (not a real IP at all) falls through to a real lookup
// attempt rather than being silently treated as non-routable here — that
// case surfaces as its own clear API error instead.
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
