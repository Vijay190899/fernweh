// Command enrich runs the content pipeline: an Asynq worker that fills
// listing gaps, a periodic scanner, and the ops/admin API — one binary,
// three concurrent duties, all sharing one trace pipeline.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/hibiken/asynq"

	"fernweh/internal/enrich"
	"fernweh/internal/inventory"
	"fernweh/internal/platform/config"
	"fernweh/internal/platform/db"
	"fernweh/internal/platform/httpx"
	"fernweh/internal/platform/llm"
	"fernweh/internal/platform/logging"
	"fernweh/internal/platform/otelx"
	"fernweh/internal/platform/redisx"
)

const scanInterval = 5 * time.Minute

func main() {
	const service = "enrich"
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
	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}
	client := asynq.NewClient(redisOpt)
	defer client.Close()
	inspector := asynq.NewInspector(redisOpt)

	llmClient := llm.New(cfg.OpenRouterKey, cfg.LLMModel, cfg.LLMTimeout, cfg.LLMDailyBudget, rdb)
	generator := enrich.NewAIGenerator(llmClient)
	processor := enrich.NewProcessor(repo, generator, log)
	scanner := enrich.NewScanner(repo, client, log)

	// Asynq worker: bounded concurrency — enrichment is background work and
	// must never starve the interactive services of DB connections.
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 2,
		Queues:      map[string]int{enrich.QueueEnrich: 1},
	})
	mux := asynq.NewServeMux()
	mux.Handle(enrich.TaskEnrichListing, processor.AsynqHandler())
	go func() {
		if err := srv.Run(mux); err != nil {
			log.Error("asynq server error", "err", err)
			os.Exit(1)
		}
	}()

	// Periodic scan keeps the 24/7 promise: new gaps get found without a
	// human pressing a button. An immediate scan primes the demo.
	go func() {
		bg := context.Background()
		if _, err := scanner.Scan(bg, 200); err != nil {
			log.Error("initial scan failed", "err", err)
		}
		for range time.Tick(scanInterval) {
			if _, err := scanner.Scan(bg, 200); err != nil {
				log.Error("periodic scan failed", "err", err)
			}
		}
	}()

	httpMux := http.NewServeMux()
	enrich.NewHandler(repo, scanner, inspector, log).Register(httpMux)
	httpx.Health(httpMux, func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return err
		}
		return rdb.Ping(ctx).Err()
	})

	httpSrv := httpx.NewServer(config.Addr("ENRICH_ADDR", ":8083"), service, httpMux)
	if err := httpx.Serve(httpSrv, log); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
	srv.Shutdown()
}
