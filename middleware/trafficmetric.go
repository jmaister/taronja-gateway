package middleware

import (
	"bytes"
	"log"
	"net/http"
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

// TrafficMetricMiddleware creates middleware for collecting request
// statistics. When excludeStaticAssets is true, requests whose path looks
// like a static asset (session.IsStaticAssetPath — CSS, JS, images, fonts,
// ...) skip this middleware's work entirely: no response-writer wrapping, no
// TrafficMetric built, no async DB write. That's most of what this
// middleware costs per request (see PERFORMANCE_ANALYSIS.md's profile of
// BenchmarkStaticRequest), and static assets are the least useful requests
// to spend it on — they carry no user action worth attributing, unlike a
// page view or API call using the same session.
func TrafficMetricMiddleware(statsRepo db.TrafficMetricRepository, excludeStaticAssets bool) func(http.Handler) http.Handler {
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
func StatisticsMiddlewareFunc(statsRepo db.TrafficMetricRepository, excludeStaticAssets bool) func(http.Handler) http.Handler {
	return TrafficMetricMiddleware(statsRepo, excludeStaticAssets)
}
