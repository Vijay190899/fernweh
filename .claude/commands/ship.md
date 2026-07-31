---
description: Take the current work from branch to merged PR, with the quality gates enforced
argument-hint: [short description of the change]
allowed-tools: Bash, Read, Grep, Glob
---

Ship the current work as a reviewed pull request. $ARGUMENTS describes the
change.

Refuse to continue if any gate fails, and say which one.

**Gates, in order**
1. `go build ./...` and `go vet ./...` are clean.
2. `go test ./...` passes. If this change touched decision logic (a parser,
   the relaxation ladder, the scorer, a claim guard, the rate limiter) and
   added no test, stop and say so. That is the standing bar in `CLAUDE.md`.
3. If the change touched the request path, the full stack must come up and
   the affected flow must be exercised for real. Compilation is not evidence.
4. No secrets staged. `.env` must not appear in `git status`, and
   `.env.example` must contain no values, only names.

**Then**
- Branch from `main` with a descriptive name (`feat/`, `fix/`, `docs/`).
- Commit with an imperative subject and a body explaining *why*, not what.
- Push, open a PR whose body describes the design decisions and the
  trade-offs rejected, and merge it.
- If the change embodies a decision worth keeping, add or update the
  corresponding note in `docs/brain/` and link it from `Home.md`.

Use `--body-file` for PR bodies. Passing multi-line text inline through
PowerShell mangles quotes.
