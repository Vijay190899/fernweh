package ranking

import (
	"fmt"
	"testing"
)

// Rank runs on every search response; 30 candidates is the production page
// size, 200 the API maximum.
func BenchmarkRank30(b *testing.B)  { benchmarkRank(b, 30) }
func BenchmarkRank200(b *testing.B) { benchmarkRank(b, 200) }

func benchmarkRank(b *testing.B, n int) {
	items := make([]Item, n)
	cats := []string{"beach", "city", "ski", "wellness"}
	for i := range items {
		items[i] = Item{
			ID: fmt.Sprintf("lst_%04d", i), BaseScore: float64(i%10) / 10,
			Category: cats[i%len(cats)], PriceCents: 8000 + i*100,
			Amenities: []string{"pool", "wifi"}, VibeTags: []string{"family"},
			MarginTier: "standard", Promoted: i%20 == 0, PromoBoost: 0.1,
		}
	}
	p := Profile{
		CategoryAffinity: map[string]float64{"beach": 0.7, "city": 0.3},
		AmenityWeight:    map[string]float64{"pool": 0.6, "wifi": 0.4},
		VibeWeight:       map[string]float64{"family": 1},
		AvgPriceCents:    12000, Events: 15,
	}
	b.ReportAllocs()
	for b.Loop() {
		Rank(items, p)
	}
}
