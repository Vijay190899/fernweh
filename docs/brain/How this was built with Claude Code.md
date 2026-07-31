---
title: How this was built with Claude Code
tags: [process, claude-code]
date: 2026-07-31
---

# How this was built with Claude Code

Claude Code generated most of the Go in this repository. The interesting
question is not whether that is possible, it is what the human has to do so
the result is worth merging.

## The working agreement

**Specification before code.** The [[PRD|product requirements]] and the
implementation plan were written and merged before any service existed, and
every later change traces to a requirement. Generation quality tracks
specification precision almost linearly: "zero empty results with disclosed
relaxations" produced a correct ladder immediately because the *product*
behaviour left nothing to invent.

**Standing guardrails in `CLAUDE.md`.** The repository root carries the
invariants: no user-facing path may depend on a model being up, business
boosts stay bounded, enrichment writes stay audited, every hop propagates
trace context, decision logic ships with table tests. These persist across
sessions, so architectural consistency does not depend on remembering to
re-explain it. This is the single highest-leverage file in the project.

**One reviewable change per capability.** Fourteen pull requests, each a
bounded capability with its tests in the same diff. Small diffs matter more
with generated code, not less, because review is the entire human
contribution.

**Tests as the review mechanism.** Generated tests get read critically rather
than trusted. See [[Bugs the AI wrote]] for what that caught.

**Run the real thing.** Every change touching the request path ends with the
full stack up and the flows exercised. Three of the six real defects in this
project were invisible to compilation and unit tests.

## Beyond prompting

The repository configures the tool rather than only talking to it:

- `CLAUDE.md` encodes architectural invariants as standing instructions.
- `.claude/commands/` holds project slash commands, so routine work
  (`/verify-stack`, `/ship`) is one word instead of a paragraph re-typed
  from memory.
- `.claude/settings.json` registers hooks that run `gofmt` after any Go edit
  and `go build` before any commit, so formatting and breakage are handled by
  the harness rather than by remembering.
- `.claude/agents/` defines a reviewer subagent with a narrow brief and
  read-only tools, used for a second pass on service code.
- Screenshot tooling drives a real browser over the DevTools protocol, so
  interface claims are verified against a rendered page instead of asserted.

## Honest accounting

Speed came from specification, guardrails, small diffs and running the
system, which is the same discipline that makes hand-written code good. The
tool applied it faster; it did not replace it.

Where it was genuinely strong: scaffolding services, writing exhaustive table
tests, and porting a pattern across four services consistently.

Where it needed supervision: anything touching the outside world. Runtime
dependency behaviour, driver strictness, and inputs a person would find
obvious.

Related: [[Bugs the AI wrote]], [[What I would do differently]]
