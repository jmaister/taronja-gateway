package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_CreateGrant(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	until := time.Now().UTC().Add(30 * 24 * time.Hour)
	grant := &db.BillingGrant{
		Subject:   "user123",
		Feature:   "premium",
		Until:     &until,
		Note:      "Trial grant",
		CreatedBy: "admin",
	}

	err := repo.CreateGrant(grant)
	require.NoError(t, err)
	assert.NotEmpty(t, grant.ID)
}

func TestBillingRepositoryDB_DeleteGrant(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	until := time.Now().UTC().Add(30 * 24 * time.Hour)
	grant := &db.BillingGrant{
		Subject:   "user123",
		Feature:   "premium",
		Until:     &until,
		CreatedBy: "admin",
	}
	require.NoError(t, repo.CreateGrant(grant))

	err := repo.DeleteGrant(grant.ID)
	require.NoError(t, err)

	// Verify it's gone
	grants, err := repo.ListGrants("user123")
	require.NoError(t, err)
	assert.Len(t, grants, 0)
}

func TestBillingRepositoryDB_DeleteGrant_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	err := repo.DeleteGrant("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_ListGrants_AllGrants(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	until := time.Now().UTC().Add(30 * 24 * time.Hour)
	grant1 := &db.BillingGrant{
		Subject:   "user1",
		Feature:   "feature1",
		Until:     &until,
		CreatedBy: "admin",
	}
	grant2 := &db.BillingGrant{
		Subject:   "user2",
		Feature:   "feature2",
		Until:     &until,
		CreatedBy: "admin",
	}
	require.NoError(t, repo.CreateGrant(grant1))
	require.NoError(t, repo.CreateGrant(grant2))

	grants, err := repo.ListGrants("")
	require.NoError(t, err)
	assert.Len(t, grants, 2)
	// Newest first
	assert.Equal(t, grant2.ID, grants[0].ID)
	assert.Equal(t, grant1.ID, grants[1].ID)
}

func TestBillingRepositoryDB_ListGrants_BySubject(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	until := time.Now().UTC().Add(30 * 24 * time.Hour)
	grant1 := &db.BillingGrant{
		Subject:   "user1",
		Feature:   "feature1",
		Until:     &until,
		CreatedBy: "admin",
	}
	grant2 := &db.BillingGrant{
		Subject:   "user2",
		Feature:   "feature2",
		Until:     &until,
		CreatedBy: "admin",
	}
	require.NoError(t, repo.CreateGrant(grant1))
	require.NoError(t, repo.CreateGrant(grant2))

	grants, err := repo.ListGrants("user1")
	require.NoError(t, err)
	assert.Len(t, grants, 1)
	assert.Equal(t, "user1", grants[0].Subject)
}

func TestBillingRepositoryDB_ListGrantsBySubjects(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	until := time.Now().UTC().Add(30 * 24 * time.Hour)
	grant1 := &db.BillingGrant{
		Subject:   "user1",
		Feature:   "feature1",
		Until:     &until,
		CreatedBy: "admin",
	}
	grant2 := &db.BillingGrant{
		Subject:   "user2",
		Feature:   "feature2",
		Until:     &until,
		CreatedBy: "admin",
	}
	grant3 := &db.BillingGrant{
		Subject:   "user3",
		Feature:   "feature3",
		Until:     &until,
		CreatedBy: "admin",
	}
	require.NoError(t, repo.CreateGrant(grant1))
	require.NoError(t, repo.CreateGrant(grant2))
	require.NoError(t, repo.CreateGrant(grant3))

	grants, err := repo.ListGrantsBySubjects([]string{"user1", "user3"})
	require.NoError(t, err)
	assert.Len(t, grants, 2)
}
