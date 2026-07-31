# Fernweh — Architecture Notes

How the system works, why it is shaped this way, and how it would grow into a
real production platform. Written to be read alongside the code.

## Request anatomy (the demo's money shot)

```
Browser ── POST /api/search ──▶ gateway (rate limit, headers, trace start)
                                  │  /v1/search
                                  ▼
                                search ── Redis: intent cache? ──▶ hit: skip LLM
                                  │  miss
                                  ├──▶ OpenRouter (budget-guarded, 2.5s cap) ──▶ intent
                                  │        └─ unavailable ──▶ deterministic parser
                                  ├──▶ Postgres: filter query (relaxation ladder)
                                  ├──▶ ranking /v1/rank (800ms cap)
                                  │        └─ error/timeout ──▶ base order, flagged
                                  ▼
                                response: results + reasons + relaxations +
                                          degraded flags + X-Trace-Id
```

Everything above is one Jaeger trace. The enrichment path extends this across
the queue: `scan → asynq(Redis) → worker → OpenRouter → Postgres` — the trace
context is serialized into the task payload (`internal/platform/otelx`,
`InjectMap`/`ExtractMap`).

## Why these shapes

**Single Go module, four binaries.** Services are isolated by package
boundaries and deployment units, not by repos. Shared platform code
(`internal/platform`) changes atomically with its consumers — no internal
version skew, one `go test ./...` for the whole platform.

**No web framework.** Go 1.22+ `http.ServeMux` has method+path routing;
`net/http` has everything else. The entire "framework" we needed is ~120 lines
in `httpx` (lifecycle, JSON, health) — reviewable in one sitting, zero
framework CVEs, no magic in the request path.

**Interfaces at the consumer.** `search.Inventory`, `search.Ranker`,
`enrich.Store`, `enrich.Generator` are declared where they are used, sized to
what the consumer needs, and faked in tests. The concrete `inventory.Repo`
satisfies several of them at once.

**The LLM is a dependency like any other — and treated as the least reliable
one.** One client (`platform/llm`) owns access; it can refuse
(`ErrUnavailable`) for budget, key, latency, or provider reasons, and every
call site is forced by the type system to have a deterministic answer ready.
This inverts the usual "AI demo" failure mode: the platform's availability is
not a function of the AI's availability.

**Sessions, not accounts.** Personalization state is a Redis hash with a
7-day TTL keyed by an anonymous session id. GDPR surface: near zero. The
`SignalStore` interface is the seam where a durable, consented profile store
would slot in.

**Queue for anything not user-facing.** Enrichment latency is irrelevant to
travelers, so it runs at bounded concurrency (2 workers) off an Asynq queue
with retries/backoff/dedup — it can never starve the interactive path of DB
connections. The status state machine (`needs_enrichment → enriching →
enriched|failed`, guarded compare-and-set transitions) makes concurrent
workers and repeated scans safe.

## Failure modes, by dependency

| Dependency | Failure behavior |
|---|---|
| OpenRouter | Fallback parser / template generator; `degraded: llm_unavailable`; UI badge flips to "rules parsed" |
| Ranking service | Search returns base relevance order; `degraded: ranking_unavailable` |
| Redis (cache/profile) | Intent extraction skips cache; ranking runs cold; budget guard fails open |
| Redis (asynq) | Enrichment pauses; interactive search unaffected |
| Postgres | Search fails (it is the inventory); health checks flip, orchestrator restarts |
| Jaeger | Spans drop; requests unaffected (batch exporter, fire-and-forget) |

## What changes at real scale

Honest answers to "this is a demo — what breaks first?":

1. **Search retrieval.** ILIKE + GIN filters are fine for 10⁴ listings. At
   10⁶+ with fuzzy destination matching, move retrieval to Postgres FTS or
   OpenSearch, keep the intent/relaxation logic unchanged (it operates on the
   `Filter` abstraction, not on SQL).
2. **Ranking features.** The linear scorer is deliberately explainable. At
   scale you'd A/B it against a learned ranker behind the same `/v1/rank`
   contract — the reasons channel stays, fed by feature attributions.
3. **Relaxation ladder does up to ~10 sequential queries** worst-case. Batch
   the rungs into one SQL query with scoring tiers, or precompute
   destination/category counts to skip empty rungs.
4. **Session profiles** move from per-request HGetAll to a small local cache
   with pub/sub invalidation if rank QPS grows.
5. **Enrichment throughput** scales by raising Asynq concurrency and sharding
   queues per content type; the claim guard already makes workers horizontal.
6. **Multi-tenancy** (the enterprise integration story): tenant_id columns +
   per-tenant business rules and budgets; the gateway becomes the SSO
   termination point (OIDC), services stay tenant-blind behind it.

## Security posture

Internet → gateway only. Internal services listen on the compose network,
unreachable from outside. Per-IP token buckets and a body-size cap at the
edge; strict CSP; no secrets in the repo; the only credential (OpenRouter) is
env-injected and budget-capped so leaking the demo URL cannot drain it.
