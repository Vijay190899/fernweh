# Build log — how Fernweh was built with Claude Code

This project is itself the answer to "share specific examples of your Claude
Code experience". It was built AI-first: Claude Code generated the large
majority of the Go code, and the human role was what an AI-first engineer's
role actually is — product decisions, architectural guardrails, reviewing
generated code, and owning quality. This log documents the workflow honestly,
including where the AI was wrong and how that was caught.

## The workflow

1. **Spec before code.** The PRD (`docs/PRD.md`) and implementation plan were
   written first and merged as PR #1. Every subsequent PR traces to a
   requirement ID. Claude Code works dramatically better when the spec pins
   down behavior — "zero empty results with disclosed relaxations" produced a
   correct ladder on the first pass because the *product* behavior was
   unambiguous.

2. **CLAUDE.md as standing guardrails.** The repo's `CLAUDE.md` encodes the
   invariants (LLM-optional paths, bounded business boosts, audited writes,
   single-trace property, testing bar). This is the difference between
   "generate code" and "generate code that fits this architecture" — the
   instructions persist across sessions, so consistency doesn't depend on
   re-explaining.

3. **One PR per bounded capability**, each with its tests in the same diff:
   scaffold → search → ranking → enrichment → gateway/frontend → ops tooling.
   Small, reviewable diffs are even more important with generated code,
   because review *is* the human contribution.

4. **Tests are the review mechanism, not an afterthought.** Table-driven
   tests were generated alongside each decision-heavy package and then read
   critically. This caught real generation bugs before merge:
   - The fallback parser matched destinations by substring — the test query
     "romantic getaway in Portugal" routed to **Rome** (`rom` ⊂ `romantic`).
     Fix: word-boundary matching over a tokenized query. (PR #3)
   - "€1,000" parsed as budget **0** — the thousands separator split the
     number and the regex matched the trailing "000". A canonical-query test
     exposed it. (PR #3)
   - The OTel semconv import pinned a schema version that conflicted with the
     SDK's default resource at container runtime — every service
     crash-looped in compose while `go build` and unit tests were green.
     Caught by actually running the full stack before merging, which is the
     habit unit tests don't replace. (PR #6)

5. **Run the real thing before claiming done.** Every PR that touches the
   request path ends with `docker compose up --build` and manual verification
   of the demo flows, plus `tools/loadgen` for the latency claims in the
   README. "It compiles and tests pass" is not "it works".

## What the human decided (and the AI didn't)

- Product scope and metrics: which three capabilities to build, what each
  must prove, what is explicitly out of scope.
- The resilience posture: LLM-optional as a hard invariant rather than a
  fallback bolted on later; budget caps as a first-class feature.
- Ranking ethics: business boosts bounded and disclosed; relaxations
  disclosed; profile transparency panel in the UI.
- Data model, service boundaries, and the "interfaces at the consumer"
  testing seam.
- Every merge. Nothing landed unreviewed.

## What this proves about AI-first velocity

Four production-shaped services — with tracing across an async queue
boundary, graceful degradation, seeded data, a polished frontend, tests, and
deploy tooling — took days, not weeks. The leverage is real, but it is
conditional: it comes from specs, guardrails, small reviewed diffs, and
running the system — the same discipline that makes human-written code good,
applied at generation speed.
