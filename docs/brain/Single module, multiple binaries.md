---
title: Single module, multiple binaries
tags: [decision, architecture]
status: accepted
date: 2026-07-31
---

# Single module, multiple binaries

**Decision.** One Go module, one `cmd/<service>/main.go` per deployable, shared code under `internal/`.

Services are isolated by package boundaries and deployment units, not by repositories. Shared platform code changes atomically with everything that consumes it, so there is no internal version skew and no release dance to update a logging helper. One `go test ./...` covers the platform.

The tradeoff is that a genuine team would eventually want independent release cadences per service, which this makes harder. At four services owned by one person it is plainly the right side of that trade, and the split can happen later along the boundaries the packages already draw.

Related: [[No web framework]], [[Interfaces declared at the consumer]]
