package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
)

// GetBillingSetup handles GET /api/billing/setup (admin): get setup checks.
func (s *StrictApiServer) GetBillingSetup(ctx context.Context, request api.GetBillingSetupRequestObject) (api.GetBillingSetupResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.GetBillingSetup401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.GetBillingSetup403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.GetBillingSetup500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	checks := s.billing.SetupChecks(ctx)
	resp := api.BillingSetup{
		Checks: make([]api.SetupCheck, 0, len(checks)),
	}

	for _, check := range checks {
		resp.Checks = append(resp.Checks, api.SetupCheck{
			Id:      check.ID,
			Title:   check.Title,
			Detail:  check.Detail,
			Status:  api.SetupCheckStatus(check.Status),
			Section: setupSection(check.ID),
			Action:  stringPtr(check.Action),
			Link:    stringPtr(check.Link),
		})
	}

	return api.GetBillingSetup200JSONResponse(resp), nil
}

// RunBillingSetupAction handles POST /api/billing/setup/actions (admin): run a setup action.
func (s *StrictApiServer) RunBillingSetupAction(ctx context.Context, request api.RunBillingSetupActionRequestObject) (api.RunBillingSetupActionResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.RunBillingSetupAction401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.RunBillingSetupAction403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.RunBillingSetupAction500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	if request.Body == nil {
		return api.RunBillingSetupAction400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}

	err := s.billing.RunSetupAction(ctx, request.Body.Action)
	if errors.Is(err, billing.ErrInvalid) {
		return api.RunBillingSetupAction400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error running setup action %s: %v", request.Body.Action, err)
		// Return the error message to the user
		return api.RunBillingSetupAction500JSONResponse{Code: http.StatusInternalServerError, Message: err.Error()}, nil
	}

	// Return the updated setup checks
	checks := s.billing.SetupChecks(ctx)
	resp := api.BillingSetup{
		Checks: make([]api.SetupCheck, 0, len(checks)),
	}

	for _, check := range checks {
		resp.Checks = append(resp.Checks, api.SetupCheck{
			Id:      check.ID,
			Title:   check.Title,
			Detail:  check.Detail,
			Status:  api.SetupCheckStatus(check.Status),
			Section: setupSection(check.ID),
			Action:  stringPtr(check.Action),
			Link:    stringPtr(check.Link),
		})
	}

	return api.RunBillingSetupAction200JSONResponse(resp), nil
}

// setupSection groups a check under Stripe or Odoo on the setup page.
func setupSection(id string) api.SetupCheckSection {
	if strings.HasPrefix(id, "odoo") {
		return api.Odoo
	}
	return api.Stripe
}
