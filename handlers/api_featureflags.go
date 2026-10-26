package handlers

import (
	"context"
	"errors"
	"log"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/featureflag"
)

func toAPIFeatureFlag(flag *db.FeatureFlag) api.FeatureFlag {
	return api.FeatureFlag{
		Key:         flag.Key,
		Description: flag.Description,
		Type:        api.FeatureFlagType(flag.Type),
		Enabled:     flag.Enabled,
		CreatedAt:   flag.CreatedAt,
		UpdatedAt:   flag.UpdatedAt,
	}
}

// GetFeatureFlagValues handles GET /flags. Public: it is listed in
// builtin.OperationWithNoSecurity.
func (s *StrictApiServer) GetFeatureFlagValues(ctx context.Context, request api.GetFeatureFlagValuesRequestObject) (api.GetFeatureFlagValuesResponseObject, error) {
	values, err := s.featureFlags.Values()
	if err != nil {
		log.Printf("Error reading feature flag values: %v", err)
		return api.GetFeatureFlagValues500JSONResponse{Code: 500, Message: "Failed to read feature flags"}, nil
	}
	return api.GetFeatureFlagValues200JSONResponse{Flags: values}, nil
}

// ListFeatureFlags handles GET /api/feature-flags
func (s *StrictApiServer) ListFeatureFlags(ctx context.Context, request api.ListFeatureFlagsRequestObject) (api.ListFeatureFlagsResponseObject, error) {
	_, access := requireAdminAccess(ctx)
	switch access {
	case accessUnauthenticated:
		return api.ListFeatureFlags401JSONResponse{Code: 401, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListFeatureFlags403JSONResponse{Code: 403, Message: "Admin access required"}, nil
	}

	flags, err := s.featureFlags.List()
	if err != nil {
		log.Printf("Error listing feature flags: %v", err)
		return api.ListFeatureFlags500JSONResponse{Code: 500, Message: "Failed to list feature flags"}, nil
	}
	resp := make(api.ListFeatureFlags200JSONResponse, 0, len(flags))
	for i := range flags {
		resp = append(resp, toAPIFeatureFlag(&flags[i]))
	}
	return resp, nil
}

// CreateFeatureFlag handles POST /api/feature-flags
func (s *StrictApiServer) CreateFeatureFlag(ctx context.Context, request api.CreateFeatureFlagRequestObject) (api.CreateFeatureFlagResponseObject, error) {
	_, access := requireAdminAccess(ctx)
	switch access {
	case accessUnauthenticated:
		return api.CreateFeatureFlag401JSONResponse{Code: 401, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.CreateFeatureFlag403JSONResponse{Code: 403, Message: "Admin access required"}, nil
	}
	if request.Body == nil {
		return api.CreateFeatureFlag400JSONResponse{Code: 400, Message: "Request body is required"}, nil
	}

	in := featureflag.CreateInput{Key: request.Body.Key}
	if request.Body.Description != nil {
		in.Description = *request.Body.Description
	}
	if request.Body.Type != nil {
		in.Type = string(*request.Body.Type)
	}
	if request.Body.Enabled != nil {
		in.Enabled = *request.Body.Enabled
	}

	flag, err := s.featureFlags.Create(in)
	switch {
	case errors.Is(err, featureflag.ErrInvalidKey), errors.Is(err, featureflag.ErrInvalidType):
		return api.CreateFeatureFlag400JSONResponse{Code: 400, Message: err.Error()}, nil
	case errors.Is(err, featureflag.ErrAlreadyExists):
		return api.CreateFeatureFlag409JSONResponse{Code: 409, Message: err.Error()}, nil
	case err != nil:
		log.Printf("Error creating feature flag %q: %v", in.Key, err)
		return api.CreateFeatureFlag500JSONResponse{Code: 500, Message: "Failed to create feature flag"}, nil
	}
	return api.CreateFeatureFlag201JSONResponse(toAPIFeatureFlag(flag)), nil
}

// GetFeatureFlag handles GET /api/feature-flags/{key}
func (s *StrictApiServer) GetFeatureFlag(ctx context.Context, request api.GetFeatureFlagRequestObject) (api.GetFeatureFlagResponseObject, error) {
	_, access := requireAdminAccess(ctx)
	switch access {
	case accessUnauthenticated:
		return api.GetFeatureFlag401JSONResponse{Code: 401, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.GetFeatureFlag403JSONResponse{Code: 403, Message: "Admin access required"}, nil
	}

	flag, err := s.featureFlags.Get(request.Key)
	switch {
	case errors.Is(err, featureflag.ErrNotFound):
		return api.GetFeatureFlag404JSONResponse{Code: 404, Message: err.Error()}, nil
	case err != nil:
		log.Printf("Error reading feature flag %q: %v", request.Key, err)
		return api.GetFeatureFlag500JSONResponse{Code: 500, Message: "Failed to read feature flag"}, nil
	}
	return api.GetFeatureFlag200JSONResponse(toAPIFeatureFlag(flag)), nil
}

// UpdateFeatureFlag handles PATCH /api/feature-flags/{key}
func (s *StrictApiServer) UpdateFeatureFlag(ctx context.Context, request api.UpdateFeatureFlagRequestObject) (api.UpdateFeatureFlagResponseObject, error) {
	_, access := requireAdminAccess(ctx)
	switch access {
	case accessUnauthenticated:
		return api.UpdateFeatureFlag401JSONResponse{Code: 401, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.UpdateFeatureFlag403JSONResponse{Code: 403, Message: "Admin access required"}, nil
	}

	var in featureflag.UpdateInput
	if request.Body != nil {
		in.Description = request.Body.Description
		in.Enabled = request.Body.Enabled
	}

	flag, err := s.featureFlags.Update(request.Key, in)
	switch {
	case errors.Is(err, featureflag.ErrNotFound):
		return api.UpdateFeatureFlag404JSONResponse{Code: 404, Message: err.Error()}, nil
	case err != nil:
		log.Printf("Error updating feature flag %q: %v", request.Key, err)
		return api.UpdateFeatureFlag500JSONResponse{Code: 500, Message: "Failed to update feature flag"}, nil
	}
	return api.UpdateFeatureFlag200JSONResponse(toAPIFeatureFlag(flag)), nil
}

// DeleteFeatureFlag handles DELETE /api/feature-flags/{key}
func (s *StrictApiServer) DeleteFeatureFlag(ctx context.Context, request api.DeleteFeatureFlagRequestObject) (api.DeleteFeatureFlagResponseObject, error) {
	_, access := requireAdminAccess(ctx)
	switch access {
	case accessUnauthenticated:
		return api.DeleteFeatureFlag401JSONResponse{Code: 401, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.DeleteFeatureFlag403JSONResponse{Code: 403, Message: "Admin access required"}, nil
	}

	err := s.featureFlags.Delete(request.Key)
	switch {
	case errors.Is(err, featureflag.ErrNotFound):
		return api.DeleteFeatureFlag404JSONResponse{Code: 404, Message: err.Error()}, nil
	case err != nil:
		log.Printf("Error deleting feature flag %q: %v", request.Key, err)
		return api.DeleteFeatureFlag500JSONResponse{Code: 500, Message: "Failed to delete feature flag"}, nil
	}
	return api.DeleteFeatureFlag204Response{}, nil
}
