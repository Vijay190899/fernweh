---
title: Search never returns nothing
tags: [decision, product, search]
status: accepted
date: 2026-07-31
---

# Search never returns nothing

**Decision.** While inventory is non-empty, search always returns results,
and every constraint it relaxed to get there is shown to the traveller.

## Context

A zero-result page is the most expensive screen in travel commerce. The
traveller has already expressed intent and the platform answers with nothing,
so they leave. The usual fix is to quietly widen the query and present the
output as if it were a match, which converts better in the short term and
erodes trust when the traveller notices the dates are wrong.

## The mechanism

Constraints relax in a fixed order, chosen by how negotiable each one is to a
person planning a trip:

```
style tags → amenities → minimum rating → budget +15% → budget +30%
           → dates → destination → country → category → unconstrained
```

The ladder stops at the first rung that yields results. The final rung has no
constraints at all, which is what makes the guarantee structural rather than
hopeful. Budget widening is computed from the original figure rather than
compounding, so "+30%" means what it says.

Every applied relaxation is returned in the response and rendered above the
results: *"No exact match, so these constraints were relaxed: relaxed style
preferences, included lower-rated stays, stretched budget by 30%."*

## Consequences

Disclosure is the part that matters. Widening silently would be a dark
pattern; widening openly turns a dead end into a negotiation the traveller
can see and correct. It also makes the feature debuggable, because the
response states exactly which rung answered.

The cost is up to ten sequential queries in the worst case. At this inventory
size that is a few milliseconds. At a million listings the rungs would be
collapsed into one query with scoring tiers; the ladder operates on a filter
abstraction rather than on SQL, so that change would not touch the product
logic. Noted in [[What I would do differently]].

Related: [[The LLM is optional at runtime]],
[[Business rules are bounded and disclosed]]
