package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CounterSummary represents a user's current counter balance
type CounterSummary struct {
	UserID      string    `json:"user_id"`
	CounterID   string    `json:"counter_id"`
	Balance     int       `json:"balance"`
	LastUpdated time.Time `json:"last_updated"`
	HasHistory  bool      `json:"has_history"` // false when user has no transactions and balance is default 0
}

// UserCounterSummary represents a user's counter balance with user information
type UserCounterSummary struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	CounterID   string    `json:"counter_id"`
	Balance     int       `json:"balance"`
	LastUpdated time.Time `json:"last_updated"`
	HasHistory  bool      `json:"has_history"` // false when user has no transactions and balance is default 0
}

// CounterHistoryResult represents paginated counter history
type CounterHistoryResult struct {
	UserID       string    `json:"user_id"`
	CounterID    string    `json:"counter_id"`
	Balance      int       `json:"current_balance"`
	HasHistory   bool      `json:"has_history"` // false when user has no transactions and balance is default 0
	Transactions []Counter `json:"transactions"`
	TotalCount   int64     `json:"total_count"`
	Limit        int       `json:"limit"`
	Offset       int       `json:"offset"`
}

// AllUserCountersResult represents paginated list of all users' counter balances
type AllUserCountersResult struct {
	CounterID  string               `json:"counter_id"`
	Users      []UserCounterSummary `json:"users"`
	TotalCount int64                `json:"total_count"`
	Limit      int                  `json:"limit"`
	Offset     int                  `json:"offset"`
}

// CountersRepository interface for abstracting counter database operations
type CountersRepository interface {
	// GetUserBalance gets the current counter balance for a user
	GetUserBalance(userID, counterID string) (*CounterSummary, error)

	// AdjustCounters adds or deducts counters for a user (thread-safe)
	// Returns the new counter transaction record and updated balance
	AdjustCounters(userID, counterID string, amount int, description string) (*Counter, error)

	// GetCounterHistory gets paginated counter transaction history for a user
	GetCounterHistory(userID, counterID string, limit, offset int) (*CounterHistoryResult, error)

	// GetAllUserCounters gets paginated list of all users' counter balances (admin only)
	GetAllUserCounters(counterID string, limit, offset int) (*AllUserCountersResult, error)

	// GetCounterTransaction gets a specific counter transaction by ID
	GetCounterTransaction(transactionID string) (*Counter, error)

	// GetAvailableCounterTypes gets a list of all available counter type IDs
	GetAvailableCounterTypes() ([]string, error)
}

// CountersRepositoryDB implements CountersRepository with a GORM database connection
type CountersRepositoryDB struct {
	db *gorm.DB
}

// userCounterScan is a helper struct for scanning raw SQL results
type userCounterScan struct {
	UserID      string `gorm:"column:user_id"`
	Username    string `gorm:"column:username"`
	Email       string `gorm:"column:email"`
	CounterID   string `gorm:"column:counter_id"`
	Balance     int    `gorm:"column:balance"`
	LastUpdated string `gorm:"column:last_updated"`
	HasHistory  int    `gorm:"column:has_history"` // SQLite returns 0/1 for boolean, convert to bool later
}

// toUserCounterSummary converts userCounterScan to UserCounterSummary
func (u *userCounterScan) toUserCounterSummary() (*UserCounterSummary, error) {
	// Try parsing with RFC3339 first (SQLite default), then fallback to Go format
	var lastUpdated time.Time
	var err error
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999 -0700 MST",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		lastUpdated, err = time.Parse(layout, u.LastUpdated)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse LastUpdated for user %s: %w", u.UserID, err)
	}
	return &UserCounterSummary{
		UserID:      u.UserID,
		Username:    u.Username,
		Email:       u.Email,
		CounterID:   u.CounterID,
		Balance:     u.Balance,
		LastUpdated: lastUpdated,
		HasHistory:  u.HasHistory > 0, // Convert SQLite integer (0/1) to boolean
	}, nil
}

// NewDBCountersRepository creates a new database-backed counters repository
func NewDBCountersRepository(db *gorm.DB) *CountersRepositoryDB {
	return &CountersRepositoryDB{
		db: db,
	}
}

// GetUserBalance gets the current counter balance for a user
func (r *CountersRepositoryDB) GetUserBalance(userID, counterID string) (*CounterSummary, error) {
	var lastCounter Counter

	// Get the most recent counter transaction to get the current balance
	result := r.db.Where("user_id = ? AND counter_id = ?", userID, counterID).Order("created_at desc").First(&lastCounter)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// No transactions found, user has 0 balance
		return &CounterSummary{
			UserID:      userID,
			CounterID:   counterID,
			Balance:     0,
			LastUpdated: lastCounter.CreatedAt, // Will be zero time
			HasHistory:  false,                 // User has no transaction history
		}, nil
	}

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user balance: %w", result.Error)
	}

	return &CounterSummary{
		UserID:      userID,
		CounterID:   counterID,
		Balance:     lastCounter.BalanceAfter,
		LastUpdated: lastCounter.CreatedAt,
		HasHistory:  true, // User has transaction history
	}, nil
}

