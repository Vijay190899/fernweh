// Package logging configures structured JSON logging with automatic trace_id
// correlation, so every log line can be joined against its Jaeger trace.
// When a Betterstack source token is configured, the same records also ship
// to Betterstack Logs asynchronously; stdout stays the authoritative stream.
package logging

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"

	"fernweh/internal/platform/betterstack"
)

// New returns a JSON slog.Logger tagged with the service name.
func New(service string) *slog.Logger {
	var h slog.Handler = &traceHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})}
	if token := os.Getenv("BETTERSTACK_LOG_TOKEN"); token != "" {
		host := os.Getenv("BETTERSTACK_INGEST_HOST")
		if host == "" {
			host = "in.logs.betterstack.com"
		}
		h = betterstack.NewShipper(h, service, host, token)
	}
	return slog.New(h).With("service", service)
}

// traceHandler injects trace_id/span_id from the context into every record.
type traceHandler struct {
	slog.Handler
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}
