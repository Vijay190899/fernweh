package ranking

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Item is a search candidate as sent by the search service.
type Item struct {
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

// Ranked is an item with its final score and the reasons behind it.
type Ranked struct {
	ID      string   `json:"id"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

// Scoring weights. Base relevance dominates; personalization adjusts within
// it; business rules are a bounded thumb on the scale, never the hand.
const (
	wBase     = 0.55
	wPersonal = 0.30
	wBusiness = 0.15

	// maxBusinessBoost caps combined margin+promo influence (pre-weight) so a
	// paid placement can outrank a peer, not a clearly better match.
	maxBusinessBoost = 1.0
)

var marginBoosts = map[string]float64{
	"standard":  0.0,
	"preferred": 0.25,
	"premium":   0.45,
}

// Score computes one item's personalized score plus explanations.
func Score(item Item, p Profile) (float64, []string) {
	var reasons []string

	personal := 0.0
	if p.Events > 0 {
		catAff := p.CategoryAffinity[item.Category]
		amenAff := overlapWeight(item.Amenities, p.AmenityWeight)
		vibeAff := overlapWeight(item.VibeTags, p.VibeWeight)
		priceFit := priceFit(item.PriceCents, p.AvgPriceCents)
		personal = catAff*0.45 + amenAff*0.20 + vibeAff*0.15 + priceFit*0.20

		if catAff >= 0.4 {
			reasons = append(reasons, fmt.Sprintf("matches your interest in %s trips", item.Category))
		}
		if amenAff >= 0.25 {
			reasons = append(reasons, "has amenities you look for")
		}
		if vibeAff >= 0.25 {
			reasons = append(reasons, "fits the style you browse")
		}
		if priceFit >= 0.75 && p.AvgPriceCents > 0 {
			reasons = append(reasons, "in your usual price range")
		}
	}

	business := marginBoosts[item.MarginTier]
	if item.Promoted {
		business += math.Min(item.PromoBoost*3, 0.45) // promo boosts arrive as 0.05-0.15
		label := item.PromoLabel
		if label == "" {
			label = "Promoted"
		}
		reasons = append(reasons, label)
	} else if item.MarginTier != "standard" {
		reasons = append(reasons, "featured partner")
	}
	business = math.Min(business, maxBusinessBoost)

	score := item.BaseScore*wBase + personal*wPersonal + business*wBusiness
	return score, reasons
}

// Rank scores and orders all items (stable: equal scores keep base order).
func Rank(items []Item, p Profile) []Ranked {
	out := make([]Ranked, len(items))
	for i, item := range items {
		s, reasons := Score(item, p)
		out[i] = Ranked{ID: item.ID, Score: round4(s), Reasons: reasons}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// overlapWeight sums the profile weight of every attribute the item has -
// "how much of what this visitor cares about does this item cover".
func overlapWeight(attrs []string, weights map[string]float64) float64 {
	var sum float64
	for _, a := range attrs {
		sum += weights[strings.ToLower(a)]
	}
	return math.Min(sum, 1)
}

// priceFit is 1 at the visitor's average observed price, falling linearly to
// 0 at ±100% deviation. No price history → neutral 0.5.
func priceFit(price, avg int) float64 {
	if avg <= 0 || price <= 0 {
		return 0.5
	}
	dev := math.Abs(float64(price-avg)) / float64(avg)
	return math.Max(0, 1-dev)
}

func round4(f float64) float64 { return math.Round(f*10000) / 10000 }
