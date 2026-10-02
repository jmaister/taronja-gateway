package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
)

// Run both benchmarks below with a fixed iteration count, e.g.
// `-bench=BenchmarkTrafficMetricCreate -benchtime=5000x`, not the default
// adaptive time-based one. Create is an in-memory append for the batched
// case (see its doc comment) — nanoseconds, not the microseconds+ a real
// HTTP request takes between Create calls — so the default adaptive
// benchtime ramps b.N until the loop alone fills ~1s, reaching into the
// tens of millions of iterations before the single background flush
// goroutine has any realistic chance to keep up, which mostly benchmarks
// "how large a backlog can build up" rather than steady-state throughput.
// A fixed count sidesteps that: both benchmarks record the same, bounded
// number of events, so their reported cost is directly comparable.

// BenchmarkTrafficMetricCreate_Unbatched issues one INSERT transaction per
// Create call — what every request paid before BatchingTrafficMetricRepository,
// and what deps.NewTest() (and so every gateway benchmark/test) still does.
func BenchmarkTrafficMetricCreate_Unbatched(b *testing.B) {
	db.SetupTestDB("BenchmarkTrafficMetricCreate_Unbatched")
	repo := db.NewTrafficMetricRepository(db.GetConnection())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := repo.Create(newTestMetric(fmt.Sprintf("/bench/%d", i))); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTrafficMetricCreate_Batched is the same b.N Create calls, but
// through BatchingTrafficMetricRepository — what deps.NewProduction() uses.
// Create itself no longer touches the database (see its doc comment), so
// per-call cost drops to an in-memory append; the actual writes happen in
// the background, coalesced into batches of up to 100. Close (called before
// b.StopTimer, so its final flush is included) accounts for whatever hadn't
// been flushed by a full batch or the ticker yet, same as a real shutdown.
func BenchmarkTrafficMetricCreate_Batched(b *testing.B) {
	db.SetupTestDB("BenchmarkTrafficMetricCreate_Batched")
	inner := db.NewTrafficMetricRepository(db.GetConnection())
	batched := db.NewBatchingTrafficMetricRepository(inner, 100, 500*time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := batched.Create(newTestMetric(fmt.Sprintf("/bench/%d", i))); err != nil {
			b.Fatal(err)
		}
	}
	batched.Close()
	b.StopTimer()
}
