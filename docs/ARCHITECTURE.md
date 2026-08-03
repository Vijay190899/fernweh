# Fernweh Architecture

How the system works, why it is shaped this way, and how it would grow into a
real production platform. Written to be read alongside the code.

## Anatomy of a search request

```mermaid
%%{init: {'theme':'base','themeVariables':{
  'primaryColor':'#f5f1e8','primaryTextColor':'#1c1917','primaryBorderColor':'#d17588',
  'actorBkg':'#fbeef1','actorBorder':'#d17588','actorTextColor':'#1c1917',
  'signalColor':'#78716c','signalTextColor':'#57534e',
  'labelBoxBkgColor':'#f5f1e8','labelBoxBorderColor':'#d17588',
  'noteBkgColor':'#fdf6ec','noteBorderColor':'#e0d7c5',
  'fontFamily':'-apple-system, Segoe UI, sans-serif'}}}%%
sequenceDiagram
    autonumber
    participant B as Browser
    participant G as gateway
    participant S as search
    participant RC as Redis
    participant L as OpenRouter
    participant P as PostgreSQL
    participant R as ranking

    B->>G: POST /api/search {query, session}
    Note over G: rate limit · request span starts
    G->>S: /v1/search (trace propagated)
    S->>RC: intent cache lookup
    alt cache miss and budget available
        S->>L: extract intent (2.5s cap)
        L-->>S: strict JSON intent
    else no key, budget spent, or provider slow
        Note over S: deterministic parser answers<br/>response marked degraded: llm_unavailable
    end
    loop relaxation ladder, strictest first
        S->>P: filter query
        P-->>S: candidates (stop at first non-empty rung)
    end
    S->>R: /v1/rank (800ms cap)
    alt ranking healthy
        R->>RC: load session profile
        R-->>S: scores + human-readable reasons
    else ranking down or slow
        Note over S: base relevance order<br/>marked degraded: ranking_unavailable
    end
    S-->>G: results + intent + relaxations + degraded flags
    G-->>B: JSON + X-Trace-Id header
    Note over B: UI shows engine badge, relaxation banner,<br/>and a link to this exact trace in Jaeger
```

Everything above is one Jaeger trace. The enrichment path extends the same
property across the queue: trace context is serialized into the Asynq task
payload (`otelx.InjectMap` / `ExtractMap`), so scan → Redis → worker →
Postgres renders as a single trace.

## The enrichment state machine

Listings carry a `content_status` that only moves through guarded SQL
compare-and-set transitions, which is what makes concurrent workers and
overlapping scans safe:

```mermaid
%%{init: {'theme':'base','themeVariables':{
  'primaryColor':'#fbeef1','primaryTextColor':'#1c1917','primaryBorderColor':'#d17588',
  'lineColor':'#78716c','fontFamily':'-apple-system, Segoe UI, sans-serif'}}}%%
stateDiagram-v2
    direction LR
    [*] --> complete: seeded with full content
    [*] --> needs_enrichment: seeded with gaps (40%)
    needs_enrichment --> enriching: worker claims<br/>(compare-and-set)
    enriching --> enriched: generate + atomic write<br/>+ audit rows
    enriching --> needs_enrichment: transient failure,<br/>claim released, asynq retries
    enriching --> failed: retries exhausted,<br/>parked for human eyes
    enriched --> needs_enrichment: source facts change<br/>(content hash differs)
```

Idempotency exists at two levels: the claim guard (a listing can only be
processed by one worker at a time) and a content hash of the source facts (if
nothing changed, re-enriching is a no-op).

## Data model

```mermaid
%%{init: {'theme':'base','themeVariables':{
  'primaryColor':'#f5f1e8','primaryTextColor':'#1c1917','primaryBorderColor':'#d17588',
  'lineColor':'#78716c','fontFamily':'-apple-system, Segoe UI, sans-serif'}}}%%
erDiagram
    LISTINGS ||--o{ PROMOTIONS : "merchandised by"
    LISTINGS ||--o{ ENRICHMENT_AUDIT : "every change recorded"
    LISTINGS {
        text id PK
        text category "beach | city | ski | wellness | countryside | adventure"
        text destination
        int price_per_night_cents
        jsonb amenities "GIN indexed"
        jsonb vibe_tags "GIN indexed"
        text content_status "state machine above"
        text content_hash "idempotency key"
        text margin_tier "bounded ranking boost"
    }
    PROMOTIONS {
        text id PK
        text listing_id FK
        float boost "0.05 to 0.15, capped in scorer"
        bool active
    }
    ENRICHMENT_AUDIT {
        bigint id PK
        text listing_id FK
        text field
        text before
        text after
        text source "ai | template"
        text model
    }
```

Session profiles deliberately do not appear here: they live in Redis with a
7-day TTL and no PII, keyed by an anonymous session id. The GDPR surface is
near zero, and the `SignalStore` interface is the seam where a durable,
consented profile store would slot in.

## Why these shapes

**Single Go module, four binaries.** Services are isolated by package
boundaries and deployment units, not by repos. Shared platform code
(`internal/platform`) changes atomically with its consumers: no internal
version skew, one `go test ./...` for the whole platform.

