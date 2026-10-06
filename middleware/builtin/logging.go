package builtin

import (
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/middleware"
)

// LoggingMiddleware logs information about each request
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response wrapper to capture the status code
		rw := NewResponseWriter(w)

		// Call the next handler
		next.ServeHTTP(rw, r)

		// Log the request details after it's handled
		duration := time.Since(start)

		// Using a standard log format: timestamp client_ip method path status response_time_ms
		timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
		responseTimeMs := float64(duration.Nanoseconds()) / 1000000.0

		log.Printf("%s - %s \"%s %s\" %d %.2fms",
			timestamp,
			r.RemoteAddr,
			r.Method,
			r.URL.Path,
			rw.Status(),
			responseTimeMs,
		)
	})
}

// responseWriter is a wrapper for http.ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// NewResponseWriter creates a new responseWriter
func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Status returns the status code
func (rw *responseWriter) Status() int {
	return rw.statusCode
}

// NameLogging logs every request. It takes no options.
const NameLogging = "logging"

type loggingFactory struct {
	middleware.ConcreteFactory
	middleware.NoConfig
}

func newLoggingFactory() *loggingFactory {
	return &loggingFactory{ConcreteFactory: middleware.NewConcreteFactory(NameLogging, "Logs each request/response (method, path, status, duration)")}
}

func (f *loggingFactory) Create(interface{}) (middleware.Middleware, error) {
	return LoggingMiddleware, nil
}
