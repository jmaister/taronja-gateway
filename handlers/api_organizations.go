package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

const errOrganizationsNotConfigured = "organizations are not configured"

func orgName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	return name, name != "" && len(name) <= 200
}

func validRole(role string) bool {
	return role == "owner" || role == "member"
}

// findUser returns the user with id, or nil when there is none.
func (s *StrictApiServer) findUser(id string) (*db.User, error) {
	user, err := s.userRepo.FindUserByIdOrUsername(id, "", "")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return user, err
}

// organizationDetail returns org with its members (email and username of
// each).
func (s *StrictApiServer) organizationDetail(org *db.Organization) (api.OrganizationDetail, error) {
	members, err := s.organizations.Members(org.ID)
	if err != nil {
		return api.OrganizationDetail{}, err
	}
	out := api.OrganizationDetail{Id: org.ID, Name: org.Name, CreatedAt: org.CreatedAt, Members: make([]api.OrganizationMember, 0, len(members))}
	for _, m := range members {
		member := api.OrganizationMember{UserId: m.UserID, Role: api.OrganizationMemberRole(m.Role)}
		user, err := s.findUser(m.UserID)
		if err != nil {
			return api.OrganizationDetail{}, err
		}
		if user != nil {
			member.Email, member.Username = user.Email, user.Username
		}
		out.Members = append(out.Members, member)
	}
	return out, nil
}

// ListOrganizations handles GET /api/organizations (admin).
func (s *StrictApiServer) ListOrganizations(ctx context.Context, request api.ListOrganizationsRequestObject) (api.ListOrganizationsResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ListOrganizations401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListOrganizations403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.ListOrganizations500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	orgs, err := s.organizations.List()
	if err != nil {
		log.Printf("Error listing organizations: %v", err)
		return api.ListOrganizations500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list organizations"}, nil
	}
	resp := make(api.ListOrganizations200JSONResponse, 0, len(orgs))
	for _, org := range orgs {
		members, err := s.organizations.Members(org.ID)
		if err != nil {
			log.Printf("Error listing members of organization %s: %v", org.ID, err)
			return api.ListOrganizations500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list organizations"}, nil
		}
		resp = append(resp, api.Organization{Id: org.ID, Name: org.Name, CreatedAt: org.CreatedAt, MemberCount: len(members)})
	}
	return resp, nil
}

// CreateOrganization handles POST /api/organizations (admin), optionally
// with its first owner.
func (s *StrictApiServer) CreateOrganization(ctx context.Context, request api.CreateOrganizationRequestObject) (api.CreateOrganizationResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.CreateOrganization401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.CreateOrganization403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.CreateOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	if request.Body == nil {
		return api.CreateOrganization400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}
	name, ok := orgName(request.Body.Name)
	if !ok {
		return api.CreateOrganization400JSONResponse{Code: http.StatusBadRequest, Message: "name: 1-200 characters"}, nil
	}
	ownerID := ""
	if request.Body.OwnerUserId != nil {
		ownerID = strings.TrimSpace(*request.Body.OwnerUserId)
	}
	if ownerID != "" {
		owner, err := s.findUser(ownerID)
		if err != nil {
			log.Printf("Error finding user %s: %v", ownerID, err)
			return api.CreateOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to create organization"}, nil
		}
		if owner == nil {
			return api.CreateOrganization400JSONResponse{Code: http.StatusBadRequest, Message: "ownerUserId: no such user"}, nil
		}
	}

	org := &db.Organization{Name: name}
	err := s.organizations.Create(org)
	if err == nil && ownerID != "" {
		err = s.organizations.SetMember(org.ID, ownerID, "owner")
		if err == nil && s.billing != nil {
			s.billing.InvalidateUser(ownerID)
		}
	}
	var detail api.OrganizationDetail
	if err == nil {
		detail, err = s.organizationDetail(org)
	}
	if err != nil {
		log.Printf("Error creating organization: %v", err)
		return api.CreateOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to create organization"}, nil
	}
	return api.CreateOrganization201JSONResponse(detail), nil
}

// GetOrganization handles GET /api/organizations/{orgId} (admin).
func (s *StrictApiServer) GetOrganization(ctx context.Context, request api.GetOrganizationRequestObject) (api.GetOrganizationResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.GetOrganization401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.GetOrganization403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.GetOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	org, err := s.organizations.Get(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.GetOrganization404JSONResponse{Code: http.StatusNotFound, Message: "Organization not found"}, nil
	}
	var detail api.OrganizationDetail
	if err == nil {
		detail, err = s.organizationDetail(org)
	}
	if err != nil {
		log.Printf("Error reading organization %s: %v", request.OrgId, err)
		return api.GetOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get organization"}, nil
	}
	return api.GetOrganization200JSONResponse(detail), nil
}

