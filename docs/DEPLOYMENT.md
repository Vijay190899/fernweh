# Fernweh — Deployment Plan

**Principle:** local mirrors production exactly. Both run the same
`docker compose` topology; production adds TLS, restart policies, and resource
limits. There is no environment-specific code path.

## Local

```bash
cp .env.example .env          # add OPENROUTER_API_KEY (optional — fallback mode works without)
docker compose up --build     # postgres, redis, jaeger + 4 services; seeds on first boot
# open http://localhost:8080  (demo)  ·  http://localhost:16686  (Jaeger)
```

Native dev loop (services on host, backing stores in compose):

```bash
docker compose up -d postgres redis jaeger
make seed && make run-all     # or: go run ./cmd/<service>
```

## Production — single free-tier VM

**Target:** Oracle Cloud Always Free ARM VM (VM.Standard.A1.Flex, up to
4 OCPU / 24 GB — genuinely free, permanent). Fallback options if signup
fails: Railway trial credit, or any ~$4 VPS (Hetzner CX22).

Topology on the VM:

```
Internet → Caddy (:443, automatic HTTPS, basic auth on /jaeger)
             ├── fernweh gateway (:8080)  → search/ranking/enrich (internal network)
             └── /jaeger → Jaeger UI (:16686, password-protected)
Postgres, Redis: internal docker network only — never exposed.
```

Steps (scripted in `deploy/setup-vm.sh`):

1. Create VM (Ubuntu 24.04 ARM), open ports 80/443 in the cloud firewall.
2. `curl -fsSL get.docker.com | sh`, clone repo, `cp .env.example .env`, set
   `OPENROUTER_API_KEY`, `DEMO_DAILY_LLM_BUDGET`, `JAEGER_UI_PASSWORD`.
3. `docker compose -f docker-compose.yml -f deploy/compose.prod.yml up -d`
   — prod overlay adds Caddy, `restart: unless-stopped`, memory limits,
   log rotation, and removes host port exposure for internals.
4. Point DNS (free subdomain via DuckDNS, or a cheap domain) at the VM;
   Caddy provisions TLS automatically.

## Operational safeguards for a public demo

- Gateway per-IP rate limit + global daily LLM budget (Redis counter) — the
  $6 OpenRouter credit cannot be drained; services fall back to deterministic
  mode at the cap.
- Jaeger memory storage capped (`--memory.max-traces`), UI behind basic auth.
- Postgres/Redis unreachable from the internet; no secrets in the repo
  (`.env` only, `.env.example` documents every variable).
- `docker compose ps` health checks on every container; Caddy serves a static
  "demo is restarting" page if the gateway is down.

## Rollback / redeploy

`git pull && docker compose up -d --build` — stateless services, migrations
are forward-only and idempotent, seed is a no-op when data exists. Rollback is
`git checkout <tag>` + same command.
