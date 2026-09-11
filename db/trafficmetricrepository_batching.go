package db

import (
	"log"
	"sync"
	"time"
)

// BatchingTrafficMetricRepository wraps a TrafficMetricRepository and
// coalesces individual Create calls into periodic bulk inserts via
// CreateBatch. It exists because TrafficMetricMiddleware calls Create once
// per HTTP request — under real traffic that's many small, separate
// transactions arriving concurrently, and on SQLite specifically every
// write transaction takes the single process-wide writer lock, so
// concurrent single-row inserts serialize against each other for no benefit.
// Batching them turns N requests' worth of writes into a handful of larger
// transactions instead of N small ones (see PERFORMANCE_ANALYSIS.md for the
// measured effect).
//
// Create itself never touches the database — it only appends to an
// in-memory buffer and returns — so it's safe to call synchronously from
// TrafficMetricMiddleware's goroutine without that goroutine blocking on
// disk I/O. A background goroutine flushes the buffer to the wrapped
// repository, via CreateBatch, whenever it reaches maxBatchSize or
// flushInterval elapses, whichever comes first.
//
// The trade-off: up to maxBatchSize records, or flushInterval worth of
// them, live only in memory at a time. A hard crash (not a graceful
// shutdown — see Close) loses whatever hadn't been flushed yet. That's an
// acceptable trade for traffic-metrics analytics data specifically, the
// same one access-log shippers and APM agents make; it would not be for
// anything the gateway treats as a durable record (sessions, users,
// tokens), none of which go through this type.
type BatchingTrafficMetricRepository struct {
	TrafficMetricRepository // embedded: every read method (ListRequestDetails,
	// GetTotalRequestCount, ...) and CreateBatch itself just delegate
	// straight through to the wrapped repository, unbatched — only Create
	// is overridden below.

	maxBatchSize  int
	flushInterval time.Duration

	mu      sync.Mutex
	pending []*TrafficMetric

	flushNow chan struct{}
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

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
		flushNow:                make(chan struct{}, 1),
		stop:                    make(chan struct{}),
		done:                    make(chan struct{}),
	}
	go b.run()
	return b
}

// Create buffers stat in memory and returns immediately — no database
// access happens on this call. It's flushed later, along with whatever
// else has accumulated, by the background goroutine started in
// NewBatchingTrafficMetricRepository.
func (b *BatchingTrafficMetricRepository) Create(stat *TrafficMetric) error {
	b.mu.Lock()
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
	b.mu.Unlock()

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
