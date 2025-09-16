package db

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/lucsky/cuid"
)

// CreditsRepositoryMemory implements CreditsRepository with in-memory storage for testing
type CreditsRepositoryMemory struct {
	credits []Credit
	users   []User
	mutex   sync.RWMutex
}

// NewMemoryCreditsRepository creates a new in-memory credits repository
func NewMemoryCreditsRepository() *CreditsRepositoryMemory {
	return &CreditsRepositoryMemory{
		credits: make([]Credit, 0),
		users:   make([]User, 0),
	}
}

// SetUsers sets the users for testing purposes
func (r *CreditsRepositoryMemory) SetUsers(users []User) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.users = users
}

// GetUserBalance gets the current credit balance for a user
func (r *CreditsRepositoryMemory) GetUserBalance(userID string) (*CreditSummary, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Find the most recent credit transaction for this user
	var lastCredit *Credit
	for i := len(r.credits) - 1; i >= 0; i-- {
		if r.credits[i].UserID == userID && !r.credits[i].DeletedAt.Valid {
			lastCredit = &r.credits[i]
			break
		}
	}

	if lastCredit == nil {
		// No transactions found, user has 0 balance
		return &CreditSummary{
			UserID:      userID,
			Balance:     0,
			LastUpdated: time.Time{},
		}, nil
	}

	return &CreditSummary{
		UserID:      userID,
		Balance:     lastCredit.BalanceAfter,
		LastUpdated: lastCredit.CreatedAt,
	}, nil
}

// AdjustCredits adds or deducts credits for a user (thread-safe)
func (r *CreditsRepositoryMemory) AdjustCredits(userID string, amount int, description string) (*Credit, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Get current balance
	currentBalance := 0
	for i := len(r.credits) - 1; i >= 0; i-- {
		if r.credits[i].UserID == userID && !r.credits[i].DeletedAt.Valid {
			currentBalance = r.credits[i].BalanceAfter
			break
		}
	}

	// Calculate new balance
	newBalance := currentBalance + amount

	// Prevent negative balance if deducting credits
	if newBalance < 0 {
		return nil, errors.New("insufficient credits: transaction would result in negative balance")
	}

	// Generate ID
	id := cuid.New()

	// Create new credit transaction
	newCredit := Credit{
		ID:           id,
		UserID:       userID,
		Amount:       amount,
		BalanceAfter: newBalance,
		Description:  description,
	}
	// Set timestamps via gorm.Model
	newCredit.Model.CreatedAt = time.Now()
	newCredit.Model.UpdatedAt = time.Now()

	r.credits = append(r.credits, newCredit)

	return &newCredit, nil
}

// GetCreditHistory gets paginated credit transaction history for a user
func (r *CreditsRepositoryMemory) GetCreditHistory(userID string, limit, offset int) (*CreditHistoryResult, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Filter and sort credits for this user
	var userCredits []Credit
	for _, credit := range r.credits {
		if credit.UserID == userID && !credit.DeletedAt.Valid {
			userCredits = append(userCredits, credit)
		}
	}

	// Sort by creation time, most recent first
	sort.Slice(userCredits, func(i, j int) bool {
		return userCredits[i].CreatedAt.After(userCredits[j].CreatedAt)
	})

	totalCount := int64(len(userCredits))

	// Apply pagination
	start := offset
	end := offset + limit
	if start > len(userCredits) {
		start = len(userCredits)
	}
	if end > len(userCredits) {
		end = len(userCredits)
	}

	paginatedCredits := userCredits[start:end]

	// Get current balance
	currentBalance := 0
	if len(userCredits) > 0 {
		currentBalance = userCredits[0].BalanceAfter
	}

	return &CreditHistoryResult{
		UserID:       userID,
		Balance:      currentBalance,
		Transactions: paginatedCredits,
		TotalCount:   totalCount,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

// GetAllUserCredits gets paginated list of all users' credit balances (admin only)
func (r *CreditsRepositoryMemory) GetAllUserCredits(limit, offset int) (*AllUserCreditsResult, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var userSummaries []UserCreditSummary

	// For each user, get their latest credit balance
	for _, user := range r.users {
		balance := 0
		lastUpdated := user.CreatedAt

		// Find the most recent credit transaction for this user
		for i := len(r.credits) - 1; i >= 0; i-- {
			if r.credits[i].UserID == user.ID && !r.credits[i].DeletedAt.Valid {
				balance = r.credits[i].BalanceAfter
				lastUpdated = r.credits[i].CreatedAt
				break
			}
		}

		userSummaries = append(userSummaries, UserCreditSummary{
			UserID:      user.ID,
			Username:    user.Username,
			Email:       user.Email,
			Balance:     balance,
			LastUpdated: lastUpdated,
		})
	}

	// Sort by username
	sort.Slice(userSummaries, func(i, j int) bool {
		return userSummaries[i].Username < userSummaries[j].Username
	})

	totalCount := int64(len(userSummaries))

	// Apply pagination
	start := offset
	end := offset + limit
	if start > len(userSummaries) {
		start = len(userSummaries)
	}
	if end > len(userSummaries) {
		end = len(userSummaries)
	}

	paginatedUsers := userSummaries[start:end]

	return &AllUserCreditsResult{
		Users:      paginatedUsers,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetCreditTransaction gets a specific credit transaction by ID
func (r *CreditsRepositoryMemory) GetCreditTransaction(transactionID string) (*Credit, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, credit := range r.credits {
		if credit.ID == transactionID && !credit.DeletedAt.Valid {
			// Find and set the user
			for _, user := range r.users {
				if user.ID == credit.UserID {
					credit.User = user
					break
				}
			}
			return &credit, nil
		}
	}

	return nil, errors.New("credit transaction not found")
}

// GetUserTotalCreditsAdded gets the total amount of credits added for a user (excluding deductions)
func (r *CreditsRepositoryMemory) GetUserTotalCreditsAdded(userID string) (int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	total := 0
	for _, credit := range r.credits {
		if credit.UserID == userID && credit.Amount > 0 && !credit.DeletedAt.Valid {
			total += credit.Amount
		}
	}

	return total, nil
}

// GetUserTotalCreditsSpent gets the total amount of credits spent by a user (negative amounts)
func (r *CreditsRepositoryMemory) GetUserTotalCreditsSpent(userID string) (int, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	total := 0
	for _, credit := range r.credits {
		if credit.UserID == userID && credit.Amount < 0 && !credit.DeletedAt.Valid {
			total += -credit.Amount // Convert to positive number
		}
	}

	return total, nil
}
