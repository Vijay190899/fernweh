package search

import "fmt"

// RelaxStep is one rung of the zero-empty-results ladder: a modified intent
// plus the human-readable note shown to the user when this rung was needed.
type RelaxStep struct {
	Intent Intent
	Note   string
}

// RelaxationLadder returns intents to try in order, strictest first. The
// ladder drops the constraints travelers consider most negotiable first
// (vibes, amenities) and identity constraints last (destination, category);
// the final rung is unconstrained, so a non-empty inventory guarantees
// results. Every applied note is surfaced to the user, silent relaxation
// would be a dark pattern.
func RelaxationLadder(in Intent) []RelaxStep {
	steps := []RelaxStep{{Intent: in, Note: ""}}
	cur := in

	if len(cur.VibeTags) > 0 {
		cur.VibeTags = nil
		steps = append(steps, RelaxStep{cur, "relaxed style preferences"})
	}
	if len(cur.Amenities) > 0 {
		cur.Amenities = nil
		steps = append(steps, RelaxStep{cur, "relaxed amenity requirements"})
	}
	if cur.MinRating > 0 {
		cur.MinRating = 0
		steps = append(steps, RelaxStep{cur, "included lower-rated stays"})
	}
	if cur.BudgetMax > 0 {
		w := cur
		w.BudgetMax = int(float64(cur.BudgetMax) * 1.15)
		steps = append(steps, RelaxStep{w, "stretched budget by 15%"})
		w.BudgetMax = int(float64(cur.BudgetMax) * 1.30)
		steps = append(steps, RelaxStep{w, "stretched budget by 30%"})
		cur = w
	}
	if cur.Month != 0 {
		cur.Month = 0
		steps = append(steps, RelaxStep{cur, "flexible travel dates"})
	}
	if cur.Destination != "" && cur.Country != "" {
		w := cur
		w.Destination = ""
		steps = append(steps, RelaxStep{w, fmt.Sprintf("similar stays elsewhere in %s", cur.Country)})
		cur = w
	}
	if cur.Destination != "" || cur.Country != "" {
		w := cur
		w.Destination, w.Country = "", ""
		note := "similar stays in other destinations"
		if cur.Category != "" {
			note = fmt.Sprintf("best %s stays anywhere", cur.Category)
		}
		steps = append(steps, RelaxStep{w, note})
		cur = w
	}
	if cur.Category != "" {
		cur.Category = ""
		steps = append(steps, RelaxStep{cur, "expanded beyond " + in.Category + " trips"})
	}
	// Terminal rung: no constraints at all.
	if !cur.IsEmpty() {
		steps = append(steps, RelaxStep{Intent{}, "top-rated stays overall"})
	}
	return steps
}