// AdjustCounters adds or deducts counters for a user (thread-safe)
func (r *CountersRepositoryDB) AdjustCounters(userID, counterID string, amount int, description string) (*Counter, error) {
	var newCounter Counter

	// Use a transaction to ensure thread-safety
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Get current balance
		var currentBalance int
		var lastCounter Counter

		result := tx.Where("user_id = ? AND counter_id = ?", userID, counterID).Order("created_at desc").First(&lastCounter)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// No previous transactions, starting balance is 0
			currentBalance = 0
		} else if result.Error != nil {
			return fmt.Errorf("failed to get current balance: %w", result.Error)
		} else {
			currentBalance = lastCounter.BalanceAfter
		}

		// Calculate new balance
		newBalance := currentBalance + amount

		// Prevent negative balance if deducting counters
		// TODO: configure if it can go negative?
		if newBalance < 0 {
			return errors.New("insufficient counters: transaction would result in negative balance")
		}

		// Create new counter transaction
		newCounter = Counter{
			UserID:       userID,
			CounterID:    counterID,
			Amount:       amount,
			BalanceAfter: newBalance,
			Description:  description,
		}

		if err := tx.Create(&newCounter).Error; err != nil {
			return fmt.Errorf("failed to create counter transaction: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &newCounter, nil
}

// GetCounterHistory gets paginated counter transaction history for a user
func (r *CountersRepositoryDB) GetCounterHistory(userID, counterID string, limit, offset int) (*CounterHistoryResult, error) {
	var counters []Counter
	var totalCount int64

	// Get total count
	if err := r.db.Model(&Counter{}).Where("user_id = ? AND counter_id = ?", userID, counterID).Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count counter transactions: %w", err)
	}

	// Get paginated transactions ordered by most recent first
	if err := r.db.Where("user_id = ? AND counter_id = ?", userID, counterID).
		Order("created_at desc").
		Limit(limit).
		Offset(offset).
		Find(&counters).Error; err != nil {
		return nil, fmt.Errorf("failed to get counter history: %w", err)
	}

	// Get current balance
	currentBalance := 0
	hasHistory := totalCount > 0
	if len(counters) > 0 {
		// If we have transactions, get the most recent one's balance
		var lastCounter Counter
		if err := r.db.Where("user_id = ? AND counter_id = ?", userID, counterID).Order("created_at desc").First(&lastCounter).Error; err == nil {
			currentBalance = lastCounter.BalanceAfter
		}
	}

	return &CounterHistoryResult{
		UserID:       userID,
		CounterID:    counterID,
		Balance:      currentBalance,
		HasHistory:   hasHistory,
		Transactions: counters,
		TotalCount:   totalCount,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

// GetAllUserCounters gets paginated list of all users' counter balances (admin only)
func (r *CountersRepositoryDB) GetAllUserCounters(counterID string, limit, offset int) (*AllUserCountersResult, error) {
	var scanResults []userCounterScan
	var totalCount int64

	// Get total count of users
	if err := r.db.Model(&User{}).Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// Query to get users with their latest counter balance
	// Using a simpler approach with correlated subquery instead of window functions
	query := `
		SELECT 
			u.id as user_id,
			u.username,
			u.email,
			? as counter_id,
			COALESCE(
				(SELECT balance_after 
				 FROM counters c1 
				 WHERE c1.user_id = u.id AND c1.counter_id = ? AND c1.deleted_at IS NULL 
				 ORDER BY c1.created_at DESC 
				 LIMIT 1), 0) as balance,
			COALESCE(
				(SELECT c2.created_at 
				 FROM counters c2 
				 WHERE c2.user_id = u.id AND c2.counter_id = ? AND c2.deleted_at IS NULL 
				 ORDER BY c2.created_at DESC 
				 LIMIT 1), u.created_at) as last_updated,
			CASE WHEN EXISTS(
				SELECT 1 FROM counters c3 
				WHERE c3.user_id = u.id AND c3.counter_id = ? AND c3.deleted_at IS NULL
			) THEN 1 ELSE 0 END as has_history
		FROM users u
		WHERE u.deleted_at IS NULL
		ORDER BY u.username
		LIMIT ? OFFSET ?
	`

	if err := r.db.Raw(query, counterID, counterID, counterID, counterID, limit, offset).Scan(&scanResults).Error; err != nil {
		return nil, fmt.Errorf("failed to get user counters: %w", err)
	}

	// Convert scan results to UserCounterSummary
	users := make([]UserCounterSummary, 0, len(scanResults))
	for _, scanResult := range scanResults {
		userSummary, err := scanResult.toUserCounterSummary()
		if err != nil {
			return nil, fmt.Errorf("failed to convert scan result for user %s: %w", scanResult.UserID, err)
		}
		users = append(users, *userSummary)
	}

	return &AllUserCountersResult{
		CounterID:  counterID,
		Users:      users,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetCounterTransaction gets a specific counter transaction by ID
func (r *CountersRepositoryDB) GetCounterTransaction(transactionID string) (*Counter, error) {
	var counter Counter

	if err := r.db.Preload("User").First(&counter, "id = ?", transactionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("counter transaction not found")
		}
		return nil, fmt.Errorf("failed to get counter transaction: %w", err)
	}

	return &counter, nil
}

// GetAvailableCounterTypes gets a list of all available counter type IDs
func (r *CountersRepositoryDB) GetAvailableCounterTypes() ([]string, error) {
	var counterTypes []string

	if err := r.db.Model(&Counter{}).
		Distinct("counter_id").
		Order("counter_id").
		Pluck("counter_id", &counterTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to get available counter types: %w", err)
	}

	return counterTypes, nil
}
