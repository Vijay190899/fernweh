// Package ranking personalizes result order from session behavior, no
// accounts, no PII, just what this visitor did in this session (and a TTL so
// even that evaporates). Personalization is bounded by business rules and
// every score comes with human-readable reasons.
package ranking

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Event is one behavioral signal from the frontend.
type Event struct {
	SessionID  string   `json:"session_id"`
	Type       string   `json:"type"` // view|click|dwell|book|search
	ListingID  string   `json:"listing_id,omitempty"`
	Category   string   `json:"category,omitempty"`
	PriceCents int      `json:"price_cents,omitempty"`
	Amenities  []string `json:"amenities,omitempty"`
	VibeTags   []string `json:"vibe_tags,omitempty"`
}

// eventWeights: how strongly each interaction type expresses preference.
var eventWeights = map[string]int64{
	"view":   1,
	"search": 2,
	"dwell":  2,
	"click":  3,
	"book":   5,
}

const profileTTL = 7 * 24 * time.Hour

// SignalStore accumulates weighted signals into a per-session Redis hash.
type SignalStore struct {
	rdb *redis.Client
}

func NewSignalStore(rdb *redis.Client) *SignalStore { return &SignalStore{rdb: rdb} }

func profileKey(sessionID string) string { return "profile:v1:" + sessionID }

// Record folds an event into the session profile.
func (s *SignalStore) Record(ctx context.Context, ev Event) error {
	w, ok := eventWeights[ev.Type]
	if !ok {
		return fmt.Errorf("unknown event type %q", ev.Type)
	}
	key := profileKey(ev.SessionID)
	pipe := s.rdb.Pipeline()
	if ev.Category != "" {
		pipe.HIncrBy(ctx, key, "cat:"+ev.Category, w)
	}
	for _, a := range ev.Amenities {
		pipe.HIncrBy(ctx, key, "amen:"+strings.ToLower(a), w)
	}
	for _, v := range ev.VibeTags {
		pipe.HIncrBy(ctx, key, "vibe:"+strings.ToLower(v), w)
	}
	if ev.PriceCents > 0 {
		pipe.HIncrBy(ctx, key, "price_sum", int64(ev.PriceCents)*w)
		pipe.HIncrBy(ctx, key, "price_n", w)
	}
	pipe.HIncrBy(ctx, key, "events", 1)
	pipe.Expire(ctx, key, profileTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// Profile is the aggregated taste of one session.
type Profile struct {
	CategoryAffinity map[string]float64 `json:"category_affinity"` // sums to 1
	AmenityWeight    map[string]float64 `json:"amenity_weight"`    // sums to 1
	VibeWeight       map[string]float64 `json:"vibe_weight"`       // sums to 1
	AvgPriceCents    int                `json:"avg_price_cents"`
	Events           int                `json:"events"`
}

// Load builds the profile from raw counters. A brand-new session yields a
// zero profile, the scorer treats that as "no personalization".
func (s *SignalStore) Load(ctx context.Context, sessionID string) (Profile, error) {
	raw, err := s.rdb.HGetAll(ctx, profileKey(sessionID)).Result()
	if err != nil {
		return Profile{}, err
	}
	return buildProfile(raw), nil
}

func buildProfile(raw map[string]string) Profile {
	p := Profile{
		CategoryAffinity: map[string]float64{},
		AmenityWeight:    map[string]float64{},
		VibeWeight:       map[string]float64{},
	}
	var catTotal, amenTotal, vibeTotal, priceSum, priceN float64
	for field, val := range raw {
		n, _ := strconv.ParseFloat(val, 64)
		switch {
		case strings.HasPrefix(field, "cat:"):
			p.CategoryAffinity[field[4:]] = n
			catTotal += n
		case strings.HasPrefix(field, "amen:"):
			p.AmenityWeight[field[5:]] = n
			amenTotal += n
		case strings.HasPrefix(field, "vibe:"):
			p.VibeWeight[field[5:]] = n
			vibeTotal += n
		case field == "price_sum":
			priceSum = n
		case field == "price_n":
			priceN = n
		case field == "events":
			p.Events = int(n)
		}
	}
	normalize(p.CategoryAffinity, catTotal)
	normalize(p.AmenityWeight, amenTotal)
	normalize(p.VibeWeight, vibeTotal)
	if priceN > 0 {
		p.AvgPriceCents = int(priceSum / priceN)
	}
	return p
}

func normalize(m map[string]float64, total float64) {
	if total <= 0 {
		return
	}
	for k := range m {
		m[k] /= total
	}
}
