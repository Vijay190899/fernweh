// Package inventory is the shared read/write model for travel listings —
// the one domain package every service speaks.
package inventory

type Listing struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Category           string   `json:"category"`
	Destination        string   `json:"destination"`
	Country            string   `json:"country"`
	PricePerNightCents int      `json:"price_per_night_cents"`
	Currency           string   `json:"currency"`
	Rating             float64  `json:"rating"`
	ReviewCount        int      `json:"review_count"`
	Amenities          []string `json:"amenities"`
	VibeTags           []string `json:"vibe_tags"`
	Description        string   `json:"description"`
	ImageURL           string   `json:"image_url"`
	MonthsBest         []int    `json:"months_best"`
	MarginTier         string   `json:"margin_tier"`
	ContentStatus      string   `json:"content_status"`
}

// Content statuses.
const (
	StatusComplete        = "complete"
	StatusNeedsEnrichment = "needs_enrichment"
	StatusEnriching       = "enriching"
	StatusEnriched        = "enriched"
	StatusFailed          = "failed"
)

// Margin tiers, in ascending commercial value.
const (
	TierStandard  = "standard"
	TierPreferred = "preferred"
	TierPremium   = "premium"
)

type Promotion struct {
	ID        string  `json:"id"`
	ListingID string  `json:"listing_id"`
	Label     string  `json:"label"`
	Boost     float64 `json:"boost"`
}

// Filter is the structured search intent applied to inventory. Zero values
// mean "no constraint".
type Filter struct {
	Destination string
	Country     string
	Category    string
	BudgetMax   int // cents per night
	BudgetMin   int
	Month       int // 1..12
	MinRating   float64
	Amenities   []string
	VibeTags    []string
	Limit       int
}
