// Command seed runs migrations and populates inventory. It is the compose
// stack's init container: idempotent, safe on every boot.
package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	fernweh "fernweh"
	"fernweh/internal/inventory/seed"
	"fernweh/internal/platform/config"
	"fernweh/internal/platform/db"
	"fernweh/internal/platform/logging"
)

func main() {
	time.Sleep(5 * time.Second) // Give Docker Compose time to attach watchers before fast-exits
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

	if err := db.Migrate(ctx, pool, fernweh.Migrations, log); err != nil {
		log.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM listings`).Scan(&existing); err != nil {
		log.Error("count failed", "err", err)
		os.Exit(1)
	}
	if existing > 0 {
		log.Info("inventory already seeded", "listings", existing)
		return
	}

	listings, promos := seed.Generate(300, 0.4)
	for _, l := range listings {
		amen, _ := json.Marshal(l.Amenities)
		vibes, _ := json.Marshal(l.VibeTags)
		months, _ := json.Marshal(l.MonthsBest)
		var desc *string
		if l.Description != "" {
			desc = &l.Description
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO listings (id, name, category, destination, country,
				price_per_night_cents, currency, rating, review_count, amenities,
				vibe_tags, description, image_url, months_best, margin_tier, content_status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (id) DO NOTHING`,
			l.ID, l.Name, l.Category, l.Destination, l.Country,
			l.PricePerNightCents, l.Currency, l.Rating, l.ReviewCount, amen,
			vibes, desc, l.ImageURL, months, l.MarginTier, l.ContentStatus)
		if err != nil {
			log.Error("insert listing failed", "id", l.ID, "err", err)
			os.Exit(1)
		}
	}
	for _, p := range promos {
		if _, err := pool.Exec(ctx, `
			INSERT INTO promotions (id, listing_id, label, boost, active)
			VALUES ($1,$2,$3,$4,true) ON CONFLICT (id) DO NOTHING`,
			p.ID, p.ListingID, p.Label, p.Boost); err != nil {
			log.Error("insert promotion failed", "id", p.ID, "err", err)
			os.Exit(1)
		}
	}
	log.Info("seeded", "listings", len(listings), "promotions", len(promos))
}
