package search

import "testing"

// The deterministic parser is the search hot path whenever the LLM is
// skipped (cache miss + fallback) and the floor for worst-case latency.
func BenchmarkFallbackParse(b *testing.B) {
	p := FallbackParser{}
	queries := []string{
		"A beach weekend under €1,000 in March",
		"Städtetrip nach Wien im Oktober, ruhig und günstig",
		"family hotel in Crete with kids club, 4 stars",
	}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		p.Parse(queries[i%len(queries)])
	}
}

func BenchmarkRelaxationLadder(b *testing.B) {
	in := Intent{
		Destination: "Crete", Country: "Greece", Category: "beach",
		BudgetMax: 150, Month: 7, MinRating: 4,
		Amenities: []string{"pool"}, VibeTags: []string{"family"},
	}
	b.ReportAllocs()
	for b.Loop() {
		RelaxationLadder(in)
	}
}