// UpdateOrganization handles PATCH /api/organizations/{orgId} (admin): rename.
func (s *StrictApiServer) UpdateOrganization(ctx context.Context, request api.UpdateOrganizationRequestObject) (api.UpdateOrganizationResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.UpdateOrganization401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.UpdateOrganization403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.UpdateOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	if request.Body == nil {
		return api.UpdateOrganization400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}
	name, ok := orgName(request.Body.Name)
	if !ok {
		return api.UpdateOrganization400JSONResponse{Code: http.StatusBadRequest, Message: "name: 1-200 characters"}, nil
	}
	org, err := s.organizations.Get(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.UpdateOrganization404JSONResponse{Code: http.StatusNotFound, Message: "Organization not found"}, nil
	}
	var detail api.OrganizationDetail
	if err == nil {
		org.Name = name
		err = s.organizations.Update(org)
	}
	if err == nil {
		detail, err = s.organizationDetail(org)
	}
	if err != nil {
		log.Printf("Error updating organization %s: %v", request.OrgId, err)
		return api.UpdateOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to update organization"}, nil
	}
	return api.UpdateOrganization200JSONResponse(detail), nil
}

// DeleteOrganization handles DELETE /api/organizations/{orgId} (admin).
func (s *StrictApiServer) DeleteOrganization(ctx context.Context, request api.DeleteOrganizationRequestObject) (api.DeleteOrganizationResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.DeleteOrganization401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.DeleteOrganization403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.DeleteOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	err := s.organizations.Delete(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.DeleteOrganization404JSONResponse{Code: http.StatusNotFound, Message: "Organization not found"}, nil
	}
	if err != nil {
		log.Printf("Error deleting organization %s: %v", request.OrgId, err)
		return api.DeleteOrganization500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to delete organization"}, nil
	}
	if s.billing != nil {
		s.billing.InvalidateAll()
	}
	return api.DeleteOrganization204Response{}, nil
}

// SetOrganizationMember handles PUT /api/organizations/{orgId}/members/{userId}
// (admin): adds the user or changes their role.
func (s *StrictApiServer) SetOrganizationMember(ctx context.Context, request api.SetOrganizationMemberRequestObject) (api.SetOrganizationMemberResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.SetOrganizationMember401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.SetOrganizationMember403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.SetOrganizationMember500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	if request.Body == nil || !validRole(string(request.Body.Role)) {
		return api.SetOrganizationMember400JSONResponse{Code: http.StatusBadRequest, Message: "role must be owner or member"}, nil
	}
	_, err := s.organizations.Get(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.SetOrganizationMember404JSONResponse{Code: http.StatusNotFound, Message: "Organization not found"}, nil
	}
	var user *db.User
	if err == nil {
		user, err = s.findUser(request.UserId)
	}
	if err == nil && user == nil {
		return api.SetOrganizationMember404JSONResponse{Code: http.StatusNotFound, Message: "User not found"}, nil
	}
	role := string(request.Body.Role)
	if err == nil {
		err = s.organizations.SetMember(request.OrgId, request.UserId, role)
	}
	if err != nil {
		log.Printf("Error setting member %s of organization %s: %v", request.UserId, request.OrgId, err)
		return api.SetOrganizationMember500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to set organization member"}, nil
	}
	if s.billing != nil {
		s.billing.InvalidateUser(request.UserId)
	}
	return api.SetOrganizationMember200JSONResponse(api.OrganizationMember{
		UserId: user.ID, Email: user.Email, Username: user.Username, Role: api.OrganizationMemberRole(role),
	}), nil
}

// RemoveOrganizationMember handles DELETE /api/organizations/{orgId}/members/{userId} (admin).
func (s *StrictApiServer) RemoveOrganizationMember(ctx context.Context, request api.RemoveOrganizationMemberRequestObject) (api.RemoveOrganizationMemberResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.RemoveOrganizationMember401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.RemoveOrganizationMember403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.organizations == nil {
		return api.RemoveOrganizationMember500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	err := s.organizations.RemoveMember(request.OrgId, request.UserId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.RemoveOrganizationMember404JSONResponse{Code: http.StatusNotFound, Message: "Member not found"}, nil
	}
	if err != nil {
		log.Printf("Error removing member %s from organization %s: %v", request.UserId, request.OrgId, err)
		return api.RemoveOrganizationMember500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to remove organization member"}, nil
	}
	if s.billing != nil {
		s.billing.InvalidateUser(request.UserId)
	}
	return api.RemoveOrganizationMember204Response{}, nil
}

// myOrganizations lists userID's organizations with their role.
func (s *StrictApiServer) myOrganizations(userID string) ([]api.MyOrganization, error) {
	memberships, err := s.organizations.OfUser(userID)
	if err != nil {
		return nil, err
	}
	out := make([]api.MyOrganization, 0, len(memberships))
	for _, m := range memberships {
		out = append(out, api.MyOrganization{Id: m.Organization.ID, Name: m.Organization.Name, Role: api.MyOrganizationRole(m.Role)})
	}
	return out, nil
}

// ListMyOrganizations handles GET /api/me/organizations.
func (s *StrictApiServer) ListMyOrganizations(ctx context.Context, request api.ListMyOrganizationsRequestObject) (api.ListMyOrganizationsResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok || !sessionObj.IsAuthenticated {
		return api.ListMyOrganizations401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.organizations == nil {
		return api.ListMyOrganizations200JSONResponse{}, nil
	}
	orgs, err := s.myOrganizations(sessionObj.UserID)
	if err != nil {
		log.Printf("Error listing organizations of %s: %v", sessionObj.UserID, err)
		return api.ListMyOrganizations500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list organizations"}, nil
	}
	return api.ListMyOrganizations200JSONResponse(orgs), nil
}
