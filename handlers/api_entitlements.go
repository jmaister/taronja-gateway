package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/api"
)

// GetEntitlements handles GET /api/entitlements: get a user's or organization's entitlements.
// Allowed to: admin, the user themselves, or a member of the organization.
func (s *StrictApiServer) GetEntitlements(ctx context.Context, request api.GetEntitlementsRequestObject) (api.GetEntitlementsResponseObject, error) {
	// Check if authenticated
	sessionObj, ok := requireSession(ctx)
	if !ok || sessionObj == nil || !sessionObj.IsAuthenticated {
		return api.GetEntitlements401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}

	if s.billing == nil {
		return api.GetEntitlements500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	// Extract parameters: exactly one of user_id or org_id
	userID := ""
	orgID := ""
	if request.Params.UserId != nil {
		userID = *request.Params.UserId
	}
	if request.Params.OrgId != nil {
		orgID = *request.Params.OrgId
	}

	// Check that exactly one is provided
	if (userID == "" && orgID == "") || (userID != "" && orgID != "") {
		return api.GetEntitlements400JSONResponse{Code: http.StatusBadRequest, Message: "Provide exactly one of user_id or org_id"}, nil
	}

	_, access := requireAdminAccess(ctx)
	if access != accessGranted {
		allowed := userID != "" && userID == sessionObj.UserID
		if orgID != "" && s.organizations != nil {
			_, err := s.organizations.Member(orgID, sessionObj.UserID)
			allowed = err == nil
		}
		if !allowed {
			return api.GetEntitlements403JSONResponse{Code: http.StatusForbidden, Message: "Only admins, the user or members of the organization"}, nil
		}
	}

	// Get entitlements
	var subject string
	if userID != "" {
		subject = "user:" + userID
	} else {
		subject = "org:" + orgID
	}

	entitlements, err := s.billing.SubjectEntitlements(subject)
	if err != nil {
		log.Printf("Error getting entitlements for %s: %v", subject, err)
		return api.GetEntitlements500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get entitlements"}, nil
	}

	// Convert to API response
	now := time.Now()
	active := entitlements.Active(now)

	features := make(map[string]api.Entitlement, len(active))
	for key, feature := range active {
		features[key] = api.Entitlement{
			Until:  feature.Until,
			Status: api.EntitlementStatus(feature.Status),
			Source: feature.Source,

			Grace:      feature.Grace,
			GraceUntil: feature.GraceUntil,
		}
	}

	return api.GetEntitlements200JSONResponse(api.EntitlementSet{
		Subject:  subject,
		Features: features,
	}), nil
}
