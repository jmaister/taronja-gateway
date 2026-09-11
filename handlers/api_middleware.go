package handlers

import (
	"context"
	"sort"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// GetMiddlewareStatus implements GET /_/api/middleware — lists every global
// middleware known to the running registry (see middleware.MiddlewareRegistryV2,
// doc/refactor01.md Phase 3), with its status, dependencies, and health where
// a check is implemented.
func (s *StrictApiServer) GetMiddlewareStatus(ctx context.Context, req api.GetMiddlewareStatusRequestObject) (api.GetMiddlewareStatusResponseObject, error) {
	// admin check, matching the rate limiter stats/config endpoints
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sess == nil || !sess.IsAuthenticated || !sess.IsAdmin {
		return api.GetMiddlewareStatus401JSONResponse{}, nil
	}

	if s.middlewareRegistry == nil {
		return api.GetMiddlewareStatus200JSONResponse{}, nil
	}

	statusByName := s.middlewareRegistry.GetStatus()
	names := make([]string, 0, len(statusByName))
	for name := range statusByName {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]api.MiddlewareStatusItem, 0, len(names))
	for _, name := range names {
		st := statusByName[name]
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
// process started (see middleware.MiddlewareRegistryV2.GetMetrics).
func (s *StrictApiServer) GetMiddlewareMetrics(ctx context.Context, req api.GetMiddlewareMetricsRequestObject) (api.GetMiddlewareMetricsResponseObject, error) {
	// admin check, matching the rate limiter stats/config endpoints
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sess == nil || !sess.IsAuthenticated || !sess.IsAdmin {
		return api.GetMiddlewareMetrics401JSONResponse{}, nil
	}

	if s.middlewareRegistry == nil {
		return api.GetMiddlewareMetrics404JSONResponse{
			Code:    404,
			Message: "middleware registry is not available",
		}, nil
	}

	snap, err := s.middlewareRegistry.GetMetrics(req.Name)
	if err != nil {
		return api.GetMiddlewareMetrics404JSONResponse{
			Code:    404,
			Message: "unknown middleware: " + req.Name,
		}, nil
	}

	return api.GetMiddlewareMetrics200JSONResponse{
		Name:              snap.Name,
		RequestCount:      snap.RequestCount,
		ErrorCount:        snap.ErrorCount,
		AverageDurationMs: snap.AverageDurationMs,
		LastInvokedAt:     snap.LastInvokedAt,
	}, nil
}

// GetAllMiddlewareMetrics implements GET /_/api/middleware/metrics — the
// in-memory request metrics for every global middleware that has been built
// into the running chain, in one call (see middleware.MiddlewareRegistryV2.GetAllMetrics).
// Added in Phase 5 (doc/refactor01.md) so a dashboard doesn't need one
// request per middleware from the status list.
func (s *StrictApiServer) GetAllMiddlewareMetrics(ctx context.Context, req api.GetAllMiddlewareMetricsRequestObject) (api.GetAllMiddlewareMetricsResponseObject, error) {
	// admin check, matching the rate limiter stats/config endpoints
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sess == nil || !sess.IsAuthenticated || !sess.IsAdmin {
		return api.GetAllMiddlewareMetrics401JSONResponse{}, nil
	}

	if s.middlewareRegistry == nil {
		return api.GetAllMiddlewareMetrics200JSONResponse{}, nil
	}

	snapshots := s.middlewareRegistry.GetAllMetrics()
	names := make([]string, 0, len(snapshots))
	for name := range snapshots {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]api.MiddlewareMetrics, 0, len(names))
	for _, name := range names {
		snap := snapshots[name]
		items = append(items, api.MiddlewareMetrics{
			Name:              snap.Name,
			RequestCount:      snap.RequestCount,
			ErrorCount:        snap.ErrorCount,
			AverageDurationMs: snap.AverageDurationMs,
			LastInvokedAt:     snap.LastInvokedAt,
		})
	}

	return api.GetAllMiddlewareMetrics200JSONResponse(items), nil
}

// stringPtrOrNil returns nil for an empty string, otherwise a pointer to s —
// matches the `message,omitempty` semantics of the generated MiddlewareHealth type.
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
