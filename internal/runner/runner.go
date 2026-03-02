package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

// Sentinel errors returned by Runner.Start.
var (
	ErrAlreadyRunning = errors.New("test already running")
	ErrURLRequired    = errors.New("URL is required")
)

// Config holds the parameters for a load test run.
type Config struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    string

	Requests    int
	Concurrency int
	Duration    string  // e.g. "30s", overrides Requests when set
	RateLimit   float64 // QPS, 0 means unlimited
	Timeout     int     // per-request timeout in seconds

	DisableKeepAlives  bool
	DisableCompression bool
	DisableRedirects   bool
	HTTP2              bool
}

// Status represents the current state of a run.
type Status int

const (
	StatusIdle Status = iota
	StatusRunning
	StatusDone
	StatusStopped
)

// TickFunc is called with a Snapshot on each tick interval.
type TickFunc func(Snapshot)

// DoneFunc is called when the run finishes.
type DoneFunc func(Snapshot, bool)

// Runner manages a single load test lifecycle.
type Runner struct {
	mu         sync.Mutex
	status     Status
	aggregator *Aggregator
	cancelFn   context.CancelFunc
}

func New() *Runner {
	return &Runner{
		status:     StatusIdle,
		aggregator: NewAggregator(),
	}
}

func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Snapshot returns the current metrics without stopping the test.
func (r *Runner) Snapshot() Snapshot {
	return r.aggregator.Snapshot()
}

// Stop gracefully stops the running test.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel := r.cancelFn
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Start launches a load test. onTick is called every tickInterval with live
// metrics. onDone is called once when the test finishes or is stopped.
func (r *Runner) Start(cfg Config, tickInterval time.Duration, onTick TickFunc, onDone DoneFunc) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return ErrURLRequired
	}

	if cfg.Duration != "" {
		if _, err := time.ParseDuration(cfg.Duration); err != nil {
			return fmt.Errorf("invalid duration %q: %w", cfg.Duration, err)
		}
	}

	r.mu.Lock()
	if r.status == StatusRunning {
		r.mu.Unlock()
		return ErrAlreadyRunning
	}
	r.status = StatusRunning
	r.aggregator = NewAggregator()
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	if cfg.Duration != "" {
		d, _ := time.ParseDuration(cfg.Duration) // already validated above
		ctx, cancel = context.WithTimeout(ctx, d)
	}

	r.mu.Lock()
	r.cancelFn = cancel
	r.mu.Unlock()

	client := buildClient(cfg)
	r.aggregator.Start()

	go r.runLoad(ctx, cancel, cfg, client, tickInterval, onTick, onDone)
	return nil
}

// runLoad is the main load test goroutine.
func (r *Runner) runLoad(
	ctx context.Context,
	cancel context.CancelFunc,
	cfg Config,
	client *http.Client,
	tickInterval time.Duration,
	onTick TickFunc,
	onDone DoneFunc,
) {
	defer cancel()

	// Ticker for live metrics.
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				onTick(r.aggregator.Snapshot())
			}
		}
	}()

	// Rate limiter.
	var rateCh <-chan time.Time
	if cfg.RateLimit > 0 {
		interval := time.Duration(float64(time.Second) / cfg.RateLimit)
		t := time.NewTicker(interval)
		defer t.Stop()
		rateCh = t.C
	}

	// Worker pool via semaphore.
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	total := cfg.Requests
	if total <= 0 {
		total = math.MaxInt
	}

	stopped := false
	for i := 0; i < total; i++ {
		// Rate limiting gate.
		if rateCh != nil {
			select {
			case <-ctx.Done():
				stopped = true
				goto done
			case <-rateCh:
			}
		}

		// Concurrency gate.
		select {
		case <-ctx.Done():
			stopped = true
			goto done
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.doRequest(ctx, cfg, client)
		}()
	}

done:
	wg.Wait()

	r.mu.Lock()
	if stopped || ctx.Err() != nil {
		r.status = StatusStopped
		stopped = true
	} else {
		r.status = StatusDone
	}
	r.mu.Unlock()

	onDone(r.aggregator.Snapshot(), stopped)
}

// doRequest executes one HTTP request and records the result.
func (r *Runner) doRequest(ctx context.Context, cfg Config, client *http.Client) {
	req, err := buildRequest(cfg)
	if err != nil {
		r.aggregator.Record(0, 0, true)
		return
	}
	req = req.WithContext(ctx)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Seconds()

	code := 0
	isError := err != nil
	if resp != nil {
		code = resp.StatusCode
		isError = isError || code >= 400
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	r.aggregator.Record(latency, code, isError)
}

func (r *Runner) setStatus(s Status) {
	r.mu.Lock()
	r.status = s
	r.mu.Unlock()
}

// buildClient creates an http.Client configured from cfg.
func buildClient(cfg Config) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        cfg.Concurrency,
		MaxIdleConnsPerHost: cfg.Concurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   cfg.DisableKeepAlives,
		DisableCompression:  cfg.DisableCompression,
	}

	if cfg.HTTP2 {
		http2.ConfigureTransport(transport)
	}

	client := &http.Client{
		Transport: transport,
	}
	if cfg.Timeout > 0 {
		client.Timeout = time.Duration(cfg.Timeout) * time.Second
	}
	if cfg.DisableRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

func buildRequest(cfg Config) (*http.Request, error) {
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if cfg.Body != "" {
		body = strings.NewReader(cfg.Body)
	}

	req, err := http.NewRequest(method, cfg.URL, body)
	if err != nil {
		return nil, err
	}

	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}
