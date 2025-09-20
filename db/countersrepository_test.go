package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountersRepository(t *testing.T) {
	SetupTestDB(t.Name()) // This line is retained as per the goal to update test DB setup
	dbConn := GetConnection()
	repo := NewDBCountersRepository(dbConn)

	// Create test users in the database
	user1 := User{
		Username: "testuser1",
		Email:    "user1@example.com",
	}
	require.NoError(t, dbConn.Create(&user1).Error)

	// Test counter ID
	counterID := "credits"

	t.Run("GetUserBalance_NoTransactions", func(t *testing.T) {
		// Test getting balance for user with no transactions
		balance, err := repo.GetUserBalance(user1.ID, counterID)
		assert.NoError(t, err)
		assert.Equal(t, user1.ID, balance.UserID)
		assert.Equal(t, counterID, balance.CounterID)
		assert.Equal(t, 0, balance.Balance)
	})

	t.Run("AdjustCounters_Basic", func(t *testing.T) {
		// Add 100 credits
		transaction, err := repo.AdjustCounters(user1.ID, counterID, 100, "Initial credit grant")
		assert.NoError(t, err)
		assert.Equal(t, user1.ID, transaction.UserID)
		assert.Equal(t, counterID, transaction.CounterID)
		assert.Equal(t, 100, transaction.Amount)
		assert.Equal(t, 100, transaction.BalanceAfter)
		assert.Equal(t, "Initial credit grant", transaction.Description)

		// Verify balance
		balance, err := repo.GetUserBalance(user1.ID, counterID)
		assert.NoError(t, err)
		assert.Equal(t, 100, balance.Balance)
	})

	t.Run("MultipleCounterTypes", func(t *testing.T) {
		pointsCounterID := "points"

		// Add points
		_, err := repo.AdjustCounters(user1.ID, pointsCounterID, 200, "Initial points")
		assert.NoError(t, err)

		// Verify balances are separate
		creditsBalance, err := repo.GetUserBalance(user1.ID, counterID)
		assert.NoError(t, err)
		assert.Equal(t, 100, creditsBalance.Balance)

		pointsBalance, err := repo.GetUserBalance(user1.ID, pointsCounterID)
		assert.NoError(t, err)
		assert.Equal(t, 200, pointsBalance.Balance)
	})
}
