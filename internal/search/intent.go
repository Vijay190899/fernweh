// Package search turns a natural-language travel wish into ranked inventory.
// Intent extraction has two implementations behind one contract, an LLM and
// a deterministic parser, so search works identically (if less cleverly)
// when no AI is available.
package search

import (
	"strings"

	"fernweh/internal/inventory"
)

// Intent is the structured meaning of a travel query. Budgets are EUR per
// night; a total trip budget is converted using the trip length (default 7
// nights) before it gets here.
type Intent struct {
	Destination string   `json:"destination,omitempty"`
	Country     string   `json:"country,omitempty"`
	Category    string   `json:"category,omitempty"`
	BudgetMax   int      `json:"budget_max_eur,omitempty"`
	BudgetMin   int      `json:"budget_min_eur,omitempty"`
	Month       int      `json:"month,omitempty"`
	Nights      int      `json:"nights,omitempty"`
	MinRating   float64  `json:"min_rating,omitempty"`
	Amenities   []string `json:"amenities,omitempty"`
	VibeTags    []string `json:"vibe_tags,omitempty"`
}

var validCategories = map[string]bool{
	"beach": true, "city": true, "ski": true,
	"wellness": true, "countryside": true, "adventure": true,
}

// Normalize clamps model/parser output to valid ranges so downstream SQL only
// ever sees sane values, wherever the intent came from.
func (in Intent) Normalize() Intent {
	if !validCategories[in.Category] {
		in.Category = ""
	}
	if in.Month < 1 || in.Month > 12 {
		in.Month = 0
	}
	if in.BudgetMax < 0 || in.BudgetMax > 100000 {
		in.BudgetMax = 0
	}
	if in.BudgetMin < 0 || in.BudgetMin >= in.BudgetMax {
		in.BudgetMin = 0
	}
	if in.MinRating < 0 || in.MinRating > 5 {
		in.MinRating = 0
	}
	if in.Nights <= 0 || in.Nights > 60 {
		in.Nights = 0
	}
	in.Destination = strings.TrimSpace(in.Destination)
	in.Country = strings.TrimSpace(in.Country)
	in.Amenities = lowerAll(in.Amenities, 5)
	in.VibeTags = lowerAll(in.VibeTags, 3)
	return in
}

// Filter converts the intent into the inventory query.
func (in Intent) Filter(limit int) inventory.Filter {
	return inventory.Filter{
		Destination: in.Destination,
		Country:     in.Country,
		Category:    in.Category,
		BudgetMax:   in.BudgetMax * 100,
		BudgetMin:   in.BudgetMin * 100,
		Month:       in.Month,
		MinRating:   in.MinRating,
		Amenities:   in.Amenities,
		VibeTags:    in.VibeTags,
		Limit:       limit,
	}
}

// IsEmpty reports whether nothing usable was extracted.
func (in Intent) IsEmpty() bool {
	return in.Destination == "" && in.Country == "" && in.Category == "" &&
		in.BudgetMax == 0 && in.Month == 0 && len(in.Amenities) == 0 &&
		len(in.VibeTags) == 0 && in.MinRating == 0
}

func lowerAll(ss []string, max int) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range ss {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == max {
			break
		}
	}
	return out
}
