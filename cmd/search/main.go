// Command search is the AI search service: natural-language queries in,
// ranked live inventory out, with or without an LLM available.
package main

import (
	"context"
	"os"
	"time"

	"fernweh/internal/inventory"
	"fernweh/internal/platform/betterstack"
	"fernweh/internal/platform/config"
	"fernweh/internal/platform/db"
	"fernweh/internal/platform/httpx"
	"fernweh/internal/platform/llm"
	"fernweh/internal/platform/logging"
	"fernweh/internal/platform/otelx"
	"fernweh/internal/platform/redisx"
	"fernweh/internal/search"

	"net/http"
)

func main() {
	const service = "search"
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

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := redisx.Connect(ctx, cfg.RedisAddr)
	if err != nil {
		log.Error("redis connect failed", "err", err)
		os.Exit(1)
	}

	repo := inventory.NewRepo(pool)
	llmClient := llm.New(cfg.OpenRouterKey, cfg.LLMModel, cfg.LLMTimeout, cfg.LLMDailyBudget, rdb)
	extractor := search.NewExtractor(llmClient, rdb)
	ranker := search.NewRankingClient(cfg.RankingURL)
	svc := search.NewService(repo, extractor, ranker, log)

	mux := http.NewServeMux()
	search.NewHandler(svc).Register(mux)
	httpx.Health(mux, func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return err
		}
		return rdb.Ping(ctx).Err()
	})

	go betterstack.Heartbeat(context.Background(), cfg.HeartbeatURL, time.Minute, log)

	srv := httpx.NewServer(config.Addr("SEARCH_ADDR", ":8081"), service, mux)
	if err := httpx.Serve(srv, log); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}
