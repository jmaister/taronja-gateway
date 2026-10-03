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

// uaParser is built once: uaparser.New parses the whole uap-core regex database
// (~100ms, tens of MB), and the parser is safe for concurrent use.
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

// IsTrustedProxy reports whether remoteIP is a loopback or private-range peer,
// i.e. one whose forwarded-for headers GetClientIP and the reverse proxy may
// believe. Public peers cannot present a private address as their TCP source.
func IsTrustedProxy(remoteIP string) bool {
	ip := net.ParseIP(remoteIP)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// GetClientIP returns the request's client IP: the TCP peer, unless that peer
// is a trusted proxy (see IsTrustedProxy), in which case the forwarded headers
// are honored. Trusting them from every peer made IP rate limiting and
// analytics trivially spoofable.
func GetClientIP(r *http.Request) string {
	remoteIP := stripPort(r.RemoteAddr)

	if IsTrustedProxy(remoteIP) {
		if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
			if ip := rightmostUntrustedForwardedFor(xForwardedFor); ip != "" {
				return ip
			}
		}
		if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
			return stripPort(xRealIP)
		}
		if xClientIP := r.Header.Get("X-Client-IP"); xClientIP != "" {
			return stripPort(xClientIP)
		}
	}

	return remoteIP
}

// rightmostUntrustedForwardedFor returns the first non-trusted entry of an
// X-Forwarded-For list, scanning from the right: entries to its left were
// supplied by the client and prove nothing. Returns "" if every entry is
// trusted or the header has no usable entry.
func rightmostUntrustedForwardedFor(xForwardedFor string) string {
	entries := strings.Split(xForwardedFor, ",")
	for i := len(entries) - 1; i >= 0; i-- {
		candidate := stripPort(strings.TrimSpace(entries[i]))
		if candidate == "" {
			continue
		}
		if !IsTrustedProxy(candidate) {
			return candidate
		}
	}
	return ""
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
		GeoLocation:     geoData.FormattedLoc,
		Latitude:        geoData.Latitude,
		Longitude:       geoData.Longitude,
		City:            geoData.City,
		ZipCode:         geoData.ZipCode,
		Country:         geoData.Country,
		CountryCode:     geoData.CountryCode,
		Region:          geoData.Region,
		Continent:       geoData.Continent,
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
		// UTC: db/timeseries.go buckets the stored value as UTC wall-clock time.
		Timestamp:     time.Now().UTC(),
		ResponseSize:  0,
		Error:         "",
		UserID:        "",
		SessionID:     "",
		IsStaticAsset: IsStaticAssetPath(req.URL.Path),
		ClientInfo:    *NewClientInfo(req),
	}
}
