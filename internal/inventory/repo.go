package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo is the Postgres-backed inventory store.
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const listingCols = `id, name, category, destination, country, price_per_night_cents,
	currency, rating, review_count, amenities, vibe_tags,
	COALESCE(description, ''), COALESCE(image_url, ''), months_best, margin_tier, content_status`

// Search runs the structured filter against inventory, best-rated first.
// Ordering here is only base relevance; personalization happens in ranking.
func (r *Repo) Search(ctx context.Context, f Filter) ([]Listing, error) {
	var (
		where []string
		args  []any
	)
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}

	if f.Destination != "" {
		add("destination ILIKE $%d", f.Destination)
	}
	if f.Country != "" {
		add("country ILIKE $%d", f.Country)
	}
	if f.Category != "" {
		add("category = $%d", f.Category)
	}
	if f.BudgetMax > 0 {
		add("price_per_night_cents <= $%d", f.BudgetMax)
	}
	if f.BudgetMin > 0 {
		add("price_per_night_cents >= $%d", f.BudgetMin)
	}
	if f.MinRating > 0 {
		add("rating >= $%d", f.MinRating)
	}
	if f.Month >= 1 && f.Month <= 12 {
		add("months_best @> $%d", jsonArr([]int{f.Month}))
	}
	for _, a := range f.Amenities {
		add("amenities @> $%d", jsonArr([]string{a}))
	}
	for _, v := range f.VibeTags {
		add("vibe_tags @> $%d", jsonArr([]string{v}))
	}

	q := "SELECT " + listingCols + " FROM listings"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY rating DESC, review_count DESC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()
	return scanListings(rows)
}

// Covers reports whether the catalogue holds anything at all for a
// destination or country.
//
// Without this check, asking for somewhere the platform does not serve falls
// through the relaxation ladder and comes back with the other side of the
// world, described only as "similar stays in other destinations". Widening a
// budget is a helpful relaxation; silently swapping India for Norway is not,
// so search needs to be able to say it has no coverage.
func (r *Repo) Covers(ctx context.Context, destination, country string) (bool, error) {
	if destination == "" && country == "" {
		return true, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM listings
			WHERE ($1 = '' OR destination ILIKE $1)
			  AND ($2 = '' OR country ILIKE $2)
		)`, destination, country).Scan(&exists)
	return exists, err
}

// ListByStatus returns listings in the given content status, oldest first.
func (r *Repo) ListByStatus(ctx context.Context, status string, limit int) ([]Listing, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+listingCols+" FROM listings WHERE content_status = $1 ORDER BY updated_at ASC LIMIT $2",
		status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanListings(rows)
}

// Get returns a single listing.
func (r *Repo) Get(ctx context.Context, id string) (Listing, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+listingCols+" FROM listings WHERE id = $1", id)
	if err != nil {
		return Listing{}, err
	}
	defer rows.Close()
	ls, err := scanListings(rows)
	if err != nil {
		return Listing{}, err
	}
	if len(ls) == 0 {
		return Listing{}, pgx.ErrNoRows
	}
	return ls[0], nil
}

// SetStatus transitions a listing's content status only from an expected
// state (optimistic guard against concurrent workers).
func (r *Repo) SetStatus(ctx context.Context, id, from, to string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE listings SET content_status = $3, updated_at = now()
		 WHERE id = $1 AND content_status = $2`, id, from, to)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ApplyEnrichment writes enriched content and its audit trail atomically.
func (r *Repo) ApplyEnrichment(ctx context.Context, id string, description string, amenities []string, contentHash, source, model string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var beforeDesc string
	var beforeAmen []byte
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(description, ''), amenities FROM listings WHERE id = $1 FOR UPDATE`, id).
		Scan(&beforeDesc, &beforeAmen); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE listings SET description = $2, amenities = $3, content_hash = $4,
		   content_status = $5, updated_at = now() WHERE id = $1`,
		id, description, jsonArr(amenities), contentHash, StatusEnriched)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO enrichment_audit (listing_id, field, before, after, source, model)
		 VALUES ($1, 'description', $2, $3, $5, $6),
		        ($1, 'amenities', $4, $7, $5, $6)`,
		id, beforeDesc, description, string(beforeAmen), source, model, string(jsonArr(amenities)))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResetForDemo puts a slice of already-enriched inventory back into a broken
// state so the enrichment pipeline can be watched running more than once.
// Purely a demo affordance; a real platform would never do this, which is why
// it is bounded and named for what it is.
func (r *Repo) ResetForDemo(ctx context.Context, n int) (int, error) {
	if n <= 0 || n > 200 {
		n = 60
	}
	// Anything that currently has content is a valid thing to break, whether
	// that content arrived with the seed or was written by the pipeline.
	// Restricting this to already-enriched rows meant that on a freshly
	// deployed stack, where nothing has been enriched yet, the button a
	// visitor presses first reported breaking nothing.
	tag, err := r.pool.Exec(ctx, `
		UPDATE listings SET description = NULL, content_status = $1,
		       content_hash = NULL, updated_at = now()
		WHERE id IN (
			SELECT id FROM listings
			WHERE content_status = ANY($2)
			ORDER BY random() LIMIT $3
		)`, StatusNeedsEnrichment, []string{StatusEnriched, StatusComplete}, n)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ContentStats summarizes inventory completeness for the ops dashboard.
type ContentStats struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
}

func (r *Repo) Stats(ctx context.Context) (ContentStats, error) {
	rows, err := r.pool.Query(ctx, `SELECT content_status, count(*) FROM listings GROUP BY content_status`)
	if err != nil {
		return ContentStats{}, err
	}
	defer rows.Close()
	s := ContentStats{ByStatus: map[string]int{}}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return ContentStats{}, err
		}
		s.ByStatus[status] = n
		s.Total += n
	}
	return s, rows.Err()
}

// ActivePromotions returns active promotions keyed by listing id.
func (r *Repo) ActivePromotions(ctx context.Context) (map[string]Promotion, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, listing_id, label, boost FROM promotions WHERE active`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Promotion{}
	for rows.Next() {
		var p Promotion
		if err := rows.Scan(&p.ID, &p.ListingID, &p.Label, &p.Boost); err != nil {
			return nil, err
		}
		out[p.ListingID] = p
	}
	return out, rows.Err()
}

// RecentAudit returns the latest enrichment audit entries for a listing.
type AuditEntry struct {
	Field     string `json:"field"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Source    string `json:"source"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
}

func (r *Repo) RecentAudit(ctx context.Context, listingID string, limit int) ([]AuditEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT field, COALESCE(before,''), after, source, COALESCE(model,''), created_at::text
		 FROM enrichment_audit WHERE listing_id = $1 ORDER BY created_at DESC LIMIT $2`,
		listingID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.Field, &e.Before, &e.After, &e.Source, &e.Model, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanListings(rows pgx.Rows) ([]Listing, error) {
	var out []Listing
	for rows.Next() {
		var l Listing
		var amenities, vibes, months []byte
		if err := rows.Scan(&l.ID, &l.Name, &l.Category, &l.Destination, &l.Country,
			&l.PricePerNightCents, &l.Currency, &l.Rating, &l.ReviewCount,
			&amenities, &vibes, &l.Description, &l.ImageURL, &months,
			&l.MarginTier, &l.ContentStatus); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(amenities, &l.Amenities)
		_ = json.Unmarshal(vibes, &l.VibeTags)
		_ = json.Unmarshal(months, &l.MonthsBest)
		out = append(out, l)
	}
	return out, rows.Err()
}

func jsonArr(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
