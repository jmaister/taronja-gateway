package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreditsRepositoryMemory(t *testing.T) {
	// Create repository instance
	repo := NewMemoryCreditsRepository()

	// Create test users
	testUsers := []User{
		{
			ID:       "user1",
			Username: "testuser1",
			Email:    "user1@example.com",
		},
		{
			ID:       "user2",
			Username: "testuser2",
			Email:    "user2@example.com",
		},
	}
	repo.SetUsers(testUsers)

	t.Run("GetUserBalance - New User", func(t *testing.T) {
		// Test getting balance for a user with no transactions
		balance, err := repo.GetUserBalance("user1")
		require.NoError(t, err)
		assert.Equal(t, "user1", balance.UserID)
		assert.Equal(t, 0, balance.Balance)
	})

	t.Run("AdjustCredits - Add Credits", func(t *testing.T) {
		// Add 100 credits to user1
		transaction, err := repo.AdjustCredits("user1", 100, "Initial credit grant")
		require.NoError(t, err)
		assert.Equal(t, "user1", transaction.UserID)
		assert.Equal(t, 100, transaction.Amount)
		assert.Equal(t, 100, transaction.BalanceAfter)
		assert.Equal(t, "Initial credit grant", transaction.Description)
		assert.NotEmpty(t, transaction.ID)

		// Verify balance
		balance, err := repo.GetUserBalance("user1")
		require.NoError(t, err)
		assert.Equal(t, 100, balance.Balance)
	})

	t.Run("AdjustCredits - Deduct Credits", func(t *testing.T) {
		// Deduct 30 credits from user1 (should have 70 left)
		transaction, err := repo.AdjustCredits("user1", -30, "Purchase item")
		require.NoError(t, err)
		assert.Equal(t, "user1", transaction.UserID)
		assert.Equal(t, -30, transaction.Amount)
		assert.Equal(t, 70, transaction.BalanceAfter)
		assert.Equal(t, "Purchase item", transaction.Description)

		// Verify balance
		balance, err := repo.GetUserBalance("user1")
		require.NoError(t, err)
		assert.Equal(t, 70, balance.Balance)
	})

	t.Run("AdjustCredits - Insufficient Balance", func(t *testing.T) {
		// Try to deduct more credits than available
		_, err := repo.AdjustCredits("user1", -100, "Large purchase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient credits")

		// Verify balance unchanged
		balance, err := repo.GetUserBalance("user1")
		require.NoError(t, err)
		assert.Equal(t, 70, balance.Balance)
	})

	t.Run("GetCreditHistory", func(t *testing.T) {
		// Get credit history for user1
		history, err := repo.GetCreditHistory("user1", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, "user1", history.UserID)
		assert.Equal(t, 70, history.Balance)
		assert.Equal(t, int64(2), history.TotalCount) // 2 transactions
		assert.Len(t, history.Transactions, 2)
		assert.Equal(t, 10, history.Limit)
		assert.Equal(t, 0, history.Offset)

		// Verify transactions are in descending order (most recent first)
		assert.True(t, history.Transactions[0].CreatedAt.After(history.Transactions[1].CreatedAt) ||
			history.Transactions[0].CreatedAt.Equal(history.Transactions[1].CreatedAt))
	})

	t.Run("GetAllUserCredits", func(t *testing.T) {
		// Add some credits to user2
		_, err := repo.AdjustCredits("user2", 50, "Welcome bonus")
		require.NoError(t, err)

		// Get all user credits
		result, err := repo.GetAllUserCredits(10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(2), result.TotalCount)
		assert.Len(t, result.Users, 2)
		assert.Equal(t, 10, result.Limit)
		assert.Equal(t, 0, result.Offset)

		// Find user1 and user2 in results
		var user1Found, user2Found bool
		for _, user := range result.Users {
			switch user.UserID {
			case "user1":
				user1Found = true
				assert.Equal(t, "testuser1", user.Username)
				assert.Equal(t, "user1@example.com", user.Email)
				assert.Equal(t, 70, user.Balance)
			case "user2":
				user2Found = true
				assert.Equal(t, "testuser2", user.Username)
				assert.Equal(t, "user2@example.com", user.Email)
				assert.Equal(t, 50, user.Balance)
			}
		}
		assert.True(t, user1Found, "user1 should be found")
		assert.True(t, user2Found, "user2 should be found")
	})

	t.Run("GetUserTotalCreditsAdded", func(t *testing.T) {
		total, err := repo.GetUserTotalCreditsAdded("user1")
		require.NoError(t, err)
		assert.Equal(t, 100, total) // Only the initial 100 credits added
	})

	t.Run("GetUserTotalCreditsSpent", func(t *testing.T) {
		total, err := repo.GetUserTotalCreditsSpent("user1")
		require.NoError(t, err)
		assert.Equal(t, 30, total) // 30 credits spent (converted to positive)
	})

	t.Run("GetCreditTransaction", func(t *testing.T) {
		// First add a transaction
		transaction, err := repo.AdjustCredits("user1", 25, "Test transaction")
		require.NoError(t, err)

		// Retrieve it by ID
		retrieved, err := repo.GetCreditTransaction(transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, transaction.ID, retrieved.ID)
		assert.Equal(t, transaction.UserID, retrieved.UserID)
		assert.Equal(t, transaction.Amount, retrieved.Amount)
		assert.Equal(t, transaction.Description, retrieved.Description)
	})

	t.Run("GetCreditTransaction - Not Found", func(t *testing.T) {
		_, err := repo.GetCreditTransaction("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Pagination", func(t *testing.T) {
		// Add more transactions to test pagination
		for i := 0; i < 5; i++ {
			_, err := repo.AdjustCredits("user1", 10, "Pagination test")
			require.NoError(t, err)
		}

		// Test first page
		history, err := repo.GetCreditHistory("user1", 3, 0)
		require.NoError(t, err)
		assert.Len(t, history.Transactions, 3)
		assert.Equal(t, 3, history.Limit)
		assert.Equal(t, 0, history.Offset)

		// Test second page
		history2, err := repo.GetCreditHistory("user1", 3, 3)
		require.NoError(t, err)
		assert.Len(t, history2.Transactions, 3)
		assert.Equal(t, 3, history2.Limit)
		assert.Equal(t, 3, history2.Offset)
	})
}
