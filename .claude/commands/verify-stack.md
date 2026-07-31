---
description: Check the running platform against every claim the README makes
allowed-tools: Bash, Read, Grep
---

Verify this repository still matches what it claims, and report only what you
actually observed. Do not infer a result you did not run.

1. **Build integrity**: `go build ./...`, `go vet ./...`, `go test ./...`.
   Report the per-package test line, not a summary.

2. **Architecture claims**:
   - exactly one `go.mod` (single module)
   - one `main.go` per service under `cmd/`
   - no web framework imported anywhere
     (`gin`, `echo`, `fiber`, `chi`, `gorilla/mux`)

3. **Stack presence**: confirm PostgreSQL, Redis, Jaeger and the four
   services are declared in `docker-compose.yml`, that Asynq is used by the
   enrichment pipeline, and that Betterstack is wired into every service
   entrypoint.

4. **If the stack is running** (`docker compose ps`), exercise it:
   - a natural-language search returns results and reports its intent source
   - an over-constrained query returns results with relaxations disclosed
   - `/api/enrich/stats` reports inventory completeness
   - stopping the ranking service still yields a 200 flagged `degraded`,
     then start it again

Report a short table of claim against observation. Anything you could not
verify is listed as unverified rather than assumed to pass.
