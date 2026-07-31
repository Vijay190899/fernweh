---
title: Fernweh Engineering Brain
tags: [moc, index]
updated: 2026-07-31
---

# Fernweh Engineering Brain

An [Obsidian](https://obsidian.md) vault kept alongside the code. Open the
`docs/brain` folder as a vault to get the graph view; on GitHub the wikilinks
read as plain text and every note still stands alone.

Its purpose is to record **why**, because the repository already records what.
Anyone reviewing this project can read a decision, see what was rejected
alongside it, and judge the reasoning rather than guess at it.

## Decisions

Architecture and platform:
- [[Single module, multiple binaries]]
- [[No web framework]]
- [[Interfaces declared at the consumer]]
- [[Self-hosted PaaS over vendor PaaS]]

The AI layer:
- [[The LLM is optional at runtime]]
- [[A daily budget cap with real unit economics]]
- [[Generated content is grounded in listing facts]]

Product behaviour:
- [[Search never returns nothing]]
- [[Business rules are bounded and disclosed]]
- [[Personalization is session scoped]]

Operations:
- [[One trace across the queue boundary]]
- [[Enrichment is idempotent twice over]]
- [[Telemetry must never block a request]]

Interface:
- [[Raw WebGL over a 3D library]]
- [[Listing artwork is generated, not fetched]]

## Process

- [[How this was built with Claude Code]]
- [[Bugs the AI wrote]]
- [[What I would do differently]]

## Reading order for a reviewer

If you have five minutes: [[The LLM is optional at runtime]], then
[[Search never returns nothing]], then [[Bugs the AI wrote]]. Those three
carry most of what this project is arguing.
