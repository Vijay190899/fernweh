---
title: Business rules are bounded and disclosed
tags: [decision, product, ranking, ethics]
status: accepted
date: 2026-07-31
---

# Business rules are bounded and disclosed

**Decision.** Commercial weighting can tip a close call between comparable
listings. It cannot bury a clearly better match, and whenever it applies, the
result card says so.

## Context

Every marketplace ranks on a blend of relevance and margin. The question is
not whether commercial interest enters the ranking, it is whether it is
bounded and whether the user can see it. Unbounded, it degrades the product
until the results stop being useful and the traveller stops trusting them.

## Mechanism

Scoring is an explicit weighted sum:

```
score = 0.55 · base relevance
      + 0.30 · personal fit
      + 0.15 · business rules
```

The business term is capped, so its maximum contribution cannot exceed the
gap between a mediocre listing and a good one. Two tests hold the line from
both directions: a promoted premium listing with a weak base score must not
outrank a clearly better unpromoted one, and between near-peers the promotion
must win. Either test failing is a product bug, not a tuning question.

Promoted results carry a visible label and a reason on the card.

## Consequences

The weights are legible and arguable, which is the point. A product manager
can read the ranking policy without reading Go, and a reviewer can see that
the ethical constraint is enforced by a test rather than asserted in a
document.

A learned ranker would score better. It would also make this property much
harder to guarantee, which is why the linear model is the baseline any
replacement has to beat while keeping the bound. Noted in
[[What I would do differently]].

Related: [[Personalization is session scoped]], [[Search never returns nothing]]
