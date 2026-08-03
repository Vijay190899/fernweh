# Deploying to Oracle Cloud Always Free

A walkthrough for putting this stack on a permanently free Oracle VM, written
around the four things that actually go wrong rather than the happy path.

Two decisions up front, because both are hard to undo:

- **Your home region is permanent.** Changing it means a new account.
- **Whoever's card verifies the account owns the account.** See below.

---

## 0. The card

Oracle asks for a card at signup even though Always Free never charges. It
places a small authorisation hold, around $1, and releases it.

The verification is not just "is this card valid". Oracle compares the
cardholder name and billing address against the account details, and a mismatch
is the single most common reason a signup is rejected outright, or approved and
then terminated days later. Accounts have been closed after the instance was
already running.

So if the card belongs to a friend, the reliable arrangement is **the friend
creates the account in their own name and address**, and then grants access:

1. Friend signs up: their name, their address, their card.
2. Once in, they open **Identity & Security → Domains → Default → Users**,
   create a user for you, and add you to a group.
3. **Identity & Security → Policies**, create a policy allowing that group to
   manage the relevant compartment:

   ```
   Allow group <your-group> to manage all-resources in compartment <name>
   ```

4. You get your own login. The account, the billing relationship, and any
   liability stay with your friend, which is what the card details already say.

Signing up in your own name using someone else's card is the arrangement that
fails Oracle's check. It is not a rule you can talk your way past afterwards,
so it is worth ten minutes to set it up the other way.

If this is more friction than it is worth, **Hetzner CX22 is about €4/month**,
takes a card without argument, has no capacity lottery, and every step from
section 3 onward is identical. Nothing in this repository changes.

---

## 1. Create the account

<https://signup.cloud.oracle.com>

- Pick the **home region** deliberately. Free ARM capacity varies a lot between
  regions, and busier ones are harder to get an instance in. A less popular
  region near you is usually a better bet than the obvious one.
- Choose the **Always Free** path. The trial credits expire; the free tier does
  not.

Signup can take anywhere from minutes to a day to finish provisioning. Until it
does, parts of the console appear but do nothing when clicked, which reads
exactly like a broken UI. If controls are greyed out, the account is still
being set up. Wait rather than debug.

---

## 2. Create the instance

**Compute → Instances → Create instance.**

| Field | Value |
|---|---|
| Image | Ubuntu 24.04 |
| Shape | `VM.Standard.A1.Flex` (Ampere, ARM) |
| OCPUs / memory | Start at 2 OCPU / 12 GB. Ask for less if capacity refuses |
| Boot volume | 50 GB is plenty; the free allowance is 200 GB total |
| SSH keys | Upload your public key, or let Oracle generate and **download it now** |

Everything in this stack has an `arm64` build, and the Dockerfile pins no
architecture, so the ARM shape needs no changes. That was checked, not assumed:
`postgres:17-alpine`, `redis:7-alpine`, `jaegertracing/all-in-one:1.60`,
`golang:1.26-alpine` and `alpine:3.20` all publish `linux/arm64`.

### When it says "Out of host capacity"

It will, probably more than once. Free ARM capacity is genuinely scarce and the
error is not about your account.

- Try a different **availability domain** in the same region.
- Ask for a smaller shape. 1 OCPU / 6 GB is much easier to get than 4 / 24, and
  runs this fine: the whole stack idles under 100 MB of RAM.
- Retry over a day. Capacity frees up unpredictably.
- Upgrading to **pay-as-you-go** improves scheduling priority and does not
  itself cost anything while you stay inside the free allowances. It does mean
  a misconfiguration could bill your friend's card, so decide that with them.

The AMD always-free shapes (`VM.Standard.E2.1.Micro`, 1 GB) are easy to get but
too small to build Go images on. Not worth it here.

---

## 3. Open the firewall. Both of them.

**This is the step that wastes people's afternoons.** Oracle has two
independent firewalls and both block by default. Open one and the instance
answers nothing at all, with no error anywhere to explain why.

**Cloud side.** Networking → Virtual Cloud Networks → your VCN → its subnet →
its Security List → **Add Ingress Rules**:

| Source | Protocol | Destination port |
|---|---|---|
| `0.0.0.0/0` | TCP | 80 |
| `0.0.0.0/0` | TCP | 443 |

**Instance side.** Oracle's Ubuntu images ship `iptables` rules that DROP
everything except SSH. The setup script in the next step handles this, but if
you are doing it by hand:

```bash
sudo iptables -I INPUT 6 -p tcp --dport 80  -m conntrack --ctstate NEW -j ACCEPT
sudo iptables -I INPUT 6 -p tcp --dport 443 -m conntrack --ctstate NEW -j ACCEPT
sudo netfilter-persistent save
```

