package db

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// ExtractClientInfo extracts client information from an HTTP request
// and adds it to a db.Session object. This remains a utility function.
func ExtractClientInfo(r *http.Request, obj *Session) {
	// Get IP address
	ipAddress := r.RemoteAddr
	// Check for forwarded IP if behind proxy
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ipAddress = forwardedFor
	}
	obj.IPAddress = ipAddress

	// Get User Agent information
	userAgent := r.UserAgent()
	obj.UserAgent = userAgent

	// Extract browser and OS information from user agent
	if userAgent != "" {
		// Simplified browser detection
		switch {
		case contains(userAgent, "Chrome") && !contains(userAgent, "Edg/"):
			obj.Browser = "Chrome"
		case contains(userAgent, "Firefox"):
			obj.Browser = "Firefox"
		case contains(userAgent, "Safari") && !contains(userAgent, "Chrome"):
			obj.Browser = "Safari"
		case contains(userAgent, "Edg/"):
			obj.Browser = "Edge"
		case contains(userAgent, "MSIE") || contains(userAgent, "Trident/"):
			obj.Browser = "Internet Explorer"
		default:
			obj.Browser = "Other"
		}

		// Simplified OS detection
		switch {
		case contains(userAgent, "Windows"):
			obj.OS = "Windows"
		case contains(userAgent, "Mac OS X"):
			obj.OS = "macOS"
		case contains(userAgent, "Linux"):
			obj.OS = "Linux"
		case contains(userAgent, "Android"):
			obj.OS = "Android"
		case contains(userAgent, "iOS") || contains(userAgent, "iPhone") || contains(userAgent, "iPad"):
			obj.OS = "iOS"
		default:
			obj.OS = "Other"
		}

		// Device type detection
		switch {
		case contains(userAgent, "Mobile"):
			obj.DeviceType = "Mobile"
		case contains(userAgent, "Tablet") || contains(userAgent, "iPad"):
			obj.DeviceType = "Tablet"
		default:
			obj.DeviceType = "Desktop"
		}
	}

	// Get referrer
	obj.Referrer = r.Referer()

	// Set last activity to current time
	obj.LastActivity = time.Now()

	// Default to Web interface
	obj.CreatedFrom = "Web"
	// Detect API requests based on Accept header
	if r.Header.Get("Accept") == "application/json" {
		obj.CreatedFrom = "API"
	}

	// Extract just the IP without port if it has one
	ip := ipAddress
	if idx := strings.Index(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}

	// Get detailed geo data from IP address and store in session object
	geoData, err := GetGeoDataFromIP(ip) // Assuming GetGeoDataFromIP is still relevant
	if err == nil {
		obj.Latitude = geoData.Latitude
		obj.Longitude = geoData.Longitude
		obj.City = geoData.City
		obj.Country = geoData.Country
		obj.CountryCode = geoData.CountryCode
		obj.Region = geoData.Region
		obj.Continent = geoData.Continent
		obj.ZipCode = geoData.ZipCode
		obj.GeoLocation = geoData.FormattedLoc
	} else {
		log.Printf("Failed to get geolocation for IP %s: %v", ip, err)
		obj.GeoLocation = "Unknown"
	}
}

// Helper function for string contains check.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
