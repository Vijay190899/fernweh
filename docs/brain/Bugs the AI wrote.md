---
title: Bugs the AI wrote
tags: [process, quality, claude-code]
date: 2026-07-31
---

# Bugs the AI wrote

The majority of this codebase was generated with Claude Code. This note
records what it got wrong, because a claim about AI-assisted development is
only worth reading if it includes the failures and how they were caught.

There is no QA team on this project. The catching mechanisms are the test
suite, running the real stack, and rendering the real page.

## Caught by tests

**"romantic" booked a trip to Rome.** The deterministic parser matched
destinations by substring, and `rom` is inside `romantic`. The query
"romantic getaway in Portugal" resolved to Rome, Italy. A table-driven test
covering a plainly reasonable query exposed it. Fixed by tokenising the query
and matching on word boundaries.

**A budget of €1,000 parsed as zero.** The thousands separator split the
number and the regex matched the trailing `000`, which then failed a sanity
check and was discarded. The query was the one printed on the front page, and
it took a test asserting the intent of the canonical example to notice.

Both bugs are the same category: code that looks correct, compiles, and is
wrong on inputs a human would consider obvious. Neither would have been found
by reading the diff.

## Caught by running the stack

**Every service crash-looped in Docker.** An OpenTelemetry import pinned a
semantic-convention schema version that conflicted with the SDK's default
resource at runtime. `go build` was clean, `go vet` was clean, and the entire
unit suite passed. The failure existed only in a real process with real
dependencies.

**The gateway panicked at boot.** A method-specific catch-all route and a
method-less subtree route overlapped, and neither was more specific under Go
1.22 mux precedence, so route registration panicked. Handler tests never ran
the registration path.

## Caught by rendering the page

**A shader linked locally and failed on strict drivers.** A uniform was
implicitly high precision in the vertex stage and medium in the fragment
stage. Lenient drivers accept it; strict ones refuse to link the program.
Found by rendering headless through a software rasteriser before merging.

**A loading curtain could outlive its own animation loop.** Progress was
driven by frame count, so throttled animation frames could leave the overlay
covering a page that was working perfectly. Rewritten to be elapsed-time
driven with a dismissal that does not depend on frames at all.

## What this actually shows

The failures cluster. Generated code is strong at structure and weak at the
boundary between the program and the world: an input a person would find
obvious, a dependency's runtime behaviour, a driver's strictness. Reviewing
the diff catches none of those. Running the thing catches all of them.

So the working agreement in [[How this was built with Claude Code]] is not
"review carefully". It is: tests alongside the code in the same change, and
the real stack up before anything merges.

Related: [[How this was built with Claude Code]], [[What I would do differently]]
