package session

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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

// trustedProxies holds the parsed form of config.ServerConfig.
// TrustedProxies — see SetTrustedProxies. An atomic pointer since
// GetClientIP reads it on every request while SetTrustedProxies only
// writes it at startup and on a config reload; nil (the zero value) means
// "nothing configured yet," treated identically to an empty list.
var trustedProxies atomic.Pointer[[]*net.IPNet]

// SetTrustedProxies sets which peers GetClientIP trusts to supply a
// request's real client IP via the X-Forwarded-For/X-Real-IP/X-Client-IP
// headers, from config.ServerConfig.TrustedProxies (CIDR ranges, or bare
// IPs treated as a /32 or /128) — see that field's doc comment for why
// this defaults to empty (nothing trusted, headers never honored) rather
// than trusting those headers unconditionally the way this function used
// to. Called once at gateway startup (main.go) and again on every config
// reload (gateway/reload.go), mirroring SetGeolocationConfig.
//
// Entries were already validated as parseable IP/CIDR syntax by
// config.LoadConfig, so a parse failure here should only ever happen for
// a config assembled by hand (e.g. a test) rather than loaded from a real
// file — logged and that one entry dropped rather than treated as fatal,
// since this only ever narrows what's trusted, never widens it.
func SetTrustedProxies(cidrs []string) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := parseIPOrCIDR(cidr)
		if err != nil {
			log.Printf("session: ignoring invalid trusted proxy entry %q: %v", cidr, err)
			continue
		}
		nets = append(nets, ipNet)
	}
	trustedProxies.Store(&nets)
}

// parseIPOrCIDR parses s as a CIDR range, or as a bare IP address widened
// to a single-address CIDR (a /32 for IPv4, a /128 for IPv6) — the two
// forms config.ServerConfig.TrustedProxies accepts.
func parseIPOrCIDR(s string) (net.IP, *net.IPNet, error) {
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, nil, fmt.Errorf("not a valid IP address or CIDR range")
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		return ip, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
	}
	return net.ParseCIDR(s)
}

// isTrustedProxy reports whether remoteIP (with any port already
// stripped) is in the trusted-proxy list SetTrustedProxies configured.
func isTrustedProxy(remoteIP string) bool {
	nets := trustedProxies.Load()
	if nets == nil || len(*nets) == 0 {
		return false
	}
	ip := net.ParseIP(remoteIP)
	if ip == nil {
		return false
	}
	for _, n := range *nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// GetClientIP extracts the request's real client IP: the TCP connection's
// own peer address (r.RemoteAddr) unless that peer is a configured
// trusted proxy (see SetTrustedProxies), in which case the client-supplied
// X-Forwarded-For/X-Real-IP/X-Client-IP headers are honored instead —
// mirroring how nginx's set_real_ip_from or Traefik's trustedIPs work.
//
// This function used to trust those headers from every request
// unconditionally, regardless of who actually sent it — which meant any
// direct client, not just a real proxy in front of this gateway, could
// set its own "IP" to anything at all, including something that isn't an
// IP address. That's not a theoretical concern: doc/TODO.md's "Fix GEO
// IP" section shows a JNDI-exploit probe's own crafted string ending up
// logged as the "client IP" and passed to the geo-lookup API — not
// because of anything in the request's URL, but because the attacker's
// own X-Forwarded-For header was trusted at face value. Beyond the log
// noise, unconditionally trusting these headers also makes IP-based rate
// limiting (middleware/ratelimiter.go) and analytics trivially spoofable
// by any client that isn't actually behind a real proxy.
func GetClientIP(r *http.Request) string {
	remoteIP := stripPort(r.RemoteAddr)

	if isTrustedProxy(remoteIP) {
		if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
			// X-Forwarded-For can contain multiple IPs, take the first one
			if ips := strings.Split(xForwardedFor, ","); len(ips) > 0 {
				return stripPort(strings.TrimSpace(ips[0]))
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
