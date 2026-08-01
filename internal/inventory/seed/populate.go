package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Populate writes the demo catalogue if the table is empty, and does nothing
// otherwise. It lives here rather than in a command so that any service can
// run it at start-up; see db.Bootstrap for why that matters.
func Populate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM listings`).Scan(&existing); err != nil {
		return fmt.Errorf("count listings: %w", err)
	}
	if existing > 0 {
		log.InfoContext(ctx, "inventory already present", "listings", existing)
		return nil
	}

	listings, promos := Generate(300, 0.4)
	for _, l := range listings {
		amen, _ := json.Marshal(l.Amenities)
		vibes, _ := json.Marshal(l.VibeTags)
		months, _ := json.Marshal(l.MonthsBest)
		var desc *string
		if l.Description != "" {
			d := l.Description
			desc = &d
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
			return fmt.Errorf("insert listing %s: %w", l.ID, err)
		}
	}
	for _, p := range promos {
		if _, err := pool.Exec(ctx, `
			INSERT INTO promotions (id, listing_id, label, boost, active)
			VALUES ($1,$2,$3,$4,true) ON CONFLICT (id) DO NOTHING`,
			p.ID, p.ListingID, p.Label, p.Boost); err != nil {
			return fmt.Errorf("insert promotion %s: %w", p.ID, err)
		}
	}
	log.InfoContext(ctx, "inventory seeded", "listings", len(listings), "promotions", len(promos))
	return nil
}
