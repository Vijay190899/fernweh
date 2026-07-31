---
title: No web framework
tags: [decision, architecture]
status: accepted
date: 2026-07-31
---

# No web framework

**Decision.** Standard library `net/http` only, with Go 1.22 method and path routing.

`http.ServeMux` now handles `POST /v1/search` and `GET /v1/enrich/listings/{id}/audit` natively, which removes the original reason most Go services import a router. Everything else a framework provides came to about 120 lines in `internal/platform/httpx`: server lifecycle with graceful shutdown, JSON helpers, health endpoints, and a traced client.

What this buys: the request path has no magic in it, there is no framework CVE surface, binaries stay small, and a reviewer can read the entire HTTP layer in one sitting.

What it costs: middleware composition and validation are hand-written. Both were about an afternoon, and the resulting code is easier to explain than a framework's conventions.

It also matches the target environment, which states a Go monorepo with no framework.

Related: [[Single module, multiple binaries]]
