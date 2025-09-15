package db

import "time"

// CreditSummary represents a user's current credit balance
type CreditSummary struct {
	UserID      string    `json:"user_id"`
	Balance     int       `json:"balance"`
	LastUpdated time.Time `json:"last_updated"`
}

// UserCreditSummary represents a user's credit balance with user information
type UserCreditSummary struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Balance     int       `json:"balance"`
	LastUpdated time.Time `json:"last_updated"`
}

// CreditHistoryResult represents paginated credit history
type CreditHistoryResult struct {
	UserID       string   `json:"user_id"`
	Balance      int      `json:"current_balance"`
	Transactions []Credit `json:"transactions"`
	TotalCount   int64    `json:"total_count"`
	Limit        int      `json:"limit"`
	Offset       int      `json:"offset"`
}

// AllUserCreditsResult represents paginated list of all users' credit balances
type AllUserCreditsResult struct {
	Users      []UserCreditSummary `json:"users"`
	TotalCount int64               `json:"total_count"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
}

// CreditsRepository interface for abstracting credit database operations
type CreditsRepository interface {
	// GetUserBalance gets the current credit balance for a user
	GetUserBalance(userID string) (*CreditSummary, error)

	// AdjustCredits adds or deducts credits for a user (thread-safe)
	// Returns the new credit transaction record and updated balance
	AdjustCredits(userID string, amount int, description string) (*Credit, error)

	// GetCreditHistory gets paginated credit transaction history for a user
	GetCreditHistory(userID string, limit, offset int) (*CreditHistoryResult, error)

	// GetAllUserCredits gets paginated list of all users' credit balances (admin only)
	GetAllUserCredits(limit, offset int) (*AllUserCreditsResult, error)

	// GetCreditTransaction gets a specific credit transaction by ID
	GetCreditTransaction(transactionID string) (*Credit, error)

	// GetUserTotalCreditsAdded gets the total amount of credits added for a user (excluding deductions)
	GetUserTotalCreditsAdded(userID string) (int, error)

	// GetUserTotalCreditsSpent gets the total amount of credits spent by a user (negative amounts)
	GetUserTotalCreditsSpent(userID string) (int, error)
}
