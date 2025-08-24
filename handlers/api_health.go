package handlers

import (
	"context"
	"time"

	"github.com/jmaister/taronja-gateway/api"
)

// HealthCheck implements the HealthCheck operation for the api.StrictServerInterface.
func (s *StrictApiServer) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	uptime := time.Since(s.startTime)

	response := api.HealthCheck200JSONResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Uptime:    uptime.String(),
	}

	return response, nil
}
