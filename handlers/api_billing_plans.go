package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db/repositories"
)

// ListBillingPlans handles GET /api/billing/plans (admin): list plans.
func (s *StrictApiServer) ListBillingPlans(ctx context.Context, request api.ListBillingPlansRequestObject) (api.ListBillingPlansResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ListBillingPlans401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListBillingPlans403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.ListBillingPlans500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	activeOnly := request.Params.IncludeArchived == nil || !*request.Params.IncludeArchived

	plans, err := s.billing.ListPlans(activeOnly)
	if err != nil {
		log.Printf("Error listing billing plans: %v", err)
		return api.ListBillingPlans500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list plans"}, nil
	}

	resp := make(api.ListBillingPlans200JSONResponse, 0, len(plans))
	for _, plan := range plans {
		resp = append(resp, planToAPI(&plan, s.billing))
	}
	return resp, nil
}

// CreateBillingPlan handles POST /api/billing/plans (admin): create a plan.
func (s *StrictApiServer) CreateBillingPlan(ctx context.Context, request api.CreateBillingPlanRequestObject) (api.CreateBillingPlanResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.CreateBillingPlan401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.CreateBillingPlan403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.CreateBillingPlan503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}
	if request.Body == nil {
		return api.CreateBillingPlan400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}

	subject := ""
	if request.Body.Subject != nil {
		subject = string(*request.Body.Subject)
	}

	starts := ""
	if request.Body.Starts != nil {
		starts = string(*request.Body.Starts)
	}

	in := billing.PlanInput{
		Key:             request.Body.Key,
		Name:            request.Body.Name,
		Description:     stringVal(request.Body.Description),
		Kind:            string(request.Body.Kind),
		Interval:        stringVal(request.Body.Interval),
		AmountCents:     request.Body.AmountCents,
		Features:        request.Body.Features,
		Subject:         subject,
		TrialDays:       intVal(request.Body.TrialDays),
		GraceDays:       intVal(request.Body.GraceDays, billing.DefaultGraceDays),
		SetupFeeCents:   int64Val(request.Body.SetupFeeCents),
		SetupFeeName:    stringVal(request.Body.SetupFeeName),
		Starts:          starts,
		AccessDays:      intVal(request.Body.AccessDays),
		CollectShipping: boolVal(request.Body.CollectShipping),
		Group:           stringVal(request.Body.Group),
		Active:          true,
	}

	plan, err := s.billing.CreatePlan(ctx, in)
	if errors.Is(err, billing.ErrInvalid) {
		return api.CreateBillingPlan400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	}
	if errors.Is(err, billing.ErrConflict) {
		return api.CreateBillingPlan409JSONResponse{Code: http.StatusConflict, Message: "Plan already exists"}, nil
	}
	if errors.Is(err, billing.ErrNotConfigured) {
		return api.CreateBillingPlan503JSONResponse{Code: http.StatusServiceUnavailable, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error creating billing plan: %v", err)
		return api.CreateBillingPlan500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to create plan"}, nil
	}

	return api.CreateBillingPlan201JSONResponse(planToAPI(plan, s.billing)), nil
}

// GetBillingPlan handles GET /api/billing/plans/{key} (admin): get a plan.
func (s *StrictApiServer) GetBillingPlan(ctx context.Context, request api.GetBillingPlanRequestObject) (api.GetBillingPlanResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.GetBillingPlan401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.GetBillingPlan403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.GetBillingPlan500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	plan, err := s.billing.GetPlan(request.Key)
	if errors.Is(err, billing.ErrNotFound) {
		return api.GetBillingPlan404JSONResponse{Code: http.StatusNotFound, Message: "Plan not found"}, nil
	}
	if err != nil {
		log.Printf("Error getting billing plan %s: %v", request.Key, err)
		return api.GetBillingPlan500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get plan"}, nil
	}

	return api.GetBillingPlan200JSONResponse(planToAPI(plan, s.billing)), nil
}

