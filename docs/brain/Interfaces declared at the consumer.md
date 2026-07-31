---
title: Interfaces declared at the consumer
tags: [decision, architecture, testing]
status: accepted
date: 2026-07-31
---

# Interfaces declared at the consumer

**Decision.** Interfaces are defined where they are used and sized to what that caller needs, not exported from the package that implements them.

`search.Inventory` needs two methods. `enrich.Store` needs four. The concrete `inventory.Repo` has many more and satisfies both without knowing either exists, because Go interfaces are structural.

The payoff is testing without a mocking library. A fake is a struct with the two methods the consumer actually calls, written inline in the test file, and it stays readable because the interface is small by construction. Every degradation path in this project (ranking unavailable, apply fails and the claim is released, a listing is unclaimable) is tested this way against a hand-written fake in a few lines.

This is the single convention most responsible for the test suite existing at all. A wide interface exported from the repository package would have made the same tests tedious enough to skip.

Related: [[Single module, multiple binaries]], [[Bugs the AI wrote]]
