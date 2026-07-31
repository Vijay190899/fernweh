package enrich

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"fernweh/internal/inventory"
)

func sample() inventory.Listing {
	return inventory.Listing{
		ID: "lst_0001", Name: "Casa Aurora", Category: "beach",
		Destination: "Algarve", Country: "Portugal", Rating: 4.4,
		ReviewCount: 512, PricePerNightCents: 12000,
		Amenities: []string{"pool", "wifi"}, VibeTags: []string{"family"},
		MonthsBest: []int{4, 5, 6, 7, 8, 9, 10},
	}
}

func TestTemplateGeneratorUsesOnlyListingFacts(t *testing.T) {
	gen, err := TemplateGenerator{}.Generate(context.Background(), sample())
	if err != nil {
		t.Fatal(err)
	}
	if gen.Source != "template" {
		t.Errorf("source = %q, want template", gen.Source)
	}
	for _, must := range []string{"Casa Aurora", "Algarve", "Portugal", "beach", "pool", "€120"} {
		if !strings.Contains(gen.Description, must) {
			t.Errorf("description missing fact %q: %s", must, gen.Description)
		}
	}
	// The guarantee that matters: no invented claims. Everything in the copy
	// must trace to a listing field — spot-check words that would signal
	// invention for this listing.
	for _, banned := range []string{"spa", "ski", "sauna", "sea view"} {
		if strings.Contains(strings.ToLower(gen.Description), banned) {
			t.Errorf("description invented %q: %s", banned, gen.Description)
		}
	}
}

func TestContentHashStability(t *testing.T) {
	a, b := sample(), sample()
	if ContentHash(a) != ContentHash(b) {
		t.Fatal("identical facts must hash identically")
	}
	b.Amenities = []string{"wifi", "pool"} // order must not matter
	if ContentHash(a) != ContentHash(b) {
		t.Error("amenity order must not change the hash")
	}
	b.Rating = 4.5
	if ContentHash(a) == ContentHash(b) {
		t.Error("changed facts must change the hash")
	}
}

func TestSanitizeAmenitiesKeepsModelHonest(t *testing.T) {
	original := []string{"pool", "wifi"}
	proposed := []string{"POOL", "casino", "helipad", "sauna", "rooftop", "wifi"}

	got := sanitizeAmenities(proposed, original)

	for _, must := range original {
		if !containsStr(got, must) {
			t.Errorf("original amenity %q must survive, got %v", must, got)
		}
	}
	if len(got) > len(original)+3 {
		t.Errorf("at most 3 additions allowed, got %v", got)
	}
	if containsStr(got, "rooftop") { // 4th addition must be dropped
		t.Errorf("addition cap not enforced: %v", got)
	}
}

// ---- Processor idempotency ----------------------------------------------

type fakeStore struct {
	listing    inventory.Listing
	claimable  bool
	statusLog  []string
	applied    bool
	applyError error
}

func (f *fakeStore) Get(_ context.Context, id string) (inventory.Listing, error) {
	return f.listing, nil
}
func (f *fakeStore) ListByStatus(context.Context, string, int) ([]inventory.Listing, error) {
	return []inventory.Listing{f.listing}, nil
}
func (f *fakeStore) SetStatus(_ context.Context, id, from, to string) (bool, error) {
	f.statusLog = append(f.statusLog, from+"->"+to)
	if from == inventory.StatusNeedsEnrichment && to == inventory.StatusEnriching {
		was := f.claimable
		f.claimable = false // a second claim must fail, like the SQL guard
		return was, nil
	}
	return true, nil
}
func (f *fakeStore) ApplyEnrichment(_ context.Context, id, desc string, amen []string, hash, source, model string) error {
	if f.applyError != nil {
		return f.applyError
	}
	f.applied = true
	return nil
}

func TestProcessorClaimsThenApplies(t *testing.T) {
	store := &fakeStore{listing: sample(), claimable: true}
	p := NewProcessor(store, TemplateGenerator{}, slog.Default())

	if err := p.Process(context.Background(), "lst_0001"); err != nil {
		t.Fatal(err)
	}
	if !store.applied {
		t.Error("enrichment was not applied")
	}
}

func TestProcessorSkipsUnclaimableListing(t *testing.T) {
	store := &fakeStore{listing: sample(), claimable: false}
	p := NewProcessor(store, TemplateGenerator{}, slog.Default())

	if err := p.Process(context.Background(), "lst_0001"); err != nil {
		t.Fatalf("unclaimable listing must be a clean skip, got %v", err)
	}
	if store.applied {
		t.Error("skipped listing must not be written")
	}
}

func TestProcessorReleasesClaimOnFailure(t *testing.T) {
	store := &fakeStore{listing: sample(), claimable: true, applyError: errors.New("db down")}
	p := NewProcessor(store, TemplateGenerator{}, slog.Default())

	if err := p.Process(context.Background(), "lst_0001"); err == nil {
		t.Fatal("apply failure must propagate so asynq retries")
	}
	released := false
	for _, s := range store.statusLog {
		if s == inventory.StatusEnriching+"->"+inventory.StatusNeedsEnrichment {
			released = true
		}
	}
	if !released {
		t.Errorf("failed listing must be released for retry, status log: %v", store.statusLog)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
