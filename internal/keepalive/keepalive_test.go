package keepalive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// The ping has to be a real inbound request — that is the entire mechanism —
// and a failing one must not take the process down with it.
func TestPingHitsTheURL(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ping(t.Context(), server.Client(), server.URL)
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1", hits.Load())
	}
}

func TestPingSurvivesFailures(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer down.Close()
	ping(t.Context(), down.Client(), down.URL) // must not panic
	ping(t.Context(), &http.Client{Timeout: time.Second}, "http://127.0.0.1:1/nope")
	ping(t.Context(), &http.Client{}, "not a url")
}

func TestRunStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { Run(ctx, "http://127.0.0.1:1/nope"); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Run ignored a cancelled context")
	}
}
