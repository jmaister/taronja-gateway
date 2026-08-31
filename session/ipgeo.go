package session

import (
	"encoding/json"
	"fmt"
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

// GetGeoDataFromIP attempts to get comprehensive geolocation data for an IP address
// Uses iplocate.io if config has API key set, otherwise falls back to freeipapi.com
func GetGeoDataFromIP(ip string) (GeoData, error) {
	// Clean up the IP address
	ip = strings.TrimSpace(ip)
	// Check if IP is empty
	if ip == "" {
		return GeoData{}, fmt.Errorf("IP address is empty")
	}
	// Check if IP is localhost or 127.x.x.x
	if strings.Index(ip, "127.") == 0 || strings.Index(ip, "localhost") == 0 {
		return GeoData{}, nil // Return empty GeoData for localhost or 127.x.x.x
	}

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

	// Cache the outcome either way (see geoSuccessTTL/geoFailureTTL) — a
	// failed lookup used to never be cached at all, so every request from
	// an IP the geolocation API couldn't be reached for paid its full
	// network timeout, forever, for as long as that IP kept sending
	// requests.
	ipCache.mutex.Lock()
	ipCache.cache[ip] = geoCacheEntry{data: geoData, err: err, at: time.Now()}
	ipCache.mutex.Unlock()

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
