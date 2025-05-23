package handlers

import (
	"context"
	"time"

	"github.com/jmaister/taronja-gateway/api"
)

// HealthCheck implements the HealthCheck operation for the api.StrictServerInterface.
func (s *StrictApiServer) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	response := api.HealthCheck200JSONResponse{
		Status:    "OK",
		Timestamp: time.Now(),
	}
	return response, nil
}
