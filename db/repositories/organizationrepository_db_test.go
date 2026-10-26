package repositories

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationRepositoryDB_Create(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{
		Name: "Test Org",
	}

	err := repo.Create(org)
	require.NoError(t, err)
	assert.NotEmpty(t, org.ID)
}

func TestOrganizationRepositoryDB_Get(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	retrieved, err := repo.Get(org.ID)
	require.NoError(t, err)
	assert.Equal(t, org.ID, retrieved.ID)
	assert.Equal(t, "Test Org", retrieved.Name)
}

func TestOrganizationRepositoryDB_Get_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	_, err := repo.Get("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestOrganizationRepositoryDB_List(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org1 := &db.Organization{Name: "Org 1"}
	org2 := &db.Organization{Name: "Org 2"}
	require.NoError(t, repo.Create(org1))
	require.NoError(t, repo.Create(org2))

	orgs, err := repo.List()
	require.NoError(t, err)
	assert.Len(t, orgs, 2)
}

func TestOrganizationRepositoryDB_Update(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Original"}
	require.NoError(t, repo.Create(org))

	org.Name = "Updated"
	err := repo.Update(org)
	require.NoError(t, err)

	retrieved, err := repo.Get(org.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", retrieved.Name)
}

func TestOrganizationRepositoryDB_Delete(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "To Delete"}
	require.NoError(t, repo.Create(org))

	// Add a member
	err := repo.SetMember(org.ID, "user1", db.OrganizationRoleMember)
	require.NoError(t, err)

	// Delete the organization
	err = repo.Delete(org.ID)
	require.NoError(t, err)

	// Verify organization is gone
	_, err = repo.Get(org.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Verify members are gone
	members, err := repo.Members(org.ID)
	require.NoError(t, err)
	assert.Len(t, members, 0)
}

func TestOrganizationRepositoryDB_Delete_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	err := repo.Delete("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestOrganizationRepositoryDB_SetMember(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	err := repo.SetMember(org.ID, "user1", db.OrganizationRoleOwner)
	require.NoError(t, err)

	member, err := repo.Member(org.ID, "user1")
	require.NoError(t, err)
	assert.Equal(t, db.OrganizationRoleOwner, member.Role)
}

func TestOrganizationRepositoryDB_SetMember_NonexistentOrg(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	err := repo.SetMember("nonexistent", "user1", db.OrganizationRoleMember)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestOrganizationRepositoryDB_SetMember_UpdateRole(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	// Set member as member
	err := repo.SetMember(org.ID, "user1", db.OrganizationRoleMember)
	require.NoError(t, err)

	// Update to owner
	err = repo.SetMember(org.ID, "user1", db.OrganizationRoleOwner)
	require.NoError(t, err)

	member, err := repo.Member(org.ID, "user1")
	require.NoError(t, err)
	assert.Equal(t, db.OrganizationRoleOwner, member.Role)
}

func TestOrganizationRepositoryDB_RemoveMember(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	require.NoError(t, repo.SetMember(org.ID, "user1", db.OrganizationRoleMember))

	err := repo.RemoveMember(org.ID, "user1")
	require.NoError(t, err)

	_, err = repo.Member(org.ID, "user1")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestOrganizationRepositoryDB_RemoveMember_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	err := repo.RemoveMember(org.ID, "nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestOrganizationRepositoryDB_Members(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	require.NoError(t, repo.SetMember(org.ID, "user1", db.OrganizationRoleOwner))
	require.NoError(t, repo.SetMember(org.ID, "user2", db.OrganizationRoleMember))

	members, err := repo.Members(org.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestOrganizationRepositoryDB_Members_Ordered(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	// Add members (they should be ordered by created_at)
	require.NoError(t, repo.SetMember(org.ID, "user1", db.OrganizationRoleOwner))
	require.NoError(t, repo.SetMember(org.ID, "user2", db.OrganizationRoleMember))

	members, err := repo.Members(org.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
	// First member created should appear first
	assert.Equal(t, "user1", members[0].UserID)
	assert.Equal(t, "user2", members[1].UserID)
}

func TestOrganizationRepositoryDB_Member(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	require.NoError(t, repo.SetMember(org.ID, "user1", db.OrganizationRoleOwner))

	member, err := repo.Member(org.ID, "user1")
	require.NoError(t, err)
	assert.Equal(t, org.ID, member.OrganizationID)
	assert.Equal(t, "user1", member.UserID)
	assert.Equal(t, db.OrganizationRoleOwner, member.Role)
}

func TestOrganizationRepositoryDB_Member_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org := &db.Organization{Name: "Test Org"}
	require.NoError(t, repo.Create(org))

	_, err := repo.Member(org.ID, "nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestOrganizationRepositoryDB_OfUser(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	org1 := &db.Organization{Name: "Org A"}
	org2 := &db.Organization{Name: "Org B"}
	require.NoError(t, repo.Create(org1))
	require.NoError(t, repo.Create(org2))

	require.NoError(t, repo.SetMember(org1.ID, "user1", db.OrganizationRoleOwner))
	require.NoError(t, repo.SetMember(org2.ID, "user1", db.OrganizationRoleMember))

	memberships, err := repo.OfUser("user1")
	require.NoError(t, err)
	assert.Len(t, memberships, 2)

	// Verify they're ordered by organization name
	assert.Equal(t, "Org A", memberships[0].Organization.Name)
	assert.Equal(t, "Org B", memberships[1].Organization.Name)

	// Verify roles
	assert.Equal(t, db.OrganizationRoleOwner, memberships[0].Role)
	assert.Equal(t, db.OrganizationRoleMember, memberships[1].Role)
}

func TestOrganizationRepositoryDB_OfUser_Empty(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	memberships, err := repo.OfUser("user1")
	require.NoError(t, err)
	assert.Len(t, memberships, 0)
}

func TestOrganizationRepositoryDB_OfUser_Ordered(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewOrganizationRepositoryDB(conn)

	// Create orgs with names that will be sorted alphabetically
	org1 := &db.Organization{Name: "Zebra Corp"}
	org2 := &db.Organization{Name: "Apple Inc"}
	org3 := &db.Organization{Name: "Banana LLC"}

	require.NoError(t, repo.Create(org1))
	require.NoError(t, repo.Create(org2))
	require.NoError(t, repo.Create(org3))

	require.NoError(t, repo.SetMember(org1.ID, "user1", db.OrganizationRoleMember))
	require.NoError(t, repo.SetMember(org2.ID, "user1", db.OrganizationRoleMember))
	require.NoError(t, repo.SetMember(org3.ID, "user1", db.OrganizationRoleMember))

	memberships, err := repo.OfUser("user1")
	require.NoError(t, err)
	assert.Len(t, memberships, 3)

	// Verify ordering by name
	assert.Equal(t, "Apple Inc", memberships[0].Organization.Name)
	assert.Equal(t, "Banana LLC", memberships[1].Organization.Name)
	assert.Equal(t, "Zebra Corp", memberships[2].Organization.Name)
}

func TestOrganizationRepositoryDB_Logo(t *testing.T) {
	db.SetupTestDB(t.Name())
	repo := NewOrganizationRepositoryDB(db.GetConnection())

	org := &db.Organization{Name: "Logo Org"}
	require.NoError(t, repo.Create(org))

	_, err := repo.Logo(org.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "no logo yet")

	png := []byte("\x89PNG\r\n\x1a\nfake")
	require.NoError(t, repo.SetLogo(org.ID, "image/png", png))

	logo, err := repo.Logo(org.ID)
	require.NoError(t, err)
	assert.Equal(t, "image/png", logo.ContentType)
	assert.Equal(t, png, logo.Data)
	assert.False(t, logo.UpdatedAt.IsZero())

	// Reads of the organization say there is a logo but do not load it.
	got, err := repo.Get(org.ID)
	require.NoError(t, err)
	assert.True(t, got.HasLogo())
	assert.Nil(t, got.Logo)
	list, err := repo.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.True(t, list[0].HasLogo())
	assert.Nil(t, list[0].Logo)

	// Renaming does not touch the logo.
	got.Name = "Renamed"
	require.NoError(t, repo.Update(got))
	logo, err = repo.Logo(org.ID)
	require.NoError(t, err)
	assert.Equal(t, png, logo.Data)

	require.NoError(t, repo.ClearLogo(org.ID))
	_, err = repo.Logo(org.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	got, err = repo.Get(org.ID)
	require.NoError(t, err)
	assert.False(t, got.HasLogo())

	assert.ErrorIs(t, repo.SetLogo("missing", "image/png", png), gorm.ErrRecordNotFound)
	assert.ErrorIs(t, repo.ClearLogo("missing"), gorm.ErrRecordNotFound)
}
