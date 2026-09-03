package session

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/ua-parser/uap-go/uaparser"
)

// uaParser is a package-level singleton. uaparser.New (and the deprecated
// NewFromSaved it replaces) parses the entire embedded uap-core regex
// database — several hundred patterns — from scratch on every call, which
// costs on the order of 100ms+ and tens of MB of garbage per call. Building
// it once and reusing it is essential: NewClientInfo runs on every request
// that goes through the traffic-metrics/session-extraction middleware, so
// constructing a parser per call (as this code used to do) turned every
// analytics-tracked request into a ~200ms, ~45MB operation. *uaparser.Parser
// itself holds an internal mutex-guarded LRU cache of parsed results and is
// documented as safe for concurrent use, so a single shared instance is the
// intended usage.
var (
	uaParserOnce sync.Once
	uaParser     *uaparser.Parser
)

func getUAParser() *uaparser.Parser {
	uaParserOnce.Do(func() {
		uaParser = uaparser.NewFromSaved()
	})
	return uaParser
}

// stripPort removes the port from an IP address if present
func stripPort(ip string) string {
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		// If SplitHostPort fails, it might be just an IP without port
		return ip
	}
	return host
}

// GetClientIP extracts the real client IP address from the request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (from load balancers/proxies)
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return stripPort(strings.TrimSpace(ips[0]))
		}
	}

	// Check X-Real-IP header (from reverse proxies)
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return stripPort(xRealIP)
	}

	// Check X-Client-IP header
	xClientIP := r.Header.Get("X-Client-IP")
	if xClientIP != "" {
		return stripPort(xClientIP)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr

	// Remove port if present using net.SplitHostPort
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		// If SplitHostPort fails, it might be just an IP without port
		return ip
	}

	return host
}

// NewClientInfo creates a ClientInfo instance from an HTTP request and geolocation data
func NewClientInfo(req *http.Request) *db.ClientInfo {
	client := getUAParser().Parse(req.UserAgent())
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

	// Pick the single best available client fingerprint — see
	// fingerprint.SelectFingerprint's doc comment for the priority order
	// (TLS JA4 > stable > JA4H) and why only one is ever stored.
	fingerprintValue, fingerprintType := fingerprint.SelectFingerprint(req)

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
		// Client fingerprinting — see fingerprint.SelectFingerprint.
		Fingerprint:     fingerprintValue,
		FingerprintType: fingerprintType,
	}
}

// Create a new TrafficMetric instance with the Request object
func NewTrafficMetric(req *http.Request) *db.TrafficMetric {

	return &db.TrafficMetric{
		HttpMethod:     req.Method,
		Path:           req.URL.Path,
		HttpStatus:     0,
		ResponseTimeNs: 0,
		// UTC explicitly, not just relying on TrafficMetric.BeforeCreate's
		// defensive normalization: db/timeseries.go's SQL-side bucketing
		// interprets the stored value as UTC wall-clock time directly, so
		// this is the one call site that actually needs to get it right,
		// not just eventually-corrected before it hits the database.
		Timestamp:     time.Now().UTC(),
		ResponseSize:  0,
		Error:         "",
		UserID:        "",
		SessionID:     "",
		IsStaticAsset: IsStaticAssetPath(req.URL.Path),
		ClientInfo:    *NewClientInfo(req),
	}
}
