package session

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// IPGeoCache provides caching to avoid excessive API calls for the same IP
type IPGeoCache struct {
	cache map[string]GeoData
	mutex sync.RWMutex
	ttl   time.Duration
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
	Timestamp    time.Time
}

// Global cache instance with 7-day TTL
var ipCache = &IPGeoCache{
	cache: make(map[string]GeoData),
	ttl:   7 * 24 * time.Hour,
}

// GetGeoDataFromIP attempts to get comprehensive geolocation data for an IP address
// Uses iplocate.io if config has API key set, otherwise falls back to freeipapi.com
func GetGeoDataFromIP(ip string) (GeoData, error) {
	// Check if IP is empty
	if ip == "" {
		return GeoData{}, fmt.Errorf("IP address is empty")
	}

	// First check the cache
	ipCache.mutex.RLock()
	cachedData, found := ipCache.cache[ip]
	ipCache.mutex.RUnlock()

	// If found in cache and not expired, use the cached data
	if found && time.Since(cachedData.Timestamp) < ipCache.ttl {
		return cachedData, nil
	}

	// Check if we have an API key for iplocate.io
	var geoData GeoData
	var err error

	if globalGeoConfig != nil && globalGeoConfig.IPLocateAPIKey != "" {
		geoData, err = getGeoDataFromIPLocate(ip, globalGeoConfig.IPLocateAPIKey)
	} else {
		geoData, err = getGeoDataFromFreeIPAPI(ip)
	}

	if err != nil {
		return GeoData{}, err
	}

	// Update the cache
	ipCache.mutex.Lock()
	ipCache.cache[ip] = geoData
	ipCache.mutex.Unlock()

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
		Timestamp:   time.Now(),
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
		Timestamp:   time.Now(),
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
