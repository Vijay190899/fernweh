---
title: Personalization is session scoped
tags: [decision, privacy, ranking]
status: accepted
date: 2026-07-31
---

# Personalization is session scoped

**Decision.** Behavioural signals live in Redis under an anonymous session key with a seven-day TTL. No accounts, no durable profiles, no personal data.

Personalization normally arrives bundled with an identity system, and with it consent flows, subject access requests, deletion guarantees and a breach surface. For a demo none of that is required, and for a European travel platform all of it is expensive.

Signals are weighted by interaction type, aggregated into category affinities, amenity and vibe weights, and an average observed price. Nothing recorded identifies a person; the key is a random session identifier and it expires on its own.

The panel in the interface exists for the same reason. If a system is going to reorder what someone sees, showing them what it inferred and offering a one-click reset is the minimum honest interface, and it doubles as the demonstration that the ranking is doing something.

`SignalStore` is the seam. A consented, durable profile store would implement the same two methods, and nothing in the scorer would change.

Related: [[Business rules are bounded and disclosed]]
