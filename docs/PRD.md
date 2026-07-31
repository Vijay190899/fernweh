# Fernweh Product Requirements Document

**Status:** Approved · **Owner:** Vijay Ananth · **Last updated:** 2026-07-31

Fernweh is a working demonstration of an AI-first travel commerce backend: three
production-shaped Go services that turn a natural-language travel wish into
personalized, bookable results, and keep the underlying inventory enriched
automatically. It is a portfolio project deliberately modeled on the product
surface of a modern AI travel platform (AI search, AI recommendations, content
enrichment) and built on the same operational stack: a Go monorepo with no web
framework, PostgreSQL, Redis, Asynq, OpenTelemetry/Jaeger, and Docker Compose
parity between local and production.

## 1. Why (the product case)

Travel platforms lose customers at two points:

1. **Search friction.** Filter-based search forces travelers to translate a
   fuzzy wish ("a beach week under €1,000 in June, good for kids") into a dozen
   dropdowns. Many give up; many more hit a "0 results" dead end and leave.
2. **Inventory quality.** Supplier feeds arrive with missing descriptions,
   empty amenity lists and no images. Incomplete listings convert measurably
   worse, and fixing them manually does not scale past a few hundred properties.

Fernweh demonstrates the fix for both, plus the personalization layer between
them. Each capability maps to a measurable business outcome (conversion rate,
time-to-result, content completeness); every feature below states the metric
it exists to move.

## 2. What we are building

### 2.1 AI Search (`search` service)

**User story:** As a traveler, I describe my trip in one sentence and get
relevant, bookable options instantly, never an empty page.

Requirements:

- **S1: Natural-language queries.** `POST /v1/search` accepts free text
  (German or English destinations, months, budgets, party composition, vibes).
- **S2: Intent extraction.** An LLM (via OpenRouter) extracts a strict,
  validated JSON intent: destination, country, category, budget range, month,
  party, required amenities, vibe tags. **Metric: 3× faster time-to-result vs.
  filter flows.**
- **S3: Deterministic fallback.** If the LLM is unavailable, over budget cap,
  or slow (>2.5 s), a rule-based parser produces the same intent shape. The
  search must *never* fail because an AI provider failed. **Metric: 100%
  search availability independent of LLM uptime.**
- **S4: Zero empty results.** When strict filters over-constrain, a staged
  relaxation ladder (drop vibe tags → widen budget 15%/30% → widen month →
  relax category) finds the nearest matches and *tells the user which
  constraints were relaxed*. **Metric: 0 dead-end searches.**
- **S5: Live inventory.** Queries run against PostgreSQL inventory (~300
  seeded European listings); no pre-baked answers.
- **S6: Intent caching.** Normalized queries cache their extracted intent in
  Redis (24 h TTL) to cut LLM cost and latency.
- **S7: Graceful degradation.** If the ranking service is down, results
  return in relevance order with a degraded flag. Partial failure is never
  total failure.

### 2.2 AI Recommendations (`ranking` service)

**User story:** As a returning visitor, the results I see first reflect what I
actually care about, without the platform losing control of its margins.

Requirements:

