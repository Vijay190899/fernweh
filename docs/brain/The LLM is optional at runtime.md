---
title: The LLM is optional at runtime
tags: [decision, architecture, resilience]
status: accepted
date: 2026-07-31
---

# The LLM is optional at runtime

**Decision.** The language model is treated as the least reliable dependency
in the platform. Every call site that reaches for it must carry a
deterministic answer, and that obligation is enforced by the type system
rather than by discipline.

## Context

An AI product whose availability equals its model provider's availability is
a product with a single point of failure it does not control. Rate limits,
latency spikes, expired credit and provider incidents are normal operating
conditions, not edge cases.

## How it is enforced

One package owns all model access (`internal/platform/llm`). It can refuse,
returning `ErrUnavailable`, for any of four reasons: no key configured, the
daily budget is spent, the provider errored, or the call exceeded its
deadline. Because the only way to reach a model is through a function that
returns that error, a caller physically cannot forget to handle absence.

Two callers exist, and each has a real fallback rather than a stub:

| Caller | With a model | Without one |
|---|---|---|
| Intent extraction | Structured JSON intent | Word-boundary lexicon parser, English and German |
| Content generation | Written description | Template composed from listing facts |

The response carries which path served it, so degradation is visible to the
user and to whoever is debugging, not silent.

## Consequences

Good: the demo runs with no API key at all. Pulling the key is a live
demonstration rather than an outage, and it removes the usual anxiety about
leaving an AI demo running in public.

Cost: two implementations of intent extraction to maintain, and the
deterministic parser needs its own tests. That cost bought the property that
the platform's uptime does not depend on a vendor's.

The fallback parser also turned out to be the specification. Writing it first
forced the intent contract to be explicit, which made the model prompt easier
to write and gave the model's output something to be validated against.

Related: [[A daily budget cap with real unit economics]],
[[Search never returns nothing]], [[Telemetry must never block a request]]