The `-I INPUT 6` matters. Appending with `-A` puts the rule *after* the
catch-all REJECT that Oracle's chain ends with, so it never matches and the
port stays shut while `iptables -L` shows a rule that looks correct.

---

## 4. Deploy

SSH in and run one command:

```bash
ssh ubuntu@<public-ip>
curl -fsSL https://raw.githubusercontent.com/Vijay190899/fernweh/main/deploy/setup.sh | bash
```

It installs Docker, adds 2 GB of swap, opens the instance firewall, clones the
repository, writes `.env`, builds every service from source, and verifies the
result. It is idempotent, so re-running it is also how you redeploy.

The swap is not incidental. The running stack idles under 100 MB, but the Go
build spikes hard, and on a 6 GB shape the OOM killer takes out the compiler.
That surfaces as a build that simply stops with no error worth reading.

First build takes a few minutes. Afterwards:

```bash
cd ~/fernweh && git pull && sudo docker compose \
  -f docker-compose.yml -f deploy/compose.prod.yml -f deploy/compose.caddy.yml \
  up -d --build
```

---

## 5. HTTPS without owning a domain

You do not need to buy one. The script defaults `SITE_ADDRESS` to
`<dashed-ip>.sslip.io` — a public DNS service that resolves any hostname of
that shape back to the IP inside it. `130-61-1-2.sslip.io` resolves to
`130.61.1.2`, no registration, no account.

That is a real hostname, so Let's Encrypt will issue a real certificate for it,
and Caddy does that automatically on first boot. You get
`https://130-61-1-2.sslip.io` with a valid padlock and nothing to renew.

It is not a pretty URL. If you want one later, point a domain's A record at the
instance, change `SITE_ADDRESS` in `.env`, and restart Caddy — it re-issues
without any other change.

Certificates take up to a minute on first boot. A browser error immediately
after deploy usually means "wait", not "broken".

### Jaeger

The trace UI is proxied at `/jaeger` behind basic auth, because an open trace
UI exposes every query anyone has run against the demo. It rejects everyone
until you set a hash:

```bash
docker run --rm caddy:2-alpine caddy hash-password --plaintext 'your-password'
# put the output in .env as JAEGER_PASSWORD_HASH, then restart caddy
```

---

## 6. Set the model key

Optional. Without it the demo runs entirely on its deterministic paths and the
UI badge reports `Rules parsed` instead of `Model parsed`; nothing breaks.

```bash
cd ~/fernweh
nano .env          # OPENROUTER_API_KEY=sk-or-v1-...
sudo docker compose -f docker-compose.yml -f deploy/compose.prod.yml \
  -f deploy/compose.caddy.yml up -d
```

`LLM_DAILY_CALL_BUDGET` caps spend in Redis across the whole platform. At the
default of 500 calls a day the worst case is roughly $0.30/day, and past the
cap every service falls back to its deterministic path rather than failing, so
a public URL cannot drain the balance.

---

## 7. Check it

```bash
curl -sS https://<your-host>/healthz
curl -sS -X POST https://<your-host>/api/search \
  -H 'content-type: application/json' \
  -d '{"query":"a beach weekend under 1000 in March"}' | head -c 300
```

Then run the audit harness against the deployed URL from your laptop, which is
the same suite used in development:

```bash
cd tools/audit && npm install
BASE=https://<your-host> node api.js
```

---

## What stays free

| | Always Free allowance | This deployment |
|---|---|---|
| Ampere A1 compute | 4 OCPU / 24 GB total | 1–2 OCPU / 6–12 GB |
| Block storage | 200 GB total | ~50 GB |
| Egress | 10 TB/month | negligible |

Nothing here approaches a limit. The one way to get billed is upgrading to
pay-as-you-go and then creating something outside the free allowances, so if
you upgrade for capacity priority, set a **budget alert at $1** in Billing →
Cost Management the same day.

---

## When it does not work

| Symptom | Cause |
|---|---|
| Console controls greyed out | Account still provisioning. Wait |
| "Out of host capacity" | Free ARM scarcity. Different AD, smaller shape, retry |
| SSH works, browser times out | The VCN Security List, nearly always. Then instance `iptables` |
| Certificate error right after deploy | Issuance takes up to a minute |
| Certificate error persisting | Port 80 not reachable; ACME validation needs it |
| Build stops with no error | OOM during the Go build. The script's swap prevents this |
| Signup rejected or account closed | Cardholder details did not match the account. Section 0 |
