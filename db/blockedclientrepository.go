package db

import "gorm.io/gorm"

// BlockedClientRepository defines the interface for persisting and querying
// the rate-limiter's block-event history — see BlockedClient's doc comment
// for why this exists separately from the in-memory RateLimiter's own,
// short-lived tracking.
type BlockedClientRepository interface {
	// Create persists one block event. Called from
	// middleware/ratelimiter.go's Handler in a background goroutine —
	// never on the hot path of the request that's actively being
	// rejected with a 429.
	Create(bc *BlockedClient) error
	// List returns block events, most recent first, optionally filtered
	// to a single IP address (pass "" for no filter), along with the
	// total count matching that filter (ignoring limit/offset) for
	// pagination.
	List(ip string, limit, offset int) ([]BlockedClient, int64, error)
}

// BlockedClientRepositoryDB is the GORM-backed implementation.
type BlockedClientRepositoryDB struct {
	db *gorm.DB
}

// NewBlockedClientRepositoryDB creates a new database-backed
// BlockedClientRepository.
func NewBlockedClientRepositoryDB(db *gorm.DB) *BlockedClientRepositoryDB {
	return &BlockedClientRepositoryDB{db: db}
}

func (r *BlockedClientRepositoryDB) Create(bc *BlockedClient) error {
	return r.db.Create(bc).Error
}

func (r *BlockedClientRepositoryDB) List(ip string, limit, offset int) ([]BlockedClient, int64, error) {
	// Two separate chains built fresh from r.db, not one *gorm.DB reused
	// across both terminal calls (Count then Find) — matching this
	// codebase's existing convention (see countersrepository.go's
	// GetCounterHistory) rather than relying on GORM's own per-call
	// cloning behavior to keep the two queries' conditions from bleeding
	// into each other.
	var total int64
	countQuery := r.db.Model(&BlockedClient{})
	if ip != "" {
		countQuery = countQuery.Where("ip_address = ?", ip)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []BlockedClient
	listQuery := r.db.Model(&BlockedClient{})
	if ip != "" {
		listQuery = listQuery.Where("ip_address = ?", ip)
	}
	if err := listQuery.Order("blocked_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
