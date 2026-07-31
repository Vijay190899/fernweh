package search

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// FallbackParser is the deterministic intent extractor: a lexicon + rules.
// It exists so the platform keeps answering when the LLM cannot, same
// contract, no dependencies, microsecond latency. It also defines the ground
// truth the LLM extractor is validated against in tests.
type FallbackParser struct{}

var destinationLexicon = map[string][2]string{ // token -> {destination, country}
	"algarve": {"Algarve", "Portugal"}, "mallorca": {"Mallorca", "Spain"},
	"majorca": {"Mallorca", "Spain"}, "crete": {"Crete", "Greece"},
	"kreta": {"Crete", "Greece"}, "santorini": {"Santorini", "Greece"},
	"amalfi": {"Amalfi Coast", "Italy"}, "costa brava": {"Costa Brava", "Spain"},
	"dubrovnik": {"Dubrovnik", "Croatia"}, "canary": {"Canary Islands", "Spain"},
	"kanaren": {"Canary Islands", "Spain"}, "sardinia": {"Sardinia", "Italy"},
	"sardinien": {"Sardinia", "Italy"}, "madeira": {"Madeira", "Portugal"},
	"cyprus": {"Cyprus", "Cyprus"}, "zypern": {"Cyprus", "Cyprus"},
	"berlin": {"Berlin", "Germany"}, "paris": {"Paris", "France"},
	"barcelona": {"Barcelona", "Spain"}, "rome": {"Rome", "Italy"},
	"rom": {"Rome", "Italy"}, "prague": {"Prague", "Czechia"},
	"prag": {"Prague", "Czechia"}, "vienna": {"Vienna", "Austria"},
	"wien": {"Vienna", "Austria"}, "amsterdam": {"Amsterdam", "Netherlands"},
	"lisbon": {"Lisbon", "Portugal"}, "lissabon": {"Lisbon", "Portugal"},
	"budapest": {"Budapest", "Hungary"}, "copenhagen": {"Copenhagen", "Denmark"},
	"zermatt": {"Zermatt", "Switzerland"}, "innsbruck": {"Innsbruck", "Austria"},
	"chamonix": {"Chamonix", "France"}, "livigno": {"Livigno", "Italy"},
	"baden-baden": {"Baden-Baden", "Germany"}, "bled": {"Lake Bled", "Slovenia"},
	"tuscany": {"Tuscany", "Italy"}, "toskana": {"Tuscany", "Italy"},
	"provence": {"Provence", "France"}, "douro": {"Douro Valley", "Portugal"},
	"azores": {"Azores", "Portugal"}, "azoren": {"Azores", "Portugal"},
	"fjords": {"Norwegian Fjords", "Norway"}, "highlands": {"Scottish Highlands", "United Kingdom"},
}

var countryLexicon = map[string]string{
	"portugal": "Portugal", "spain": "Spain", "spanien": "Spain",
	"greece": "Greece", "griechenland": "Greece", "italy": "Italy",
	"italien": "Italy", "croatia": "Croatia", "kroatien": "Croatia",
	"france": "France", "frankreich": "France", "germany": "Germany",
	"deutschland": "Germany", "austria": "Austria", "österreich": "Austria",
	"switzerland": "Switzerland", "schweiz": "Switzerland",
	"netherlands": "Netherlands", "hungary": "Hungary", "denmark": "Denmark",
	"czechia": "Czechia", "slovenia": "Slovenia", "norway": "Norway",
	"norwegen": "Norway", "cyprus": "Cyprus", "scotland": "United Kingdom",
}

var monthLexicon = map[string]int{
	"january": 1, "januar": 1, "february": 2, "februar": 2, "march": 3,
	"märz": 3, "april": 4, "may": 5, "mai": 5, "june": 6, "juni": 6,
	"july": 7, "juli": 7, "august": 8, "september": 9, "october": 10,
	"oktober": 10, "november": 11, "december": 12, "dezember": 12,
}

var categoryRules = []struct {
	keywords []string
	category string
}{
	{[]string{"beach", "strand", "sea", "meer", "island", "coast"}, "beach"},
	{[]string{"ski", "snowboard", "slopes", "piste", "winter sports"}, "ski"},
	{[]string{"spa", "wellness", "thermal", "massage", "relax retreat"}, "wellness"},
	{[]string{"city break", "city trip", "städtetrip", "museum", "sightseeing", "city"}, "city"},
	{[]string{"countryside", "vineyard", "wine", "rural", "farm"}, "countryside"},
	{[]string{"hiking", "adventure", "outdoor", "trekking", "wandern"}, "adventure"},
}

var vibeRules = []struct {
	keywords []string
	vibe     string
}{
	{[]string{"family", "kids", "children", "familie", "kinder"}, "family"},
	{[]string{"romantic", "honeymoon", "anniversary", "romantisch"}, "romantic"},
	{[]string{"party", "nightlife", "feiern"}, "party"},
	{[]string{"quiet", "calm", "peaceful", "ruhig"}, "quiet"},
	{[]string{"luxury", "luxurious", "5-star", "luxus"}, "luxury"},
	{[]string{"cheap", "budget", "affordable", "günstig"}, "budget"},
	{[]string{"food", "foodie", "culinary", "restaurants"}, "foodie"},
	{[]string{"culture", "cultural", "history", "kultur"}, "culture"},
	{[]string{"nature", "natur", "scenery"}, "nature"},
}

