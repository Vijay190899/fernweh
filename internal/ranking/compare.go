package ranking

// Side-by-side comparison of the same candidates ranked twice.
//
// The evaluation report says personalization beats the cold baseline, and the
// numbers are real, but a number is not a demonstration. Somebody reading the
// page cannot see the reordering, because in the live product the SQL filters
// have already narrowed the candidate set before the scorer touches it: by the
// time results reach the page, most of what personalization would have moved
// was never a candidate.
//
// This holds the candidate set fixed and varies only the profile. Same items,
// same scorer, same weights, one run with an empty profile and one with a
// declared persona. Whatever moves, moves because of personalization and
// nothing else.

// Placement is one item's position in one of the two orderings, carrying the
// score and reasons that put it there.
type Placement struct {
	Rank    int      `json:"rank"` // 1-based
	ID      string   `json:"id"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons,omitempty"`
	// Was is this item's rank in the other ordering. Both sides carry it,
	// because a page shows a window onto the ranking rather than all of it: an
	// item can be third on one side and fortieth on the other, and without the
	// counterpart rank the UI cannot say where an arrival came from or where a
	// departure went.
	Was int `json:"was"`
	// Delta is positions gained against the cold ordering: positive means it
	// moved up. Always 0 on the cold side, which is the reference.
	Delta int `json:"delta"`
}

// Comparison is one query's candidate set ranked cold and warm.
type Comparison struct {
	Persona Persona     `json:"persona"`
	Profile Profile     `json:"profile"`
	Cold    []Placement `json:"cold"`
	Warm    []Placement `json:"warm"`
	// Moved counts items whose position changed, out of Compared. Both are
	// over the full candidate set, not a truncated page, so the figure cannot
	// be flattered by choosing a cutoff.
	Moved    int `json:"moved"`
	Compared int `json:"compared"`
}

// PersonaByName looks up a declared persona. Callers must not accept an
// arbitrary profile from the client: the whole claim of this comparison is
// that both sides ran against a profile stated in source.
func PersonaByName(name string) (Persona, bool) {
	for _, p := range Personas() {
		if p.Name == name {
			return p, true
		}
	}
	return Persona{}, false
}

// Compare ranks items twice and annotates the warm ordering with how far each
// item travelled.
func Compare(items []Item, p Persona) Comparison {
	profile := p.Profile()
	cold := Rank(items, Profile{})
	warm := Rank(items, profile)

	coldRank := make(map[string]int, len(cold))
	warmRank := make(map[string]int, len(warm))
	for i, r := range cold {
		coldRank[r.ID] = i + 1
	}
	for i, r := range warm {
		warmRank[r.ID] = i + 1
	}

	out := Comparison{
		Persona:  p,
		Profile:  profile,
		Cold:     make([]Placement, len(cold)),
		Warm:     make([]Placement, len(warm)),
		Compared: len(items),
	}
	for i, r := range cold {
		// Cold is the reference side, so its delta stays zero; Was points at
		// where personalization sent this item. Its reasons are kept: an empty
		// profile explains nothing about the visitor, but a paid placement
		// still has to disclose itself, on both sides.
		out.Cold[i] = Placement{
			Rank: i + 1, ID: r.ID, Score: r.Score, Reasons: r.Reasons,
			Was: warmRank[r.ID],
		}
	}
	for i, r := range warm {
		pos := i + 1
		// Both orderings cover the same slice, so every id is present in both
		// maps. A zero would mean a bug rather than a state worth rendering.
		before := coldRank[r.ID]
		if before != pos {
			out.Moved++
		}
		out.Warm[i] = Placement{
			Rank: pos, ID: r.ID, Score: r.Score, Reasons: r.Reasons,
			Was: before, Delta: before - pos,
		}
	}
	return out
}
