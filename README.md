# 🧭 Fernweh

*Fernweh (German): the ache for far-off places.*

A working slice of an AI-first travel platform: **natural-language search
with zero dead ends**, **personalized ranking with bounded business rules**,
and a **self-healing content pipeline** — four Go services, no web framework,
built AI-first with Claude Code and owned end-to-end: spec → code → tests →
tracing → deploy.

> **Why this exists:** it's a portfolio project deliberately shaped like a
> production travel backend — same stack, same operational concerns, same
> product trade-offs. The specs are in [docs/PRD.md](docs/PRD.md); how it was
> built with Claude Code (including the bugs the AI wrote and how they were
> caught) is in [docs/BUILDLOG-claude-code.md](docs/BUILDLOG-claude-code.md).

## Try it in two minutes

```bash
git clone https://github.com/Vijay190899/fernweh && cd fernweh
cp .env.example .env        # optional: add an OpenRouter key for live AI
docker compose up --build   # postgres + redis + jaeger + 4 services, seeded
```

- **Demo:** http://localhost:8080 — type *"A beach weekend under €1,000 in
  March"*, or auf Deutsch: *"Städtetrip nach Wien im Oktober, ruhig und
  günstig"*
- **Jaeger:** http://localhost:16686 — every search response links its own
  trace (`trace ↗` in the results bar)
- **No API key?** Everything still works — the deterministic fallbacks take
  over and the UI badge shows `⚙️ rules parsed` instead of `🤖 AI parsed`.

## What to look at

| The claim | Where to see it |
|---|---|
| One sentence → ranked live inventory | Search tab; intent pills show what was understood, and by which engine (LLM / rules / cache) |
| **Zero dead ends** — over-constrained queries relax stepwise, disclosed | Search *"5-star ski chalet in Zermatt under €50"* and read the yellow banner |
| Personalization you can inspect | Click/book a few beach stays, search again — watch re-ranking, per-result "why" pills, and the profile panel |
| Business rules, bounded and disclosed | Promoted listings carry a ribbon + reason; they can tip near-peers but never bury a better match (tested) |
| Self-healing content | Content Ops tab: 40% of seeds have gaps; run a scan, watch the Asynq queue drain, click enriched rows for before/after diffs with audit provenance |
| One trace across the queue | Open a trace in Jaeger: gateway → search → LLM/Postgres/ranking; enrichment traces span scan → Redis queue → worker → Postgres |
| Degrades, never dies | `docker compose stop ranking` — search keeps answering (flagged `degraded`); remove the API key — everything still works |

## Architecture

```
                       ┌──────────────────────────────┐
  internet ──────────▶ │ gateway :8080                │  static UI · /api proxy
                       │ rate limit · CSP · body caps │  per-IP token buckets
                       └──────┬───────────────────────┘
              ┌───────────────┼──────────────────┐
              ▼               ▼                  ▼
      ┌──────────────┐ ┌──────────────┐ ┌────────────────┐
      │ search :8081 │ │ ranking :8082│ │ enrich :8083   │
      │ NL → intent  │ │ signals →    │ │ Asynq workers  │
      │ → SQL + ladder│ │ profile →   │ │ scan → claim → │
      │ LLM ⊕ rules  │ │ score+reasons│ │ generate → audit│
      └──────┬───────┘ └──────┬───────┘ └───────┬────────┘
             │                │                 │
      ┌──────▼────────────────▼─────────────────▼──────┐
      │ PostgreSQL (inventory · promotions · audit)    │
      │ Redis (profiles · intent cache · queue · LLM budget) │
      │ Jaeger (OTLP traces, one per request)          │
      └────────────────────────────────────────────────┘
```

Design notes, failure-mode table, and honest "what breaks at real scale"
answers: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

**Key invariant:** the LLM (OpenRouter, budget-capped in Redis) is treated as
the *least reliable* dependency. Every AI call site has a deterministic
fallback, enforced by a single `ErrUnavailable` contract — the platform's
availability is never a function of the AI's.

## Quality

No QA team: the test suite and the traces are the QA.

```bash
go test ./...     # parsers, relaxation ladder, scorer bounds, claim/release,
                  # rate limiter, degradation paths — table-driven, no mocks
go vet ./...
go run ./tools/loadgen -rps 50 -duration 30s   # measured, not asserted:
```

Load (the whole stack — 4 services + Postgres + Redis + Jaeger — in Docker
Desktop on a dev laptop, mixed natural-language queries, deterministic-parser
mode, per-IP rate limit raised for the test):

```
50 rps · 30s   →  1500 requests, 0 errors, all HTTP 200
                  p50 4ms · p95 5ms · p99 6ms · max 19ms
200 rps · 30s  →  5997 requests, 0 errors, all HTTP 200
                  p50 4ms · p95 5ms · p99 6ms · max 30ms
```

Latency stays flat from 50→200 rps — the interactive path is nowhere near
saturation on a laptop, which is the point of keeping it stdlib + two
indexed queries + Redis lookups.

Also verified by hand: `docker compose stop ranking` mid-session → searches
keep answering with `degraded: [ranking_unavailable]`; no OpenRouter key →
everything runs on the deterministic fallbacks.

## Repo map

```
cmd/            gateway · search · ranking · enrich · seed (one main each)
internal/
  platform/     config · slog+trace logging · otelx · httpx · pgx+migrations
                · redis · the LLM door (budget guard + fallback contract)
  inventory/    domain model · repo (dynamic filters, guarded transitions,
                audited enrichment writes)
  search/       intent (LLM ⊕ deterministic parser) · relaxation ladder
  ranking/      signal store · profile · explainable scorer
  enrich/       scanner · Asynq pipeline · fact-grounded generators
web/            the demo UI (vanilla, embedded in the gateway binary)
deploy/         prod compose overlay · Caddy TLS · VM bootstrap script
docs/           PRD · architecture · deployment · Claude Code build log
```

## Deploying

Local mirrors production by design — production is the same compose file plus
a TLS edge and resource limits on a single free-tier VM:
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

---

Built by **Vijay Ananth** · AI-first with Claude Code · Go · PostgreSQL ·
Redis · Asynq · OpenTelemetry · no web framework
