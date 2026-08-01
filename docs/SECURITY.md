# Security notes

What the platform assumes, what it enforces, and what was found and fixed by
reviewing it. Written so a reader can check the claims against the code rather
than take them on trust.

## Trust boundaries

The internet reaches the **gateway** and nothing else. Search, ranking,
enrichment, PostgreSQL, Redis and Jaeger listen only on the container network
and publish no host ports in the production overlay. The gateway is therefore
the only place request-shaped input enters the system.

Model output is treated as **untrusted input**, not as data the platform
produced. It is validated on the way out of the LLM client, and escaped again
on the way into the DOM.

## What is enforced

**No dynamic SQL.** `internal/inventory/repo.go` builds its `WHERE` clause
from hardcoded fragments whose only variable is the parameter index
(`"destination ILIKE $%d"`); every value travels as a pgx parameter. Request
data never reaches SQL text.

**Model output is allowlisted.** `Intent.Normalize` holds `Category`,
`Destination` and `Country` to fixed sets and clamps every numeric field to a
sane range. A model that returns something unexpected, whether through
prompt injection or its own error, yields an empty field rather than an
arbitrary string travelling onward to the browser.

**The browser escapes anyway.** Every value interpolated into `innerHTML` in
`web/app.js` passes through `esc()`. The server allowlist and the client
escaping are deliberately redundant: either alone would do, and neither is
relied on.

**Content Security Policy.** `default-src 'self'`, `script-src 'self'` with no
`unsafe-inline`, `img-src 'self' data:`, `frame-ancestors 'none'`,
`base-uri 'none'`, `form-action 'self'`. Fonts, scripts, styles and imagery
are all first-party, which is why the policy can be this tight.

**One gated write.** `POST /v1/enrich/demo-reset` is the only externally
reachable mutation. It is disabled unless `DEMO_RESET_TOKEN` is set, requires
that value in an `X-Demo-Token` header, compares it with
`subtle.ConstantTimeCompare`, and affects at most 200 rows.

**Edge protections.** Per-IP token buckets, a 1 MB body cap, timeouts on every
hop, and a platform-wide daily LLM budget in Redis so a public URL cannot
drain an API balance.

**No secrets in the repository.** `.env` is git-ignored; `.env.example`
carries names with empty values. The only committed credential is the local
compose Postgres password, which is unreachable from outside the container
network.

## Findings from review, and what changed

**Unauthenticated destructive endpoint (medium, fixed).** `demo-reset` was
reachable by anyone once the demo was public, and could hold the catalogue in
a broken state indefinitely. Now token-gated and off by default. Getting this
wrong on a public URL means somebody else decides what a visitor sees.

**Model-derived values reaching `innerHTML` (fixed).** `Normalize` allowlisted
`Category` but only trimmed `Destination` and `Country`, and those values are
echoed into the coverage notice and intent pills. There was no delivery path,
since the search box never reads from the URL, and the CSP blocks inline
handlers independently. Both mitigations were incidental rather than designed,
though: adding `?q=` deep linking or relaxing the CSP would have made it live.
Fixed at both layers instead of relying on either.

## Known limits

The deterministic parser only recognises European places, so with the model
disabled an out-of-region query extracts no destination and the coverage check
cannot fire. The fallback preserves availability; it is not equivalent.

Personalization is session-scoped in Redis with a seven-day expiry and no
personal data, so there is no account system to attack and no profile to
exfiltrate. A production version with durable, consented profiles would need
authentication and an access-control model that does not exist here.

Jaeger carries no request bodies, but traces do carry query text. In
production it sits behind basic auth on a separate route.
