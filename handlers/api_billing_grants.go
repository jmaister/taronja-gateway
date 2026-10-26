package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"gorm.io/gorm"
)

// ListBillingGrants handles GET /api/billing/grants (admin): list grants.
func (s *StrictApiServer) ListBillingGrants(ctx context.Context, request api.ListBillingGrantsRequestObject) (api.ListBillingGrantsResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ListBillingGrants401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListBillingGrants403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.ListBillingGrants500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	subject := ""
	if request.Params.Subject != nil {
		subject = *request.Params.Subject
	}

	grants, err := s.billing.Repo().ListGrants(subject)
	if err != nil {
		log.Printf("Error listing billing grants: %v", err)
		return api.ListBillingGrants500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list grants"}, nil
	}

	resp := make(api.ListBillingGrants200JSONResponse, 0, len(grants))
	for i := range grants {
		resp = append(resp, api.BillingGrant{
			Id:        grants[i].ID,
			Subject:   grants[i].Subject,
			Feature:   grants[i].Feature,
			Until:     grants[i].Until,
			GraceDays: grants[i].GraceDays,
			Note:      grants[i].Note,
			CreatedBy: grants[i].CreatedBy,
			CreatedAt: grants[i].CreatedAt,
		})
	}
	return resp, nil
}

// CreateBillingGrant handles POST /api/billing/grants (admin): create grants.
func (s *StrictApiServer) CreateBillingGrant(ctx context.Context, request api.CreateBillingGrantRequestObject) (api.CreateBillingGrantResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.CreateBillingGrant401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.CreateBillingGrant403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.CreateBillingGrant500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}
	if request.Body == nil {
		return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}

	subject := strings.TrimSpace(request.Body.Subject)
	if subject == "" {
		return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: "subject is required"}, nil
	}

	if userID, ok := strings.CutPrefix(subject, "user:"); ok {
		user, err := s.findUser(userID)
		if err != nil {
			log.Printf("Error finding user %s: %v", userID, err)
			return api.CreateBillingGrant500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to create grant"}, nil
		}
		if user == nil {
			return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: "user not found"}, nil
		}
	}
	if len(request.Body.Features) == 0 {
		return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: "features: at least one"}, nil
	}
	for _, feature := range request.Body.Features {
		err := billing.ValidateFeature(strings.TrimSpace(feature))
		if err != nil {
			return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
		}
	}

	// Get the session to record creator ID
	sessionObj, ok := requireSession(ctx)
	creatorID := ""
	if ok && sessionObj != nil {
		creatorID = sessionObj.UserID
	}

	note := ""
	if request.Body.Note != nil {
		note = *request.Body.Note
	}

	// Create one grant per feature
	resp := make(api.CreateBillingGrant201JSONResponse, 0, len(request.Body.Features))
	for _, feature := range request.Body.Features {
		grant, err := s.billing.CreateGrant(subject, strings.TrimSpace(feature), request.Body.Until, request.Body.GraceDays, note, creatorID)
		if errors.Is(err, billing.ErrInvalid) {
			return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
		}
		if errors.Is(err, billing.ErrNotFound) {
			return api.CreateBillingGrant400JSONResponse{Code: http.StatusBadRequest, Message: "organization not found"}, nil
		}
		if err != nil {
			log.Printf("Error creating grant: %v", err)
			return api.CreateBillingGrant500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to create grant"}, nil
		}

		resp = append(resp, api.BillingGrant{
			Id:        grant.ID,
			Subject:   grant.Subject,
			Feature:   grant.Feature,
			Until:     grant.Until,
			GraceDays: grant.GraceDays,
			Note:      grant.Note,
			CreatedBy: grant.CreatedBy,
			CreatedAt: grant.CreatedAt,
		})
	}

	return resp, nil
}

// DeleteBillingGrant handles DELETE /api/billing/grants/{id} (admin): delete a grant.
func (s *StrictApiServer) DeleteBillingGrant(ctx context.Context, request api.DeleteBillingGrantRequestObject) (api.DeleteBillingGrantResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.DeleteBillingGrant401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.DeleteBillingGrant403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.DeleteBillingGrant500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	err := s.billing.DeleteGrant(request.Id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.DeleteBillingGrant404JSONResponse{Code: http.StatusNotFound, Message: "Grant not found"}, nil
	}
	if err != nil {
		log.Printf("Error deleting grant %s: %v", request.Id, err)
		return api.DeleteBillingGrant500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to delete grant"}, nil
	}

	return api.DeleteBillingGrant204Response{}, nil
}
