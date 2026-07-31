package seed

import (
	"reflect"
	"testing"

	"fernweh/internal/inventory"
)

func TestGenerateIsDeterministic(t *testing.T) {
	a, promosA := Generate(300, 0.4)
	b, promosB := Generate(300, 0.4)
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(promosA, promosB) {
		t.Fatal("seed data must be identical across runs and environments")
	}
}

func TestGenerateShape(t *testing.T) {
	listings, promos := Generate(300, 0.4)
	if len(listings) != 300 {
		t.Fatalf("got %d listings", len(listings))
	}

	ids := map[string]bool{}
	gaps := 0
	for _, l := range listings {
		if ids[l.ID] {
			t.Fatalf("duplicate id %s", l.ID)
		}
		ids[l.ID] = true
		if l.PricePerNightCents <= 0 || l.Rating < 3.5 || l.Rating > 5 {
			t.Errorf("listing %s has implausible price/rating: %d / %.1f",
				l.ID, l.PricePerNightCents, l.Rating)
		}
		if l.ContentStatus == inventory.StatusNeedsEnrichment {
			gaps++
			if l.Description != "" && len(l.Description) > 60 {
				t.Errorf("gap listing %s has a full description", l.ID)
			}
		} else if l.Description == "" {
			t.Errorf("complete listing %s missing description", l.ID)
		}
	}
	// The gap ratio drives the enrichment demo; keep it in a sane band.
	if gaps < 90 || gaps > 150 {
		t.Errorf("gap count %d outside expected band for ratio 0.4", gaps)
	}

	for _, p := range promos {
		if !ids[p.ListingID] {
			t.Errorf("promotion %s points at unknown listing %s", p.ID, p.ListingID)
		}
		if p.Boost < 0.05 || p.Boost > 0.15 {
			t.Errorf("promotion boost %f outside contract [0.05, 0.15]", p.Boost)
		}
	}
}
