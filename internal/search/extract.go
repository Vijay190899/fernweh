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

// cached is the cache envelope. The source travels with the intent because a
// cache hit has to be able to say what originally produced the value.
//
// Storing the bare intent lost that: a query answered by the deterministic
// parser while the model was unavailable was cached, and every later hit was
// reported as "cache" with no degradation flagged. The response then claimed,
// truthfully but misleadingly, that the intent came from the cache, with
// nothing to say the model had never seen the query. Degradation that stops
// being disclosed after the first request is worse than not degrading
// gracefully at all, because it is invisible.
type cached struct {
	Intent Intent `json:"intent"`
	Source string `json:"source"`
}

// Extract returns the intent and its source: "cache", "llm", or "fallback".
// A cache hit whose value was produced by the fallback parser still reports
// "fallback", so the disclosure survives caching.
func (e *Extractor) Extract(ctx context.Context, query string) (Intent, string) {
	key := cacheKey(query)

	if e.rdb != nil {
		if raw, err := e.rdb.Get(ctx, key).Bytes(); err == nil {
			var c cached
			if json.Unmarshal(raw, &c) == nil && c.Source != "" {
				if c.Source == "fallback" {
					return c.Intent.Normalize(), "fallback"
				}
				return c.Intent.Normalize(), "cache"
			}
		}
	}

	if raw, err := e.llm.CompleteJSON(ctx, systemPrompt, query); err == nil {
		var in Intent
		if json.Unmarshal(raw, &in) == nil {
			in = in.Normalize()
			// A model reply that extracted nothing from a non-trivial query is
			// suspect, cross-check with rules rather than trusting silence.
			if in.IsEmpty() {
				in = e.fallback.Parse(query)
			}
			e.cache(ctx, key, in, "llm")
			return in, "llm"
		}
	}

	in := e.fallback.Parse(query)
	// Cached for a short window only. The rule parser costs nothing to re-run,
	// and the reason to remember its answer at all is to avoid retrying a model
	// that is currently down on every single request. Once it recovers, queries
	// should go back to it quickly rather than being pinned to a degraded
	// reading for a day.
	e.cache(ctx, key, in, "fallback")
	return in, "fallback"
}

// Model answers are worth remembering for a day; a degraded reading is worth
// remembering only long enough to stop a downed model being retried on every
// request. Different values, so different lifetimes.
const (
	llmIntentTTL      = 24 * time.Hour
	fallbackIntentTTL = 10 * time.Minute
)

func (e *Extractor) cache(ctx context.Context, key string, in Intent, source string) {
	if e.rdb == nil {
		return
	}
	ttl := llmIntentTTL
	if source == "fallback" {
		ttl = fallbackIntentTTL
	}
	if b, err := json.Marshal(cached{Intent: in, Source: source}); err == nil {
		e.rdb.Set(ctx, key, b, ttl)
	}
}

// The version in the key is not decoration. The cached value changed shape
// when the source moved inside it, and entries written by the previous build
// would silently unmarshal into a zero-valued envelope.
func cacheKey(query string) string {
	norm := strings.Join(strings.Fields(strings.ToLower(query)), " ")
	sum := sha256.Sum256([]byte(norm))
	return "intent:v2:" + hex.EncodeToString(sum[:8])
}
