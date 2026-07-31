package search

import "testing"

func TestRelaxationLadder(t *testing.T) {
	full := Intent{
		Destination: "Crete", Country: "Greece", Category: "beach",
		BudgetMax: 150, Month: 7, MinRating: 4,
		Amenities: []string{"pool"}, VibeTags: []string{"family"},
	}

	steps := RelaxationLadder(full)

	if steps[0].Note != "" {
		t.Fatal("first rung must be the unrelaxed intent")
	}
	last := steps[len(steps)-1]
	if !last.Intent.IsEmpty() {
		t.Fatalf("terminal rung must be unconstrained, got %+v", last.Intent)
	}

	// Constraints must disappear monotonically in negotiability order:
	// vibes before amenities before budget before destination.
	order := map[string]int{}
	for i, s := range steps {
		order[s.Note] = i
	}
	if !(order["relaxed style preferences"] < order["relaxed amenity requirements"]) {
		t.Error("vibes must relax before amenities")
	}
	if !(order["relaxed amenity requirements"] < order["stretched budget by 15%"]) {
		t.Error("amenities must relax before budget")
	}
	if !(order["stretched budget by 30%"] < order["similar stays elsewhere in Greece"]) {
		t.Error("budget must relax before destination")
	}

	// Budget rungs widen from the original, not compounding.
	var got15, got30 int
	for _, s := range steps {
		switch s.Note {
		case "stretched budget by 15%":
			got15 = s.Intent.BudgetMax
		case "stretched budget by 30%":
			got30 = s.Intent.BudgetMax
		}
	}
	if got15 != 172 || got30 != 195 {
		t.Errorf("budget widening wrong: 15%%=%d (want 172), 30%%=%d (want 195)", got15, got30)
	}
}

func TestRelaxationLadderEmptyIntent(t *testing.T) {
	steps := RelaxationLadder(Intent{})
	if len(steps) != 1 {
		t.Fatalf("empty intent needs no relaxation rungs, got %d", len(steps))
	}
}

func TestRelaxationLadderCategoryOnly(t *testing.T) {
	steps := RelaxationLadder(Intent{Category: "ski"})
	last := steps[len(steps)-1]
	if !last.Intent.IsEmpty() {
		t.Fatalf("ladder must terminate unconstrained, got %+v", last.Intent)
	}
}
