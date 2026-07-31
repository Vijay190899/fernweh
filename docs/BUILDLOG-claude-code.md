# Build log: how Fernweh was built with Claude Code

This project is itself the answer to "share specific examples of your Claude
Code experience". It was built AI-first: Claude Code generated the large
majority of the Go code, and the human role was what an AI-first engineer's
role actually is: product decisions, architectural guardrails, reviewing
generated code, and owning quality. This log documents the workflow honestly,
including where the AI was wrong and how that was caught.

## The workflow

1. **Spec before code.** The PRD (`docs/PRD.md`) and implementation plan were
   written first and merged as PR #1. Every subsequent PR traces to a
   requirement ID. Claude Code works dramatically better when the spec pins
   down behavior: "zero empty results with disclosed relaxations" produced a
   correct ladder on the first pass because the *product* behavior was
   unambiguous.

2. **CLAUDE.md as standing guardrails.** The repo's `CLAUDE.md` encodes the
   invariants (LLM-optional paths, bounded business boosts, audited writes,
   single-trace property, testing bar). This is the difference between
   "generate code" and "generate code that fits this architecture": the
   instructions persist across sessions, so consistency does not depend on
   re-explaining.

3. **One PR per bounded capability**, each with its tests in the same diff:
   scaffold, search, ranking, enrichment, gateway and frontend, observability,
   ops tooling. Small, reviewable diffs matter even more with generated code,
   because review *is* the human contribution.

4. **Tests are the review mechanism, not an afterthought.** Table-driven
   tests were generated alongside each decision-heavy package and then read
   critically. Together with running the real stack, this caught real
   generation bugs before they shipped:
   - The fallback parser matched destinations by substring; the test query
     "romantic getaway in Portugal" routed to **Rome** (`rom` inside
     `romantic`). Fix: word-boundary matching over a tokenized query. (PR #3)
   - "€1,000" parsed as budget **0** because the thousands separator split
     the number and the regex matched the trailing "000". A canonical-query
     test exposed it. (PR #3)
   - The OTel semconv import pinned a schema version that conflicted with
     the SDK's default resource at container runtime. Every service
     crash-looped in compose while `go build` and unit tests stayed green.
     Caught by running the full stack before merging. (PR #6)
   - A Go 1.22 ServeMux pattern conflict: `GET /` overlaps the method-less
     `/api/` subtree and neither is more specific, so route registration
     panics at boot. Again invisible to unit tests, visible the moment the
     container started. (PR #7)
   - A WebGL uniform precision mismatch (`highp` in the vertex stage,
     `mediump` in the fragment stage) linked fine on lenient drivers and
     failed on strict ANGLE. Found by rendering the page headless through
     SwiftShader before merging. (PR #10)

5. **Run the real thing before claiming done.** Every PR that touches the
   request path ends with `docker compose up --build`, manual verification of
   the demo flows, and `tools/loadgen` for any latency claim in the README.
   "It compiles and tests pass" is not "it works".

## What the human decided (and the AI did not)

- Product scope and metrics: which three capabilities to build, what each
  must prove, and what is explicitly out of scope.
- The resilience posture: LLM-optional as a hard invariant rather than a
  fallback bolted on later; budget caps as a first-class feature.
- Ranking ethics: business boosts bounded and disclosed; relaxations
  disclosed; the profile transparency panel in the UI.
- The data model, service boundaries, and the "interfaces at the consumer"
  testing seam.
- Every merge. Nothing landed unreviewed.

## What this proves about velocity

Four production-shaped services, with tracing across an async queue boundary,
graceful degradation, seeded data, a designed frontend, benchmarks, tests,
and deploy tooling, took days rather than weeks. The speed multiplier is real,
but it is conditional: it comes from specs, guardrails, small reviewed diffs, and
running the system. The same discipline that makes human-written code good,
applied at generation speed.
