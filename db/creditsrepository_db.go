package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CreditsRepositoryDB implements CreditsRepository with a GORM database connection
type CreditsRepositoryDB struct {
	db *gorm.DB
}

// userCreditScan is a helper struct for scanning raw SQL results
type userCreditScan struct {
	UserID      string `gorm:"column:user_id"`
	Username    string `gorm:"column:username"`
	Email       string `gorm:"column:email"`
	Balance     int    `gorm:"column:balance"`
	LastUpdated string `gorm:"column:last_updated"`
}

// toUserCreditSummary converts userCreditScan to UserCreditSummary
func (u *userCreditScan) toUserCreditSummary() (*UserCreditSummary, error) {
	lastUpdated, err := time.Parse("2006-01-02 15:04:05.999999 -0700 MST", u.LastUpdated)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LastUpdated for user %s: %w", u.UserID, err)
	}
	return &UserCreditSummary{
		UserID:      u.UserID,
		Username:    u.Username,
		Email:       u.Email,
		Balance:     u.Balance,
		LastUpdated: lastUpdated,
	}, nil
}

// NewDBCreditsRepository creates a new database-backed credits repository
func NewDBCreditsRepository(db *gorm.DB) *CreditsRepositoryDB {
	return &CreditsRepositoryDB{
		db: db,
	}
}

// GetUserBalance gets the current credit balance for a user
func (r *CreditsRepositoryDB) GetUserBalance(userID string) (*CreditSummary, error) {
	var lastCredit Credit

	// Get the most recent credit transaction to get the current balance
	result := r.db.Where("user_id = ?", userID).Order("created_at desc").First(&lastCredit)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// No transactions found, user has 0 balance
		return &CreditSummary{
			UserID:      userID,
			Balance:     0,
			LastUpdated: lastCredit.CreatedAt, // Will be zero time
		}, nil
	}

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user balance: %w", result.Error)
	}

	return &CreditSummary{
		UserID:      userID,
		Balance:     lastCredit.BalanceAfter,
		LastUpdated: lastCredit.CreatedAt,
	}, nil
}

// AdjustCredits adds or deducts credits for a user (thread-safe)
func (r *CreditsRepositoryDB) AdjustCredits(userID string, amount int, description string) (*Credit, error) {
	var newCredit Credit

	// Use a transaction to ensure thread-safety
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Get current balance
		var currentBalance int
		var lastCredit Credit

		result := tx.Where("user_id = ?", userID).Order("created_at desc").First(&lastCredit)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// No previous transactions, starting balance is 0
			currentBalance = 0
		} else if result.Error != nil {
			return fmt.Errorf("failed to get current balance: %w", result.Error)
		} else {
			currentBalance = lastCredit.BalanceAfter
		}

		// Calculate new balance
		newBalance := currentBalance + amount

		// Prevent negative balance if deducting credits
		if newBalance < 0 {
			return errors.New("insufficient credits: transaction would result in negative balance")
		}

		// Create new credit transaction
		newCredit = Credit{
			UserID:       userID,
			Amount:       amount,
			BalanceAfter: newBalance,
			Description:  description,
		}

		if err := tx.Create(&newCredit).Error; err != nil {
			return fmt.Errorf("failed to create credit transaction: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &newCredit, nil
}

// GetCreditHistory gets paginated credit transaction history for a user
func (r *CreditsRepositoryDB) GetCreditHistory(userID string, limit, offset int) (*CreditHistoryResult, error) {
	var credits []Credit
	var totalCount int64

	// Get total count
	if err := r.db.Model(&Credit{}).Where("user_id = ?", userID).Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count credit transactions: %w", err)
	}

	// Get paginated transactions ordered by most recent first
	if err := r.db.Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(limit).
		Offset(offset).
		Find(&credits).Error; err != nil {
		return nil, fmt.Errorf("failed to get credit history: %w", err)
	}

	// Get current balance
	currentBalance := 0
	if len(credits) > 0 {
		// If we have transactions, get the most recent one's balance
		var lastCredit Credit
		if err := r.db.Where("user_id = ?", userID).Order("created_at desc").First(&lastCredit).Error; err == nil {
			currentBalance = lastCredit.BalanceAfter
		}
	}

	return &CreditHistoryResult{
		UserID:       userID,
		Balance:      currentBalance,
		Transactions: credits,
		TotalCount:   totalCount,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

// GetAllUserCredits gets paginated list of all users' credit balances (admin only)
func (r *CreditsRepositoryDB) GetAllUserCredits(limit, offset int) (*AllUserCreditsResult, error) {
	var scanResults []userCreditScan
	var totalCount int64

	// Get total count of users
	if err := r.db.Model(&User{}).Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// Query to get users with their latest credit balance
	// Using a simpler approach with correlated subquery instead of window functions
	query := `
		SELECT 
			u.id as user_id,
			u.username,
			u.email,
			COALESCE(
				(SELECT balance_after 
				 FROM credits c1 
				 WHERE c1.user_id = u.id AND c1.deleted_at IS NULL 
				 ORDER BY c1.created_at DESC 
				 LIMIT 1), 0) as balance,
			COALESCE(
				(SELECT c2.created_at 
				 FROM credits c2 
				 WHERE c2.user_id = u.id AND c2.deleted_at IS NULL 
				 ORDER BY c2.created_at DESC 
				 LIMIT 1), u.created_at) as last_updated
		FROM users u
		WHERE u.deleted_at IS NULL
		ORDER BY u.username
		LIMIT ? OFFSET ?
	`

	if err := r.db.Raw(query, limit, offset).Scan(&scanResults).Error; err != nil {
		return nil, fmt.Errorf("failed to get user credits: %w", err)
	}

	// Convert scan results to UserCreditSummary
	users := make([]UserCreditSummary, 0, len(scanResults))
	for _, scanResult := range scanResults {
		userSummary, err := scanResult.toUserCreditSummary()
		if err != nil {
			return nil, fmt.Errorf("failed to convert scan result for user %s: %w", scanResult.UserID, err)
		}
		users = append(users, *userSummary)
	}

	return &AllUserCreditsResult{
		Users:      users,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetCreditTransaction gets a specific credit transaction by ID
func (r *CreditsRepositoryDB) GetCreditTransaction(transactionID string) (*Credit, error) {
	var credit Credit

	if err := r.db.Preload("User").First(&credit, "id = ?", transactionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("credit transaction not found")
		}
		return nil, fmt.Errorf("failed to get credit transaction: %w", err)
	}

	return &credit, nil
}

// GetUserTotalCreditsAdded gets the total amount of credits added for a user (excluding deductions)
func (r *CreditsRepositoryDB) GetUserTotalCreditsAdded(userID string) (int, error) {
	var total int64

	if err := r.db.Model(&Credit{}).
		Where("user_id = ? AND amount > 0", userID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("failed to get total credits added: %w", err)
	}

	return int(total), nil
}

// GetUserTotalCreditsSpent gets the total amount of credits spent by a user (negative amounts)
func (r *CreditsRepositoryDB) GetUserTotalCreditsSpent(userID string) (int, error) {
	var total int64

	if err := r.db.Model(&Credit{}).
		Where("user_id = ? AND amount < 0", userID).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("failed to get total credits spent: %w", err)
	}

	return int(total), nil
}
