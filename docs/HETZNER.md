# Deploying to Hetzner Cloud

The boring path. Around **€4 a month**, provisions in about thirty seconds, no
capacity lottery, no second firewall to discover. If Oracle's free ARM tier is
refusing to give you an instance, this is the ten-minute version of the same
result.

`docs/ORACLE.md` is the free alternative. Everything from section 3 onward is
identical between them, because the deployment does not know or care which
host it is on.

---

## What it costs

| | |
|---|---|
| CAX11 (2 vCPU ARM, 4 GB, 40 GB) | ~€3.79/month |
| CX22 (2 vCPU x86, 4 GB, 40 GB) | ~€4.35/month |
| IPv4 address | ~€0.50/month |
| Backups | +20%, not needed here |

Billed hourly against a monthly cap, so deleting the server tomorrow costs
pennies. Take the **ARM (CAX11)** shape: cheaper, and every image in this stack
publishes `linux/arm64`.

Check current pricing rather than trusting these numbers; Hetzner adjusts them.

---

## 1. Account

<https://console.hetzner.cloud>

Card or PayPal. New accounts are sometimes asked for **identity verification**,
a photo of an ID document, before the first server can be created. It usually
clears within a few hours. It is the only real friction in this process, and
worth starting before you need the server rather than after.

---

## 2. Server

**New Project** → name it `fernweh` → **Add Server**.

| Field | Value |
|---|---|
| Location | Falkenstein or Nuremberg (cheapest, EU) |
| Image | **Ubuntu 24.04** |
| Type | **Shared vCPU → Arm64 (Ampere)** → **CAX11** |
| Networking | Public IPv4 **on**, IPv6 on |
| SSH keys | Add your public key |
| Firewalls | leave none attached |
| Backups | off |
| Name | `fernweh` |

**Create & Buy now.** It is ready in about half a minute.

Two notes:

**Leave Firewalls unattached.** Hetzner has no firewall by default, and that is
what you want here: nothing to misconfigure, and the only ports reachable are
the ones a process is actually listening on. If you attach one later, it must
allow inbound TCP 22, 80 and 443.

**4 GB is enough**, but only because the setup script adds swap. The running
stack idles under 100 MB; the Go build is the memory peak, and without swap the
OOM killer takes out the compiler on a 4 GB box. That failure looks like a
build that simply stops.

---

## 3. Deploy

Hetzner gives you `root`, not an unprivileged user:

```bash
ssh -i ~/.ssh/your_key root@<server-ip>
curl -fsSL https://raw.githubusercontent.com/Vijay190899/fernweh/main/deploy/setup.sh | bash
```

The script detects the host it is on, so it does not go looking for the
iptables rules and cloud Security List that only Oracle has.

First build takes a few minutes on 2 ARM cores. It is idempotent, so re-running
it is also how you redeploy after a `git push`.

---

## 4. What you get

`https://<dashed-ip>.sslip.io`, with a real Let's Encrypt certificate and no
domain purchased. `sslip.io` resolves any hostname of that shape back to the IP
inside it, which is enough for certificate issuance.

To use a real domain later: point an A record at the server, change
`SITE_ADDRESS` in `~/fernweh/.env`, and restart. Caddy re-issues on its own.

---

## 5. Check it

```bash
curl -sS https://<your-host>/healthz
```

Then point the audit suite at the deployment from your own machine:

```bash
cd tools/audit && npm install
BASE=https://<your-host> node api.js
```

That is the same suite used in development: every endpoint, every input
validation, the search invariants, the ranking comparison invariants, and the
enrichment queue draining to zero.

---

## 6. Optional: the model key

Without it the demo runs entirely on its deterministic paths and the UI badge
reports `Rules parsed` instead of `Model parsed`. Nothing breaks.

```bash
cd ~/fernweh && nano .env      # OPENROUTER_API_KEY=sk-or-v1-...
docker compose -f docker-compose.yml -f deploy/compose.prod.yml \
  -f deploy/compose.caddy.yml up -d
```

`LLM_DAILY_CALL_BUDGET` bounds worst-case spend in Redis across the platform.
At the default of 500 calls a day that is roughly $0.30, and past the cap every
service falls back to its deterministic path rather than failing, so a public
URL cannot drain the balance.

---

## When it does not work

| Symptom | Cause |
|---|---|
| Cannot create a server | Identity verification still pending |
| SSH refused | Wrong key, or using `ubuntu@` instead of `root@` |
| Browser times out | Only possible if you attached a Cloud Firewall. Allow 80 and 443 |
| Certificate error right after deploy | Issuance takes up to a minute |
| Certificate error persisting | Port 80 unreachable; ACME validation needs it |
| Build stops with no error | OOM. Confirm swap exists: `free -h` |

---

## Why this is documented at all

The free Oracle tier is the better answer when it works: permanently free, more
memory, and it never expires. It also hands out capacity by lottery, and in
popular regions that lottery can stay closed for days.

A demo nobody can open is worth nothing, however free the host is. Four euro
converts an indefinite wait into a URL, and the two paths converge completely
after the machine exists.
