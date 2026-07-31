package ranking

import (
	"math"
	"sort"
)

// Offline evaluation of the ranker.
//
// A demo has no users, so there is no click log and there are no real
// relevance judgments. Rather than invent them, this harness states its
// ground truth openly: a small set of declared personas, and a grading rule
// that turns each persona's stated preferences into graded relevance over the
// real catalogue. Every number produced here is reproducible from the seeded
// inventory and the rules below.
//
// What this can honestly measure: whether personalization ranks relevant
// inventory higher than an unpersonalized baseline, how much of the catalogue
// the ranker can reach, and how varied a result page is.
//
// What it cannot measure, and does not pretend to: click-through rate and
// conversion, which require live traffic. Rating-error metrics (RMSE, MAE,
// MSE) do not apply at all, because this system ranks and never predicts a
// rating.

// Persona is a declared traveller profile. It is synthetic and labelled as
// such wherever these results are shown.
type Persona struct {
	Name       string
	Category   string
	Amenities  []string
	Vibes      []string
	PriceMin   int // cents per night
	PriceMax   int
}

// Personas span the catalogue's categories and price bands so coverage is
// not accidentally flattering.
func Personas() []Persona {
	return []Persona{
		{"Family by the sea", "beach", []string{"kids club", "pool"}, []string{"family"}, 8000, 18000},
		{"Alpine luxury", "ski", []string{"sauna", "spa"}, []string{"luxury"}, 22000, 60000},
		{"City on a budget", "city", []string{"breakfast", "wifi"}, []string{"budget"}, 5000, 11000},
		{"Quiet wellness", "wellness", []string{"spa", "thermal baths"}, []string{"quiet"}, 9000, 30000},
		{"Countryside table", "countryside", []string{"vineyard", "farm-to-table dining"}, []string{"foodie"}, 7000, 25000},
		{"Outdoors, unfussy", "adventure", []string{"guided hikes", "gear rental"}, []string{"nature"}, 6000, 20000},
	}
}

// Profile turns a persona into the same profile shape the live scorer
// consumes, so the evaluation exercises production code rather than a
// parallel implementation.
func (p Persona) Profile() Profile {
	amen := map[string]float64{}
	for _, a := range p.Amenities {
		amen[a] = 1 / float64(len(p.Amenities))
	}
	vibes := map[string]float64{}
	for _, v := range p.Vibes {
		vibes[v] = 1 / float64(len(p.Vibes))
	}
	return Profile{
		CategoryAffinity: map[string]float64{p.Category: 1},
		AmenityWeight:    amen,
		VibeWeight:       vibes,
		AvgPriceCents:    (p.PriceMin + p.PriceMax) / 2,
		Events:           12,
	}
}

// Relevance grades an item for a persona on a 0..3 scale.
//
//	3  right category, price in band, and at least one wanted amenity
//	2  right category, and either price or an amenity matches
//	1  right category only
//	0  wrong category
//
// The rule is deliberately simple and stated in full so a reader can decide
// whether they believe the resulting numbers.
func (p Persona) Relevance(it Item) int {
	if it.Category != p.Category {
		return 0
	}
	inBand := it.PriceCents >= p.PriceMin && it.PriceCents <= p.PriceMax
	amenityHit := false
	for _, want := range p.Amenities {
		for _, has := range it.Amenities {
			if has == want {
				amenityHit = true
			}
		}
	}
	switch {
	case inBand && amenityHit:
		return 3
	case inBand || amenityHit:
		return 2
	default:
		return 1
	}
}

// Metrics is one evaluation run.
type Metrics struct {
	NDCG10      float64 `json:"ndcg_at_10"`
	Precision10 float64 `json:"precision_at_10"`
	Recall10    float64 `json:"recall_at_10"`
	MAP         float64 `json:"map"`
	Diversity10 float64 `json:"diversity_at_10"`
	Coverage    float64 `json:"catalogue_coverage"`
}

// Report is the whole evaluation: the personalized ranker against an
// unpersonalized baseline over the same candidates.
type Report struct {
	Personalized Metrics            `json:"personalized"`
	Baseline     Metrics            `json:"baseline"`
	PerPersona   map[string]Metrics `json:"per_persona"`
	Catalogue    int                `json:"catalogue_size"`
	Personas     int                `json:"personas"`
	GeneratedAt  string             `json:"generated_at,omitempty"`
	Notes        []string           `json:"notes"`
}

const cutoff = 10