**No web framework.** Go 1.22+ `http.ServeMux` has method and path routing;
`net/http` has everything else. The entire "framework" needed is about 120
lines in `httpx` (lifecycle, JSON, health), reviewable in one sitting, with
no magic in the request path.

**Interfaces at the consumer.** `search.Inventory`, `search.Ranker`,
`enrich.Store`, and `enrich.Generator` are declared where they are used,
sized to what the consumer needs, and faked in tests without a mock library.

**The LLM is treated as the least reliable dependency.** One client
(`platform/llm`) owns access. It can refuse (`ErrUnavailable`) for budget,
key, latency, or provider reasons, and every call site is forced by that
contract to have a deterministic answer ready.

**Queues for anything not user-facing.** Enrichment latency is irrelevant to
travelers, so it runs at bounded concurrency (2 workers) off an Asynq queue
with retries, backoff, and TaskID dedup. It can never starve the interactive
path of database connections.

## Showing that ranking does anything

```mermaid
flowchart LR
  Q["query + persona name"] --> S["search<br/>rule parser + relaxation ladder"]
  S --> C["one candidate set<br/>real listings, real filters"]
  C --> R["ranking · POST /v1/rank/compare"]
  R --> A["Rank(items, Profile{})<br/>cold"]
  R --> B["Rank(items, persona.Profile())<br/>warm"]
  A --> D["per-item position delta"]
  B --> D
  D --> U["two columns, warm animates<br/>from the cold ordering"]

  style Q fill:#f6e9d8,stroke:#c98f4f,color:#3a2a18
  style S fill:#e4eef7,stroke:#4f93b8,color:#12303f
  style C fill:#e4eef7,stroke:#4f93b8,color:#12303f
  style R fill:#efe4f7,stroke:#a86fe0,color:#2d1840
  style A fill:#f1f1f1,stroke:#9a9a9a,color:#1a1a1a
  style B fill:#efe4f7,stroke:#a86fe0,color:#2d1840
  style D fill:#e2f3ea,stroke:#4fb894,color:#12332a
  style U fill:#f6e9d8,stroke:#c98f4f,color:#3a2a18
```

Ranking is the hardest service to believe, because a working ranker and a
broken one produce the same-looking page. Worse, the pipeline hides its own
work: the SQL filters narrow candidates before the scorer runs, so most of what
personalization would have reordered was never a candidate.

`POST /api/compare` holds the candidate set still and varies only the profile.
Search owns the endpoint because the candidates must come from the database
through the real relaxation ladder; ranking owns the double scoring because the
scorer and the personas live there. The persona is passed by name and resolved
against `Personas()`, so a caller cannot supply a profile and manufacture a
result. Numbers and method are in [EVALUATION.md](EVALUATION.md).

## Failure modes, by dependency

| Dependency | Failure behavior |
|---|---|
| OpenRouter | Fallback parser / template generator; `degraded: llm_unavailable`; UI badge flips to "rules parsed" |
| Ranking service | Search returns base relevance order; `degraded: ranking_unavailable` |
| Redis (cache, profiles) | Intent extraction skips cache; ranking runs cold; budget guard fails open |
| Redis (Asynq) | Enrichment pauses; interactive search unaffected |
| PostgreSQL | Search fails (it is the inventory); health checks flip and the orchestrator restarts |
| Jaeger | Spans drop; requests unaffected (batch exporter, fire and forget) |
| Betterstack | Log copies drop from a bounded queue; stdout stays authoritative; requests unaffected |

## What changes at real scale

Honest answers to "this is a demo, what breaks first?":

1. **Search retrieval.** ILIKE plus GIN filters are fine for tens of
   thousands of listings. Past a million, with fuzzy destination matching,
   retrieval moves to Postgres FTS or OpenSearch; the intent and relaxation
   logic operates on the `Filter` abstraction and does not change.
2. **Ranking features.** The linear scorer is deliberately explainable. At
   scale it becomes the baseline in an A/B test against a learned ranker
   behind the same `/v1/rank` contract, with the reasons channel fed by
   feature attributions.
3. **The relaxation ladder** runs up to about ten sequential queries in the
   worst case. Batch the rungs into one query with scoring tiers, or
   precompute destination and category counts to skip empty rungs.
4. **Session profiles** move from per-request HGetAll to a small local cache
   with pub/sub invalidation if rank QPS grows.
5. **Enrichment throughput** scales by raising Asynq concurrency and
   sharding queues per content type; the claim guard already makes workers
   horizontal.
6. **Multi-tenancy** (the enterprise integration story): tenant ids plus
   per-tenant business rules and budgets; the gateway becomes the SSO
   termination point (OIDC) and services stay tenant-blind behind it.

## Security posture

Internet reaches the gateway only. Internal services listen on the compose
network. Per-IP token buckets and a body-size cap at the edge; strict CSP; no
secrets in the repo; the only credential (OpenRouter) is env-injected and
budget-capped, so leaking the demo URL cannot drain it. Jaeger sits behind
basic auth in production.
