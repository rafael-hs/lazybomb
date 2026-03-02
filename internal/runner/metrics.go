package runner

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Snapshot holds a point-in-time view of test metrics.
type Snapshot struct {
	// Elapsed is the total time since the test started.
	Elapsed time.Duration

	// Total is the number of completed requests (success + errors).
	Total int

	// Success is the number of requests that returned a 2xx or 3xx status code.
	Success int

	// Errors is the number of requests that failed (network error or 4xx/5xx status code).
	Errors int

	// RPS is the requests per second rate computed as Total / Elapsed.
	RPS float64

	// Fastest is the lowest recorded latency in seconds.
	Fastest float64

	// Slowest is the highest recorded latency in seconds.
	Slowest float64

	// Average is the mean latency across all completed requests in seconds.
	Average float64

	// P50 is the 50th percentile (median) latency in seconds.
	P50 float64

	// P75 is the 75th percentile latency in seconds.
	P75 float64

	// P90 is the 90th percentile latency in seconds.
	P90 float64

	// P95 is the 95th percentile latency in seconds.
	P95 float64

	// P99 is the 99th percentile latency in seconds, useful for spotting tail latency spikes.
	P99 float64

	// StatusCodes maps each HTTP status code to the number of times it was received.
	StatusCodes map[int]int

	// Histogram contains 10 equally-sized latency buckets between Fastest and Slowest,
	// each with a count and relative frequency, used to render the ASCII bar chart.
	Histogram []Bucket

	// LatencyOverTime holds the average latency per elapsed second, used to render the sparkline.
	LatencyOverTime []float64
}

// Bucket represents a single bar in the latency histogram.
type Bucket struct {
	Label     string
	Count     int
	Frequency float64
}

// Aggregator collects per-request results and computes live metrics.
type Aggregator struct {
	mu          sync.Mutex
	startTime   time.Time
	latencies   []float64
	statusCodes map[int]int
	errors      int
	// second-bucketed averages for sparkline
	timeBuckets map[int][]float64
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		statusCodes: make(map[int]int),
		timeBuckets: make(map[int][]float64),
	}
}

func (a *Aggregator) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startTime = time.Now()
}

// Record adds a single request result.
func (a *Aggregator) Record(latency float64, statusCode int, isError bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.latencies = append(a.latencies, latency)
	a.statusCodes[statusCode]++
	if isError {
		a.errors++
	}

	// Bucket by second for sparkline
	elapsed := int(time.Since(a.startTime).Seconds())
	a.timeBuckets[elapsed] = append(a.timeBuckets[elapsed], latency)
}

// Snapshot returns a point-in-time summary of all collected data.
func (a *Aggregator) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.latencies) == 0 {
		return Snapshot{StatusCodes: make(map[int]int)}
	}

	elapsed := time.Since(a.startTime)
	total := len(a.latencies)
	errorCount := a.errors
	success := total - errorCount

	sorted := make([]float64, total)
	copy(sorted, a.latencies)
	sort.Float64s(sorted)

	fastest := sorted[0]
	slowest := sorted[total-1]

	sum := 0.0
	for _, l := range sorted {
		sum += l
	}
	avg := sum / float64(total)

	rps := 0.0
	if elapsed.Seconds() > 0 {
		rps = float64(total) / elapsed.Seconds()
	}

	codes := make(map[int]int, len(a.statusCodes))
	for k, v := range a.statusCodes {
		codes[k] = v
	}

	// Latency over time sparkline (average per second bucket)
	maxBucket := 0
	for sec := range a.timeBuckets {
		if sec > maxBucket {
			maxBucket = sec
		}
	}
	latencyOverTime := make([]float64, maxBucket+1)
	for sec, lats := range a.timeBuckets {
		s := 0.0
		for _, l := range lats {
			s += l
		}
		latencyOverTime[sec] = s / float64(len(lats))
	}

	return Snapshot{
		Elapsed:         elapsed,
		Total:           total,
		Success:         success,
		Errors:          errorCount,
		RPS:             rps,
		Fastest:         fastest,
		Slowest:         slowest,
		Average:         avg,
		P50:             percentile(sorted, 50),
		P75:             percentile(sorted, 75),
		P90:             percentile(sorted, 90),
		P95:             percentile(sorted, 95),
		P99:             percentile(sorted, 99),
		StatusCodes:     codes,
		Histogram:       buildHistogram(sorted, fastest, slowest),
		LatencyOverTime: latencyOverTime,
	}
}

func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func buildHistogram(sorted []float64, fastest, slowest float64) []Bucket {
	const numBuckets = 10
	if len(sorted) == 0 || fastest == slowest {
		return nil
	}

	buckets := make([]Bucket, numBuckets)
	width := (slowest - fastest) / float64(numBuckets)

	for i := range buckets {
		mark := fastest + width*float64(i+1)
		buckets[i].Label = formatMs(mark)
	}

	for _, l := range sorted {
		idx := int((l - fastest) / width)
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		buckets[idx].Count++
	}

	total := len(sorted)
	for i := range buckets {
		buckets[i].Frequency = float64(buckets[i].Count) / float64(total)
	}

	return buckets
}

func formatMs(seconds float64) string {
	ms := seconds * 1000
	if ms < 1 {
		return "< 1ms"
	}
	if ms >= 1000 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	return fmt.Sprintf("%.1fms", ms)
}
