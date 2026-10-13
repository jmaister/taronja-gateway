package repositories

import (
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// BlockedClientRepository defines the interface for persisting and querying
// the rate-limiter's block-event history — see BlockedClient's doc comment
// for why this exists separately from the in-memory RateLimiter's own,
// short-lived tracking.
type BlockedClientRepository interface {
	// Create persists one block event. Called from
	// middleware/builtin/ratelimiter.go's Handler in a background goroutine —
	// never on the hot path of the request that's actively being
	// rejected with a 429.
	Create(bc *db.BlockedClient) error
	// List returns block events matching filter, most recent first, along
	// with the total count matching it (ignoring limit/offset) for
	// pagination.
	List(filter BlockedClientFilter, limit, offset int) ([]db.BlockedClient, int64, error)
	// DeleteOlderThan permanently removes every block event whose
	// BlockedAt is before cutoff, returning how many rows were removed.
	// Called periodically by middleware.RateLimiter (see its
	// pruneBlockedClientsLoop) so this table doesn't grow without bound —
	// see the rate_limiter middleware's blockedClientRetentionDays option.
	DeleteOlderThan(cutoff time.Time) (int64, error)
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

func (r *BlockedClientRepositoryDB) Create(bc *db.BlockedClient) error {
	return r.db.Create(bc).Error
}

func (r *BlockedClientRepositoryDB) List(filter BlockedClientFilter, limit, offset int) ([]db.BlockedClient, int64, error) {
	var total int64
	if err := filter.apply(r.db.Model(&db.BlockedClient{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []db.BlockedClient
	err := filter.apply(r.db.Model(&db.BlockedClient{})).Order("blocked_at DESC").Limit(limit).Offset(offset).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// BlockedClientFilter narrows List; empty fields are not applied.
type BlockedClientFilter struct {
	IP      string
	Country string
}

func (f BlockedClientFilter) apply(q *gorm.DB) *gorm.DB {
	if f.IP != "" {
		q = q.Where("ip_address = ?", f.IP)
	}
	if f.Country != "" {
		q = q.Where("country = ?", f.Country)
	}
	return q
}

func (r *BlockedClientRepositoryDB) DeleteOlderThan(cutoff time.Time) (int64, error) {
	// Unscoped(): BlockedClient embeds gorm.Model, so a plain Delete would
	// only soft-delete (set deleted_at) — the row, and the unbounded
	// growth this exists to bound, would still be sitting in the table.
	// This is a retention prune, not a user-facing removal, so an actual
	// hard DELETE is what's wanted here.
	result := r.db.Unscoped().Where("blocked_at < ?", cutoff).Delete(&db.BlockedClient{})
	return result.RowsAffected, result.Error
}
