---
title: Enrichment is idempotent twice over
tags: [decision, reliability, queue]
status: accepted
date: 2026-07-31
---

# Enrichment is idempotent twice over

**Decision.** Two independent guards make repeated enrichment safe: a claim on the listing row, and a hash of the facts the content was derived from.

Background pipelines get replayed. Scans overlap, tasks retry after backoff, a worker dies mid-write and the job returns to the queue. Any of these can double-process, and doubled AI generation costs money and rewrites content that was already correct.

The first guard is a compare-and-set in SQL. A worker moves a listing from `needs_enrichment` to `enriching` only if it is still in the first state, and checks that exactly one row changed. Two workers racing for the same listing means one claims it and the other sees zero rows and cleanly skips. No locks, no coordination service.

The second guard is a content hash over the fields the copy is generated from, with amenities sorted so ordering cannot change it. If the source facts have not changed, regenerating produces nothing new and can be skipped.

Failure paths matter as much as success. A transient error releases the claim so a retry can pick the listing up again, and exhausting retries parks it as `failed` where the dashboard shows it. Work is never lost silently and never invisibly stuck.

Enqueueing is deduplicated too, keyed on the listing, so overlapping scans are harmless by construction rather than by timing.

Related: [[One trace across the queue boundary]], [[Generated content is grounded in listing facts]]
