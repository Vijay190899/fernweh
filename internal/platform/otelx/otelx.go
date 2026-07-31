// Package otelx wires OpenTelemetry tracing: OTLP/HTTP export (Jaeger
// all-in-one accepts it natively) plus W3C context propagation, so one trace
// spans gateway → search → ranking/LLM/Postgres — and, via the task payload,
// across the Asynq queue into enrichment workers.
package otelx

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Setup installs a global tracer provider. The returned shutdown func flushes
// pending spans; call it on service exit. When disabled, tracing is a no-op.
func Setup(ctx context.Context, service, endpoint string, enabled bool) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if !enabled {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, fmt.Errorf("otel exporter: %w", err)
	}
	res, err := sdkresource.Merge(sdkresource.Default(),
		sdkresource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(service)))
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Tracer returns the named tracer from the global provider.
func Tracer(name string) trace.Tracer { return otel.Tracer(name) }

// Middleware creates a server span per request and extracts upstream context.
func Middleware(service string, next http.Handler) http.Handler {
	tracer := otel.Tracer(service)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		w.Header().Set("X-Trace-Id", span.SpanContext().TraceID().String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Inject writes the current trace context into an outbound request's headers.
func Inject(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

// MapCarrier round-trips trace context through a plain map — used to carry
// context inside Asynq task payloads across the queue boundary.
func InjectMap(ctx context.Context) map[string]string {
	m := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, m)
	return m
}

func ExtractMap(ctx context.Context, m map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(m))
}
