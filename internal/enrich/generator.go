// Package enrich fills content gaps in inventory: an Asynq-queued pipeline
// that scans for thin listings, generates descriptions and amenities from
// facts already on the listing (never invented claims), and records every
// change in an audit trail.
package enrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fernweh/internal/inventory"
	"fernweh/internal/platform/llm"
)

// Generated is the produced content plus its provenance.
type Generated struct {
	Description string
	Amenities   []string
	Source      string // "ai" | "template"
	Model       string
}

// Generator produces enriched content for a listing.
type Generator interface {
	Generate(ctx context.Context, l inventory.Listing) (Generated, error)
}

// ---- LLM generator with template fallback -------------------------------

type AIGenerator struct {
	llm      *llm.Client
	fallback TemplateGenerator
}

func NewAIGenerator(client *llm.Client) *AIGenerator {
	return &AIGenerator{llm: client}
}

const genSystemPrompt = `You improve travel listing content. You will receive the
structured facts of one listing. Write marketing copy using ONLY those facts -
never invent pools, views, distances, or claims not present in the input.
Reply with ONLY a JSON object:
  description: 2-3 sentences, warm but factual, mention destination and category
  amenities: the input amenities, optionally extended ONLY with items clearly
    implied by the input (e.g. "spa" implies "massage" is NOT allowed, only
    add what is strictly implied, or return the input list unchanged)`

func (g *AIGenerator) Generate(ctx context.Context, l inventory.Listing) (Generated, error) {
	facts, _ := json.Marshal(map[string]any{
		"name": l.Name, "category": l.Category, "destination": l.Destination,
		"country": l.Country, "rating": l.Rating, "review_count": l.ReviewCount,
		"price_per_night_eur": l.PricePerNightCents / 100,
		"amenities":           l.Amenities, "vibe_tags": l.VibeTags,
		"best_months": l.MonthsBest,
	})

	raw, err := g.llm.CompleteJSON(ctx, genSystemPrompt, string(facts))
	if err != nil {
		// ErrUnavailable (no key / budget / provider down) → template path.
		gen, terr := g.fallback.Generate(ctx, l)
		if terr != nil {
			return Generated{}, terr
		}
		return gen, nil
	}

	var out struct {
		Description string   `json:"description"`
		Amenities   []string `json:"amenities"`
	}
	if jerr := json.Unmarshal(raw, &out); jerr != nil || strings.TrimSpace(out.Description) == "" {
		return Generated{}, fmt.Errorf("model returned unusable content: %v", jerr)
	}
	amenities := sanitizeAmenities(out.Amenities, l.Amenities)
	return Generated{
		Description: strings.TrimSpace(out.Description),
		Amenities:   amenities,
		Source:      "ai",
		Model:       g.llm.Model(),
	}, nil
}

// sanitizeAmenities keeps the model honest: the result must contain every
// original amenity, adds at most 3 new ones, and everything is lowercased.
func sanitizeAmenities(proposed, original []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range original {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	added := 0
	for _, a := range proposed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" || seen[a] || added >= 3 || len(a) > 40 {
			continue
		}
		seen[a] = true
		out = append(out, a)
		added++
	}
	return out
}

// ---- Deterministic template generator -----------------------------------

// TemplateGenerator writes serviceable copy from listing facts alone. It is
// the guarantee that enrichment progresses even with zero LLM budget.
type TemplateGenerator struct{}

var monthNames = []string{"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

func (TemplateGenerator) Generate(_ context.Context, l inventory.Listing) (Generated, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s welcomes guests to %s, %s, a %s stay rated %.1f by %d travelers.",
		l.Name, l.Destination, l.Country, l.Category, l.Rating, l.ReviewCount)
	if len(l.Amenities) > 0 {
		fmt.Fprintf(&b, " On site: %s.", strings.Join(l.Amenities, ", "))
	}
	if len(l.MonthsBest) > 0 {
		fmt.Fprintf(&b, " Best visited %s to %s.",
			monthNames[l.MonthsBest[0]], monthNames[l.MonthsBest[len(l.MonthsBest)-1]])
	}
	fmt.Fprintf(&b, " Rates from €%d per night.", l.PricePerNightCents/100)

	return Generated{
		Description: b.String(),
		Amenities:   sanitizeAmenities(nil, l.Amenities),
		Source:      "template",
	}, nil
}

// ContentHash fingerprints the facts enrichment is derived from; if the facts
// have not changed, re-enriching is a no-op. Fields are sorted for stability.
func ContentHash(l inventory.Listing) string {
	amen := append([]string(nil), l.Amenities...)
	sort.Strings(amen)
	payload := fmt.Sprintf("%s|%s|%s|%s|%.1f|%d|%s",
		l.Name, l.Category, l.Destination, l.Country, l.Rating,
		l.PricePerNightCents, strings.Join(amen, ","))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:16])
}
