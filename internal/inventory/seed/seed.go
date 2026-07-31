// Package seed generates a deterministic European travel inventory. A fixed
// PRNG seed makes every environment identical and re-runs idempotent (stable
// listing IDs, ON CONFLICT DO NOTHING).
package seed

import (
	"fmt"
	"math/rand"
	"strings"

	"fernweh/internal/inventory"
)

type destination struct {
	name     string
	country  string
	category string
	months   []int
	minPrice int // EUR/night
	maxPrice int
}

var destinations = []destination{
	// beach
	{"Algarve", "Portugal", "beach", []int{4, 5, 6, 7, 8, 9, 10}, 55, 320},
	{"Mallorca", "Spain", "beach", []int{5, 6, 7, 8, 9}, 70, 400},
	{"Crete", "Greece", "beach", []int{5, 6, 7, 8, 9, 10}, 50, 300},
	{"Santorini", "Greece", "beach", []int{5, 6, 7, 8, 9}, 120, 650},
	{"Amalfi Coast", "Italy", "beach", []int{5, 6, 7, 8, 9}, 140, 700},
	{"Costa Brava", "Spain", "beach", []int{5, 6, 7, 8, 9}, 60, 280},
	{"Dubrovnik", "Croatia", "beach", []int{5, 6, 7, 8, 9}, 80, 380},
	{"Canary Islands", "Spain", "beach", []int{1, 2, 3, 4, 5, 10, 11, 12}, 65, 350},
	{"Sardinia", "Italy", "beach", []int{5, 6, 7, 8, 9}, 75, 420},
	{"Madeira", "Portugal", "beach", []int{3, 4, 5, 6, 9, 10, 11}, 60, 300},
	{"Cyprus", "Cyprus", "beach", []int{4, 5, 6, 9, 10}, 55, 260},
	// city
	{"Berlin", "Germany", "city", []int{4, 5, 6, 7, 8, 9, 10}, 65, 320},
	{"Paris", "France", "city", []int{3, 4, 5, 6, 9, 10}, 110, 600},
	{"Barcelona", "Spain", "city", []int{3, 4, 5, 6, 9, 10}, 90, 450},
	{"Rome", "Italy", "city", []int{3, 4, 5, 9, 10, 11}, 85, 480},
	{"Prague", "Czechia", "city", []int{4, 5, 6, 9, 10}, 55, 260},
	{"Vienna", "Austria", "city", []int{4, 5, 6, 9, 10, 12}, 75, 380},
	{"Amsterdam", "Netherlands", "city", []int{4, 5, 6, 7, 8, 9}, 100, 480},
	{"Lisbon", "Portugal", "city", []int{3, 4, 5, 6, 9, 10}, 70, 350},
	{"Budapest", "Hungary", "city", []int{4, 5, 6, 9, 10}, 50, 240},
	{"Copenhagen", "Denmark", "city", []int{5, 6, 7, 8}, 110, 500},
	// ski
	{"Zermatt", "Switzerland", "ski", []int{12, 1, 2, 3, 4}, 160, 800},
	{"Innsbruck", "Austria", "ski", []int{12, 1, 2, 3}, 90, 420},
	{"Chamonix", "France", "ski", []int{12, 1, 2, 3, 4}, 110, 550},
	{"Livigno", "Italy", "ski", []int{12, 1, 2, 3}, 80, 380},
	// wellness / countryside
	{"Baden-Baden", "Germany", "wellness", []int{3, 4, 5, 9, 10, 11}, 95, 450},
	{"Lake Bled", "Slovenia", "wellness", []int{5, 6, 7, 8, 9}, 85, 400},
	{"Tuscany", "Italy", "countryside", []int{4, 5, 6, 9, 10}, 80, 420},
	{"Provence", "France", "countryside", []int{5, 6, 7, 9}, 75, 380},
	{"Douro Valley", "Portugal", "countryside", []int{5, 6, 9, 10}, 65, 320},
	// adventure
	{"Azores", "Portugal", "adventure", []int{5, 6, 7, 8, 9}, 60, 280},
	{"Norwegian Fjords", "Norway", "adventure", []int{5, 6, 7, 8}, 120, 520},
	{"Scottish Highlands", "United Kingdom", "adventure", []int{5, 6, 7, 8, 9}, 70, 340},
}

var amenityPools = map[string][]string{
	"beach":       {"pool", "beach access", "sea view", "spa", "kids club", "all-inclusive", "water sports", "air conditioning", "breakfast", "wifi", "bar"},
	"city":        {"breakfast", "wifi", "gym", "rooftop bar", "concierge", "air conditioning", "bike rental", "pet friendly", "parking"},
	"ski":         {"ski-in/ski-out", "sauna", "fireplace", "spa", "ski storage", "breakfast", "wifi", "restaurant", "hot tub"},
	"wellness":    {"spa", "thermal baths", "yoga classes", "pool", "massage", "vegetarian menu", "quiet zone", "wifi", "garden"},
	"countryside": {"vineyard", "pool", "farm-to-table dining", "bike rental", "garden", "cooking classes", "wifi", "parking", "pet friendly"},
	"adventure":   {"guided hikes", "gear rental", "sauna", "breakfast", "packed lunches", "wifi", "parking", "hot tub"},
}

