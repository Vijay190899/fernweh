package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"fernweh/internal/platform/llm"
)

// Extractor resolves a query to an Intent, trying cache → LLM → fallback.
// The Source of the winning path is reported so the UI (and Jaeger) can show
// exactly how each query was understood.
type Extractor struct {
	llm      *llm.Client
	rdb      *redis.Client
	fallback FallbackParser
}

func NewExtractor(l *llm.Client, rdb *redis.Client) *Extractor {
	return &Extractor{llm: l, rdb: rdb}
}

const systemPrompt = `You extract structured travel search intent from a user query
(English or German). Reply with ONLY a JSON object, no prose, with any of these
optional fields:
  destination: one of [Algarve, Mallorca, Crete, Santorini, Amalfi Coast, Costa
    Brava, Dubrovnik, Canary Islands, Sardinia, Madeira, Cyprus, Berlin, Paris,
    Barcelona, Rome, Prague, Vienna, Amsterdam, Lisbon, Budapest, Copenhagen,
    Zermatt, Innsbruck, Chamonix, Livigno, Baden-Baden, Lake Bled, Tuscany,
    Provence, Douro Valley, Azores, Norwegian Fjords, Scottish Highlands]
  country: full English country name, only if clearly implied
  category: one of [beach, city, ski, wellness, countryside, adventure]
  budget_max_eur: integer, EUR PER NIGHT. If the user gives a total trip
    budget, divide by trip length in nights (assume 7 nights if unstated).
  month: integer 1-12
  nights: integer trip length in nights (weekend = 2, week = 7)
  min_rating: number 0-5 (e.g. "4 stars" -> 4)
  amenities: array from [pool, beach access, sea view, spa, kids club,
    all-inclusive, water sports, air conditioning, breakfast, wifi, bar, gym,
    sauna, parking, pet friendly]
  vibe_tags: array from [family, romantic, party, quiet, luxury, budget,
    foodie, culture, nature]
Omit any field the query does not imply. Never invent constraints.`

// Extract returns the intent and its source: "cache", "llm", or "fallback".
func (e *Extractor) Extract(ctx context.Context, query string) (Intent, string) {
	key := cacheKey(query)

	if e.rdb != nil {
		if raw, err := e.rdb.Get(ctx, key).Bytes(); err == nil {
			var in Intent
			if json.Unmarshal(raw, &in) == nil {
				return in.Normalize(), "cache"
			}
		}
	}

	if raw, err := e.llm.CompleteJSON(ctx, systemPrompt, query); err == nil {
		var in Intent
		if json.Unmarshal(raw, &in) == nil {
			in = in.Normalize()
			// A model reply that extracted nothing from a non-trivial query is
			// suspect — cross-check with rules rather than trusting silence.
			if in.IsEmpty() {
				in = e.fallback.Parse(query)
			}
			e.cache(ctx, key, in)
			return in, "llm"
		}
	}

	in := e.fallback.Parse(query)
	e.cache(ctx, key, in)
	return in, "fallback"
}

func (e *Extractor) cache(ctx context.Context, key string, in Intent) {
	if e.rdb == nil {
		return
	}
	if b, err := json.Marshal(in); err == nil {
		e.rdb.Set(ctx, key, b, 24*time.Hour)
	}
}

func cacheKey(query string) string {
	norm := strings.Join(strings.Fields(strings.ToLower(query)), " ")
	sum := sha256.Sum256([]byte(norm))
	return "intent:v1:" + hex.EncodeToString(sum[:8])
}
