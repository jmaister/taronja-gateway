package middleware

import (
	"bytes"
	"log"
	"net/http"
	"regexp"
	"time"

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

// TrafficMetricMiddleware creates middleware for collecting request statistics
func TrafficMetricMiddleware(statsRepo db.TrafficMetricRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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

			// Create the statistic record
			stat := session.NewTrafficMetric(req)
			// Setting the rest of the fields, values not coming from req *http.Request
			stat.Timestamp = startTime
			stat.HttpStatus = resp.Status()
			stat.ResponseTimeNs = responseTime
			stat.ResponseSize = resp.Size()
			stat.Error = errorMsg
			stat.UserID = userID
			stat.SessionID = sessionID

			// Store the statistic (async to avoid blocking the response)
			go func() {
				if err := statsRepo.Create(stat); err != nil {
					log.Printf("Failed to store request statistic: %v", err)
				}
			}()
		})
	}
}

// StatisticsMiddlewareFunc creates an api.MiddlewareFunc for OpenAPI generated handlers
func StatisticsMiddlewareFunc(statsRepo db.TrafficMetricRepository) func(http.Handler) http.Handler {
	return TrafficMetricMiddleware(statsRepo)
}

// Helper function to check if a path should be excluded from statistics
func shouldExcludeFromStats(path string) bool {
	// Define patterns to exclude (health checks, static assets, etc.)
	excludePatterns := []string{
		`^/health$`,
		`^/favicon\.ico$`,
		`^/robots\.txt$`,
		`^/sitemap\.xml$`,
		`^/_/static/.*`, // Assuming static files are under /_/static/
	}

	for _, pattern := range excludePatterns {
		matched, err := regexp.MatchString(pattern, path)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// ConditionalStatisticsMiddleware wraps StatisticsMiddleware with path exclusion logic
func ConditionalStatisticsMiddleware(statsRepo db.TrafficMetricRepository) func(http.Handler) http.Handler {
	statsMiddleware := TrafficMetricMiddleware(statsRepo)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip statistics collection for certain paths
			if shouldExcludeFromStats(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Apply statistics middleware
			statsMiddleware(next).ServeHTTP(w, r)
		})
	}
}