var vibePools = map[string][]string{
	"beach":       {"family", "romantic", "party", "quiet", "luxury", "budget"},
	"city":        {"culture", "foodie", "party", "romantic", "budget", "luxury"},
	"ski":         {"family", "luxury", "party", "quiet"},
	"wellness":    {"quiet", "romantic", "luxury"},
	"countryside": {"quiet", "romantic", "foodie", "family", "nature"},
	"adventure":   {"nature", "quiet", "family", "budget"},
}

var namePatterns = map[string][]string{
	"beach":       {"Hotel %s %s", "%s Beach Resort", "Casa %s", "%s Playa Suites", "Villa %s"},
	"city":        {"Hotel %s", "%s City Apartments", "The %s House", "Boutique Hotel %s", "%s Central"},
	"ski":         {"Chalet %s", "Alpenhotel %s", "%s Lodge", "Berghaus %s"},
	"wellness":    {"%s Spa Retreat", "Therme %s", "Villa %s Wellness"},
	"countryside": {"Quinta %s", "Agriturismo %s", "Domaine %s", "%s Country House"},
	"adventure":   {"%s Basecamp", "Lodge %s", "%s Explorer Huts"},
}

var nameWords = []string{"Aurora", "Meridian", "Sol", "Luna", "Mar Azul", "Bellevue", "Panorama",
	"Estrela", "Verde", "Alba", "Serena", "Horizon", "Mirador", "Laguna", "Riviera", "Vista",
	"Amara", "Cielo", "Brisa", "Onda", "Perla", "Sirena", "Fortuna", "Palma", "Corona"}

// Generate produces n listings plus promotions. gapRatio of listings get
// deliberately missing/thin content (the enrichment demo's raw material).
func Generate(n int, gapRatio float64) ([]inventory.Listing, []inventory.Promotion) {
	rng := rand.New(rand.NewSource(42))
	listings := make([]inventory.Listing, 0, n)

	for i := 0; i < n; i++ {
		d := destinations[rng.Intn(len(destinations))]
		pattern := namePatterns[d.category][rng.Intn(len(namePatterns[d.category]))]
		word := nameWords[rng.Intn(len(nameWords))]
		name := pattern
		if strings.Count(pattern, "%s") == 2 {
			name = fmt.Sprintf(pattern, word, d.name)
		} else {
			name = fmt.Sprintf(pattern, word)
		}

		price := d.minPrice + rng.Intn(d.maxPrice-d.minPrice+1)
		rating := 3.5 + rng.Float64()*1.5
		pool := amenityPools[d.category]
		amenities := pick(rng, pool, 3+rng.Intn(4))
		vibes := pick(rng, vibePools[d.category], 1+rng.Intn(2))

		id := fmt.Sprintf("lst_%04d", i+1)
		l := inventory.Listing{
			ID:                 id,
			Name:               name,
			Category:           d.category,
			Destination:        d.name,
			Country:            d.country,
			PricePerNightCents: price * 100,
			Currency:           "EUR",
			Rating:             float64(int(rating*10)) / 10,
			ReviewCount:        20 + rng.Intn(1800),
			Amenities:          amenities,
			VibeTags:           vibes,
			ImageURL:           fmt.Sprintf("https://picsum.photos/seed/%s/640/420", id),
			MonthsBest:         d.months,
			MarginTier:         tier(rng),
			ContentStatus:      inventory.StatusComplete,
		}

		if rng.Float64() < gapRatio {
			// content gap: missing or thin description, often missing amenities
			l.ContentStatus = inventory.StatusNeedsEnrichment
			switch rng.Intn(3) {
			case 0:
				l.Description = ""
			case 1:
				l.Description = "Nice place to stay in " + d.name + "."
			case 2:
				l.Description = ""
				l.Amenities = l.Amenities[:1]
			}
		} else {
			l.Description = fullDescription(l)
		}
		listings = append(listings, l)
	}

	// Promotions on ~5% of listings — the merchandising layer ranking must honor.
	labels := []string{"Summer Deal", "Partner Highlight", "Early Bird", "Last Minute"}
	var promos []inventory.Promotion
	for i, l := range listings {
		if rng.Float64() < 0.05 {
			promos = append(promos, inventory.Promotion{
				ID:        fmt.Sprintf("pro_%04d", i+1),
				ListingID: l.ID,
				Label:     labels[rng.Intn(len(labels))],
				Boost:     0.05 + rng.Float64()*0.10,
			})
		}
	}
	return listings, promos
}

func pick(rng *rand.Rand, pool []string, n int) []string {
	idx := rng.Perm(len(pool))
	if n > len(pool) {
		n = len(pool)
	}
	out := make([]string, 0, n)
	for _, i := range idx[:n] {
		out = append(out, pool[i])
	}
	return out
}

func tier(rng *rand.Rand) string {
	switch r := rng.Float64(); {
	case r < 0.10:
		return inventory.TierPremium
	case r < 0.30:
		return inventory.TierPreferred
	default:
		return inventory.TierStandard
	}
}

// fullDescription writes the "already complete" copy for non-gap listings.
func fullDescription(l inventory.Listing) string {
	return fmt.Sprintf("%s is a %.1f-star rated %s stay in %s, %s. Guests enjoy %s. "+
		"An ideal base for a %s getaway, with nightly rates from €%d.",
		l.Name, l.Rating, l.Category, l.Destination, l.Country,
		strings.Join(l.Amenities, ", "), strings.Join(l.VibeTags, " & "),
		l.PricePerNightCents/100)
}
