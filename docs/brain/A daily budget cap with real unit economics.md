---
title: A daily budget cap with real unit economics
tags: [decision, cost, ai]
status: accepted
date: 2026-07-31
---

# A daily budget cap with real unit economics

**Decision.** All model access spends against a platform-wide daily counter in Redis, set from measured per-call cost rather than a round number.

The prompts were measured rather than estimated. At the configured model's rates, intent extraction costs roughly 385 input and 50 output tokens, about $0.00064 per uncached query; content generation about $0.00094 per listing, so repairing all 126 seeded gaps costs about $0.12.

That arithmetic sets the cap. 500 calls per day bounds worst-case spend near $0.32 per day, and it sits in `.env.example` beside the knob so the next person to change it can see what it costs.

Two properties make this safe rather than merely frugal. Intent results cache for 24 hours, so the example queries on the landing page cost nothing after the first visitor. And exceeding the cap is not an outage: every service falls back to its deterministic path, so a public URL cannot drain a prepaid balance and cannot break either.

Failing open matters here too. If Redis is unreachable the counter cannot be read, and the request proceeds rather than failing. Protecting spend must not become a way to take the product down.

Related: [[The LLM is optional at runtime]], [[Telemetry must never block a request]]
