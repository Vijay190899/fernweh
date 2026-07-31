---
title: Generated content is grounded in listing facts
tags: [decision, ai, content]
status: accepted
date: 2026-07-31
---

# Generated content is grounded in listing facts

**Decision.** Enrichment writes copy only from structured facts already present on the listing, and the output is validated rather than trusted.

A model asked to write a hotel description will produce an infinity pool and sea views if given room to. In travel commerce that is not a quality problem, it is a consumer protection problem: the platform would be publishing claims about a property that are not true.

So the prompt receives only the listing's own fields and is told not to invent, and the reply is then checked in code. `sanitizeAmenities` guarantees every original amenity survives, caps additions at three, and normalises case, so the model cannot quietly drop or invent facilities. A description that comes back empty is rejected outright and the task retries.

The template generator is the floor. It composes a serviceable description from the same facts with no model at all, which is what makes enrichment continue to work at zero budget, and gives the AI path something to be compared against.

Every write records its before, its after, and whether a model or the template produced it. Provenance is a first-class column, not a log line.

Related: [[The LLM is optional at runtime]], [[Enrichment is idempotent twice over]]
