---
title: One trace across the queue boundary
tags: [decision, observability]
status: accepted
date: 2026-07-31
---

# One trace across the queue boundary

**Decision.** Trace context is serialised into the Asynq task payload, so a
scan, its Redis hop, the worker that picks it up and the database write it
performs all appear as a single trace.

## Context

Distributed tracing across HTTP is solved: headers carry the context and the
instrumentation is standard. Queues are where traces usually break. A job is
enqueued in one process and executed in another, minutes later, and most
systems treat those as unrelated traces. The result is that background work,
which is exactly where failures hide, is the least observable part of the
platform.

## Mechanism

`otelx.InjectMap` writes the W3C trace context into a plain string map that
is marshalled with the task. `otelx.ExtractMap` restores it in the worker
before any handler logic runs. The payload carries its own provenance.

Because the enqueue side and the execute side agree on nothing except the
payload, the propagation survives restarts, retries and backoff delays.

## Consequences

Debugging enrichment is the same activity as debugging search: open one trace
and read it top to bottom. A task that failed twice and succeeded on its
third attempt shows all three attempts under the scan that created it.

This is also the property most likely to break silently as the system grows,
because a new producer simply forgetting to inject would still work. That is
why it is written into `CLAUDE.md` as an invariant rather than left as a
convention: a change that breaks the single-trace property is not finished.

Related: [[Enrichment is idempotent twice over]],
[[Telemetry must never block a request]]
