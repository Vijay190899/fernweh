package ranking

import "testing"

// Candidates deliberately span two categories and two price bands so the
// beach persona has something to pull up and something to push down.
func compareFixture() []Item {
	return []Item{
		// Highest base score, wrong category for the beach persona.
		{ID: "ski-top", BaseScore: 0.98, Category: "ski", PriceCents: 40000,
			Amenities: []string{"sauna"}, VibeTags: []string{"luxury"}, MarginTier: "standard"},
		{ID: "ski-mid", BaseScore: 0.90, Category: "ski", PriceCents: 30000,
			Amenities: []string{"spa"}, VibeTags: []string{"luxury"}, MarginTier: "standard"},
		// Weakest base score, but exactly what the beach persona asked for.
		{ID: "beach-match", BaseScore: 0.60, Category: "beach", PriceCents: 13000,
			Amenities: []string{"kids club", "pool"}, VibeTags: []string{"family"}, MarginTier: "standard"},
		{ID: "beach-plain", BaseScore: 0.70, Category: "beach", PriceCents: 50000,
			Amenities: []string{"bar"}, VibeTags: []string{"party"}, MarginTier: "standard"},
	}
}

func beachPersona(t *testing.T) Persona {
	t.Helper()
	p, ok := PersonaByName("Family by the sea")
	if !ok {
		t.Fatal("Family by the sea persona missing from Personas()")
	}
	return p
}

func TestCompareColdSideIgnoresPersona(t *testing.T) {
	items := compareFixture()
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	cmp := Compare(items, beachPersona(t))

	// Cold is pure base relevance, so the strongest base score leads however
	// well another item matches the persona. If this ever changes, the
	// comparison stops isolating personalization and means nothing.
	if cmp.Cold[0].ID != "ski-top" {
		t.Fatalf("cold ordering should follow base score, got %q first", cmp.Cold[0].ID)
	}
	for _, p := range cmp.Cold {
		if p.Delta != 0 {
			t.Errorf("cold placement %s carries delta %d, the cold side is the reference", p.ID, p.Delta)
		}
		// Every cold placement must be exactly what an empty profile produces,
		// score and explanation alike. Anything else means the persona leaked
		// into the reference side.
		want, wantReasons := Score(byID[p.ID], Profile{})
		if round4(want) != p.Score {
			t.Errorf("cold %s score = %v, unpersonalized scoring gives %v", p.ID, p.Score, round4(want))
		}
		if len(wantReasons) != len(p.Reasons) {
			t.Errorf("cold %s reasons = %v, unpersonalized scoring gives %v", p.ID, p.Reasons, wantReasons)
		}
	}
}

// A page shows a window onto the ranking, so an item can be visible on one
// side and far down the other. Both sides carry the counterpart rank, or the
// UI has no way to say where an arrival came from.
func TestCompareCarriesCounterpartRanks(t *testing.T) {
	items := compareFixture()
	cmp := Compare(items, beachPersona(t))

	warmRank := map[string]int{}
	for _, p := range cmp.Warm {
		warmRank[p.ID] = p.Rank
	}
	for _, p := range cmp.Cold {
		if p.Was != warmRank[p.ID] {
			t.Errorf("cold %s says it went to #%d, warm has it at #%d", p.ID, p.Was, warmRank[p.ID])
		}
		if p.Was == 0 {
			t.Errorf("cold %s has no counterpart rank", p.ID)
		}
	}
	coldRank := map[string]int{}
	for _, p := range cmp.Cold {
		coldRank[p.ID] = p.Rank
	}
	for _, p := range cmp.Warm {
		if p.Was != coldRank[p.ID] {
			t.Errorf("warm %s says it came from #%d, cold has it at #%d", p.ID, p.Was, coldRank[p.ID])
		}
		if p.Was-p.Rank != p.Delta {
			t.Errorf("warm %s: was %d, rank %d, delta %d do not agree", p.ID, p.Was, p.Rank, p.Delta)
		}
	}
}

func TestCompareWarmPromotesTheMatchingListing(t *testing.T) {
	items := compareFixture()
	cmp := Compare(items, beachPersona(t))

	if cmp.Warm[0].ID != "beach-match" {
		t.Fatalf("warm ordering should lead with the persona's match, got %q", cmp.Warm[0].ID)
	}
	if cmp.Warm[0].Delta <= 0 {
		t.Errorf("beach-match should have gained positions, delta=%d", cmp.Warm[0].Delta)
	}
	if len(cmp.Warm[0].Reasons) == 0 {
		t.Error("a promoted item must say why it was promoted")
	}
}

func TestCompareDeltasAreConsistent(t *testing.T) {
	items := compareFixture()
	cmp := Compare(items, beachPersona(t))

	if cmp.Compared != len(items) {
		t.Errorf("compared = %d, want %d", cmp.Compared, len(items))
	}
	if len(cmp.Cold) != len(items) || len(cmp.Warm) != len(items) {
		t.Fatalf("both sides must rank every candidate, got cold=%d warm=%d",
			len(cmp.Cold), len(cmp.Warm))
	}

	coldRank := map[string]int{}
	for _, p := range cmp.Cold {
		coldRank[p.ID] = p.Rank
	}
	moved, sum := 0, 0
	for _, p := range cmp.Warm {
		if want := coldRank[p.ID] - p.Rank; p.Delta != want {
			t.Errorf("%s delta = %d, want %d", p.ID, p.Delta, want)
		}
		if p.Delta != 0 {
			moved++
		}
		sum += p.Delta
	}
	if cmp.Moved != moved {
		t.Errorf("Moved = %d, counted %d", cmp.Moved, moved)
	}
	// Every position gained is a position lost by something else.
	if sum != 0 {
		t.Errorf("deltas sum to %d over a fixed candidate set, want 0", sum)
	}
}

func TestCompareEmptyProfilePersonaMovesNothing(t *testing.T) {
	// A persona whose profile cannot fire leaves the ordering untouched. This
	// is the guard against the comparison appearing to work because of some
	// incidental difference between the two calls rather than the profile.
	items := compareFixture()
	cmp := Compare(items, Persona{Name: "empty"})
	for _, p := range cmp.Warm {
		if p.Delta != 0 {
			t.Errorf("%s moved %d positions with a persona that matches nothing", p.ID, p.Delta)
		}
	}
	if cmp.Moved != 0 {
		t.Errorf("Moved = %d, want 0", cmp.Moved)
	}
}

func TestPersonaByNameRejectsUnknown(t *testing.T) {
	if _, ok := PersonaByName("Not a declared persona"); ok {
		t.Error("unknown persona accepted; the client must not be able to supply a profile")
	}
}
