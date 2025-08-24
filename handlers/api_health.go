package handlers

import (
	"context"
	"time"

	"github.com/jmaister/taronja-gateway/api"
)

// HealthCheck implements the HealthCheck operation for the api.StrictServerInterface.
func (s *StrictApiServer) HealthCheck(ctx context.Context, request api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	uptime := time.Since(s.startTime)

	// Check database health
	dbStatus := "ok"
	var openConnections *int

	if s.dbConnection != nil {
		sqlDB, err := s.dbConnection.DB()
		if err != nil {
			dbStatus = "error"
		} else {
			// Check if we can ping the database
			if err := sqlDB.Ping(); err != nil {
				dbStatus = "error"
			} else {
				// Get database stats
				stats := sqlDB.Stats()
				openConnections = &stats.OpenConnections
			}
		}
	} else {
		// For tests or when no database connection is available
		dbStatus = "not_available"
	}

	response := api.HealthCheck200JSONResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Uptime:    uptime.String(),
		Database: struct {
			OpenConnections *int   `json:"open_connections,omitempty"`
			Status          string `json:"status"`
		}{
			Status:          dbStatus,
			OpenConnections: openConnections,
		},
	}

	return response, nil
}
