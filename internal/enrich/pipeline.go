package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"fernweh/internal/inventory"
	"fernweh/internal/platform/otelx"
)

const (
	TaskEnrichListing = "enrich:listing"
	QueueEnrich       = "enrich"
	maxRetries        = 3
)

// Store is the slice of the inventory repo the pipeline needs.
type Store interface {
	Get(ctx context.Context, id string) (inventory.Listing, error)
	ListByStatus(ctx context.Context, status string, limit int) ([]inventory.Listing, error)
	SetStatus(ctx context.Context, id, from, to string) (bool, error)
	ApplyEnrichment(ctx context.Context, id, description string, amenities []string, contentHash, source, model string) error
}

// payload travels through Redis; Trace carries W3C context across the queue
// boundary so scan → queue → worker → Postgres is ONE Jaeger trace.
type payload struct {
	ListingID string            `json:"listing_id"`
	Trace     map[string]string `json:"trace,omitempty"`
}

// ---- Scanner: finds gaps, enqueues work ---------------------------------

type Scanner struct {
	store  Store
	client *asynq.Client
	log    *slog.Logger
}

func NewScanner(store Store, client *asynq.Client, log *slog.Logger) *Scanner {
	return &Scanner{store: store, client: client, log: log}
}

// Scan enqueues one task per listing needing enrichment. TaskID dedup makes
// overlapping scans harmless: a listing already queued is skipped, not doubled.
func (s *Scanner) Scan(ctx context.Context, limit int) (int, error) {
	ctx, span := otel.Tracer("enrich").Start(ctx, "enrich.scan")
	defer span.End()

	listings, err := s.store.ListByStatus(ctx, inventory.StatusNeedsEnrichment, limit)
	if err != nil {
		return 0, fmt.Errorf("scan: %w", err)
	}

	enqueued := 0
	for _, l := range listings {
		body, _ := json.Marshal(payload{ListingID: l.ID, Trace: otelx.InjectMap(ctx)})
		_, err := s.client.EnqueueContext(ctx,
			asynq.NewTask(TaskEnrichListing, body),
			asynq.Queue(QueueEnrich),
			asynq.TaskID("enrich-"+l.ID),
			asynq.MaxRetry(maxRetries),
			asynq.Timeout(60*time.Second),
		)
		switch {
		case errors.Is(err, asynq.ErrTaskIDConflict):
			continue // already queued, dedup working as intended
		case err != nil:
			return enqueued, fmt.Errorf("enqueue %s: %w", l.ID, err)
		default:
			enqueued++
		}
	}
	span.SetAttributes(attribute.Int("enrich.enqueued", enqueued))
	s.log.InfoContext(ctx, "scan complete", "gaps", len(listings), "enqueued", enqueued)
	return enqueued, nil
}

// ---- Processor: the worker-side logic, asynq-free for testability -------

type Processor struct {
	store Store
	gen   Generator
	log   *slog.Logger
}

func NewProcessor(store Store, gen Generator, log *slog.Logger) *Processor {
	return &Processor{store: store, gen: gen, log: log}
}

// Process enriches one listing. Idempotent at two levels: the status guard
// (only needs_enrichment → enriching proceeds) and the content hash (facts
// unchanged → skip regeneration).
func (p *Processor) Process(ctx context.Context, listingID string) error {
	ctx, span := otel.Tracer("enrich").Start(ctx, "enrich.process")
	defer span.End()
	span.SetAttributes(attribute.String("listing.id", listingID))

	claimed, err := p.store.SetStatus(ctx, listingID, inventory.StatusNeedsEnrichment, inventory.StatusEnriching)
	if err != nil {
		return err
	}
	if !claimed {
		span.SetAttributes(attribute.Bool("enrich.skipped", true))
		p.log.InfoContext(ctx, "listing not claimable, skipping", "id", listingID)
		return nil
	}

	l, err := p.store.Get(ctx, listingID)
	if err != nil {
		p.release(ctx, listingID)
		return err
	}

	gen, err := p.gen.Generate(ctx, l)
	if err != nil {
		p.release(ctx, listingID)
		return err // asynq retries with backoff; final failure hits FailureHandler
	}

	hash := ContentHash(l)
	if err := p.store.ApplyEnrichment(ctx, l.ID, gen.Description, gen.Amenities, hash, gen.Source, gen.Model); err != nil {
		p.release(ctx, listingID)
		return err
	}

	span.SetAttributes(attribute.String("enrich.source", gen.Source))
	p.log.InfoContext(ctx, "listing enriched", "id", l.ID, "source", gen.Source)
	return nil
}

// release returns a claimed listing to the queue-visible state so a retry
// can claim it again.
func (p *Processor) release(ctx context.Context, id string) {
	if _, err := p.store.SetStatus(ctx, id, inventory.StatusEnriching, inventory.StatusNeedsEnrichment); err != nil {
		p.log.ErrorContext(ctx, "release failed", "id", id, "err", err)
	}
}

// MarkFailed parks a listing after retries are exhausted so the dashboard
// shows it needs human eyes, failure must be visible, not silent.
func (p *Processor) MarkFailed(ctx context.Context, id string) {
	for _, from := range []string{inventory.StatusEnriching, inventory.StatusNeedsEnrichment} {
		if ok, _ := p.store.SetStatus(ctx, id, from, inventory.StatusFailed); ok {
			return
		}
	}
}

// AsynqHandler adapts Process to asynq, restoring the enqueue-time trace
// context so the whole journey is one trace.
func (p *Processor) AsynqHandler() asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var pl payload
		if err := json.Unmarshal(t.Payload(), &pl); err != nil {
			return fmt.Errorf("bad payload: %w: %s", asynq.SkipRetry, err)
		}
		ctx = otelx.ExtractMap(ctx, pl.Trace)

		err := p.Process(ctx, pl.ListingID)
		if err != nil {
			retried, _ := asynq.GetRetryCount(ctx)
			max, _ := asynq.GetMaxRetry(ctx)
			if retried >= max { // this was the final attempt
				p.MarkFailed(ctx, pl.ListingID)
			}
		}
		return err
	}
}
