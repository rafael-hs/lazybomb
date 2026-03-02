package runner

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newTestServer returns an httptest.Server that responds with the given status code.
func newTestServer(t *testing.T, statusCode int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBuildRequest(t *testing.T) {
	t.Run("should set method to GET when method is empty", func(t *testing.T) {
		cfg := Config{URL: "http://example.com", Method: ""}

		req, err := buildRequest(cfg)

		assert.NoError(t, err)
		assert.Equal(t, "GET", req.Method)
	})

	t.Run("should uppercase method when method is lowercase", func(t *testing.T) {
		cfg := Config{URL: "http://example.com", Method: "post"}

		req, err := buildRequest(cfg)

		assert.NoError(t, err)
		assert.Equal(t, "POST", req.Method)
	})

	t.Run("should set all headers when headers are provided", func(t *testing.T) {
		cfg := Config{
			URL:    "http://example.com",
			Method: "GET",
			Headers: map[string]string{
				"Authorization": "Bearer token",
				"Content-Type":  "application/json",
			},
		}

		req, err := buildRequest(cfg)

		assert.NoError(t, err)
		assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	})

	t.Run("should set body when body is provided", func(t *testing.T) {
		cfg := Config{URL: "http://example.com", Method: "POST", Body: `{"key":"value"}`}

		req, err := buildRequest(cfg)

		assert.NoError(t, err)
		assert.NotNil(t, req.Body)
	})

	t.Run("should return error when URL is invalid", func(t *testing.T) {
		cfg := Config{URL: "://bad-url", Method: "GET"}

		_, err := buildRequest(cfg)

		assert.Error(t, err)
	})
}

func TestBuildClient(t *testing.T) {
	t.Run("should set timeout when timeout is configured", func(t *testing.T) {
		cfg := Config{Concurrency: 10, Timeout: 5}

		client := buildClient(cfg)

		assert.Equal(t, 5*time.Second, client.Timeout)
	})

	t.Run("should have no timeout when timeout is zero", func(t *testing.T) {
		cfg := Config{Concurrency: 10, Timeout: 0}

		client := buildClient(cfg)

		assert.Equal(t, time.Duration(0), client.Timeout)
	})

	t.Run("should block redirects when DisableRedirects is true", func(t *testing.T) {
		cfg := Config{Concurrency: 1, DisableRedirects: true}

		client := buildClient(cfg)

		assert.NotNil(t, client.CheckRedirect)
	})

	t.Run("should allow redirects when DisableRedirects is false", func(t *testing.T) {
		cfg := Config{Concurrency: 1, DisableRedirects: false}

		client := buildClient(cfg)

		assert.Nil(t, client.CheckRedirect)
	})
}

func TestStart(t *testing.T) {
	t.Run("should return ErrURLRequired when URL is empty", func(t *testing.T) {
		r := New()

		err := r.Start(Config{}, time.Second, func(Snapshot) {}, func(Snapshot, bool) {})

		assert.ErrorIs(t, err, ErrURLRequired)
		assert.Equal(t, StatusIdle, r.Status())
	})

	t.Run("should return ErrAlreadyRunning when already running", func(t *testing.T) {
		srv := newTestServer(t, http.StatusOK)
		r := New()
		done := make(chan struct{})

		_ = r.Start(
			Config{URL: srv.URL, Requests: 100, Concurrency: 1},
			time.Second,
			func(Snapshot) {},
			func(Snapshot, bool) { close(done) },
		)

		err := r.Start(Config{URL: srv.URL}, time.Second, func(Snapshot) {}, func(Snapshot, bool) {})
		assert.ErrorIs(t, err, ErrAlreadyRunning)

		<-done
	})

	t.Run("should return error when duration is invalid", func(t *testing.T) {
		r := New()

		err := r.Start(
			Config{URL: "http://example.com", Duration: "not-a-duration"},
			time.Second,
			func(Snapshot) {},
			func(Snapshot, bool) {},
		)

		assert.Error(t, err)
		assert.Equal(t, StatusIdle, r.Status())
	})

	t.Run("should complete all requests when requests count is set", func(t *testing.T) {
		srv := newTestServer(t, http.StatusOK)
		r := New()
		done := make(chan Snapshot, 1)

		err := r.Start(
			Config{URL: srv.URL, Requests: 10, Concurrency: 2},
			time.Second,
			func(Snapshot) {},
			func(snap Snapshot, _ bool) { done <- snap },
		)

		assert.NoError(t, err)
		snap := <-done
		assert.Equal(t, 10, snap.Total)
		assert.Equal(t, StatusDone, r.Status())
	})

	t.Run("should record errors when server returns 5xx", func(t *testing.T) {
		srv := newTestServer(t, http.StatusInternalServerError)
		r := New()
		done := make(chan Snapshot, 1)

		_ = r.Start(
			Config{URL: srv.URL, Requests: 5, Concurrency: 1},
			time.Second,
			func(Snapshot) {},
			func(snap Snapshot, _ bool) { done <- snap },
		)

		snap := <-done
		assert.Equal(t, 5, snap.Errors)
		assert.Equal(t, 0, snap.Success)
	})

	t.Run("should stop early when Stop is called", func(t *testing.T) {
		srv := newTestServer(t, http.StatusOK)
		r := New()
		done := make(chan bool, 1)

		_ = r.Start(
			Config{URL: srv.URL, Requests: 10_000, Concurrency: 1},
			time.Second,
			func(Snapshot) {},
			func(_ Snapshot, stopped bool) { done <- stopped },
		)

		r.Stop()

		stopped := <-done
		assert.True(t, stopped)
		assert.Equal(t, StatusStopped, r.Status())
	})

	t.Run("should call onTick at least once when test runs long enough", func(t *testing.T) {
		srv := newTestServer(t, http.StatusOK)
		r := New()
		var tickCount atomic.Int32
		firstTick := make(chan struct{}, 1)
		done := make(chan struct{})

		_ = r.Start(
			Config{URL: srv.URL, Requests: 10_000, Concurrency: 4},
			50*time.Millisecond,
			func(Snapshot) {
				tickCount.Add(1)
				select {
				case firstTick <- struct{}{}:
				default:
				}
			},
			func(Snapshot, bool) { close(done) },
		)

		<-firstTick // wait for at least one tick before stopping
		r.Stop()
		<-done

		assert.Greater(t, tickCount.Load(), int32(0))
	})
}
