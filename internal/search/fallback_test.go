package search

import (
	"reflect"
	"testing"
)

func TestFallbackParser(t *testing.T) {
	p := FallbackParser{}

	tests := []struct {
		name  string
		query string
		want  Intent
	}{
		{
			name:  "the canonical marketing query",
			query: "A beach weekend under €1,000 in March",
			want:  Intent{Category: "beach", Month: 3, Nights: 2, BudgetMax: 500},
		},
		{
			name:  "destination with total budget and week",
			query: "one week in Mallorca under 1400 euro with pool",
			want: Intent{Destination: "Mallorca", Country: "Spain",
				Nights: 7, BudgetMax: 200, Amenities: []string{"pool"}},
		},
		{
			name:  "german city break",
			query: "Städtetrip nach Wien im Oktober, ruhig und günstig",
			want: Intent{Destination: "Vienna", Country: "Austria", Category: "city",
				Month: 10, VibeTags: []string{"quiet", "budget"}},
		},
		{
			name:  "per-night budget stays per-night",
			query: "ski chalet in Zermatt max €400 per night",
			want: Intent{Destination: "Zermatt", Country: "Switzerland",
				Category: "ski", BudgetMax: 400},
		},
		{
			name:  "family beach with rating",
			query: "family hotel in Crete with kids club, 4 stars",
			want: Intent{Destination: "Crete", Country: "Greece",
				MinRating: 4, VibeTags: []string{"family"}, Amenities: []string{"kids club"}},
		},
		{
			name:  "country only",
			query: "romantic getaway in Portugal",
			want:  Intent{Country: "Portugal", VibeTags: []string{"romantic"}},
		},
		{
			name:  "no signal at all",
			query: "surprise me",
			want:  Intent{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.query)
			if !reflect.DeepEqual(got, tt.want.Normalize()) {
				t.Errorf("Parse(%q)\n got: %+v\nwant: %+v", tt.query, got, tt.want.Normalize())
			}
		})
	}
}

func TestPerNightBudget(t *testing.T) {
	tests := []struct {
		amount, nights int
		explicit       bool
		want           int
	}{
		{250, 0, false, 250},  // small amounts read as nightly
		{1000, 2, false, 500}, // weekend total
		{1400, 7, false, 200}, // week total
		{2100, 0, false, 300}, // unknown length defaults to 7 nights
		{800, 2, true, 800},   // explicit per-night wins regardless
	}
	for _, tt := range tests {
		if got := perNightBudget(tt.amount, tt.nights, tt.explicit); got != tt.want {
			t.Errorf("perNightBudget(%d, %d, %v) = %d, want %d",
				tt.amount, tt.nights, tt.explicit, got, tt.want)
		}
	}
}

func TestNormalizeClampsHostileValues(t *testing.T) {
	in := Intent{
		Category:  "casino", // not a real category
		Month:     13,       // invalid
		BudgetMax: -5,       // negative
		MinRating: 9.9,      // above scale
		Nights:    500,      // absurd
		Amenities: []string{"Pool", "pool", "POOL", "spa", "gym", "bar", "wifi"},
	}.Normalize()

	if in.Category != "" || in.Month != 0 || in.BudgetMax != 0 || in.MinRating != 0 || in.Nights != 0 {
		t.Errorf("hostile values not clamped: %+v", in)
	}
	if len(in.Amenities) != 5 {
		t.Errorf("amenities not deduped/capped: %v", in.Amenities)
	}
	if in.Amenities[0] != "pool" {
		t.Errorf("amenities not lowercased: %v", in.Amenities)
	}
}
