package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMetric(path string) *db.TrafficMetric {
	return &db.TrafficMetric{
		HttpMethod:     "GET",
		Path:           path,
		HttpStatus:     200,
		ResponseTimeNs: 1000,
		Timestamp:      time.Now(),
	}
}

func TestBatchingTrafficMetricRepository_FlushesOnBatchSize(t *testing.T) {
	db.SetupTestDB(t.Name())
	inner := db.NewTrafficMetricRepository(db.GetConnection())
	batched := db.NewBatchingTrafficMetricRepository(inner, 3, time.Hour) // flushInterval long enough to never fire on its own
	defer batched.Close()

	for i := 0; i < 3; i++ {
		require.NoError(t, batched.Create(newTestMetric(fmt.Sprintf("/batch-size/%d", i))))
	}

	// The 3rd Create should have triggered a flush; give the background
	// goroutine a moment to run it rather than asserting instantaneously.
	require.Eventually(t, func() bool {
		count, err := inner.GetTotalRequestCount(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		return err == nil && count == 3
	}, time.Second, 5*time.Millisecond, "expected all 3 records to be flushed once maxBatchSize was reached")
}

func TestBatchingTrafficMetricRepository_FlushesOnInterval(t *testing.T) {
	db.SetupTestDB(t.Name())
	inner := db.NewTrafficMetricRepository(db.GetConnection())
	batched := db.NewBatchingTrafficMetricRepository(inner, 100, 20*time.Millisecond) // maxBatchSize high enough to never trigger on its own
	defer batched.Close()

	require.NoError(t, batched.Create(newTestMetric("/interval-only")))

	require.Eventually(t, func() bool {
		count, err := inner.GetTotalRequestCount(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		return err == nil && count == 1
	}, time.Second, 5*time.Millisecond, "expected the record to be flushed once flushInterval elapsed, even though the batch never filled up")
}

func TestBatchingTrafficMetricRepository_CloseFlushesRemaining(t *testing.T) {
	db.SetupTestDB(t.Name())
	inner := db.NewTrafficMetricRepository(db.GetConnection())
	batched := db.NewBatchingTrafficMetricRepository(inner, 100, time.Hour)

	require.NoError(t, batched.Create(newTestMetric("/close-flush-1")))
	require.NoError(t, batched.Create(newTestMetric("/close-flush-2")))

	batched.Close() // should flush both before returning, with no wait needed

	count, err := inner.GetTotalRequestCount(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestBatchingTrafficMetricRepository_ReadsDelegateToInner(t *testing.T) {
	db.SetupTestDB(t.Name())
	inner := db.NewTrafficMetricRepository(db.GetConnection())
	batched := db.NewBatchingTrafficMetricRepository(inner, 100, time.Hour)
	defer batched.Close()

	require.NoError(t, inner.Create(newTestMetric("/written-directly-to-inner")))

	// Reads issued through the wrapper (nothing buffered by it in this
	// test) should still see what's actually in the database, confirming
	// the embedded TrafficMetricRepository delegates correctly.
	stats, err := batched.FindByPath("/written-directly-to-inner", 10)
	require.NoError(t, err)
	assert.Len(t, stats, 1)
}
