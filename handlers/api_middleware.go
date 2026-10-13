package handlers

import (
	"context"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/middleware"
)

// GetMiddlewareStatus implements GET /_/api/middleware — lists every global
// built-in middleware, with its status, dependencies, and health where a check
// is implemented.
func (s *StrictApiServer) GetMiddlewareStatus(ctx context.Context, req api.GetMiddlewareStatusRequestObject) (api.GetMiddlewareStatusResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetMiddlewareStatus401JSONResponse{}, nil
	}

	if s.middlewareChain == nil {
		return api.GetMiddlewareStatus200JSONResponse{}, nil
	}

	statuses := s.middlewareChain.Status()
	items := make([]api.MiddlewareStatusItem, 0, len(statuses))
	for _, st := range statuses {
		item := api.MiddlewareStatusItem{
			Name:         st.Name,
			Description:  st.Description,
			Status:       api.MiddlewareStatusItemStatus(st.Status),
			Enabled:      st.Enabled,
			Dependencies: st.Dependencies,
		}
		if st.Health != nil {
			item.Health = &api.MiddlewareHealth{
				Status:  api.MiddlewareHealthStatus(st.Health.Status),
				Message: stringPtrOrNil(st.Health.Message),
			}
		}
		items = append(items, item)
	}

	return api.GetMiddlewareStatus200JSONResponse(items), nil
}

// GetMiddlewareMetrics implements GET /_/api/middleware/{name}/metrics — the
// in-memory request metrics recorded for a single global middleware since the
// process started.
func (s *StrictApiServer) GetMiddlewareMetrics(ctx context.Context, req api.GetMiddlewareMetricsRequestObject) (api.GetMiddlewareMetricsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetMiddlewareMetrics401JSONResponse{}, nil
	}

	if s.middlewareChain == nil {
		return api.GetMiddlewareMetrics404JSONResponse{
			Code:    404,
			Message: "middleware chain is not available",
		}, nil
	}

	snap, ok := s.middlewareChain.Metrics(req.Name)
	if !ok {
		return api.GetMiddlewareMetrics404JSONResponse{
			Code:    404,
			Message: "unknown middleware: " + req.Name,
		}, nil
	}

	return api.GetMiddlewareMetrics200JSONResponse(toAPIMetrics(snap)), nil
}

// GetAllMiddlewareMetrics implements GET /_/api/middleware/metrics — the
// in-memory request metrics for every global middleware that has been built
// into the running chain, in one call, so a dashboard doesn't need one request
// per middleware from the status list.
func (s *StrictApiServer) GetAllMiddlewareMetrics(ctx context.Context, req api.GetAllMiddlewareMetricsRequestObject) (api.GetAllMiddlewareMetricsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.GetAllMiddlewareMetrics401JSONResponse{}, nil
	}

	if s.middlewareChain == nil {
		return api.GetAllMiddlewareMetrics200JSONResponse{}, nil
	}

	snapshots := s.middlewareChain.AllMetrics()
	items := make([]api.MiddlewareMetrics, 0, len(snapshots))
	for _, snap := range snapshots {
		items = append(items, toAPIMetrics(snap))
	}

	return api.GetAllMiddlewareMetrics200JSONResponse(items), nil
}

func toAPIMetrics(snap middleware.MiddlewareMetricsSnapshot) api.MiddlewareMetrics {
	return api.MiddlewareMetrics{
		Name:              snap.Name,
		RequestCount:      snap.RequestCount,
		ErrorCount:        snap.ErrorCount,
		AverageDurationMs: snap.AverageDurationMs,
		LastInvokedAt:     snap.LastInvokedAt,
	}
}

// stringPtrOrNil returns nil for an empty string, otherwise a pointer to s —
// matches the `message,omitempty` semantics of the generated MiddlewareHealth type.
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
