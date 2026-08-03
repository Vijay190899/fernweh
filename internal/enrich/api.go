package enrich

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hibiken/asynq"

	"fernweh/internal/inventory"
	"fernweh/internal/platform/httpx"
)

// AdminStore adds the read endpoints the ops dashboard needs.
type AdminStore interface {
	Store
	Stats(ctx context.Context) (inventory.ContentStats, error)
	RecentAudit(ctx context.Context, listingID string, limit int) ([]inventory.AuditEntry, error)
	ResetForDemo(ctx context.Context, n int) (int, error)
}

// Handler is the enrichment admin API: stats, manual scan, before/after audit.
// resetCooldown throttles the demo reset. In-process is sufficient because
// enrichment runs as a single instance; a horizontally scaled deployment
// would move this counter into Redis alongside the LLM budget.
const resetCooldown = 45 * time.Second

type Handler struct {
	store     AdminStore
	scanner   *Scanner
	inspector *asynq.Inspector
	log       *slog.Logger

	resetMu   sync.Mutex
	lastReset time.Time
}

func NewHandler(store AdminStore, scanner *Scanner, inspector *asynq.Inspector, log *slog.Logger) *Handler {
	return &Handler{store: store, scanner: scanner, inspector: inspector, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/enrich/stats", h.stats)
	mux.HandleFunc("POST /v1/enrich/scan", h.scan)
	mux.HandleFunc("GET /v1/enrich/listings", h.listings)
	mux.HandleFunc("GET /v1/enrich/listings/{id}/audit", h.audit)
	mux.HandleFunc("POST /v1/enrich/demo-reset", h.demoReset)
}

// demoReset re-breaks a bounded slice of inventory so the pipeline can be
// demonstrated repeatedly. Without it the queue drains once and the most
// legible part of this service can never be shown again.
//
// It is the only externally reachable write, so it is made safe rather than
// gated. A token would keep it secure and make the demo useless: the person
// this exists for is a visitor clicking a button, and asking them for a secret
// means they never see the pipeline run.
//
// Safe here means bounded and self-repairing. It touches at most 60 rows of
// synthetic catalogue, the enrichment workers rebuild every one of them within
// about a minute, and a cooldown stops it being held down. The worst outcome
// somebody can force is a demo that briefly shows the exact thing it is meant
// to demonstrate, and then fixes itself.
func (h *Handler) demoReset(w http.ResponseWriter, r *http.Request) {
	h.resetMu.Lock()
	since := time.Since(h.lastReset)
	if since < resetCooldown {
		h.resetMu.Unlock()
		w.Header().Set("Retry-After", strconv.Itoa(int((resetCooldown-since).Seconds())+1))
		httpx.Error(w, http.StatusTooManyRequests,
			"a reset just ran; the queue is still draining")
		return
	}
	h.lastReset = time.Now()
	h.resetMu.Unlock()

	n, err := h.store.ResetForDemo(r.Context(), 60)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "reset failed")
		return
	}
	h.log.InfoContext(r.Context(), "demo inventory reset", "listings", n)
	httpx.JSON(w, http.StatusOK, map[string]int{"reset": n})
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.Stats(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "stats unavailable")
		return
	}

	queue := map[string]any{}
	if info, err := h.inspector.GetQueueInfo(QueueEnrich); err == nil {
		queue = map[string]any{
			"pending": info.Pending, "active": info.Active,
			"retry": info.Retry, "archived": info.Archived,
			"processed_total": info.Processed, "failed_total": info.Failed,
		}
	}

	complete := stats.ByStatus[inventory.StatusComplete] + stats.ByStatus[inventory.StatusEnriched]
	completeness := 0.0
	if stats.Total > 0 {
		completeness = float64(complete) / float64(stats.Total)
	}

	// The dashboard wants the counters and a sample of each side together, and
	// it wants them repeatedly while a queue drains. Served as three endpoints
	// that was three round trips per tick, which is most of a rate limit spent
	// on watching one thing happen. The samples are small and the caller always
	// needs them, so they belong in the same response.
	samples := map[string][]inventory.Listing{}
	for _, status := range []string{inventory.StatusNeedsEnrichment, inventory.StatusEnriched} {
		ls, err := h.store.ListByStatus(r.Context(), status, 8)
		if err != nil {
			continue // counters are still worth serving without the samples
		}
		samples[status] = ls
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"inventory":    stats.ByStatus,
		"total":        stats.Total,
		"completeness": completeness,
		"queue":        queue,
		"samples":      samples,
	})
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	n, err := h.scanner.Scan(r.Context(), 200)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "scan failed")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]int{"enqueued": n})
}

// listings returns items in a given content status for the dashboard table.
func (h *Handler) listings(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = inventory.StatusNeedsEnrichment
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	ls, err := h.store.ListByStatus(r.Context(), status, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "listing query failed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"listings": ls})
}

func (h *Handler) audit(w http.ResponseWriter, r *http.Request) {
	entries, err := h.store.RecentAudit(r.Context(), r.PathValue("id"), 10)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "audit query failed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"audit": entries})
}
