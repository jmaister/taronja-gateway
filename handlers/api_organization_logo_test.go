package handlers_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

func TestOrganizationLogoHandlers(t *testing.T) {
	dependencies := deps.NewTestWithName("TestOrganizationLogoHandlers")
	orgRepo := repositories.NewOrganizationRepositoryDB(db.GetConnection())
	userRepo := repositories.NewDBUserRepository(db.GetConnection())
	s := handlers.NewStrictApiServer(dependencies.SessionStore, userRepo, dependencies.TrafficMetricRepo,
		dependencies.TokenRepo, dependencies.CountersRepo, dependencies.BlockedClientRepo,
		dependencies.TokenService, dependencies.StartTime, nil, nil, dependencies.NotificationService)
	s.SetOrganizations(orgRepo)

	users := map[string]*db.User{}
	for _, name := range []string{"owner", "member", "outsider"} {
		u := &db.User{Username: "logo" + name, Email: name + "@logo.example.com", Name: name}
		require.NoError(t, userRepo.CreateUser(u))
		users[name] = u
	}
	org := &db.Organization{Name: "Logo Org"}
	require.NoError(t, orgRepo.Create(org))
	require.NoError(t, orgRepo.SetMember(org.ID, users["owner"].ID, db.OrganizationRoleOwner))
	require.NoError(t, orgRepo.SetMember(org.ID, users["member"].ID, db.OrganizationRoleMember))

	ctxFor := func(userID string, admin bool) context.Context {
		return context.WithValue(context.Background(), session.SessionKey, &db.Session{
			UserID: userID, IsAuthenticated: true, IsAdmin: admin, ValidUntil: time.Now().Add(time.Hour),
		})
	}
	adminCtx := ctxFor("admin-user", true)
	ownerCtx := ctxFor(users["owner"].ID, false)
	memberCtx := ctxFor(users["member"].ID, false)
	outsiderCtx := ctxFor(users["outsider"].ID, false)

	put := func(ctx context.Context, orgID string, body []byte) api.SetOrganizationLogoResponseObject {
		resp, err := s.SetOrganizationLogo(ctx, api.SetOrganizationLogoRequestObject{OrgId: orgID, ContentType: "image/*", Body: bytes.NewReader(body)})
		require.NoError(t, err)
		return resp
	}
	get := func(ctx context.Context, ifNoneMatch *string) api.GetOrganizationLogoResponseObject {
		resp, err := s.GetOrganizationLogo(ctx, api.GetOrganizationLogoRequestObject{OrgId: org.ID, Params: api.GetOrganizationLogoParams{IfNoneMatch: ifNoneMatch}})
		require.NoError(t, err)
		return resp
	}
	visit := func(resp api.GetOrganizationLogoResponseObject) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		require.NoError(t, resp.VisitGetOrganizationLogoResponse(rec))
		return rec
	}

	t.Run("no logo: 404, empty logoUrl", func(t *testing.T) {
		assert.IsType(t, api.GetOrganizationLogo404JSONResponse{}, get(memberCtx, nil))
		detail, ok := mustDetail(t, s, adminCtx, org.ID)
		require.True(t, ok)
		assert.Empty(t, detail.LogoUrl)
	})

	t.Run("anonymous 401, outsider 403", func(t *testing.T) {
		assert.IsType(t, api.GetOrganizationLogo401JSONResponse{}, get(context.Background(), nil))
		assert.IsType(t, api.GetOrganizationLogo403JSONResponse{}, get(outsiderCtx, nil))
		assert.IsType(t, api.SetOrganizationLogo401JSONResponse{}, put(context.Background(), org.ID, pngBytes))
		assert.IsType(t, api.SetOrganizationLogo403JSONResponse{}, put(outsiderCtx, org.ID, pngBytes))
	})

	t.Run("plain member cannot change the logo", func(t *testing.T) {
		assert.IsType(t, api.SetOrganizationLogo403JSONResponse{}, put(memberCtx, org.ID, pngBytes))
		resp, err := s.DeleteOrganizationLogo(memberCtx, api.DeleteOrganizationLogoRequestObject{OrgId: org.ID})
		require.NoError(t, err)
		assert.IsType(t, api.DeleteOrganizationLogo403JSONResponse{}, resp)
	})

	t.Run("rejects bad uploads", func(t *testing.T) {
		assert.IsType(t, api.SetOrganizationLogo400JSONResponse{}, put(ownerCtx, org.ID, nil))
		assert.IsType(t, api.SetOrganizationLogo400JSONResponse{}, put(ownerCtx, org.ID, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)))
		assert.IsType(t, api.SetOrganizationLogo400JSONResponse{}, put(ownerCtx, org.ID, []byte("GIF89a....")))
		big := append(append([]byte{}, pngBytes...), make([]byte, handlers.MaxOrganizationLogoBytes)...)
		assert.IsType(t, api.SetOrganizationLogo413JSONResponse{}, put(ownerCtx, org.ID, big))
		assert.IsType(t, api.SetOrganizationLogo404JSONResponse{}, put(adminCtx, "missing", pngBytes))
	})

	t.Run("owner uploads, members read with ETag", func(t *testing.T) {
		resp := put(ownerCtx, org.ID, pngBytes)
		ok, isOK := resp.(api.SetOrganizationLogo200JSONResponse)
		require.True(t, isOK, "%T", resp)
		assert.Contains(t, ok.LogoUrl, "/api/organizations/"+org.ID+"/logo?v=")

		rec := visit(get(memberCtx, nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "default-src 'none'", rec.Header().Get("Content-Security-Policy"))
		body, _ := io.ReadAll(rec.Body)
		assert.Equal(t, pngBytes, body)

		etag := rec.Header().Get("ETag")
		require.NotEmpty(t, etag)
		rec = visit(get(memberCtx, &etag))
		assert.Equal(t, http.StatusNotModified, rec.Code)
		assert.Equal(t, 0, rec.Body.Len())
		stale := `"other"`
		assert.Equal(t, http.StatusOK, visit(get(memberCtx, &stale)).Code)
	})

	t.Run("logoUrl in my organizations and list", func(t *testing.T) {
		detail, ok := mustDetail(t, s, adminCtx, org.ID)
		require.True(t, ok)
		assert.NotEmpty(t, detail.LogoUrl)
		list, err := s.ListOrganizations(adminCtx, api.ListOrganizationsRequestObject{})
		require.NoError(t, err)
		found := false
		for _, o := range list.(api.ListOrganizations200JSONResponse) {
			if o.Id == org.ID {
				found = true
				assert.NotEmpty(t, o.LogoUrl)
			}
		}
		assert.True(t, found)
	})

	t.Run("admin removes it", func(t *testing.T) {
		resp, err := s.DeleteOrganizationLogo(adminCtx, api.DeleteOrganizationLogoRequestObject{OrgId: org.ID})
		require.NoError(t, err)
		assert.IsType(t, api.DeleteOrganizationLogo204Response{}, resp)
		assert.IsType(t, api.GetOrganizationLogo404JSONResponse{}, get(ownerCtx, nil))
		resp, err = s.DeleteOrganizationLogo(adminCtx, api.DeleteOrganizationLogoRequestObject{OrgId: "missing"})
		require.NoError(t, err)
		assert.IsType(t, api.DeleteOrganizationLogo404JSONResponse{}, resp)
	})
}

func mustDetail(t *testing.T, s *handlers.StrictApiServer, ctx context.Context, id string) (api.GetOrganization200JSONResponse, bool) {
	t.Helper()
	resp, err := s.GetOrganization(ctx, api.GetOrganizationRequestObject{OrgId: id})
	require.NoError(t, err)
	d, ok := resp.(api.GetOrganization200JSONResponse)
	return d, ok
}
