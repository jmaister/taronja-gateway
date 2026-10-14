package builtin

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/middleware"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// responseWriterWithStats wraps http.ResponseWriter to capture response details
type responseWriterWithStats struct {
	http.ResponseWriter
	statusCode   int
	responseSize int64
	body         *bytes.Buffer
}

// NewResponseWriterWithStats creates a new responseWriterWithStats
func NewResponseWriterWithStats(w http.ResponseWriter) *responseWriterWithStats {
	return &responseWriterWithStats{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status code
		body:           &bytes.Buffer{},
	}
}

// WriteHeader captures the status code
func (rw *responseWriterWithStats) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response body and size
func (rw *responseWriterWithStats) Write(data []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(data)
	if err == nil {
		rw.responseSize += int64(n)
		// Optionally capture response body for error analysis
		if rw.statusCode >= 400 {
			rw.body.Write(data)
		}
	}
	return n, err
}

// Status returns the captured status code
func (rw *responseWriterWithStats) Status() int {
	return rw.statusCode
}

// Size returns the captured response size
func (rw *responseWriterWithStats) Size() int64 {
	return rw.responseSize
}

// Body returns the captured response body (only for error responses)
func (rw *responseWriterWithStats) Body() string {
	return rw.body.String()
}

// TrafficMetricMiddleware collects request statistics. With excludeStaticAssets,
// static-asset paths (session.IsStaticAssetPath) skip it entirely, avoiding most of
// its per-request cost (see doc/PERFORMANCE_ANALYSIS.md) for the least useful requests.
func TrafficMetricMiddleware(statsRepo repositories.TrafficMetricRepository, excludeStaticAssets bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if excludeStaticAssets && session.IsStaticAssetPath(req.URL.Path) {
				next.ServeHTTP(w, req)
				return
			}

			startTime := time.Now()

			// Wrap the response writer to capture statistics
			resp := NewResponseWriterWithStats(w)

			// Call the next handler
			next.ServeHTTP(resp, req)

			// Calculate response time
			responseTime := time.Since(startTime).Nanoseconds()

			// Extract session information if available
			var userID, sessionID string
			if sessionData, exists := req.Context().Value(session.SessionKey).(*db.Session); exists && sessionData != nil {
				userID = sessionData.UserID
				sessionID = sessionData.Token
			}

			// Capture error message for failed requests
			var errorMsg string
			if resp.Status() >= 400 {
				errorMsg = resp.Body()
				// Limit error message length
				if len(errorMsg) > 1000 {
					errorMsg = errorMsg[:1000] + "..."
				}
			}

			// Build the record in the background, not just the write: session.NewTrafficMetric
			// does a geo-IP lookup, a blocking HTTP call on a cache miss. Nothing reads
			// req.Body, so req is safe to use after the handler returns.
			go func() {
				stat := session.NewTrafficMetric(req)
				stat.Timestamp = startTime
				stat.HttpStatus = resp.Status()
				stat.ResponseTimeNs = responseTime
				stat.ResponseSize = resp.Size()
				stat.Error = errorMsg
				stat.UserID = userID
				stat.SessionID = sessionID

				if err := statsRepo.Create(stat); err != nil {
					log.Printf("Failed to store request statistic: %v", err)
				}
			}()
		})
	}
}

// NameTrafficMetrics records every request for the statistics dashboard.
const NameTrafficMetrics = "traffic_metrics"

// TrafficMetricsConfig holds the traffic_metrics middleware options.
type TrafficMetricsConfig struct {
	// ExcludeStaticAssets skips requests for static assets (see
	// session.IsStaticAssetPath). Default: false.
	ExcludeStaticAssets bool `yaml:"excludeStaticAssets"`
}

// Both dependencies are real reads: the session from the request context and,
// through session.NewClientInfo, the JA4H fingerprint header.
var trafficMetricsDefinition = Definition{
	Name:        NameTrafficMetrics,
	Description: "Records per-request traffic metrics (status, size, duration, user)",
	DependsOn:   []string{NameSessionExtraction, NameJA4Fingerprint},
	Decode:      decodeOptions(TrafficMetricsConfig{}),
	New: func(opts any, d GlobalDeps) (middleware.Middleware, error) {
		if d.TrafficMetricRepo == nil {
			return nil, fmt.Errorf("needs a traffic metric repository")
		}
		return TrafficMetricMiddleware(d.TrafficMetricRepo, opts.(TrafficMetricsConfig).ExcludeStaticAssets), nil
	},
}
