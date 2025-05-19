package handlers

import (
	"context" // Added for StrictServerInterface
	"time"    // Added for HealthResponse

	"github.com/jmaister/taronja-gateway/api" // Ensure this path is correct
)

// --- Strict API Server Implementation ---

// StrictApiServer provides an implementation of the api.StrictServerInterface.
type StrictApiServer struct {
	// You can add dependencies here if needed, similar to ApiServer
}

// NewStrictApiServer creates a new StrictApiServer.
func NewStrictApiServer() *StrictApiServer {
	return &StrictApiServer{}
}

// HealthCheck implements the HealthCheck operation for the api.StrictServerInterface.
func (s *StrictApiServer) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	response := api.HealthCheck200JSONResponse{
		Status:    "OK",
		Timestamp: time.Now(),
	}
	return response, nil
}

// Ensure StrictApiServer implements StrictServerInterface
var _ api.StrictServerInterface = (*StrictApiServer)(nil)
