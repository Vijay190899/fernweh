// Command ranking is the personalization service: behavioral signals in,
// explainable per-session result ordering out.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"fernweh/internal/platform/config"
	"fernweh/internal/platform/httpx"
	"fernweh/internal/platform/logging"
	"fernweh/internal/platform/otelx"
	"fernweh/internal/platform/redisx"
	"fernweh/internal/ranking"
)

func main() {
	const service = "ranking"
	log := logging.New(service)
	cfg := config.Load(service)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	shutdown, err := otelx.Setup(ctx, service, cfg.OTLPEndpoint, cfg.TracesEnabled)
	if err != nil {
		log.Error("otel setup failed", "err", err)
		os.Exit(1)
	}
	defer shutdown(context.Background()) //nolint:errcheck

	rdb, err := redisx.Connect(ctx, cfg.RedisAddr)
	if err != nil {
		log.Error("redis connect failed", "err", err)
		os.Exit(1)
	}

	store := ranking.NewSignalStore(rdb)
	mux := http.NewServeMux()
	ranking.NewHandler(store, log).Register(mux)
	httpx.Health(mux, func(ctx context.Context) error { return rdb.Ping(ctx).Err() })

	srv := httpx.NewServer(config.Addr("RANKING_ADDR", ":8082"), service, mux)
	if err := httpx.Serve(srv, log); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}
