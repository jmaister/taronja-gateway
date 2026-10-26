package handlers_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationHandlers(t *testing.T) {
	// Setup isolated test database
	dependencies := deps.NewTestWithName("TestOrganizationHandlers")

	// Create server with organizations service
	orgRepo := repositories.NewOrganizationRepositoryDB(db.GetConnection())
	userRepo := repositories.NewDBUserRepository(db.GetConnection())
	billingRepo := repositories.NewBillingRepositoryDB(db.GetConnection())

	s := handlers.NewStrictApiServer(
		dependencies.SessionStore,
		userRepo,
		dependencies.TrafficMetricRepo,
		dependencies.TokenRepo,
		dependencies.CountersRepo,
		dependencies.BlockedClientRepo,
		dependencies.TokenService,
		dependencies.StartTime,
		nil,
		nil,
		dependencies.NotificationService,
	)
	s.SetOrganizations(orgRepo)

	// Create a billing service for testing InvalidateUser/InvalidateAll
	billingService := billing.NewService(billing.Config{
		Repo: billingRepo,
		Orgs: orgRepo,
	})
	s.SetBilling(billingService)

	// Create test users
	user1 := &db.User{
		Username: "testuser1",
		Email:    "user1@example.com",
		Name:     "Test User 1",
	}
	err := userRepo.CreateUser(user1)
	require.NoError(t, err)

	user2 := &db.User{
		Username: "testuser2",
		Email:    "user2@example.com",
		Name:     "Test User 2",
	}
	err = userRepo.CreateUser(user2)
	require.NoError(t, err)

	// Setup test contexts
	adminCtx := context.WithValue(context.Background(), session.SessionKey, &db.Session{
		UserID:          "admin-user",
		IsAuthenticated: true,
		IsAdmin:         true,
		ValidUntil:      time.Now().Add(1 * time.Hour),
	})
	userCtx := context.WithValue(context.Background(), session.SessionKey, &db.Session{
		UserID:          user1.ID,
		IsAuthenticated: true,
		IsAdmin:         false,
		ValidUntil:      time.Now().Add(1 * time.Hour),
	})
	anonCtx := context.Background()

	t.Run("admin endpoints: 401 anonymous, 403 non-admin", func(t *testing.T) {
		member := &api.SetOrganizationMemberJSONRequestBody{Role: "member"}
		calls := map[string]func(ctx context.Context) (any, error){
			"list": func(ctx context.Context) (any, error) {
				return s.ListOrganizations(ctx, api.ListOrganizationsRequestObject{})
			},
			"create": func(ctx context.Context) (any, error) {
				return s.CreateOrganization(ctx, api.CreateOrganizationRequestObject{Body: &api.OrganizationCreateRequest{Name: "x"}})
			},
			"get": func(ctx context.Context) (any, error) {
				return s.GetOrganization(ctx, api.GetOrganizationRequestObject{OrgId: "o"})
			},
			"update": func(ctx context.Context) (any, error) {
				return s.UpdateOrganization(ctx, api.UpdateOrganizationRequestObject{OrgId: "o", Body: &api.UpdateOrganizationJSONRequestBody{Name: "x"}})
			},
			"delete": func(ctx context.Context) (any, error) {
				return s.DeleteOrganization(ctx, api.DeleteOrganizationRequestObject{OrgId: "o"})
			},
			"set": func(ctx context.Context) (any, error) {
				return s.SetOrganizationMember(ctx, api.SetOrganizationMemberRequestObject{OrgId: "o", UserId: "u", Body: member})
			},
			"remove": func(ctx context.Context) (any, error) {
				return s.RemoveOrganizationMember(ctx, api.RemoveOrganizationMemberRequestObject{OrgId: "o", UserId: "u"})
			},
		}
		for name, call := range calls {
			resp, err := call(anonCtx)
			require.NoError(t, err)
			assert.Contains(t, fmt.Sprintf("%T", resp), "401JSONResponse", name)
			resp, err = call(userCtx)
			require.NoError(t, err)
			assert.Contains(t, fmt.Sprintf("%T", resp), "403JSONResponse", name)
		}
	})

	t.Run("create with an unknown owner creates nothing", func(t *testing.T) {
		before, err := orgRepo.List()
		require.NoError(t, err)
		missing := "no-such-user"
		resp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{Body: &api.OrganizationCreateRequest{Name: "Ghost", OwnerUserId: &missing}})
		require.NoError(t, err)
		assert.IsType(t, api.CreateOrganization400JSONResponse{}, resp)
		after, err := orgRepo.List()
		require.NoError(t, err)
		assert.Len(t, after, len(before))
	})

	t.Run("create with an owner", func(t *testing.T) {
		resp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{Body: &api.OrganizationCreateRequest{Name: "Owned", OwnerUserId: &user1.ID}})
		require.NoError(t, err)
		created, ok := resp.(api.CreateOrganization201JSONResponse)
		require.True(t, ok, "%T", resp)
		require.Len(t, created.Members, 1)
		assert.Equal(t, user1.ID, created.Members[0].UserId)
		assert.Equal(t, api.OrganizationMemberRole("owner"), created.Members[0].Role)
	})

	t.Run("admin CRUD happy path", func(t *testing.T) {
		// Create
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "My Organization"},
		})
		require.NoError(t, err)
		created, ok := createResp.(api.CreateOrganization201JSONResponse)
		require.True(t, ok, "got %T", createResp)
		assert.Equal(t, "My Organization", created.Name)
		assert.NotEmpty(t, created.Id)
		assert.Equal(t, 0, len(created.Members))

		orgID := created.Id

		// Read
		getResp, err := s.GetOrganization(adminCtx, api.GetOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		getOrg, ok := getResp.(api.GetOrganization200JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "My Organization", getOrg.Name)
		assert.Equal(t, 0, len(getOrg.Members))

		// Update
		updateResp, err := s.UpdateOrganization(adminCtx, api.UpdateOrganizationRequestObject{
			OrgId: orgID,
			Body: &api.OrganizationUpdateRequest{
				Name: "Updated Organization",
			},
		})
		require.NoError(t, err)
		updated, ok := updateResp.(api.UpdateOrganization200JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "Updated Organization", updated.Name)

		// List
		listResp, err := s.ListOrganizations(adminCtx, api.ListOrganizationsRequestObject{})
		require.NoError(t, err)
		list, ok := listResp.(api.ListOrganizations200JSONResponse)
		require.True(t, ok)
		assert.Greater(t, len(list), 0)
		found := false
		for _, org := range list {
			if org.Id == orgID {
				assert.Equal(t, "Updated Organization", org.Name)
				found = true
				break
			}
		}
		assert.True(t, found)

		// Delete
		deleteResp, err := s.DeleteOrganization(adminCtx, api.DeleteOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		assert.IsType(t, api.DeleteOrganization204Response{}, deleteResp)

		// Verify deleted
		getGoneResp, err := s.GetOrganization(adminCtx, api.GetOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		assert.IsType(t, api.GetOrganization404JSONResponse{}, getGoneResp)
	})

	t.Run("SetOrganizationMember happy path", func(t *testing.T) {
		// Create org
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Member Test Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		// Add member as owner
		setResp, err := s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("owner"),
			},
		})
		require.NoError(t, err)
		member, ok := setResp.(api.SetOrganizationMember200JSONResponse)
		require.True(t, ok)
		assert.Equal(t, user1.ID, member.UserId)
		assert.Equal(t, "owner", string(member.Role))

		// Get organization and verify member
		getResp, err := s.GetOrganization(adminCtx, api.GetOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		detail := getResp.(api.GetOrganization200JSONResponse)
		assert.Equal(t, 1, len(detail.Members))
		assert.Equal(t, user1.ID, detail.Members[0].UserId)

		// Change role to member
		setResp, err = s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("member"),
			},
		})
		require.NoError(t, err)
		member = setResp.(api.SetOrganizationMember200JSONResponse)
		assert.Equal(t, "member", string(member.Role))

		// Verify role changed
		getResp, err = s.GetOrganization(adminCtx, api.GetOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		detail = getResp.(api.GetOrganization200JSONResponse)
		assert.Equal(t, 1, len(detail.Members))
		assert.Equal(t, "member", string(detail.Members[0].Role))
	})

	t.Run("SetOrganizationMember with invalid role returns 400", func(t *testing.T) {
		// Create org
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Invalid Role Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		// Try to add member with invalid role
		setResp, err := s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("invalid"),
			},
		})
		require.NoError(t, err)
		assert.IsType(t, api.SetOrganizationMember400JSONResponse{}, setResp)
	})

	t.Run("SetOrganizationMember with unknown user returns 404", func(t *testing.T) {
		// Create org
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Unknown User Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		// Try to add non-existent member
		setResp, err := s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: "nonexistent-user",
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("member"),
			},
		})
		require.NoError(t, err)
		assert.IsType(t, api.SetOrganizationMember404JSONResponse{}, setResp)
	})

	t.Run("RemoveOrganizationMember", func(t *testing.T) {
		// Create org and add member
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Remove Member Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		_, err = s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("owner"),
			},
		})
		require.NoError(t, err)

		// Remove member
		removeResp, err := s.RemoveOrganizationMember(adminCtx, api.RemoveOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
		})
		require.NoError(t, err)
		assert.IsType(t, api.RemoveOrganizationMember204Response{}, removeResp)

		// Verify removed
		getResp, err := s.GetOrganization(adminCtx, api.GetOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		detail := getResp.(api.GetOrganization200JSONResponse)
		assert.Equal(t, 0, len(detail.Members))
	})

	t.Run("RemoveOrganizationMember with non-member returns 404", func(t *testing.T) {
		// Create org
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Non-member Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		// Try to remove non-member
		removeResp, err := s.RemoveOrganizationMember(adminCtx, api.RemoveOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
		})
		require.NoError(t, err)
		assert.IsType(t, api.RemoveOrganizationMember404JSONResponse{}, removeResp)
	})

	t.Run("DeleteOrganization removes members", func(t *testing.T) {
		// Create org
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Delete With Members Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		// Add members
		_, err = s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("owner"),
			},
		})
		require.NoError(t, err)

		_, err = s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user2.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("member"),
			},
		})
		require.NoError(t, err)

		// Delete organization
		deleteResp, err := s.DeleteOrganization(adminCtx, api.DeleteOrganizationRequestObject{OrgId: orgID})
		require.NoError(t, err)
		assert.IsType(t, api.DeleteOrganization204Response{}, deleteResp)

		// Verify members are gone
		members, err := orgRepo.Members(orgID)
		// Should error because org doesn't exist or members should be empty
		if err == nil {
			assert.Equal(t, 0, len(members))
		}
	})

	t.Run("ListMyOrganizations returns only caller's orgs", func(t *testing.T) {
		// Create org 1
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "User1 Org 1"},
		})
		require.NoError(t, err)
		org1 := createResp.(api.CreateOrganization201JSONResponse)

		// Create org 2
		createResp, err = s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "User1 Org 2"},
		})
		require.NoError(t, err)
		org2 := createResp.(api.CreateOrganization201JSONResponse)

		// Add user1 to both orgs
		_, err = s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  org1.Id,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("owner"),
			},
		})
		require.NoError(t, err)

		_, err = s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  org2.Id,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("member"),
			},
		})
		require.NoError(t, err)

		// List user1's orgs
		listResp, err := s.ListMyOrganizations(userCtx, api.ListMyOrganizationsRequestObject{})
		require.NoError(t, err)
		list, ok := listResp.(api.ListMyOrganizations200JSONResponse)
		require.True(t, ok)

		// Should have at least 2 orgs
		assert.GreaterOrEqual(t, len(list), 2)

		// Find our test orgs
		org1Found := false
		org2Found := false
		for _, org := range list {
			if org.Id == org1.Id {
				assert.Equal(t, "owner", string(org.Role))
				org1Found = true
			}
			if org.Id == org2.Id {
				assert.Equal(t, "member", string(org.Role))
				org2Found = true
			}
		}
		assert.True(t, org1Found, "org1 not found in user's organizations")
		assert.True(t, org2Found, "org2 not found in user's organizations")
	})

	t.Run("ListMyOrganizations rejects unauthenticated", func(t *testing.T) {
		resp, err := s.ListMyOrganizations(anonCtx, api.ListMyOrganizationsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, api.ListMyOrganizations401JSONResponse{}, resp)
	})

	t.Run("CreateOrganization with name validation", func(t *testing.T) {
		// Empty name
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: ""},
		})
		require.NoError(t, err)
		assert.IsType(t, api.CreateOrganization400JSONResponse{}, createResp)

		// Name too long
		longName := ""
		for i := 0; i < 201; i++ {
			longName += "a"
		}
		createResp, err = s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: longName},
		})
		require.NoError(t, err)
		assert.IsType(t, api.CreateOrganization400JSONResponse{}, createResp)

		// Valid name with spaces should be trimmed
		createResp, err = s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "  Valid Org  "},
		})
		require.NoError(t, err)
		created, ok := createResp.(api.CreateOrganization201JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "Valid Org", created.Name)
	})

	t.Run("billing cache invalidation", func(t *testing.T) {
		// Create org
		createResp, err := s.CreateOrganization(adminCtx, api.CreateOrganizationRequestObject{
			Body: &api.OrganizationCreateRequest{Name: "Cache Test Org"},
		})
		require.NoError(t, err)
		org := createResp.(api.CreateOrganization201JSONResponse)
		orgID := org.Id

		// Prime the billing cache by calling ForUser before adding the member
		_, _ = billingService.ForUser(user1.ID)

		// Add member and verify InvalidateUser was called
		setResp, err := s.SetOrganizationMember(adminCtx, api.SetOrganizationMemberRequestObject{
			OrgId:  orgID,
			UserId: user1.ID,
			Body: &api.OrganizationMemberRequest{
				Role: api.OrganizationMemberRequestRole("owner"),
			},
		})
		require.NoError(t, err)
		_, ok := setResp.(api.SetOrganizationMember200JSONResponse)
		require.True(t, ok)

		// Call ForUser again - should reflect the new membership
		// This would fail if InvalidateUser wasn't called
		userEntitlements, err := billingService.ForUser(user1.ID)
		require.NoError(t, err)
		assert.NotNil(t, userEntitlements)
		// Verify that the org membership is in the result
		assert.NotNil(t, userEntitlements.Orgs)
		_, found := userEntitlements.Orgs[orgID]
		assert.True(t, found, "organization not found in user entitlements after cache invalidation")
	})

	t.Run("organizations nil returns 500", func(t *testing.T) {
		// Create server without organizations
		sNoOrgs := handlers.NewStrictApiServer(
			dependencies.SessionStore,
			userRepo,
			dependencies.TrafficMetricRepo,
			dependencies.TokenRepo,
			dependencies.CountersRepo,
			dependencies.BlockedClientRepo,
			dependencies.TokenService,
			dependencies.StartTime,
			nil,
			nil,
			dependencies.NotificationService,
		)

		resp, err := sNoOrgs.ListOrganizations(adminCtx, api.ListOrganizationsRequestObject{})
		require.NoError(t, err)
		errResp, ok := resp.(api.ListOrganizations500JSONResponse)
		require.True(t, ok)
		assert.Equal(t, "organizations are not configured", errResp.Message)
	})
}
