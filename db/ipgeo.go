package db

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

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

// Global cache instance with 24-hour TTL
var ipCache = &IPGeoCache{
	cache: make(map[string]GeoData),
	ttl:   24 * time.Hour,
}

// GetGeoDataFromIP attempts to get comprehensive geolocation data for an IP address
// This implementation uses freeipapi.com which is free to use
func GetGeoDataFromIP(ip string) (GeoData, error) {
	// First check the cache
	ipCache.mutex.RLock()
	cachedData, found := ipCache.cache[ip]
	ipCache.mutex.RUnlock()

	// If found in cache and not expired, use the cached data
	if found && time.Since(cachedData.Timestamp) < ipCache.ttl {
		return cachedData, nil
	}

	// Otherwise, call the API
	url := fmt.Sprintf("https://freeipapi.com/api/json/%s", ip)
	client := http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return GeoData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GeoData{}, fmt.Errorf("API returned status code %d", resp.StatusCode)
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

	// Format the location string
	if geoData.City != "" && geoData.Region != "" && geoData.Country != "" {
		geoData.FormattedLoc = fmt.Sprintf("%s, %s, %s", geoData.City, geoData.Region, geoData.Country)
	} else if geoData.City != "" && geoData.Country != "" {
		geoData.FormattedLoc = fmt.Sprintf("%s, %s", geoData.City, geoData.Country)
	} else if geoData.Country != "" {
		geoData.FormattedLoc = geoData.Country
	} else {
		geoData.FormattedLoc = "Unknown"
	}

	// Update the cache
	ipCache.mutex.Lock()
	ipCache.cache[ip] = geoData
	ipCache.mutex.Unlock()

	return geoData, nil
}
