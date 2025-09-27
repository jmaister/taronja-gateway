package db

import (
	"testing"
	"time"

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
		assert.False(t, balance.HasHistory, "User with no transactions should have HasHistory=false")
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
		assert.True(t, balance.HasHistory, "User with transactions should have HasHistory=true")
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

	t.Run("HasHistory_Flag_Tests", func(t *testing.T) {
		// Add a small delay to ensure test isolation
		time.Sleep(1 * time.Millisecond)
		// Create another test user for isolated testing
		user2 := User{
			Username: "testuser2_hashistory",
			Email:    "user2_hashistory@example.com",
		}
		require.NoError(t, dbConn.Create(&user2).Error)

		testCounterID := "tokens_hashistory"

		// Test 1: User with no transactions should have HasHistory=false
		balance, err := repo.GetUserBalance(user2.ID, testCounterID)
		assert.NoError(t, err)
		assert.Equal(t, 0, balance.Balance)
		assert.False(t, balance.HasHistory, "New user should have HasHistory=false")

		// Test 2: User with transactions should have HasHistory=true
		_, err = repo.AdjustCounters(user2.ID, testCounterID, 50, "Initial tokens")
		assert.NoError(t, err)

		balance, err = repo.GetUserBalance(user2.ID, testCounterID)
		assert.NoError(t, err)
		assert.Equal(t, 50, balance.Balance)
		assert.True(t, balance.HasHistory, "User with transactions should have HasHistory=true")

		// Test 3: User with zero balance but transaction history should have HasHistory=true
		// Add a small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
		_, err = repo.AdjustCounters(user2.ID, testCounterID, -50, "Spend all tokens")
		assert.NoError(t, err)

		balance, err = repo.GetUserBalance(user2.ID, testCounterID)
		assert.Equal(t, 0, balance.Balance)
		assert.True(t, balance.HasHistory, "User with zero balance but transaction history should have HasHistory=true")
	})

	t.Run("GetCounterHistory_HasHistory_Flag", func(t *testing.T) {
		// Create another test user for isolated testing
		user3 := User{
			Username: "testuser3_counterhistory",
			Email:    "user3_counterhistory@example.com",
		}
		require.NoError(t, dbConn.Create(&user3).Error)

		testCounterID := "gems_counterhistory"

		// Test 1: User with no transactions - GetCounterHistory should have HasHistory=false
		history, err := repo.GetCounterHistory(user3.ID, testCounterID, 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, user3.ID, history.UserID)
		assert.Equal(t, testCounterID, history.CounterID)
		assert.Equal(t, 0, history.Balance)
		assert.False(t, history.HasHistory, "User with no transactions should have HasHistory=false in history")
		assert.Equal(t, int64(0), history.TotalCount)
		assert.Empty(t, history.Transactions)

		// Test 2: Add transactions and verify HasHistory=true
		_, err = repo.AdjustCounters(user3.ID, testCounterID, 25, "Initial gems")
		assert.NoError(t, err)
		// Add a small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
		_, err = repo.AdjustCounters(user3.ID, testCounterID, 10, "Bonus gems")
		assert.NoError(t, err)

		history, err = repo.GetCounterHistory(user3.ID, testCounterID, 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, 35, history.Balance)
		assert.True(t, history.HasHistory, "User with transactions should have HasHistory=true in history")
		assert.Equal(t, int64(2), history.TotalCount)
		assert.Len(t, history.Transactions, 2)
	})

	t.Run("GetAllUserCounters_HasHistory_Flag", func(t *testing.T) {
		// Create test users for isolated testing
		userNoHistory := User{
			Username: "user_no_history_allusers",
			Email:    "nohistory_allusers@example.com",
		}
		require.NoError(t, dbConn.Create(&userNoHistory).Error)

		userWithHistory := User{
			Username: "user_with_history_allusers",
			Email:    "withhistory_allusers@example.com",
		}
		require.NoError(t, dbConn.Create(&userWithHistory).Error)

		testCounterID := "coins_allusers"

		// Add transactions only for userWithHistory
		_, err := repo.AdjustCounters(userWithHistory.ID, testCounterID, 75, "Initial coins")
		assert.NoError(t, err)

		// Get all user counters and verify HasHistory flags
		result, err := repo.GetAllUserCounters(testCounterID, 10, 0)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, testCounterID, result.CounterID)

		// Find our test users in the results
		var noHistoryUser, withHistoryUser *UserCounterSummary
		for i := range result.Users {
			user := &result.Users[i]
			if user.UserID == userNoHistory.ID {
				noHistoryUser = user
			}
			if user.UserID == userWithHistory.ID {
				withHistoryUser = user
			}
		}

		// Verify user with no history
		assert.NotNil(t, noHistoryUser, "User with no history should be in results")
		if noHistoryUser != nil {
			assert.Equal(t, 0, noHistoryUser.Balance)
			assert.False(t, noHistoryUser.HasHistory, "User with no transactions should have HasHistory=false in GetAllUserCounters")
		}

		// Verify user with history
		assert.NotNil(t, withHistoryUser, "User with history should be in results")
		if withHistoryUser != nil {
			assert.Equal(t, 75, withHistoryUser.Balance)
			assert.True(t, withHistoryUser.HasHistory, "User with transactions should have HasHistory=true in GetAllUserCounters")
		}
	})

	t.Run("GetAllUserCounters_RealWorld_Debug", func(t *testing.T) {
		// Debug test to reproduce the issue where user has balance but has_history is false
		debugUser := User{
			Username: "debug_user",
			Email:    "debug@example.com",
		}
		require.NoError(t, dbConn.Create(&debugUser).Error)

		testCounterID := "debug_pepes"

		// Add a large balance like in the real example
		_, err := repo.AdjustCounters(debugUser.ID, testCounterID, 10000, "Large balance test")
		assert.NoError(t, err)

		// Verify the individual balance query works correctly
		balance, err := repo.GetUserBalance(debugUser.ID, testCounterID)
		assert.NoError(t, err)
		assert.Equal(t, 10000, balance.Balance)
		assert.True(t, balance.HasHistory, "Individual balance query should show has_history=true")

		// Now test the GetAllUserCounters query (this was failing before the fix)
		result, err := repo.GetAllUserCounters(testCounterID, 10, 0)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Find our debug user in the results
		var debugUserResult *UserCounterSummary
		for i := range result.Users {
			user := &result.Users[i]
			if user.UserID == debugUser.ID {
				debugUserResult = user
				break
			}
		}

		// This should now pass after fixing the SQLite boolean scanning issue
		assert.NotNil(t, debugUserResult, "Debug user should be in results")
		if debugUserResult != nil {
			assert.Equal(t, 10000, debugUserResult.Balance, "Balance should be 10000")
			assert.True(t, debugUserResult.HasHistory, "User with 10000 balance should have HasHistory=true (this was the bug)")
		}
	})
}
