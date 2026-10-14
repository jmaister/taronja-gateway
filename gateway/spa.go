package gateway

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jmaister/taronja-gateway/config"
)

// createSPAHandler wraps a file server handler with SPA (Single Page Application) routing logic.
// When a file is not found (404), it serves the index.html from the root of the static folder.
func (g *Gateway) createSPAHandler(handler http.Handler, fsPath string, routeConfig config.RouteConfig) http.Handler {
	return &spaHandler{
		handler:     handler,
		fsPath:      fsPath,
		routeConfig: routeConfig,
	}
}

// spaHandler implements http.Handler and provides SPA routing functionality
type spaHandler struct {
	handler     http.Handler
	fsPath      string
	routeConfig config.RouteConfig
}

func (s *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create a custom ResponseWriter to capture 404 errors
	recorder := &spaResponseRecorder{
		ResponseWriter: w,
		status:         200, // Default to 200
	}

	// Let the original handler process the request
	s.handler.ServeHTTP(recorder, r)

	// If we got a 404 and this is a SPA route, serve index.html instead
	if recorder.status == 404 && !recorder.responseWritten {
		indexPath := filepath.Join(s.fsPath, "index.html")

		// Check if index.html exists
		if _, err := os.Stat(indexPath); err == nil {
			log.Printf("Static Route [%s]: SPA fallback - File not found, serving index.html: %s", s.routeConfig.Name, indexPath)

			// Clear any headers and set appropriate content type
			for key := range w.Header() {
				w.Header().Del(key)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")

			// Serve the index.html file directly (this will handle status code)
			http.ServeFile(w, r, indexPath)
			return
		} else {
			log.Printf("Static Route [%s]: SPA fallback failed - index.html not found at: %s", s.routeConfig.Name, indexPath)
			// Write the 404 response if index.html doesn't exist
			w.WriteHeader(404)
			return
		}
	}

	// If not a 404 or response was already written, write the captured response
	if recorder.responseWritten {
		return // Response already sent
	}

	// Write the status and data if response wasn't written yet
	w.WriteHeader(recorder.status)
	if len(recorder.data) > 0 {
		w.Write(recorder.data)
	}
}

// spaResponseRecorder is a custom ResponseWriter that captures the status code
type spaResponseRecorder struct {
	http.ResponseWriter
	status          int
	wroteHeader     bool
	responseWritten bool
	data            []byte
}

func (r *spaResponseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
		// Don't write to the underlying response yet
	}
}

func (r *spaResponseRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(200)
	}
	// Capture the data instead of writing it immediately
	r.data = append(r.data, data...)
	return len(data), nil
}

// --- Utility Functions ---
