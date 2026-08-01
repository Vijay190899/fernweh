package search

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"fernweh/internal/inventory"
	"fernweh/internal/platform/llm"
)

// fakeInventory returns listings only when the filter matches a canned rule,
// letting tests drive the relaxation ladder deterministically.
type fakeInventory struct {
	match     func(inventory.Filter) []inventory.Listing
	queries   []inventory.Filter
	uncovered map[string]bool
}

func (f *fakeInventory) Search(_ context.Context, fl inventory.Filter) ([]inventory.Listing, error) {
	f.queries = append(f.queries, fl)
	return f.match(fl), nil
}

func (f *fakeInventory) ActivePromotions(context.Context) (map[string]inventory.Promotion, error) {
	return map[string]inventory.Promotion{}, nil
}

func (f *fakeInventory) Covers(_ context.Context, dest, country string) (bool, error) {
	if f.uncovered == nil {
		return true, nil
	}
	return !f.uncovered[dest] && !f.uncovered[country], nil
}

type fakeRanker struct {
	err error
}

func (r *fakeRanker) Rank(_ context.Context, _ string, items []RankItem) ([]RankedItem, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]RankedItem, len(items))
	for i, it := range items {
		out[i] = RankedItem{ID: it.ID, Score: it.BaseScore, Reasons: []string{"test"}}
	}
	return out, nil
}

// testExtractor uses an LLM client with no key: every extraction takes the
// deterministic fallback path, which is exactly the no-AI production mode.
func testExtractor() *Extractor {
	return NewExtractor(llm.New("", "test", time.Second, 0, nil), nil)
}

func TestSearchDegradesWhenRankingDown(t *testing.T) {
	listing := inventory.Listing{ID: "lst_1", Rating: 4.5, ReviewCount: 100}
	inv := &fakeInventory{match: func(inventory.Filter) []inventory.Listing {
		return []inventory.Listing{listing}
	}}
	svc := NewService(inv, testExtractor(), &fakeRanker{err: errors.New("down")}, slog.Default())

	resp, err := svc.Search(context.Background(), "beach in Crete", "s1")
	if err != nil {
		t.Fatalf("search must not fail when ranking is down: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected base-ordered results, got %d", len(resp.Results))
	}
	if !contains(resp.Degraded, "ranking_unavailable") {
		t.Errorf("degradation must be reported, got %v", resp.Degraded)
	}
}

func TestSearchWalksLadderAndReportsRelaxations(t *testing.T) {
	listing := inventory.Listing{ID: "lst_2", Rating: 4.0}
	inv := &fakeInventory{match: func(fl inventory.Filter) []inventory.Listing {
		if len(fl.VibeTags) > 0 { // strict query matches nothing
			return nil
		}
		return []inventory.Listing{listing}
	}}
	svc := NewService(inv, testExtractor(), &fakeRanker{}, slog.Default())

	resp, err := svc.Search(context.Background(), "romantic beach in Crete", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("ladder should have found the listing, got %d results", len(resp.Results))
	}
	if len(resp.Relaxations) == 0 || resp.Relaxations[0] != "relaxed style preferences" {
		t.Errorf("applied relaxations must be surfaced, got %v", resp.Relaxations)
	}
}

func TestUncoveredDestinationIsStatedNotSubstituted(t *testing.T) {
	// The catalogue is regional. Asking for somewhere it does not serve must
	// say so; quietly widening and presenting the other side of the world as a
	// near match is worse than admitting no coverage.
	// Crete is used because the deterministic parser recognises it; the
	// catalogue is then told it holds nothing there.
	inv := &fakeInventory{
		uncovered: map[string]bool{"Crete": true},
		match: func(f inventory.Filter) []inventory.Listing {
			if f.Destination != "" || f.Country != "" {
				t.Errorf("uncovered place should have been dropped, got %+v", f)
			}
			return []inventory.Listing{{ID: "lst_9", Rating: 4.1}}
		},
	}
	svc := NewService(inv, testExtractor(), &fakeRanker{}, slog.Default())

	resp, err := svc.Search(context.Background(), "a beach trip to Crete", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Unsupported != "Crete" {
		t.Errorf("expected the uncovered place to be reported, got %q", resp.Unsupported)
	}
	if len(resp.Results) == 0 {
		t.Error("should still offer alternatives rather than an empty page")
	}
}

func TestCoveredDestinationIsNotFlagged(t *testing.T) {
	inv := &fakeInventory{match: func(inventory.Filter) []inventory.Listing {
		return []inventory.Listing{{ID: "lst_1", Rating: 4.5}}
	}}
	svc := NewService(inv, testExtractor(), &fakeRanker{}, slog.Default())

	resp, err := svc.Search(context.Background(), "beach in Crete", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Unsupported != "" {
		t.Errorf("covered destination must not be flagged, got %q", resp.Unsupported)
	}
}

func TestSearchExactMatchReportsNoRelaxations(t *testing.T) {
	inv := &fakeInventory{match: func(inventory.Filter) []inventory.Listing {
		return []inventory.Listing{{ID: "lst_3", Rating: 4.2}}
	}}
	svc := NewService(inv, testExtractor(), &fakeRanker{}, slog.Default())

	resp, err := svc.Search(context.Background(), "beach in Crete", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Relaxations) != 0 {
		t.Errorf("exact match must report no relaxations, got %v", resp.Relaxations)
	}
	if resp.IntentSource != "fallback" {
		t.Errorf("without an LLM key the source must be fallback, got %s", resp.IntentSource)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
