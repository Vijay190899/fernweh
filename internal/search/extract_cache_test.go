package search

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"fernweh/internal/platform/llm"
	"fernweh/internal/platform/redisx"
)

// A degradation that stops being reported after the first request is worse
// than no graceful degradation at all, because it is invisible. These tests
// exist because a live deployment reported intent_source "cache" with nothing
// degraded, for a query the model had never seen.
//
// They run against the compose Redis rather than a fake, because the bug was
// in what gets written to and read back from it. A fake that round-trips the
// same struct would have agreed with the broken version.
func redisExtractor(t *testing.T) *Extractor {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rdb, err := redisx.Connect(ctx, "localhost:6379")
	if err != nil {
		t.Skip("no Redis on localhost:6379; run docker compose up -d redis")
	}
	t.Cleanup(func() { rdb.Close() })
	// A real client with no API key, which is precisely the state of the
	// deployed demo: CompleteJSON refuses with ErrUnavailable before touching
	// the network, so every extraction takes the deterministic path. Passing
	// nil here would test a configuration that cannot occur.
	return &Extractor{
		llm: llm.New("", "", time.Second, 0, nil),
		rdb: rdb,
	}
}

func TestCachedFallbackStillReportsFallback(t *testing.T) {
	ex := redisExtractor(t)
	q := "a quiet countryside stay in June, cached-fallback-test"
	t.Cleanup(func() { ex.rdb.Del(context.Background(), cacheKey(q)) })

	if _, src := ex.Extract(context.Background(), q); src != "fallback" {
		t.Fatalf("first call source = %q, want fallback", src)
	}

	// Second call is a cache hit. It must not launder a degraded reading into
	// something that looks like the model understood the query.
	in, src := ex.Extract(context.Background(), q)
	if src != "fallback" {
		t.Errorf("cached fallback reported as %q; the degradation stopped being disclosed", src)
	}
	if in.Category == "" && in.Month == 0 {
		t.Error("cache hit returned an empty intent")
	}
}

func TestCachedModelAnswerReportsCache(t *testing.T) {
	ex := redisExtractor(t)
	q := "a beach week in May, cached-llm-test"
	key := cacheKey(q)
	t.Cleanup(func() { ex.rdb.Del(context.Background(), key) })

	blob, err := json.Marshal(cached{
		Intent: Intent{Category: "beach", Month: 5},
		Source: "llm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ex.rdb.Set(context.Background(), key, blob, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}

	in, src := ex.Extract(context.Background(), q)
	if src != "cache" {
		t.Errorf("source = %q, want cache", src)
	}
	if in.Category != "beach" || in.Month != 5 {
		t.Errorf("cached intent came back as %+v", in)
	}
}

// Entries written by the previous build hold a bare Intent, not an envelope.
// Those unmarshal cleanly into a zero-valued envelope with an empty Source,
// which would be served as a cache hit carrying an empty intent. The key is
// versioned so they are never read, and the guard on Source is the belt to
// that pair of braces.
func TestEntryWithNoRecordedSourceIsIgnored(t *testing.T) {
	ex := redisExtractor(t)
	q := "somewhere warm in March, legacy-entry-test"
	key := cacheKey(q)
	t.Cleanup(func() { ex.rdb.Del(context.Background(), key) })

	blob, _ := json.Marshal(Intent{Category: "beach"}) // old shape, no source
	if err := ex.rdb.Set(context.Background(), key, blob, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}

	if _, src := ex.Extract(context.Background(), q); src == "cache" {
		t.Error("an entry with no recorded source was served as a cache hit")
	}
}

func TestFallbackCachedForMuchLessTimeThanModelAnswers(t *testing.T) {
	// A model that is down must not pin a query to its degraded reading for a
	// day; it should go back to the model shortly after recovery.
	if fallbackIntentTTL >= llmIntentTTL {
		t.Errorf("fallback TTL %v is not shorter than the model TTL %v",
			fallbackIntentTTL, llmIntentTTL)
	}
	if fallbackIntentTTL > time.Hour {
		t.Errorf("fallback TTL %v is long enough to outlive an outage", fallbackIntentTTL)
	}
}
