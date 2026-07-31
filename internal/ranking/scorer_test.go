package ranking

import (
	"strings"
	"testing"
)

func beachFan() Profile {
	return Profile{
		CategoryAffinity: map[string]float64{"beach": 0.8, "city": 0.2},
		AmenityWeight:    map[string]float64{"pool": 0.6, "spa": 0.4},
		VibeWeight:       map[string]float64{"family": 1},
		AvgPriceCents:    15000,
		Events:           12,
	}
}

func TestPersonalizationReordersForAffinity(t *testing.T) {
	// Same base quality; one matches the visitor's taste.
	beach := Item{ID: "beach", BaseScore: 0.8, Category: "beach",
		PriceCents: 15000, Amenities: []string{"pool"}, VibeTags: []string{"family"}, MarginTier: "standard"}
	city := Item{ID: "city", BaseScore: 0.8, Category: "city",
		PriceCents: 40000, MarginTier: "standard"}

	ranked := Rank([]Item{city, beach}, beachFan())
	if ranked[0].ID != "beach" {
		t.Fatalf("beach fan should see beach first, got %s", ranked[0].ID)
	}
	if len(ranked[0].Reasons) == 0 {
		t.Error("personalized winner must carry reasons")
	}
}

func TestColdSessionKeepsBaseOrder(t *testing.T) {
	a := Item{ID: "a", BaseScore: 0.9, Category: "beach", MarginTier: "standard"}
	b := Item{ID: "b", BaseScore: 0.7, Category: "city", MarginTier: "standard"}

	ranked := Rank([]Item{a, b}, Profile{})
	if ranked[0].ID != "a" || ranked[1].ID != "b" {
		t.Fatalf("cold session must follow base relevance, got %v", ranked)
	}
}

func TestBusinessBoostIsBounded(t *testing.T) {
	// A promoted premium listing with a mediocre base must NOT outrank a
	// clearly better unpromoted match.
	great := Item{ID: "great", BaseScore: 0.95, Category: "beach", MarginTier: "standard"}
	paid := Item{ID: "paid", BaseScore: 0.55, Category: "beach", MarginTier: "premium",
		Promoted: true, PromoBoost: 0.15, PromoLabel: "Summer Deal"}

	ranked := Rank([]Item{paid, great}, Profile{})
	if ranked[0].ID != "great" {
		t.Fatalf("bounded business rules must not let paid placement beat a much better match, got %s first", ranked[0].ID)
	}

	// But between near-peers, the promotion should tip the scale.
	peer := Item{ID: "peer", BaseScore: 0.60, Category: "beach", MarginTier: "standard"}
	ranked = Rank([]Item{peer, paid}, Profile{})
	if ranked[0].ID != "paid" {
		t.Fatalf("promotion should win between peers, got %s first", ranked[0].ID)
	}
	if !hasReason(ranked[0].Reasons, "Summer Deal") {
		t.Errorf("promotion must be disclosed in reasons, got %v", ranked[0].Reasons)
	}
}

func TestPriceFit(t *testing.T) {
	tests := []struct {
		price, avg int
		min, max   float64
	}{
		{15000, 15000, 0.99, 1.0}, // exact
		{30000, 15000, 0.0, 0.01}, // double = no fit
		{18000, 15000, 0.75, 0.85},
		{15000, 0, 0.5, 0.5}, // no history = neutral
	}
	for _, tt := range tests {
		got := priceFit(tt.price, tt.avg)
		if got < tt.min || got > tt.max {
			t.Errorf("priceFit(%d, %d) = %.3f, want in [%.2f, %.2f]",
				tt.price, tt.avg, got, tt.min, tt.max)
		}
	}
}

func TestBuildProfileFromRawCounters(t *testing.T) {
	p := buildProfile(map[string]string{
		"cat:beach": "6", "cat:city": "2",
		"amen:pool": "3", "amen:spa": "1",
		"price_sum": "120000", "price_n": "8",
		"events": "9",
	})
	if got := p.CategoryAffinity["beach"]; got != 0.75 {
		t.Errorf("beach affinity = %v, want 0.75", got)
	}
	if p.AvgPriceCents != 15000 {
		t.Errorf("avg price = %d, want 15000", p.AvgPriceCents)
	}
	if p.Events != 9 {
		t.Errorf("events = %d, want 9", p.Events)
	}
}

func hasReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}
