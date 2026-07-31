# Fernweh, Implementation Plan

## Monorepo layout (single Go module, no framework)

```
fernweh/
├── cmd/                    # one main per deployable
│   ├── gateway/
│   ├── search/
│   ├── ranking/
│   ├── enrich/
│   └── seed/               # migration runner + inventory seeder
├── internal/
│   ├── platform/           # shared infrastructure (the "no framework" framework)
│   │   ├── config/         # env-based config, fail-fast validation
│   │   ├── logging/        # slog JSON + trace_id correlation
│   │   ├── otelx/          # tracer/exporter setup, HTTP middleware
│   │   ├── httpx/          # server wiring, middleware, JSON helpers, client
│   │   ├── llm/            # OpenRouter client, budget guard, fallback interface
│   │   ├── db/             # pgx pool, migration runner
│   │   └── redisx/         # go-redis client factory
│   ├── inventory/          # domain model + repository (shared read model)
│   ├── search/             # intent types, LLM extractor, fallback parser,
│   │                       # query builder, relaxation ladder, handler
│   ├── ranking/            # signal store, profile builder, scorer, rules, handler
│   └── enrich/             # gap scanner, Asynq tasks/handlers, generator, handler
├── web/                    # static frontend (vanilla JS/CSS, no build step)
├── migrations/             # 00X_*.sql, embedded via go:embed
├── tools/loadgen/          # Go load generator
├── deploy/                 # production compose overlay
├── docs/                   # PRD, this plan, architecture, deployment, build log
├── docker-compose.yml      # full local stack (mirrors prod)
├── Makefile
├── CLAUDE.md
└── README.md
```

**Dependencies (deliberately few):** `jackc/pgx/v5`, `redis/go-redis/v9`,
`hibiken/asynq`, `go.opentelemetry.io/otel` (+ otlptracehttp exporter).
Everything else is stdlib (`net/http` with Go 1.22+ method routing, `slog`,
`encoding/json`, `go:embed`).

## Build order

1. **Scaffold**, go.mod, Makefile, compose (Postgres/Redis/Jaeger), migrations
   (listings, promotions, enrichment_audit), seed generator (~300 deterministic
   European listings, ~40% with deliberate content gaps).
2. **Platform packages**, config, logging, otelx, httpx, redisx, db, llm
   (OpenRouter chat-completions client + Redis daily budget counter + cache).
3. **Search service**, fallback parser first (it defines the intent contract
   and is fully unit-testable), then LLM extractor conforming to the same
   interface, query builder + relaxation ladder, HTTP handler, ranking client
   with timeout/degradation.
4. **Ranking service**, signal ingestion, profile aggregation, scorer with
   explicit weights + business-rule boosts, rank handler with reasons.
5. **Enrich service**, gap scanner, Asynq task enqueue/handler with trace
   propagation, LLM + template generators, audit writes, stats API.
6. **Gateway**, static file serving, reverse proxy with request-ID and
   rate limiting, security headers.
7. **Frontend**, three-view SPA against the gateway API.
8. **Quality pass**, unit tests alongside each package (written with the
   code, not after), loadgen, full-stack compose verification, README.

## Key technical decisions (ADR summaries)

- **Single Go module, multiple binaries.** Simplest dependency story; services
  stay isolated by package boundaries, not repo boundaries. Matches "Go
  monorepo, 3+ services".
- **stdlib `net/http` + `http.ServeMux` method patterns.** Go 1.22+ routing
  makes frameworks unnecessary; keeps binaries small and reviewable.
- **Session-scoped personalization in Redis with TTL.** No accounts, no PII,
  demo-appropriate; interface designed so a durable profile store could slot in.
- **LLM behind an interface with two implementations** (`openrouter`,
  `deterministic`). Selection per request: budget guard → availability →
  timeout. This is the core resilience story.
- **Asynq for enrichment.** Redis-backed, supports retries/backoff/DLQ and
  scheduled jobs; trace context serialized into task payloads for end-to-end
  traces across the queue boundary.
- **Migrations via embedded SQL + tiny runner** (no external tool): fewer
  moving parts, versioned in-repo, runs at seed time.
- **Frontend with no build step.** One less toolchain; the artifact is the
  backend, but the demo must still look excellent.

## Testing strategy

- Unit (no I/O): intent fallback parser table tests, relaxation ladder,
  scorer/rules math, gap detection, idempotency hash, token bucket.
- Handler tests: `httptest` against in-memory fakes of repos/stores.
- Integration (build tag `integration`, needs compose): seed → search → rank
  → enrich happy path + degradation paths (LLM off, ranking down).
- Load: `tools/loadgen -rps 50 -duration 60s` mixed query set; record
  p50/p95/p99 in README.

## Risks

- **OpenRouter budget ($6).** Mitigated by intent cache, daily budget counter,
  cheap default model, fallback mode. Worst case: demo runs 100% deterministic.
- **Free-tier VM RAM (Jaeger + Postgres + Redis + 4 services).** Use Jaeger
  all-in-one with memory storage capped, Postgres tuned small; ARM VM has
  headroom (24 GB on Oracle free tier). Fallback: drop Jaeger in prod, keep
  locally.
- **Windows dev environment.** All services run in compose; Go builds natively
  cross-platform; Makefile targets have PowerShell-safe equivalents documented.
