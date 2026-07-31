---
description: Scaffold a new service that satisfies every platform invariant
argument-hint: <service-name> <one-line purpose>
allowed-tools: Bash, Read, Write, Edit, Grep, Glob
---

Add a new service called $ARGUMENTS to the monorepo. Follow the shape the
existing four already use rather than inventing a new one; read
`cmd/ranking/main.go` first, it is the smallest complete example.

The service is not finished until all of these hold:

- `cmd/<name>/main.go` loads config from the environment, sets up tracing,
  connects only to the stores it needs, and serves through
  `internal/platform/httpx` with graceful shutdown.
- Logging goes through `internal/platform/logging`, and every log call in a
  request path uses the `Context` variants so trace ids attach.
- `/healthz` and `/readyz` exist, and readiness genuinely checks its
  dependencies.
- Outbound calls to other services propagate trace context. A feature that
  breaks the single-trace property is not done.
- If it touches a model, it goes through `internal/platform/llm` and handles
  `ErrUnavailable` with a real deterministic fallback, reported in the
  response. This is not optional; see `docs/brain/The LLM is optional at
  runtime.md`.
- Domain interfaces are declared in the consuming package and sized to what
  it uses, so tests can fake them without a mocking library.
- Table-driven tests cover the decision logic, in the same change.
- `docker-compose.yml` gains the service, `.env.example` documents any new
  variables with empty values, and `deploy/compose.prod.yml` gains its
  restart policy and memory limit with no published host port.

Finish by running the full test suite and bringing the stack up.