// Evaluate runs every persona over the supplied catalogue and returns both
// the personalized and the baseline numbers. Baseline is the same scorer with
// an empty profile, which is exactly what a first-time visitor receives.
func Evaluate(catalogue []Item) Report {
	personas := Personas()
	rep := Report{
		PerPersona: map[string]Metrics{},
		Catalogue:  len(catalogue),
		Personas:   len(personas),
		Notes: []string{
			"Relevance judgments come from declared synthetic personas, not from real users.",
			"Click-through and conversion are omitted: they require live traffic and cannot be measured here.",
			"Rating-error metrics (RMSE, MAE, MSE) do not apply; this system ranks and never predicts a rating.",
			"Baseline is the identical scorer with an empty profile, which is what a first-time visitor gets.",
		},
	}
	if len(catalogue) == 0 {
		return rep
	}

	var pAcc, bAcc []Metrics
	seenPersonalized := map[string]bool{}
	seenBaseline := map[string]bool{}

	for _, persona := range personas {
		ranked := Rank(catalogue, persona.Profile())
		base := Rank(catalogue, Profile{})

		pm := score(persona, catalogue, ranked, seenPersonalized)
		bm := score(persona, catalogue, base, seenBaseline)

		rep.PerPersona[persona.Name] = pm
		pAcc = append(pAcc, pm)
		bAcc = append(bAcc, bm)
	}

	rep.Personalized = mean(pAcc)
	rep.Baseline = mean(bAcc)
	rep.Personalized.Coverage = float64(len(seenPersonalized)) / float64(len(catalogue))
	rep.Baseline.Coverage = float64(len(seenBaseline)) / float64(len(catalogue))
	return rep
}

func score(p Persona, catalogue []Item, ranked []Ranked, seen map[string]bool) Metrics {
	byID := make(map[string]Item, len(catalogue))
	for _, it := range catalogue {
		byID[it.ID] = it
	}

	top := ranked
	if len(top) > cutoff {
		top = top[:cutoff]
	}

	gains := make([]int, 0, len(top))
	relevantInTop := 0
	categories := map[string]bool{}
	for _, r := range top {
		it := byID[r.ID]
		g := p.Relevance(it)
		gains = append(gains, g)
		if g >= 2 {
			relevantInTop++
		}
		categories[it.Category] = true
		seen[r.ID] = true
	}

	// Ideal ordering for DCG: every item graded, sorted descending.
	ideal := make([]int, 0, len(catalogue))
	totalRelevant := 0
	for _, it := range catalogue {
		g := p.Relevance(it)
		ideal = append(ideal, g)
		if g >= 2 {
			totalRelevant++
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ideal)))
	if len(ideal) > cutoff {
		ideal = ideal[:cutoff]
	}

	m := Metrics{
		NDCG10:      safeDiv(dcg(gains), dcg(ideal)),
		Precision10: float64(relevantInTop) / float64(max(1, len(top))),
		Recall10:    safeDiv(float64(relevantInTop), float64(totalRelevant)),
		MAP:         averagePrecision(p, byID, ranked),
		Diversity10: float64(len(categories)) / float64(max(1, len(top))),
	}
	return m
}

func dcg(gains []int) float64 {
	var sum float64
	for i, g := range gains {
		if g == 0 {
			continue
		}
		sum += (math.Pow(2, float64(g)) - 1) / math.Log2(float64(i+2))
	}
	return sum
}

func averagePrecision(p Persona, byID map[string]Item, ranked []Ranked) float64 {
	hits := 0
	var sum float64
	limit := len(ranked)
	if limit > cutoff {
		limit = cutoff
	}
	for i := 0; i < limit; i++ {
		if p.Relevance(byID[ranked[i].ID]) >= 2 {
			hits++
			sum += float64(hits) / float64(i+1)
		}
	}
	return safeDiv(sum, float64(max(1, hits)))
}

func mean(ms []Metrics) Metrics {
	if len(ms) == 0 {
		return Metrics{}
	}
	var out Metrics
	for _, m := range ms {
		out.NDCG10 += m.NDCG10
		out.Precision10 += m.Precision10
		out.Recall10 += m.Recall10
		out.MAP += m.MAP
		out.Diversity10 += m.Diversity10
	}
	n := float64(len(ms))
	out.NDCG10 /= n
	out.Precision10 /= n
	out.Recall10 /= n
	out.MAP /= n
	out.Diversity10 /= n
	return out
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
