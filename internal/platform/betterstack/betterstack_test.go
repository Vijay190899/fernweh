package betterstack

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// capture collects ingest posts so tests can assert on shipped batches.
type capture struct {
	mu      sync.Mutex
	batches [][]map[string]any
	auth    string
}

func (c *capture) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var batch []map[string]any
	_ = json.Unmarshal(body, &batch)
	c.mu.Lock()
	c.batches = append(c.batches, batch)
	c.auth = r.Header.Get("Authorization")
	c.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

func (c *capture) entries() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	for _, b := range c.batches {
		out = append(out, b...)
	}
	return out
}

func newTestShipper(t *testing.T, srvURL, token string) *Shipper {
	t.Helper()
	s := NewShipper(slog.NewTextHandler(io.Discard, nil), "test-svc",
		"ignored", token)
	s.url = srvURL // point at the test server instead of Betterstack
	return s
}

func TestShipperDeliversRecordsWithServiceAndAuth(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(cap.handler))
	defer srv.Close()

	log := slog.New(newTestShipper(t, srv.URL, "tok_123"))
	log.Info("search served", "results", 12)
	log.With("component", "ladder").Warn("relaxed")

	deadline := time.Now().Add(5 * time.Second)
	for len(cap.entries()) < 2 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}

	entries := cap.entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 shipped entries, got %d", len(entries))
	}
	if cap.auth != "Bearer tok_123" {
		t.Errorf("auth header = %q", cap.auth)
	}
	first := entries[0]
	if first["service"] != "test-svc" || first["message"] != "search served" {
		t.Errorf("entry missing fields: %v", first)
	}
	if first["level"] != "INFO" {
		t.Errorf("level = %v", first["level"])
	}
	second := entries[1]
	if second["component"] != "ladder" {
		t.Errorf("WithAttrs attributes must ship, got %v", second)
	}
}

func TestShipperNeverBlocksWhenIngestIsDown(t *testing.T) {
	s := newTestShipper(t, "http://127.0.0.1:1", "tok") // nothing listens here
	log := slog.New(s)

	done := make(chan struct{})
	go func() {
		for i := 0; i < queueDepth*2; i++ { // overflow the queue on purpose
			log.Info("spam", "i", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("logging blocked when the ingest endpoint was unreachable")
	}
}

func TestHeartbeatPingsAndStopsOnCancel(t *testing.T) {
	var mu sync.Mutex
	pings := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pings++
		mu.Unlock()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go Heartbeat(ctx, srv.URL, 50*time.Millisecond, slog.Default())

	time.Sleep(300 * time.Millisecond)
	cancel()
	mu.Lock()
	got := pings
	mu.Unlock()
	if got < 2 {
		t.Fatalf("expected several pings, got %d", got)
	}

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	after := pings
	mu.Unlock()
	if after > got+1 {
		t.Errorf("heartbeat kept pinging after cancel: %d -> %d", got, after)
	}
}

func TestHeartbeatNoopOnEmptyURL(t *testing.T) {
	done := make(chan struct{})
	go func() {
		Heartbeat(context.Background(), "", time.Millisecond, slog.Default())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("empty heartbeat URL must return immediately")
	}
}

func TestShipperEnabledDelegates(t *testing.T) {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn})
	s := NewShipper(inner, "svc", "h", "t")
	if s.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled must delegate to the inner handler's level")
	}
	if !strings.HasPrefix(s.url, "https://") {
		t.Errorf("ingest URL must be https, got %s", s.url)
	}
}
