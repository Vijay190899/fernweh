// Command seed migrates and populates the catalogue.
//
// The services now bootstrap themselves at start-up, so this is not part of
// the deployed stack. It stays because it is useful locally: run it against a
// bare database to get a seeded one without starting anything else. It calls
// the same code path the services use, so the two cannot drift.
package main

import (
	"context"
	"os"
	"time"

	fernweh "fernweh"
	"fernweh/internal/inventory/seed"
	"fernweh/internal/platform/config"
	"fernweh/internal/platform/db"
	"fernweh/internal/platform/logging"
)

func main() {
	log := logging.New("seed")
	cfg := config.Load("seed")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Bootstrap(ctx, pool, fernweh.Migrations, log, seed.Populate); err != nil {
		log.Error("bootstrap failed", "err", err)
		os.Exit(1)
	}
}
