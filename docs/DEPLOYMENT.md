# Fernweh Deployment

**Principle:** local mirrors production exactly. The same `docker-compose.yml`
runs on a laptop and in production; a container platform manages the stack in
production, and a small overlay adds restart policies and memory limits. There
is no environment-specific code path anywhere in the repository.

## Local

```bash
cp .env.example .env          # add OPENROUTER_API_KEY (optional, see below)
docker compose up --build     # postgres, redis, jaeger + 4 services, seeded
# http://localhost:8080  demo    ·  http://localhost:16686  Jaeger
```

Native development loop, with the backing stores still in containers:

```bash
docker compose up -d postgres redis jaeger
make seed && make run-all     # or: go run ./cmd/<service>
```

Without an `OPENROUTER_API_KEY` the platform runs entirely on its
deterministic paths. Nothing breaks; the engine badge in the UI reports
`Rules parsed` instead of `Model parsed`.

## Production

Production runs the **same compose stack** under a container platform, so the
services, PostgreSQL, Redis and Jaeger in production are the ones described in
this repository rather than substitutes.

The platform is [Coolify](https://coolify.io), a self-hosted PaaS that manages
container lifecycle, builds from git, issues TLS through Traefik, and handles
health checks, log streaming and rollbacks. [Dokploy](https://dokploy.com) is
an equivalent alternative with a smaller memory footprint if the host is tight.

To be precise about what this is: the container platform is self-hosted rather
than a vendor-operated control plane. The deployment model is the same, and
the host underneath it is free indefinitely, which is why it was chosen for a
demo that needs to stay reachable for months.

```
Internet
   │  443, TLS issued and renewed by the platform
   ▼
Traefik  (managed by Coolify)
   │
   ├── gateway :8080          the only service with a route
   │      └── search · ranking · enrich   (internal network only)
   │
   └── /jaeger                protected, operators only

PostgreSQL · Redis            internal network only, never published
```

### Without a PaaS

Coolify is one option, not a requirement, and it is a second thing that can
fail during a deployment that is already failing. `deploy/compose.caddy.yml`
does the same job with one container: Caddy in front of the gateway, automatic
Let's Encrypt certificates, nothing to renew.

```bash
docker compose -f docker-compose.yml \
               -f deploy/compose.prod.yml \
               -f deploy/compose.caddy.yml up -d
```

Caddy is then the only process bound to the host. The gateway, both stores and
Jaeger are reachable only on the compose network. `docs/ORACLE.md` is a full
walkthrough for a free Oracle ARM instance using this path, including how to
get real HTTPS without owning a domain.

### Host

Any host with 2 vCPU and 2 GB of RAM upward. The reference target is an
**Oracle Cloud Always Free ARM instance** (VM.Standard.A1.Flex), free
indefinitely. Oracle reduced the free ARM allocation to 2 OCPU and 12 GB in
June 2026, which still leaves comfortable headroom: the platform needs about
1 GB and the full stack about 2 GB more.

Two things to know before choosing this host. Free ARM capacity is genuinely
scarce, and instance creation often returns *"Out of host capacity"*; it is
usually a matter of retrying in a different availability domain over a day or
so. Upgrading the account to pay-as-you-go improves scheduling priority and
does not by itself incur charges while resources stay inside the free
allowances. If that is more friction than it is worth, a small VPS such as
Hetzner CX22 runs this identically for a few euro a month, and nothing in
this document changes.

### Steps

1. Create the instance (Ubuntu 24.04, ARM64) and open 80, 443 and 8000 in the
   cloud firewall. Port 8000 is the Coolify dashboard and can be closed again
   once setup is complete.

2. Install the platform:

   ```bash
   curl -fsSL https://cdn.coollabs.io/coolify/install.sh | sudo bash
   ```

3. Open the dashboard, add this repository as a **Docker Compose** resource,
   and point it at `docker-compose.yml`. The platform builds every service
   from the repository's own Dockerfile; there is no registry step and no
   separate production image.

4. Set the environment variables from `.env.example` in the platform's
   environment editor. At minimum:

   | Variable | Value |
   |---|---|
   | `OPENROUTER_API_KEY` | your key, or empty for deterministic mode |
   | `LLM_DAILY_CALL_BUDGET` | `500` bounds worst-case spend near $0.32/day |
   | `JAEGER_UI_URL` | `https://<domain>/jaeger` so trace links resolve |
   | `BETTERSTACK_LOG_TOKEN` | optional, enables log shipping |
   | `BETTERSTACK_HEARTBEAT_URL` | optional, enables uptime alerting |

5. Assign the domain to the **gateway** service only, on port 8080. Leave
   every other service without a route so nothing else is reachable from the
   internet. Apply basic authentication to the Jaeger route.

6. Deploy. The platform runs `docker compose up`. Every service takes a
   Postgres advisory lock at start-up and whichever wins migrates and seeds
   inventory, so there is no ordering requirement and no one-shot container
   whose exit the others must wait on. That waiting was what made an earlier
   Coolify deployment restart-loop.

Redeploys are a git push followed by the platform's deploy action. Migrations
are forward-only and idempotent, the seeder is a no-op once inventory exists,
and the services hold no local state, so rolling back is redeploying an
earlier commit.

### Production overlay

`deploy/compose.prod.yml` adds restart policies, memory limits, and a smaller
Jaeger trace buffer, and stops publishing host ports for anything except the
gateway. Apply it alongside the base file:

```bash
docker compose -f docker-compose.yml -f deploy/compose.prod.yml up -d
```

Coolify applies additional compose files through its configuration, or the
same command can be run directly on the host.

## Operating a public demo safely

- Per-IP token buckets and a 1 MB body cap at the gateway.
- A platform-wide daily LLM call budget held in Redis. Past the cap every
  service falls back to its deterministic path, so the demo degrades in
  function rather than breaking, and the API balance cannot be drained.
- PostgreSQL and Redis are never published; only the gateway has a route.
- Jaeger's trace buffer is capped and its route requires authentication.
- No secrets in the repository. `.env` is git-ignored and `.env.example`
  documents the variable names with empty values.

## Alternatives considered

A vendor-operated PaaS such as Render was evaluated. It fits the deployment
model, but its free tier sleeps services after fifteen minutes, expires
PostgreSQL after thirty days, and pushes Redis and the database onto external
providers, which would mean carrying TLS and credential plumbing purely to
work around a free tier and running a production stack that no longer matches
the one in this repository. For a demo that must stay warm and intact for
months, a self-hosted platform on free infrastructure was the better trade.
