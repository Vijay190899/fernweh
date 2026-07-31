// Package betterstack ships structured logs to Betterstack Logs and emits
// liveness heartbeats to Betterstack Uptime. Both are optional at runtime:
// without tokens configured, the platform logs to stdout only and skips
// heartbeats. Shipping is asynchronous and lossy by design; telemetry must
// never block or fail the request path.
package betterstack

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const (
	batchSize     = 50
	flushInterval = 2 * time.Second
	queueDepth    = 1000
)

// Shipper is a slog.Handler that forwards every record to a wrapped handler
// (stdout) and enqueues a copy for delivery to the Betterstack ingest API.
type Shipper struct {
	inner   slog.Handler
	queue   chan map[string]any
	attrs   []slog.Attr
	group   string
	client  *http.Client
	url     string
	token   string
	service string
}

// NewShipper wraps inner and starts the background sender. host is the
// ingest hostname from the Betterstack source settings.
func NewShipper(inner slog.Handler, service, host, token string) *Shipper {
	s := &Shipper{
		inner:   inner,
		queue:   make(chan map[string]any, queueDepth),
		client:  &http.Client{Timeout: 5 * time.Second},
		url:     "https://" + host,
		token:   token,
		service: service,
	}
	go s.sender()
	return s
}

func (s *Shipper) Enabled(ctx context.Context, level slog.Level) bool {
	return s.inner.Enabled(ctx, level)
}

func (s *Shipper) Handle(ctx context.Context, r slog.Record) error {
	entry := map[string]any{
		"dt":      r.Time.UTC().Format(time.RFC3339Nano),
		"level":   r.Level.String(),
		"message": r.Message,
		"service": s.service,
	}
	for _, a := range s.attrs {
		entry[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		entry[a.Key] = a.Value.Any()
		return true
	})
	select {
	case s.queue <- entry:
	default: // queue full: drop the shipped copy, stdout still has it
	}
	return s.inner.Handle(ctx, r)
}

func (s *Shipper) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *s
	c.inner = s.inner.WithAttrs(attrs)
	c.attrs = append(append([]slog.Attr{}, s.attrs...), attrs...)
	return &c
}

func (s *Shipper) WithGroup(name string) slog.Handler {
	c := *s
	c.inner = s.inner.WithGroup(name)
	c.group = name
	return &c
}

// sender batches queued entries and posts them. Failures are dropped after
// one retry; the authoritative log stream is stdout.
func (s *Shipper) sender() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	batch := make([]map[string]any, 0, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		body, err := json.Marshal(batch)
		batch = batch[:0]
		if err != nil {
			return
		}
		for attempt := 0; attempt < 2; attempt++ {
			req, err := http.NewRequest(http.MethodPost, s.url, bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Authorization", "Bearer "+s.token)
			req.Header.Set("Content-Type", "application/json")
			resp, err := s.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 300 {
					return
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	for {
		select {
		case entry := <-s.queue:
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Heartbeat pings a Betterstack Uptime heartbeat URL on an interval. A
// missing ping raises an alert on their side; the alerting rules live in
// Betterstack, the service only has to stay alive. No-op when url is empty.
func Heartbeat(ctx context.Context, url string, interval time.Duration, log *slog.Logger) {
	if url == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				continue
			}
			if resp, err := client.Do(req); err != nil {
				log.Warn("heartbeat failed", "err", err)
			} else {
				resp.Body.Close()
			}
		}
	}
}
