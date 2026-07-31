# CLAUDE.md — working agreements for AI-assisted development in this repo

This repo is built AI-first with Claude Code. These are the standing
instructions that keep generated code consistent with the architecture.

## Project shape

- Single Go module `fernweh`, multiple binaries under `cmd/` (gateway, search,
  ranking, enrich, seed). Shared infra lives in `internal/platform/*`; domain
  logic in `internal/{search,ranking,enrich,inventory}`.
- **No web framework.** stdlib `net/http` with Go 1.22+ method patterns
  (`mux.HandleFunc("POST /v1/search", ...)`). Server plumbing goes through
  `internal/platform/httpx` — do not hand-roll servers or JSON helpers.
- Dependencies are deliberate: pgx, go-redis, asynq, otel, x/time. Propose
  before adding anything else.

## Non-negotiable invariants

1. **No user-facing path may depend on an LLM being up.** Anything calling
   `platform/llm` must handle `ErrUnavailable` with a deterministic fallback
   and surface the degradation in its response (`degraded`, `intent_source`,
   `source` fields).
2. **Search never returns zero results** while inventory is non-empty: new
   constraints must be added to the relaxation ladder (`internal/search/relax.go`)
   with a user-facing note. Silent relaxation is a bug.
3. **Business-rule boosts stay bounded** (`internal/ranking/scorer.go`):
   promotions may tip near-peers, never bury a clearly better match; promoted
   items always carry a disclosure reason.
4. **Enrichment writes are audited and idempotent**: any new enrichment field
   goes through the claim guard + `ApplyEnrichment` transaction + audit rows.
5. **Every cross-service and cross-queue hop propagates trace context**
   (`otelx.Inject` / `InjectMap`). A feature that breaks the single-trace
   property is not done.
6. **Logs are slog JSON with ctx** (`log.InfoContext`) so trace ids attach.

## Testing bar (there is no QA team)

- Decision logic (parsers, ladders, scorers, guards) gets table-driven unit
  tests in the same PR — not after.
- HTTP handlers get `httptest` coverage with consumer-side fakes; interfaces
  are defined at the consumer for exactly this reason.
- `go build ./... && go vet ./... && go test ./...` must pass before any
  commit. Run it; don't assume.

## Conventions

- Errors: wrap with context (`fmt.Errorf("scan: %w", err)`); sentinel errors
  only when callers branch on them (`llm.ErrUnavailable`).
- Config comes only from env via `platform/config`; new vars get a default,
  a `.env.example` entry, and a compose wiring.
- SQL lives in the repo layer (`internal/inventory`); services never import
  pgx directly.
- Migrations: append-only numbered files in `migrations/`; never edit an
  applied migration.
- Commits: imperative subject, body explains the why; PRs describe design
  decisions, not file lists.

## Local verification loop

```
docker compose up -d postgres redis jaeger   # stores only
go run ./cmd/seed                            # migrate + seed
go run ./cmd/search & go run ./cmd/ranking & go run ./cmd/enrich & go run ./cmd/gateway
# or the whole thing: docker compose up --build
go test ./... && go vet ./...
go run ./tools/loadgen -rps 50 -duration 30s # before claiming performance
```
