package repositories

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingRepositoryDB_SavePlan(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	plan := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "basic",
			Name:        "Basic Plan",
			Kind:        db.PlanKindSubscription,
			AmountCents: 1000,
			Currency:    "eur",
		},
		Features: []string{"feature1", "feature2"},
	}

	err := repo.SavePlan(plan)
	require.NoError(t, err)

	// Verify it was saved
	retrieved, err := repo.GetPlan("basic")
	require.NoError(t, err)
	assert.Equal(t, "basic", retrieved.Key)
	assert.Equal(t, "Basic Plan", retrieved.Name)
	assert.Len(t, retrieved.Features, 2)
	assert.Equal(t, "feature1", retrieved.Features[0])
	assert.Equal(t, "feature2", retrieved.Features[1])
}

func TestBillingRepositoryDB_SavePlan_ReplaceFeatures(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	plan1 := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "basic",
			Name:        "Basic Plan",
			Kind:        db.PlanKindSubscription,
			AmountCents: 1000,
			Currency:    "eur",
		},
		Features: []string{"feature1", "feature2"},
	}
	require.NoError(t, repo.SavePlan(plan1))

	// Save again with different features
	plan2 := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "basic",
			Name:        "Basic Plan",
			Kind:        db.PlanKindSubscription,
			AmountCents: 1000,
			Currency:    "eur",
		},
		Features: []string{"feature3", "feature4", "feature5"},
	}
	require.NoError(t, repo.SavePlan(plan2))

	// Verify features were replaced
	retrieved, err := repo.GetPlan("basic")
	require.NoError(t, err)
	assert.Len(t, retrieved.Features, 3)
	assert.Equal(t, "feature3", retrieved.Features[0])
	assert.Equal(t, "feature4", retrieved.Features[1])
	assert.Equal(t, "feature5", retrieved.Features[2])
}

func TestBillingRepositoryDB_GetPlan(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	plan := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "pro",
			Name:        "Pro Plan",
			Kind:        db.PlanKindSubscription,
			AmountCents: 5000,
			Currency:    "eur",
		},
		Features: []string{"feature_a", "feature_b"},
	}
	require.NoError(t, repo.SavePlan(plan))

	retrieved, err := repo.GetPlan("pro")
	require.NoError(t, err)
	assert.Equal(t, "pro", retrieved.Key)
	assert.Equal(t, "Pro Plan", retrieved.Name)
	assert.Len(t, retrieved.Features, 2)
}

func TestBillingRepositoryDB_GetPlan_NotFound(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	_, err := repo.GetPlan("nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBillingRepositoryDB_ListPlans(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	plan1 := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "basic",
			Name:        "Basic",
			Kind:        db.PlanKindSubscription,
			AmountCents: 1000,
			Currency:    "eur",
			Active:      true,
		},
		Features: []string{"f1"},
	}
	plan2 := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "pro",
			Name:        "Pro",
			Kind:        db.PlanKindSubscription,
			AmountCents: 5000,
			Currency:    "eur",
			Active:      true,
		},
		Features: []string{"f2", "f3"},
	}

	require.NoError(t, repo.SavePlan(plan1))
	require.NoError(t, repo.SavePlan(plan2))

	plans, err := repo.ListPlans(false)
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestBillingRepositoryDB_ListPlans_ActiveOnly(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	plan1 := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "basic",
			Name:        "Basic",
			Kind:        db.PlanKindSubscription,
			AmountCents: 1000,
			Currency:    "eur",
			Active:      true,
		},
		Features: []string{},
	}
	plan2 := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "archived",
			Name:        "Archived",
			Kind:        db.PlanKindSubscription,
			AmountCents: 5000,
			Currency:    "eur",
			Active:      false,
		},
		Features: []string{},
	}

	require.NoError(t, repo.SavePlan(plan1))
	require.NoError(t, repo.SavePlan(plan2))

	plans, err := repo.ListPlans(true)
	require.NoError(t, err)
	assert.Len(t, plans, 1)
	assert.Equal(t, "basic", plans[0].Key)
}

func TestBillingRepositoryDB_ListPlans_FeaturesOrdered(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	plan := &BillingPlanWithFeatures{
		BillingPlan: db.BillingPlan{
			Key:         "test",
			Name:        "Test",
			Kind:        db.PlanKindSubscription,
			AmountCents: 1000,
			Currency:    "eur",
		},
		Features: []string{"zebra", "apple", "banana"},
	}
	require.NoError(t, repo.SavePlan(plan))

	plans, err := repo.ListPlans(false)
	require.NoError(t, err)
	assert.Len(t, plans, 1)
	// Features should be alphabetically ordered
	assert.Equal(t, "apple", plans[0].Features[0])
	assert.Equal(t, "banana", plans[0].Features[1])
	assert.Equal(t, "zebra", plans[0].Features[2])
}
