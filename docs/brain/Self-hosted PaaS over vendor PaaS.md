---
title: Self-hosted PaaS over vendor PaaS
tags: [decision, deployment, platform]
status: accepted
date: 2026-07-31
supersedes: single VM with a hand-rolled reverse proxy
---

# Self-hosted PaaS over vendor PaaS

**Decision.** Production runs the same `docker-compose.yml` under a
self-hosted container platform (Coolify) on an always-free host, rather than
on a vendor-operated PaaS.

## What this replaced

The first plan provisioned a VM, installed Docker over SSH, and ran a
hand-written Caddy reverse proxy. That is infrastructure work. Every other
layer of this project is shaped like a modern container platform, and then
deployment contradicted it. The bootstrap script told a reviewer the opposite
story to the rest of the repository.

## What was rejected, and why

A vendor-operated PaaS fits the deployment model exactly, so it was the
obvious candidate. Its free tier does not survive contact with the
requirement:

| Constraint | Effect |
|---|---|
| Services sleep after 15 minutes idle | A reviewer clicking the link waits about a minute |
| Managed PostgreSQL expires after 30 days | The database dies mid-process during a hiring cycle |
| Database and cache move to external providers | TLS and credential plumbing written purely to work around a free tier |

The last row is the disqualifying one. Adopting it would have meant removing
self-hosted PostgreSQL, Redis and Jaeger from production in order to claim
the production stack matched the repository. Changing the stack to appear to
match the stack.

## What was chosen

Coolify is a self-hosted PaaS with native Docker Compose support: it builds
from git and manages container lifecycle, TLS, health checks, log streaming
and rollbacks. It runs on ARM, and needs about 2 GB against an always-free
host that provides considerably more.

The result: production runs the same compose file, so PostgreSQL, Redis,
Jaeger and the four services in production are the ones described in this
repository. No code changed to get there, and the deployment got smaller,
because the platform owns ingress and certificates and the hand-rolled proxy
was deleted.

## Being precise about the claim

This is a self-hosted control plane, not a vendor-operated one. The
deployment model is the same and the host is free indefinitely, which is why
it suits a demo that must stay reachable for months. Worth stating plainly
rather than letting "PaaS" do ambiguous work.

Related: [[Single module, multiple binaries]], [[One trace across the queue boundary]]