// UpdateBillingPlan handles PATCH /api/billing/plans/{key} (admin): update a plan.
func (s *StrictApiServer) UpdateBillingPlan(ctx context.Context, request api.UpdateBillingPlanRequestObject) (api.UpdateBillingPlanResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.UpdateBillingPlan401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.UpdateBillingPlan403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.UpdateBillingPlan503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}
	if request.Body == nil {
		return api.UpdateBillingPlan400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}

	// Load the existing plan
	existing, err := s.billing.GetPlan(request.Key)
	if errors.Is(err, billing.ErrNotFound) {
		return api.UpdateBillingPlan404JSONResponse{Code: http.StatusNotFound, Message: "Plan not found"}, nil
	}
	if err != nil {
		log.Printf("Error getting billing plan %s: %v", request.Key, err)
		return api.UpdateBillingPlan500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to update plan"}, nil
	}

	// Convert to PlanInput and apply partial updates
	in := billing.PlanInputFrom(*existing)

	if request.Body.Name != nil {
		in.Name = *request.Body.Name
	}
	if request.Body.Description != nil {
		in.Description = *request.Body.Description
	}
	if request.Body.AmountCents != nil {
		in.AmountCents = *request.Body.AmountCents
	}
	if request.Body.Active != nil {
		in.Active = *request.Body.Active
	}
	if request.Body.Features != nil {
		in.Features = *request.Body.Features
	}
	if request.Body.TrialDays != nil {
		in.TrialDays = *request.Body.TrialDays
	}
	if request.Body.GraceDays != nil {
		in.GraceDays = *request.Body.GraceDays
	}
	if request.Body.SetupFeeCents != nil {
		in.SetupFeeCents = *request.Body.SetupFeeCents
	}
	if request.Body.SetupFeeName != nil {
		in.SetupFeeName = *request.Body.SetupFeeName
	}
	if request.Body.AccessDays != nil {
		in.AccessDays = *request.Body.AccessDays
	}
	if request.Body.CollectShipping != nil {
		in.CollectShipping = *request.Body.CollectShipping
	}
	if request.Body.Group != nil {
		in.Group = *request.Body.Group
	}

	plan, err := s.billing.UpdatePlan(ctx, request.Key, in)
	if errors.Is(err, billing.ErrInvalid) {
		return api.UpdateBillingPlan400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	}
	if errors.Is(err, billing.ErrNotFound) {
		return api.UpdateBillingPlan404JSONResponse{Code: http.StatusNotFound, Message: "Plan not found"}, nil
	}
	if errors.Is(err, billing.ErrNotConfigured) {
		return api.UpdateBillingPlan503JSONResponse{Code: http.StatusServiceUnavailable, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error updating billing plan %s: %v", request.Key, err)
		return api.UpdateBillingPlan500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to update plan"}, nil
	}

	return api.UpdateBillingPlan200JSONResponse(planToAPI(plan, s.billing)), nil
}

// Helper to convert BillingPlanWithFeatures to API response
func planToAPI(plan *repositories.BillingPlanWithFeatures, svc *billing.Service) api.BillingPlan {
	return api.BillingPlan{
		Key:             plan.Key,
		Name:            plan.Name,
		Description:     plan.Description,
		Kind:            api.BillingPlanKind(plan.Kind),
		Interval:        stringPtr(plan.Interval),
		AmountCents:     plan.AmountCents,
		Currency:        plan.Currency,
		Features:        plan.Features,
		Subject:         api.BillingPlanSubject(plan.Subject),
		TrialDays:       plan.TrialDays,
		GraceDays:       plan.GraceDays,
		SetupFeeCents:   plan.SetupFeeCents,
		SetupFeeName:    stringPtr(plan.SetupFeeName),
		Starts:          api.BillingPlanStarts(plan.Starts),
		AccessDays:      plan.AccessDays,
		CollectShipping: plan.CollectShipping,
		Group:           stringPtr(plan.GroupKey),
		Active:          plan.Active,
		StripeProductId: stringPtr(plan.StripeProductID),
		StripePriceId:   stringPtr(plan.StripePriceID),
		PayLink:         svc.PayLink(plan.Key),
		CreatedAt:       plan.CreatedAt,
		UpdatedAt:       plan.UpdatedAt,
	}
}

// Helper functions for optional fields
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intVal(i *int, defaults ...int) int {
	if i == nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return *i
}

func int64Val(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func boolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