var amenityRules = []struct {
	keywords []string
	amenity  string
}{
	{[]string{"pool"}, "pool"},
	{[]string{"all inclusive", "all-inclusive"}, "all-inclusive"},
	{[]string{"kids club"}, "kids club"},
	{[]string{"sea view", "meerblick"}, "sea view"},
	{[]string{"sauna"}, "sauna"},
	{[]string{"gym", "fitness"}, "gym"},
	{[]string{"parking"}, "parking"},
	{[]string{"pet friendly", "dog", "hund"}, "pet friendly"},
	{[]string{"breakfast", "frühstück"}, "breakfast"},
	{[]string{"wifi", "wlan"}, "wifi"},
}

var (
	thousandsRe = regexp.MustCompile(`(\d)[,.](\d{3})\b`)
	budgetRe    = regexp.MustCompile(`(?:€|eur|euro)?\s*(\d{2,5})\s*(?:€|eur|euros?)?`)
	nightsRe    = regexp.MustCompile(`(\d{1,2})\s*(?:nights?|nächte)`)
	daysRe      = regexp.MustCompile(`(\d{1,2})\s*(?:days?|tage)`)
	starsRe     = regexp.MustCompile(`(\d(?:\.\d)?)\s*(?:\+\s*)?stars?`)
	perNightRe  = regexp.MustCompile(`per night|/night|a night|pro nacht`)
	underRe     = regexp.MustCompile(`(?:under|below|less than|max|up to|unter|bis)\s*(?:€|eur|euro)?\s*(\d{2,5})`)
)

// Parse extracts intent with rules only. Always succeeds; empty intent means
// "show me anything good". Lexicon matching is word-boundary based, plain
// substring matching turns "romantic" into a trip to Rome.
func (FallbackParser) Parse(query string) Intent {
	q := " " + strings.ToLower(strings.TrimSpace(query)) + " "
	q = thousandsRe.ReplaceAllString(q, "$1$2") // "€1,000" / "1.000" -> "1000"
	ts := tokenize(q)
	var in Intent

	for token, dc := range destinationLexicon {
		if hasPhrase(ts, token) {
			in.Destination, in.Country = dc[0], dc[1]
			break
		}
	}
	if in.Destination == "" {
		for token, country := range countryLexicon {
			if hasPhrase(ts, token) {
				in.Country = country
				break
			}
		}
	}

	for word, m := range monthLexicon {
		if hasPhrase(ts, word) {
			in.Month = m
			break
		}
	}

	for _, rule := range categoryRules {
		if containsAny(ts, rule.keywords) {
			in.Category = rule.category
			break
		}
	}
	for _, rule := range vibeRules {
		if containsAny(ts, rule.keywords) {
			in.VibeTags = append(in.VibeTags, rule.vibe)
		}
	}
	for _, rule := range amenityRules {
		if containsAny(ts, rule.keywords) {
			in.Amenities = append(in.Amenities, rule.amenity)
		}
	}

	// Trip length: explicit nights/days, or wording.
	if m := nightsRe.FindStringSubmatch(q); m != nil {
		in.Nights, _ = strconv.Atoi(m[1])
	} else if m := daysRe.FindStringSubmatch(q); m != nil {
		if d, _ := strconv.Atoi(m[1]); d > 1 {
			in.Nights = d - 1
		}
	} else if containsAny(ts, []string{"weekend", "wochenende"}) {
		in.Nights = 2
	} else if containsAny(ts, []string{"fortnight", "two weeks"}) {
		in.Nights = 14
	} else if containsAny(ts, []string{"week", "weeks", "woche", "wochen"}) {
		in.Nights = 7
	}

	// Budget: prefer an explicit "under X"; interpret as total trip budget
	// unless "per night" is stated or the amount is small. Convert totals to
	// per-night using trip length (default 7 nights).
	if m := underRe.FindStringSubmatch(q); m != nil {
		amount, _ := strconv.Atoi(m[1])
		in.BudgetMax = perNightBudget(amount, in.Nights, perNightRe.MatchString(q))
	} else if m := budgetRe.FindStringSubmatch(q); m != nil && strings.ContainsAny(query, "€") {
		amount, _ := strconv.Atoi(m[1])
		in.BudgetMax = perNightBudget(amount, in.Nights, perNightRe.MatchString(q))
	}

	if m := starsRe.FindStringSubmatch(q); m != nil {
		in.MinRating, _ = strconv.ParseFloat(m[1], 64)
	}

	return in.Normalize()
}

// perNightBudget converts a stated amount to EUR/night. Amounts ≤ 300 read as
// nightly rates; larger amounts read as trip totals divided by trip length.
func perNightBudget(amount, nights int, explicitPerNight bool) int {
	if explicitPerNight || amount <= 300 {
		return amount
	}
	if nights <= 0 {
		nights = 7
	}
	return amount / nights
}

// tokenize collapses the query to space-separated word tokens (hyphens kept,
// e.g. "baden-baden"), padded so every word has space boundaries.
func tokenize(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-'
	})
	return " " + strings.Join(fields, " ") + " "
}

// hasPhrase matches a word or multi-word phrase on word boundaries.
func hasPhrase(ts, phrase string) bool {
	return strings.Contains(ts, " "+phrase+" ")
}

func containsAny(ts string, keywords []string) bool {
	for _, k := range keywords {
		if hasPhrase(ts, k) {
			return true
		}
	}
	return false
}