- **R1: Behavioral signals.** `POST /v1/signals` ingests view, click, dwell
  and search events per session into Redis (TTL'd, no PII).
- **R2: Traveler profile.** Signals aggregate into category affinities, price
  band preference, and amenity affinities per session.
- **R3: Personalized re-ranking.** `POST /v1/rank` re-orders search
  candidates by explainable weighted scoring: base relevance + personal
  affinity + freshness. **Metric: higher first-page conversion.**
- **R4: Business rules.** Merchandising rules from Postgres (promoted
  listings, margin tiers) apply as bounded boosts; personalization never
  overrides commercial guardrails. **Metric: promoted inventory visibility
  with 0 manual sorting.**
- **R5: Explainability.** Every ranked item carries human-readable "why"
  reasons (e.g. "matches your beach affinity", "promoted partner"). The demo
  UI surfaces them; a real platform would use them for QA and trust.

### 2.3 Content Enrichment (`enrich` service)

**User story:** As an inventory manager, listings with missing content fix
themselves overnight, and I can audit every change.

Requirements:

- **E1: Gap scanning.** A scan (on-demand + scheduled) detects listings with
  missing/thin descriptions or empty amenities. **Metric: % inventory
  completeness, visible in the dashboard.**
- **E2: Queued enrichment.** Each gap becomes an Asynq task on a dedicated
  queue with bounded concurrency, exponential-backoff retries, and a
  dead-letter queue after N failures.
- **E3: AI generation with guardrails.** The LLM writes descriptions *only
  from structured facts already on the listing* (name, location, category,
  rating, amenities); no invented claims. Template-based fallback when the
  LLM is unavailable.
- **E4: Idempotency & audit.** Tasks are idempotent (content-hash keyed);
  every enrichment stores before/after and its source (`ai` vs `template`)
  for audit. Re-running a scan never double-processes.
- **E5: Ops visibility.** `GET /v1/enrich/stats` exposes queue depth,
  processed/failed counts, completeness %; the demo UI shows live progress and
  before/after diffs.

### 2.4 Gateway (`gateway` service)

The public edge: serves the demo frontend, proxies the APIs, and protects the
platform.

- **G1: Single entry point** on `:8080`; static frontend + `/api/*` reverse
  proxy with request-ID propagation.
- **G2: Abuse protection.** Per-IP token-bucket rate limiting; global daily
  LLM-call budget in Redis; when the budget is exceeded, services silently switch to
  deterministic fallbacks. A public demo must be safe to leave running on a
  $6 API budget.
- **G3: Security headers,** CORS policy, body-size limits, timeouts on every
  hop.

### 2.5 Demo frontend

A single polished page (vanilla JS, no build step) with three views:
**Search** (hero query box with example chips, result cards, relaxation
notices, per-result "why" explanations, live profile panel), **Enrichment**
(completeness gauge, scan trigger, live queue progress, before/after diffs),
and **Under the hood** (architecture diagram, links to Jaeger traces for the
user's own recent requests).

## 3. Cross-cutting requirements

- **O1: Tracing.** Every request produces one OpenTelemetry trace across
  gateway → search → (LLM, Postgres, Redis, ranking), including trace context
  propagated *through the Asynq queue* into enrichment workers. Exported to
  Jaeger.
- **O2: Logging.** Structured JSON logs (`slog`) with `trace_id` correlation
  on every line, ready for a log shipper (Betterstack et al.).
- **O3: Health.** `/healthz` (liveness) and `/readyz` (dependency checks) on
  every service.
- **Q1: Testing.** Table-driven unit tests for all decision logic (fallback
  parser, relaxation ladder, scoring, business rules, idempotency, rate
  limiter). There is no QA team; the test suite is the QA team.
- **Q2: Load.** A Go load generator (`tools/loadgen`) drives mixed realistic
  traffic and reports p50/p95/p99; results documented in the README.
- **D1: Compose parity.** `docker compose up` starts the entire platform
  (4 services + Postgres + Redis + Jaeger + seed) identically to production.

## 4. Non-goals

Real supplier/GDS integrations, payments/booking flow, user accounts/SSO
(architecture notes only), image generation, multi-region, Kubernetes. Each is
discussed in `docs/ARCHITECTURE.md` as "how this would grow", not built.

## 5. Success criteria

1. A cold visitor types one sentence and gets relevant, personalized, explained
   results in <1.5 s (warm cache) with a Jaeger trace to prove the path.
2. Killing the LLM key, the ranking service, or Redis degrades the experience
   gracefully, never a 500 on the search path.
3. `docker compose up` → seeded, working platform in under two minutes.
4. `go test ./...` green; loadgen p95 under 300 ms for cached-intent searches
   at 50 RPS on a free-tier VM.
