package search

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"fernweh/internal/inventory"
)

// Inventory is the slice of the repo this service needs (interface at the
// consumer, per Go convention, also what unit tests fake).
type Inventory interface {
	Search(ctx context.Context, f inventory.Filter) ([]inventory.Listing, error)
	ActivePromotions(ctx context.Context) (map[string]inventory.Promotion, error)
	Covers(ctx context.Context, destination, country string) (bool, error)
}

// Ranker personalizes result order. Implementations must be failure-safe:
// returning an error degrades to base ordering, never breaks search.
type Ranker interface {
	Rank(ctx context.Context, sessionID string, items []RankItem) ([]RankedItem, error)
}

type RankItem struct {
	ID         string   `json:"id"`
	BaseScore  float64  `json:"base_score"`
	Category   string   `json:"category"`
	PriceCents int      `json:"price_cents"`
	Amenities  []string `json:"amenities"`
	VibeTags   []string `json:"vibe_tags"`
	MarginTier string   `json:"margin_tier"`
	Promoted   bool     `json:"promoted"`
	PromoBoost float64  `json:"promo_boost"`
	PromoLabel string   `json:"promo_label,omitempty"`
}

type RankedItem struct {
	ID      string   `json:"id"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

type Result struct {
	Listing inventory.Listing `json:"listing"`
	Score   float64           `json:"score"`
	Reasons []string          `json:"reasons,omitempty"`
	Promo   string            `json:"promo,omitempty"`
}

type Response struct {
	Query        string   `json:"query"`
	Intent       Intent   `json:"intent"`
	IntentSource string   `json:"intent_source"`
	Results      []Result `json:"results"`
	Relaxations  []string `json:"relaxations,omitempty"`
	Degraded     []string `json:"degraded,omitempty"`
	// Unsupported names a place the traveller asked for that this platform
	// holds no inventory for. Stated plainly rather than quietly relaxed away.
	Unsupported string `json:"unsupported,omitempty"`
	TookMS      int64  `json:"took_ms"`
}

type Service struct {
	inv       Inventory
	extractor *Extractor
	ranker    Ranker
	log       *slog.Logger
}

func NewService(inv Inventory, ex *Extractor, ranker Ranker, log *slog.Logger) *Service {
	return &Service{inv: inv, extractor: ex, ranker: ranker, log: log}
}

// Search is the full pipeline: understand → retrieve (with relaxation) →
// personalize → explain. Partial failures degrade, they never abort.
func (s *Service) Search(ctx context.Context, query, sessionID string) (Response, error) {
	start := time.Now()
	tracer := otel.Tracer("search")
	ctx, span := tracer.Start(ctx, "search.pipeline")
	defer span.End()

	var resp Response
	resp.Query = query

	// 1. Understand.
	ctx1, extractSpan := tracer.Start(ctx, "search.extract_intent")
	intent, source := s.extractor.Extract(ctx1, query)
	extractSpan.SetAttributes(attribute.String("intent.source", source))
	extractSpan.End()
	resp.Intent, resp.IntentSource = intent, source
	if source == "fallback" {
		resp.Degraded = append(resp.Degraded, "llm_unavailable")
	}

	// The catalogue is regional. If the traveller named a place it does not
	// cover, say so and drop the constraint, rather than letting the ladder
	// widen to the whole world and present the result as a near match.
	if intent.Destination != "" || intent.Country != "" {
		covered, err := s.inv.Covers(ctx, intent.Destination, intent.Country)
		if err == nil && !covered {
			resp.Unsupported = intent.Destination
			if resp.Unsupported == "" {
				resp.Unsupported = intent.Country
			}
			intent.Destination, intent.Country = "", ""
			s.log.InfoContext(ctx, "destination not covered", "asked", resp.Unsupported)
		}
	}

	// 2. Retrieve, walking the relaxation ladder until something matches.
	ctx2, yieldSpan := tracer.Start(ctx, "search.retrieve")
	listings, relaxations, err := s.retrieve(ctx2, intent)
	yieldSpan.SetAttributes(attribute.Int("results.count", len(listings)),
		attribute.Int("results.relaxations", len(relaxations)))
	yieldSpan.End()
	if err != nil {
		span.RecordError(err)
		return resp, err
	}
	resp.Relaxations = relaxations

	// 3. Personalize, degrade to base order if ranking is unavailable.
	resp.Results = s.rank(ctx, sessionID, listings)
	if len(resp.Results) == 0 {
		for _, l := range listings {
			resp.Results = append(resp.Results, Result{Listing: l, Score: baseScore(l)})
		}
		resp.Degraded = append(resp.Degraded, "ranking_unavailable")
	}

	resp.TookMS = time.Since(start).Milliseconds()
	s.log.InfoContext(ctx, "search served", "query", query,
		"intent_source", source, "results", len(resp.Results),
		"relaxations", len(relaxations), "took_ms", resp.TookMS)
	return resp, nil
}

func (s *Service) retrieve(ctx context.Context, intent Intent) ([]inventory.Listing, []string, error) {
	var notes []string
	for _, step := range RelaxationLadder(intent) {
		listings, err := s.inv.Search(ctx, step.Intent.Filter(30))
		if err != nil {
			return nil, nil, err
		}
		if step.Note != "" {
			notes = append(notes, step.Note)
		}
		if len(listings) > 0 {
			if step.Note == "" {
				notes = nil // exact match, no relaxation applied
			}
			return listings, notes, nil
		}
	}
	return nil, notes, nil // only possible with an empty inventory
}

// rank calls the ranking service with a hard deadline; nil means degrade.
func (s *Service) rank(ctx context.Context, sessionID string, listings []inventory.Listing) []Result {
	if s.ranker == nil || len(listings) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	promos, err := s.inv.ActivePromotions(ctx)
	if err != nil {
		promos = map[string]inventory.Promotion{} // promos are enhancement, not dependency
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

	ranked, err := s.ranker.Rank(ctx, sessionID, items)
	if err != nil {
		s.log.WarnContext(ctx, "ranking degraded", "err", err)
		trace.SpanFromContext(ctx).AddEvent("ranking_degraded")
		return nil
	}
	out := make([]Result, 0, len(ranked))
	for _, r := range ranked {
		l, ok := byID[r.ID]
		if !ok {
			continue
		}
		res := Result{Listing: l, Score: r.Score, Reasons: r.Reasons}
		if p, ok := promos[l.ID]; ok {
			res.Promo = p.Label
		}
		out = append(out, res)
	}
	return out
}

// baseScore is pre-personalization relevance: mostly rating, tempered by how
// many reviews back it up.
func baseScore(l inventory.Listing) float64 {
	reviews := float64(l.ReviewCount)
	if reviews > 2000 {
		reviews = 2000
	}
	return (l.Rating/5.0)*0.75 + (reviews/2000.0)*0.25
}
