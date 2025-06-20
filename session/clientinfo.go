package session

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/ua-parser/uap-go/uaparser"
)

// GetClientIP extracts the real client IP address from the request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (from load balancers/proxies)
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header (from reverse proxies)
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return xRealIP
	}

	// Check X-Client-IP header
	xClientIP := r.Header.Get("X-Client-IP")
	if xClientIP != "" {
		return xClientIP
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr

	// Remove port if present
	if lastColon := strings.LastIndex(ip, ":"); lastColon != -1 {
		ip = ip[:lastColon]
	}

	return ip
}

// NewClientInfo creates a ClientInfo instance from an HTTP request and geolocation data
func NewClientInfo(req *http.Request) *db.ClientInfo {
	parser := uaparser.NewFromSaved()
	client := parser.Parse(req.UserAgent())
	ipAddress := GetClientIP(req)

	geoData := GeoData{}
	if ipAddress != "" {
		g, err := GetGeoDataFromIP(ipAddress)
		if err != nil {
			// Log the error but continue with empty geo data
			log.Printf("Error getting geo data for IP %s: %v", ipAddress, err)
		} else {
			geoData = g
		}
	}

	return &db.ClientInfo{
		IPAddress:      ipAddress,
		UserAgent:      req.UserAgent(),
		Referrer:       req.Referer(),
		BrowserFamily:  client.UserAgent.Family,
		BrowserVersion: client.UserAgent.ToVersionString(),
		OSFamily:       client.Os.Family,
		OSVersion:      client.Os.ToVersionString(),
		DeviceFamily:   client.Device.Family,
		DeviceBrand:    client.Device.Brand,
		DeviceModel:    client.Device.Model,
		// Geographic fields populated from geoData
		GeoLocation: geoData.FormattedLoc,
		Latitude:    geoData.Latitude,
		Longitude:   geoData.Longitude,
		City:        geoData.City,
		ZipCode:     geoData.ZipCode,
		Country:     geoData.Country,
		CountryCode: geoData.CountryCode,
		Region:      geoData.Region,
		Continent:   geoData.Continent,
	}
}

// Create a new TrafficMetric instance with the Request object
func NewTrafficMetric(req *http.Request) *db.TrafficMetric {

	return &db.TrafficMetric{
		HttpMethod:     req.Method,
		Path:           req.URL.Path,
		HttpStatus:     0,
		ResponseTimeMs: 0,
		Timestamp:      time.Now(),
		ResponseSize:   0,
		Error:          "",
		UserID:         "",
		SessionID:      "",
		ClientInfo:     *NewClientInfo(req),
	}
}
