package ranking

import (
	"fmt"
	"testing"
)

// A catalogue shaped like the seeded inventory: several categories, a spread
// of prices, and amenities that only sometimes match a persona.
func evalCatalogue() []Item {
	cats := []string{"beach", "ski", "city", "wellness", "countryside", "adventure"}
	amenities := map[string][]string{
		"beach":       {"kids club", "pool", "sea view"},
		"ski":         {"sauna", "spa", "fireplace"},
		"city":        {"breakfast", "wifi", "gym"},
		"wellness":    {"spa", "thermal baths", "yoga classes"},
		"countryside": {"vineyard", "farm-to-table dining", "garden"},
		"adventure":   {"guided hikes", "gear rental", "sauna"},
	}
	var out []Item
	for i := 0; i < 300; i++ {
		cat := cats[i%len(cats)]
		pool := amenities[cat]
		// Vary how many amenities each listing carries, so relevance grading
		// produces a spread rather than a single value.
		n := 1 + i%3
		item := Item{
			ID:         fmt.Sprintf("lst_%04d", i),
			BaseScore:  float64((i*37)%100) / 100,
			Category:   cat,
			PriceCents: 4000 + (i*911)%56000,
			Amenities:  pool[:n],
			VibeTags:   []string{"family", "luxury", "budget", "quiet", "foodie", "nature"}[i%6 : i%6+1],
			MarginTier: []string{"standard", "preferred", "premium"}[i%3],
		}
		out = append(out, item)
	}
	return out
}

func TestPersonalizationBeatsBaseline(t *testing.T) {
	rep := Evaluate(evalCatalogue())

	if rep.Catalogue != 300 || rep.Personas == 0 {
		t.Fatalf("evaluation did not run: %+v", rep)
	}
	// The whole claim of the ranking service is that a known profile surfaces
	// relevant inventory better than no profile at all. If that stops being
	// true, the product claim is false and this must fail.
	if rep.Personalized.NDCG10 <= rep.Baseline.NDCG10 {
		t.Errorf("personalized NDCG@10 %.4f did not beat baseline %.4f",
			rep.Personalized.NDCG10, rep.Baseline.NDCG10)
	}
	if rep.Personalized.Precision10 <= rep.Baseline.Precision10 {
		t.Errorf("personalized P@10 %.4f did not beat baseline %.4f",
			rep.Personalized.Precision10, rep.Baseline.Precision10)
	}
	t.Logf("NDCG@10 %.4f vs %.4f, P@10 %.4f vs %.4f, coverage %.3f",
		rep.Personalized.NDCG10, rep.Baseline.NDCG10,
		rep.Personalized.Precision10, rep.Baseline.Precision10,
		rep.Personalized.Coverage)
}

func TestMetricsStayInRange(t *testing.T) {
	rep := Evaluate(evalCatalogue())
	checks := map[string]float64{
		"ndcg": rep.Personalized.NDCG10, "precision": rep.Personalized.Precision10,
		"recall": rep.Personalized.Recall10, "map": rep.Personalized.MAP,
		"diversity": rep.Personalized.Diversity10, "coverage": rep.Personalized.Coverage,
	}
	for name, v := range checks {
		if v < 0 || v > 1 {
			t.Errorf("%s = %v, outside 0..1", name, v)
		}
	}
}

func TestRelevanceGrading(t *testing.T) {
	p := Persona{Name: "t", Category: "beach", Amenities: []string{"pool"}, PriceMin: 10000, PriceMax: 20000}
	tests := []struct {
		name string
		item Item
		want int
	}{
		{"wrong category", Item{Category: "ski", PriceCents: 15000, Amenities: []string{"pool"}}, 0},
		{"category only", Item{Category: "beach", PriceCents: 50000, Amenities: []string{"bar"}}, 1},
		{"price only", Item{Category: "beach", PriceCents: 15000, Amenities: []string{"bar"}}, 2},
		{"amenity only", Item{Category: "beach", PriceCents: 50000, Amenities: []string{"pool"}}, 2},
		{"both", Item{Category: "beach", PriceCents: 15000, Amenities: []string{"pool"}}, 3},
	}
	for _, tt := range tests {
		if got := p.Relevance(tt.item); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestEmptyCatalogueIsSafe(t *testing.T) {
	rep := Evaluate(nil)
	if rep.Catalogue != 0 || rep.Personalized.NDCG10 != 0 {
		t.Errorf("empty catalogue should produce zeroed metrics, got %+v", rep)
	}
}
