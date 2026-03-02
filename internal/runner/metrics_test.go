package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	t.Run("should return zero when slice is empty", func(t *testing.T) {
		result := percentile([]float64{}, 50)
		assert.Equal(t, 0.0, result)
	})

	t.Run("should return the only element when slice has one item", func(t *testing.T) {
		result := percentile([]float64{42}, 99)
		assert.Equal(t, 42.0, result)
	})

	t.Run("should return median when p is 50", func(t *testing.T) {
		result := percentile(sorted, 50)
		assert.Equal(t, 5.0, result)
	})

	t.Run("should return last element when p is 99", func(t *testing.T) {
		result := percentile(sorted, 99)
		assert.Equal(t, 10.0, result)
	})

	t.Run("should return first element when p is 0", func(t *testing.T) {
		result := percentile(sorted, 0)
		assert.Equal(t, 1.0, result)
	})
}

func TestFormatMs(t *testing.T) {
	t.Run("should return less than 1ms when value is below 1ms", func(t *testing.T) {
		result := formatMs(0.0005)
		assert.Equal(t, "< 1ms", result)
	})

	t.Run("should return ms string when value is between 1ms and 1s", func(t *testing.T) {
		result := formatMs(0.025)
		assert.Equal(t, "25.0ms", result)
	})

	t.Run("should return seconds string when value is 1s or more", func(t *testing.T) {
		result := formatMs(1.5)
		assert.Equal(t, "1.5s", result)
	})
}

func TestBuildHistogram(t *testing.T) {
	t.Run("should return nil when slice is empty", func(t *testing.T) {
		result := buildHistogram([]float64{}, 0, 0)
		assert.Nil(t, result)
	})

	t.Run("should return nil when all values are equal", func(t *testing.T) {
		result := buildHistogram([]float64{0.1, 0.1, 0.1}, 0.1, 0.1)
		assert.Nil(t, result)
	})

	t.Run("should return 10 buckets when data has a range", func(t *testing.T) {
		sorted := []float64{0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09, 0.10}
		result := buildHistogram(sorted, 0.01, 0.10)
		assert.Len(t, result, 10)
	})

	t.Run("should distribute all requests across buckets when data has a range", func(t *testing.T) {
		sorted := []float64{0.01, 0.02, 0.03, 0.04, 0.05}
		result := buildHistogram(sorted, 0.01, 0.05)
		total := 0
		for _, b := range result {
			total += b.Count
		}
		assert.Equal(t, len(sorted), total)
	})

	t.Run("should set frequency relative to total when building buckets", func(t *testing.T) {
		sorted := []float64{0.01, 0.02, 0.03, 0.04}
		result := buildHistogram(sorted, 0.01, 0.04)
		totalFreq := 0.0
		for _, b := range result {
			totalFreq += b.Frequency
		}
		assert.InDelta(t, 1.0, totalFreq, 0.0001)
	})
}

func TestRecord(t *testing.T) {
	t.Run("should count total requests when recording", func(t *testing.T) {
		a := NewAggregator()
		a.Start()
		a.Record(0.01, 200, false)
		a.Record(0.02, 200, false)
		a.Record(0.03, 500, true)

		snap := a.Snapshot()

		assert.Equal(t, 3, snap.Total)
	})

	t.Run("should separate success and errors when recording mixed results", func(t *testing.T) {
		a := NewAggregator()
		a.Start()
		a.Record(0.01, 200, false)
		a.Record(0.02, 200, false)
		a.Record(0.03, 500, true)

		snap := a.Snapshot()

		assert.Equal(t, 2, snap.Success)
		assert.Equal(t, 1, snap.Errors)
	})

	t.Run("should group counts by status code when recording", func(t *testing.T) {
		a := NewAggregator()
		a.Start()
		a.Record(0.01, 200, false)
		a.Record(0.02, 200, false)
		a.Record(0.03, 404, true)

		snap := a.Snapshot()

		assert.Equal(t, 2, snap.StatusCodes[200])
		assert.Equal(t, 1, snap.StatusCodes[404])
	})
}

func TestSnapshot(t *testing.T) {
	t.Run("should return empty snapshot when no requests recorded", func(t *testing.T) {
		a := NewAggregator()
		a.Start()

		snap := a.Snapshot()

		assert.Equal(t, 0, snap.Total)
		assert.NotNil(t, snap.StatusCodes)
	})

	t.Run("should set fastest and slowest correctly when requests have different latencies", func(t *testing.T) {
		a := NewAggregator()
		a.Start()
		a.Record(0.05, 200, false)
		a.Record(0.01, 200, false)
		a.Record(0.10, 200, false)

		snap := a.Snapshot()

		assert.InDelta(t, 0.01, snap.Fastest, 0.0001)
		assert.InDelta(t, 0.10, snap.Slowest, 0.0001)
	})

	t.Run("should compute average correctly when requests have different latencies", func(t *testing.T) {
		a := NewAggregator()
		a.Start()
		a.Record(0.10, 200, false)
		a.Record(0.20, 200, false)
		a.Record(0.30, 200, false)

		snap := a.Snapshot()

		assert.InDelta(t, 0.20, snap.Average, 0.0001)
	})

	t.Run("should compute all percentiles when enough data is recorded", func(t *testing.T) {
		a := NewAggregator()
		a.Start()
		for i := 1; i <= 100; i++ {
			a.Record(float64(i)*0.001, 200, false)
		}

		snap := a.Snapshot()

		assert.Greater(t, snap.P50, 0.0)
		assert.Greater(t, snap.P75, snap.P50)
		assert.Greater(t, snap.P90, snap.P75)
		assert.Greater(t, snap.P95, snap.P90)
		assert.Greater(t, snap.P99, snap.P95)
	})
}
