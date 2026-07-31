---
title: What I would do differently
tags: [process, retrospective]
date: 2026-07-31
---

# What I would do differently

Honest limits of this project, written down rather than waited for in an interview.

**Retrieval will not scale as written.** `ILIKE` with GIN filters is fine at this inventory size and will not hold at a million listings with fuzzy destination matching. Retrieval would move to full-text search or a search engine. The intent and relaxation logic operates on a filter abstraction rather than on SQL, so that change should not reach the product layer, which was the reason for the abstraction.

**The relaxation ladder is sequential.** Worst case is roughly ten queries. At scale the rungs collapse into one query with scoring tiers, or destination and category counts get precomputed so empty rungs are skipped without asking the database.

**Ranking should eventually be learned.** The linear scorer was chosen for explainability, and it is the baseline a learned ranker has to beat while preserving the bound described in [[Business rules are bounded and disclosed]]. The reasons channel would then be fed by feature attribution rather than by if statements.

**No integration test suite in CI.** Unit tests are thorough and the full stack is exercised by hand before every merge, which caught real defects. That verification should be automated against a compose stack in CI rather than depending on the person doing it.

**Multi-tenancy is designed but not built.** The enterprise story needs tenant identifiers, per-tenant business rules and budgets, and single sign-on terminating at the gateway. The architecture notes describe it; nothing implements it.

**Seed data is synthetic.** Deterministic and plausible, but generated. Real supplier feeds are messier in ways that would surface bugs this inventory never will.

Related: [[Bugs the AI wrote]], [[How this was built with Claude Code]]
