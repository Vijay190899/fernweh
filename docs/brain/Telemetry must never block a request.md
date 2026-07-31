---
title: Telemetry must never block a request
tags: [decision, observability, reliability]
status: accepted
date: 2026-07-31
---

# Telemetry must never block a request

**Decision.** Log shipping is asynchronous, bounded and lossy. Standard output remains the authoritative stream.

Observability is added to make a system more reliable, and it routinely does the opposite: a logging backend slows down, the shipping call is synchronous, and request latency now depends on a vendor that has nothing to do with serving the user.

So the Betterstack handler writes to stdout first, then offers a copy to a bounded queue. If the queue is full the copy is dropped. Delivery happens on a background goroutine in batches, retries once, then gives up. Nothing about the request path can wait on it.

The trace exporter follows the same principle through batching, and the daily budget counter fails open for the same reason.

There is a test that makes this a guarantee rather than an intention: it points the shipper at a dead endpoint and writes twice the queue capacity, asserting that logging returns promptly. Dropping telemetry under pressure is correct behaviour; blocking a traveller's search to record that it happened is not.

Related: [[One trace across the queue boundary]], [[A daily budget cap with real unit economics]]
