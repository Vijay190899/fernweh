# Audit harness

Two scripts that drive the running stack the way a visitor would and fail on
anything a visitor would notice. They exist because `go test` cannot see a
broken link, a dead button, a page that scrolls sideways on a phone, or a
request that succeeds while the browser logs it as failed.

Every defect listed below was found by running these, not by reading code.

```
docker compose up -d
cd tools/audit && npm install     # puppeteer-core only, uses system Chrome
node browser.js                   # pages, controls, layout
node api.js                       # endpoints, limits, invariants
```

Both exit non-zero on the first problem, so they drop into CI unchanged.

## browser.js

Loads every page, clicks every control, and reports:

- uncaught exceptions and console errors
- failed requests and any response at 400 or above
- internal links that do not resolve
- buttons whose state never changes when pressed
- horizontal overflow, sampled *throughout* the entrance animation rather than
  after it, at 390, 430, 768 and 1440 wide
- the comparison rendering 10 cards a side for all six personas, with a badge
  on every card, no `undefined` in any of them, and no orphans after rapid
  persona switching

## api.js

Probes the endpoints directly:

- malformed, empty, oversized and hostile input against every route
- the 1 MB body cap, the per-IP rate limit, and the security headers
- one `X-Trace-Id` per response, since the UI builds a Jaeger link out of it
- the search invariants: never empty, coverage stated rather than substituted,
  relaxations disclosed, thousands separators parsed, `romantic` not matching
  Rome
- the comparison invariants: identical cold column across all personas,
  deltas agreeing with both orderings, every placement carrying a counterpart
  rank, repeatable across calls
- enrichment end to end: reset, cooldown, scan, and the queue draining to zero

It paces itself to about 4 requests a second. Unpaced it measures the rate
limiter rather than the application, which is a real thing to know and a
useless thing to assert.

## What it caught

| Defect | Why tests missed it |
| --- | --- |
| Duplicated `X-Trace-Id`, so every trace link was dead | Both values were correct; only the header count was wrong |
| 502 for an oversized body, an unknown path, and an upstream 4xx | Every one returned *a* response, just the wrong assertion about fault |
| Signals logged as `ERR_ABORTED` while succeeding | The write landed; only the browser disagreed |
| Pages draggable sideways on a phone for 800 ms | Only true while an animation was mid-flight |
| The enrichment page spending 172 requests to watch one queue drain | Nothing failed; it was simply close enough to the rate limit to break under a second tab |
| The background dimming itself to grey under `prefers-color-scheme: dark` | The stylesheet has no dark mode, so nothing else responded to the query |

`node_modules` is git-ignored. `npm install` pulls puppeteer-core, which
downloads no browser and drives the Chrome already on the machine.
