package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"gorm.io/gorm"
)

// MaxOrganizationLogoBytes is the largest logo image accepted.
const MaxOrganizationLogoBytes = 512 << 10

// logoTypes are the image types a logo can have, by what the bytes say.
var logoTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// orgLogoURL is where org's logo is served, "" without one. The version
// changes with every upload, so a browser never shows a stale logo.
func orgLogoURL(org *db.Organization) string {
	if !org.HasLogo() {
		return ""
	}
	return config.ManagementPrefix + "/api/organizations/" + url.PathEscape(org.ID) + "/logo?v=" + strconv.FormatInt(org.LogoUpdatedAt.UnixNano(), 36)
}

type logoAccess int

const (
	logoUnauthenticated logoAccess = iota
	logoForbidden
	logoAllowed
)

// logoAccessFor decides whether the session may read (any member) or change
// (an owner) the logo of orgID; admins may do both. Non-admins who aren't
// members get "forbidden" whether or not the organization exists.
func (s *StrictApiServer) logoAccessFor(ctx context.Context, orgID string, change bool) (logoAccess, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok || !sessionObj.IsAuthenticated {
		return logoUnauthenticated, nil
	}
	if builtin.CheckAdminAccess(sessionObj, true) {
		return logoAllowed, nil
	}
	member, err := s.organizations.Member(orgID, sessionObj.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return logoForbidden, nil
	}
	if err != nil {
		return logoForbidden, err
	}
	if change && member.Role != db.OrganizationRoleOwner {
		return logoForbidden, nil
	}
	return logoAllowed, nil
}

// logoResponse serves the image: private, revalidated by ETag, and inert
// even if a browser were to open it as a page.
type logoResponse struct {
	logo *repositories.OrganizationLogo
	etag string
}

func setLogoHeaders(w http.ResponseWriter, etag string) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
}

func (r logoResponse) VisitGetOrganizationLogoResponse(w http.ResponseWriter) error {
	setLogoHeaders(w, r.etag)
	w.Header().Set("Content-Type", r.logo.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(r.logo.Data)))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(r.logo.Data)
	return err
}

type logoNotModified struct{ etag string }

func (r logoNotModified) VisitGetOrganizationLogoResponse(w http.ResponseWriter) error {
	setLogoHeaders(w, r.etag)
	w.WriteHeader(http.StatusNotModified)
	return nil
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag || candidate == "*" {
			return true
		}
	}
	return false
}

// GetOrganizationLogo handles GET /api/organizations/{orgId}/logo.
func (s *StrictApiServer) GetOrganizationLogo(ctx context.Context, request api.GetOrganizationLogoRequestObject) (api.GetOrganizationLogoResponseObject, error) {
	if s.organizations == nil {
		return api.GetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	access, err := s.logoAccessFor(ctx, request.OrgId, false)
	if err != nil {
		log.Printf("Error checking access to the logo of organization %s: %v", request.OrgId, err)
		return api.GetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get logo"}, nil
	}
	switch access {
	case logoUnauthenticated:
		return api.GetOrganizationLogo401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case logoForbidden:
		return api.GetOrganizationLogo403JSONResponse{Code: http.StatusForbidden, Message: "Only admins and members of the organization can see its logo"}, nil
	}
	logo, err := s.organizations.Logo(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.GetOrganizationLogo404JSONResponse{Code: http.StatusNotFound, Message: "The organization has no logo"}, nil
	}
	if err != nil {
		log.Printf("Error reading the logo of organization %s: %v", request.OrgId, err)
		return api.GetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get logo"}, nil
	}
	etag := `"` + strconv.FormatInt(logo.UpdatedAt.UnixNano(), 36) + `"`
	if request.Params.IfNoneMatch != nil && etagMatches(*request.Params.IfNoneMatch, etag) {
		return logoNotModified{etag: etag}, nil
	}
	return logoResponse{logo: logo, etag: etag}, nil
}

// SetOrganizationLogo handles PUT /api/organizations/{orgId}/logo.
func (s *StrictApiServer) SetOrganizationLogo(ctx context.Context, request api.SetOrganizationLogoRequestObject) (api.SetOrganizationLogoResponseObject, error) {
	if s.organizations == nil {
		return api.SetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	access, err := s.logoAccessFor(ctx, request.OrgId, true)
	if err != nil {
		log.Printf("Error checking access to the logo of organization %s: %v", request.OrgId, err)
		return api.SetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to set logo"}, nil
	}
	switch access {
	case logoUnauthenticated:
		return api.SetOrganizationLogo401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case logoForbidden:
		return api.SetOrganizationLogo403JSONResponse{Code: http.StatusForbidden, Message: "Only admins and owners of the organization can change its logo"}, nil
	}
	org, err := s.organizations.Get(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.SetOrganizationLogo404JSONResponse{Code: http.StatusNotFound, Message: "Organization not found"}, nil
	}
	if err != nil {
		log.Printf("Error reading organization %s: %v", request.OrgId, err)
		return api.SetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to set logo"}, nil
	}

	data, err := io.ReadAll(io.LimitReader(request.Body, MaxOrganizationLogoBytes+1))
	if err != nil {
		return api.SetOrganizationLogo400JSONResponse{Code: http.StatusBadRequest, Message: "Could not read the image"}, nil
	}
	if len(data) > MaxOrganizationLogoBytes {
		return api.SetOrganizationLogo413JSONResponse{Code: http.StatusRequestEntityTooLarge, Message: "The logo must be at most 512 KiB"}, nil
	}
	contentType := http.DetectContentType(bytes.Clone(data))
	if !logoTypes[contentType] {
		return api.SetOrganizationLogo400JSONResponse{Code: http.StatusBadRequest, Message: "The logo must be a PNG, JPEG or WebP image"}, nil
	}

	err = s.organizations.SetLogo(org.ID, contentType, data)
	var detail api.OrganizationDetail
	if err == nil {
		org, err = s.organizations.Get(org.ID)
	}
	if err == nil {
		detail, err = s.organizationDetail(org)
	}
	if err != nil {
		log.Printf("Error setting the logo of organization %s: %v", request.OrgId, err)
		return api.SetOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to set logo"}, nil
	}
	return api.SetOrganizationLogo200JSONResponse(detail), nil
}

// DeleteOrganizationLogo handles DELETE /api/organizations/{orgId}/logo.
func (s *StrictApiServer) DeleteOrganizationLogo(ctx context.Context, request api.DeleteOrganizationLogoRequestObject) (api.DeleteOrganizationLogoResponseObject, error) {
	if s.organizations == nil {
		return api.DeleteOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: errOrganizationsNotConfigured}, nil
	}
	access, err := s.logoAccessFor(ctx, request.OrgId, true)
	if err != nil {
		log.Printf("Error checking access to the logo of organization %s: %v", request.OrgId, err)
		return api.DeleteOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to remove logo"}, nil
	}
	switch access {
	case logoUnauthenticated:
		return api.DeleteOrganizationLogo401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case logoForbidden:
		return api.DeleteOrganizationLogo403JSONResponse{Code: http.StatusForbidden, Message: "Only admins and owners of the organization can change its logo"}, nil
	}
	err = s.organizations.ClearLogo(request.OrgId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.DeleteOrganizationLogo404JSONResponse{Code: http.StatusNotFound, Message: "Organization not found"}, nil
	}
	if err != nil {
		log.Printf("Error removing the logo of organization %s: %v", request.OrgId, err)
		return api.DeleteOrganizationLogo500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to remove logo"}, nil
	}
	return api.DeleteOrganizationLogo204Response{}, nil
}
