package search

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"

	"fernweh/internal/inventory"
)

// The cold/warm comparison.
//
// Search owns this endpoint rather than ranking because the candidates have to
// be real. Ranking has no database; handing it a hand-picked list would make
// the comparison a demonstration of nothing. So the same query runs through the
// same extractor and the same relaxation ladder that serve live traffic, and
// the resulting candidate set is what gets ranked twice.
//
// Only the profile differs between the two sides. That is the entire point:
// anything that moves, moves because of personalization.

// Comparer ranks a candidate set twice, once cold and once for a named
// persona. Unlike Ranker this has no fallback: the comparison is the feature,
// so if it cannot run there is nothing honest to show and the caller gets an
// error rather than two identical columns.
type Comparer interface {
	Compare(ctx context.Context, persona string, items []RankItem) (Comparison, error)
	Personas(ctx context.Context) ([]map[string]any, error)
}

// Placement mirrors ranking.Placement across the service boundary.
type Placement struct {
	Rank    int      `json:"rank"`
	ID      string   `json:"id"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons,omitempty"`
	Delta   int      `json:"delta"`
}

// Comparison is the ranking service's answer, before listings are attached.
type Comparison struct {
	Persona  map[string]any `json:"persona"`
	Cold     []Placement    `json:"cold"`
	Warm     []Placement    `json:"warm"`
	Moved    int            `json:"moved"`
	Compared int            `json:"compared"`
}

// ComparedResult is one card: a real listing at its position on one side.
type ComparedResult struct {
	Rank    int               `json:"rank"`
	Listing inventory.Listing `json:"listing"`
	Score   float64           `json:"score"`
	Reasons []string          `json:"reasons,omitempty"`
	Delta   int               `json:"delta"`
}

// CompareResponse is what the page renders.
type CompareResponse struct {
	Query       string           `json:"query"`
	Intent      Intent           `json:"intent"`
	Persona     map[string]any   `json:"persona"`
	Cold        []ComparedResult `json:"cold"`
	Warm        []ComparedResult `json:"warm"`
	Moved       int              `json:"moved"`
	Compared    int              `json:"compared"`
	Relaxations []string         `json:"relaxations,omitempty"`
	TookMS      int64            `json:"took_ms"`
}

// Compare runs the retrieval half of the pipeline once, then ranks the result
// twice. The intent extractor is skipped in favour of the deterministic parser
// so repeated clicks give identical candidates: a comparison whose left-hand
// column changes between runs proves nothing about the right-hand one.
func (s *Service) Compare(ctx context.Context, query, persona string, limit int) (CompareResponse, error) {
	start := time.Now()
	ctx, span := otel.Tracer("search").Start(ctx, "search.compare")
	defer span.End()

	intent := s.extractor.fallback.Parse(query)
	listings, relaxations, err := s.retrieve(ctx, intent)
	if err != nil {
		span.RecordError(err)
		return CompareResponse{}, err
	}

	items, byID := s.candidates(ctx, listings)
	cmp, err := s.comparer.Compare(ctx, persona, items)
	if err != nil {
		span.RecordError(err)
		return CompareResponse{}, err
	}

	resp := CompareResponse{
		Query: query, Intent: intent, Persona: cmp.Persona,
		Moved: cmp.Moved, Compared: cmp.Compared, Relaxations: relaxations,
		Cold: attach(cmp.Cold, byID, limit),
		Warm: attach(cmp.Warm, byID, limit),
	}
	resp.TookMS = time.Since(start).Milliseconds()
	s.log.InfoContext(ctx, "comparison served", "query", query, "persona", persona,
		"candidates", cmp.Compared, "moved", cmp.Moved, "took_ms", resp.TookMS)
	return resp, nil
}

// candidates builds the rank payload, including live promotions, so the
// comparison carries the same business-rule term production does.
func (s *Service) candidates(ctx context.Context, listings []inventory.Listing) ([]RankItem, map[string]inventory.Listing) {
	promos, err := s.inv.ActivePromotions(ctx)
	if err != nil {
		promos = map[string]inventory.Promotion{}
	}
	byID := make(map[string]inventory.Listing, len(listings))
	items := make([]RankItem, 0, len(listings))
	for _, l := range listings {
		byID[l.ID] = l
		item := RankItem{
			ID: l.ID, BaseScore: baseScore(l), Category: l.Category,
			PriceCents: l.PricePerNightCents, Amenities: l.Amenities,
			VibeTags: l.VibeTags, MarginTier: l.MarginTier,
		}
		if p, ok := promos[l.ID]; ok {
			item.Promoted, item.PromoBoost, item.PromoLabel = true, p.Boost, p.Label
		}
		items = append(items, item)
	}
	return items, byID
}

func attach(ps []Placement, byID map[string]inventory.Listing, limit int) []ComparedResult {
	if limit > 0 && len(ps) > limit {
		ps = ps[:limit]
	}
	out := make([]ComparedResult, 0, len(ps))
	for _, p := range ps {
		l, ok := byID[p.ID]
		if !ok {
			continue
		}
		out = append(out, ComparedResult{
			Rank: p.Rank, Listing: l, Score: p.Score, Reasons: p.Reasons, Delta: p.Delta,
		})
	}
	return out
}
