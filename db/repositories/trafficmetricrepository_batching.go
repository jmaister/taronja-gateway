package repositories

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/db"
)

// BatchingTrafficMetricRepository coalesces Create calls into periodic bulk
// inserts via CreateBatch: every SQLite write takes the process-wide writer lock,
// so per-request inserts would serialize (see doc/PERFORMANCE_ANALYSIS.md).
//
// Create only appends to an in-memory buffer. A background goroutine flushes it at
// maxBatchSize or flushInterval. A hard crash loses unflushed records, which is
// acceptable for analytics data and not for sessions, users or tokens.
type BatchingTrafficMetricRepository struct {
	TrafficMetricRepository // embedded: every read method (ListRequestDetails,
	// GetTotalRequestCount, ...) and CreateBatch itself just delegate
	// straight through to the wrapped repository, unbatched — only Create
	// is overridden below.

	maxBatchSize  int
	flushInterval time.Duration
	// maxPending hard-caps len(pending): dropping the newest record when full is a
	// smaller loss than running out of memory over droppable data.
	maxPending int

	mu      sync.Mutex
	pending []*db.TrafficMetric
	// dropped counts records Create has discarded because maxPending was
	// reached — read/reset only by flush's periodic log line, never by
	// Create itself, so incrementing it needs no separate lock beyond the
	// one already held for pending.
	dropped int

	flushNow chan struct{}
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// maxPendingMultiplier bounds pending at this many multiples of
// maxBatchSize — generous enough that an ordinary flushInterval-sized
// delay (the flush goroutine briefly losing the CPU, a slightly slow
// CreateBatch) never drops anything, while still bounding worst-case
// memory to a small, fixed multiple of one batch instead of nothing at
// all.
const maxPendingMultiplier = 20

// NewBatchingTrafficMetricRepository wraps inner, batching up to
// maxBatchSize records or flushInterval of elapsed time (whichever comes
// first) before writing them to inner via CreateBatch. Starts a background
// goroutine immediately; call Close when done to flush anything still
// buffered and stop that goroutine.
func NewBatchingTrafficMetricRepository(inner TrafficMetricRepository, maxBatchSize int, flushInterval time.Duration) *BatchingTrafficMetricRepository {
	b := &BatchingTrafficMetricRepository{
		TrafficMetricRepository: inner,
		maxBatchSize:            maxBatchSize,
		flushInterval:           flushInterval,
		maxPending:              maxBatchSize * maxPendingMultiplier,
		flushNow:                make(chan struct{}, 1),
		stop:                    make(chan struct{}),
		done:                    make(chan struct{}),
	}
	go b.run()
	return b
}

// Create buffers stat and returns immediately; the background goroutine flushes it.
// If the buffer is at maxPending the record is dropped and an error is returned.
func (b *BatchingTrafficMetricRepository) Create(stat *db.TrafficMetric) error {
	b.mu.Lock()
	if len(b.pending) >= b.maxPending {
		b.dropped++
		b.mu.Unlock()
		return fmt.Errorf("buffer full (%d pending records already), dropping this one", b.maxPending)
	}
	b.pending = append(b.pending, stat)
	full := len(b.pending) >= b.maxBatchSize
	b.mu.Unlock()

	if full {
		select {
		case b.flushNow <- struct{}{}:
		default:
			// A flush is already pending/running; it'll pick up everything
			// buffered so far once it runs.
		}
	}
	return nil
}

func (b *BatchingTrafficMetricRepository) run() {
	defer close(b.done)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.flushNow:
			b.flush()
		case <-b.stop:
			b.flush() // final flush of whatever arrived before Close
			return
		}
	}
}

func (b *BatchingTrafficMetricRepository) flush() {
	b.mu.Lock()
	batch := b.pending
	b.pending = nil
	dropped := b.dropped
	b.dropped = 0
	b.mu.Unlock()

	// One summary line per flush interval rather than one per dropped
	// record — sustained overload severe enough to hit maxPending would
	// otherwise mean one log line per dropped request, adding its own
	// load right when the system is already struggling.
	if dropped > 0 {
		log.Printf("BatchingTrafficMetricRepository: dropped %d request statistics since the last flush — buffer was full (maxPending=%d)", dropped, b.maxPending)
	}

	if len(batch) == 0 {
		return
	}
	if err := b.TrafficMetricRepository.CreateBatch(batch); err != nil {
		log.Printf("BatchingTrafficMetricRepository: failed to flush %d buffered request statistics: %v", len(batch), err)
	}
}

// Close stops the background flush goroutine after it performs one final
// flush of anything still buffered, and blocks until that finishes. Safe to
// call once during graceful shutdown; calling Create after Close still
// buffers the record but it will never be flushed, so don't.
func (b *BatchingTrafficMetricRepository) Close() {
	b.stopOnce.Do(func() { close(b.stop) })
	<-b.done
}
