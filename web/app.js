/* Fernweh demo UI, vanilla JS, talks to the gateway API only. */
"use strict";

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => [...document.querySelectorAll(sel)];

/* Everything rendered here originates in the database or in a model reply.
 * The server allowlists what it can, but the browser should not depend on
 * that: anything interpolated into innerHTML goes through esc() so a value
 * that ever slips through is inert markup rather than live HTML. */
const ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
const esc = (v) => String(v ?? "").replace(/[&<>"']/g, (c) => ESCAPES[c]);

/* ---------- session ---------- */
function newSession() {
  const id = "s_" + crypto.randomUUID().slice(0, 18);
  localStorage.setItem("fernweh_session", id);
  return id;
}
let session = localStorage.getItem("fernweh_session") || newSession();

/* ---------- API ---------- */
async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  if (!res.ok) throw new Error(`${path} → ${res.status}`);
  return { data: await res.json(), traceId: res.headers.get("X-Trace-Id") };
}

const signal = (payload) =>
  fetch("/api/signals", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ session_id: session, ...payload }),
  }).catch(() => {});

/* ---------- loader ----------
 * The percentage tracks real readiness: fonts decoded and the platform
 * answering its health check.
 *
 * Progress is driven by elapsed time rather than by frame count, and the
 * dismissal is guaranteed by a timer that does not depend on animation
 * frames at all. A loading curtain that can outlive its own animation loop
 * is worse than no curtain, because it hides a working page. */
(function loader() {
  const el = document.getElementById("loader");
  const pct = document.getElementById("loader-pct");
  if (!el || !pct) return;

  const RATE = 55;        // percent per second while waiting
  let shown = 0, target = 8, settled = false, last = performance.now();
  const bump = (v) => { target = Math.max(target, v); };

  (document.fonts ? document.fonts.ready : Promise.resolve())
    .then(() => bump(60)).catch(() => bump(60));
  fetch("/healthz").then(() => bump(90)).catch(() => bump(90));
  window.addEventListener("load", () => bump(100));

  function dismiss() {
    if (settled) return;
    settled = true;
    pct.textContent = "100";
    el.classList.add("done");
    setTimeout(() => el.remove(), 800);
  }

  // Hard ceiling: the page is revealed after this regardless of what any
  // dependency is doing.
  setTimeout(dismiss, 3500);

  (function tick(now) {
    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    shown = Math.min(target, shown + RATE * dt);
    pct.textContent = Math.round(shown);
    if (shown >= 99.5) { dismiss(); return; }
    if (!settled) requestAnimationFrame(tick);
  })(last);
})();

/* ---------- section highlighting ---------- */
(function navHighlight() {
  const links = $$(".tab");
  if (!links.length || !("IntersectionObserver" in window)) return;
  const byId = new Map(links.map((a) => [a.getAttribute("href").slice(1), a]));
  const obs = new IntersectionObserver((entries) => {
    entries.forEach((e) => {
      const link = byId.get(e.target.id);
      if (link && e.isIntersecting) {
        links.forEach((l) => l.style.removeProperty("color"));
        link.style.color = "var(--ink)";
      }
    });
  }, { rootMargin: "-45% 0px -45% 0px" });
  byId.forEach((_, id) => {
    const sec = document.getElementById(id);
    if (sec) obs.observe(sec);
  });
})();

/* ---------- config (Jaeger link) ---------- */
let jaegerURL = "http://localhost:16686";
api("/api-config").then(({ data }) => {
  if (data.jaeger_url) {
    jaegerURL = data.jaeger_url;
    const link = $("#jaeger-link");
    if (link) link.href = jaegerURL;
  }
}).catch(() => {});

/* ================= SEARCH =================
 * Each page carries only the part of the interface it needs, so every
 * initialiser below is guarded rather than assuming a single document. */
const form = $("#search-form");
const input = $("#q");

$("#examples")?.addEventListener("click", (e) => {
  if (!e.target.classList.contains("chip")) return;
  input.value = e.target.textContent;
  form.requestSubmit();
});

form?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const query = input.value.trim();
  if (!query) return;

  $("#go").disabled = true;
  $("#results-zone").classList.remove("hidden");
  $("#results").innerHTML = '<div class="skeleton"></div><div class="skeleton"></div><div class="skeleton"></div>';
  $("#meta").innerHTML = "";
  $("#relaxations").classList.add("hidden");

  try {
    const { data, traceId } = await api("/api/search", {
      method: "POST",
      body: JSON.stringify({ query, session_id: session }),
    });
    renderMeta(data, traceId);
    renderCoverage(data);
    renderRelaxations(data.relaxations);
    renderResults(data.results);
    // teach the profile what was searched for
    if (data.intent && (data.intent.category || data.intent.budget_max_eur)) {
      signal({
        type: "search",
        category: data.intent.category || "",
        price_cents: (data.intent.budget_max_eur || 0) * 100,
        vibe_tags: data.intent.vibe_tags || [],
        amenities: data.intent.amenities || [],
      }).then(refreshProfile);
    } else {
      refreshProfile();
    }
    document.querySelector("#results-zone").scrollIntoView({ behavior: "smooth", block: "start" });
  } catch (err) {
    $("#results").innerHTML = `<div class="error-note">Search failed (${err.message}). The stack may still be starting, try again in a few seconds.</div>`;
  } finally {
    $("#go").disabled = false;
  }
});

function intentPills(intent) {
  const pills = [];
  const add = (label, val) => val && pills.push(`<span class="pill">${esc(label)} <b>${esc(val)}</b></span>`);
  add("place", intent.destination || intent.country);
  add("type", intent.category);
  add("month", intent.month ? ["", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"][intent.month] : "");
  add("max €/night", intent.budget_max_eur);
  add("nights", intent.nights);
  add("min rating", intent.min_rating);
  (intent.amenities || []).forEach((a) => add("with", a));
  (intent.vibe_tags || []).forEach((v) => add("vibe", v));
  return pills.join("");
}

function renderMeta(data, traceId) {
  // Typographic labels rather than emoji: these are status indicators, and
  // emoji render differently on every platform.
  const src = {
    llm: ["ai", "Model parsed"],
    fallback: ["rules", "Rules parsed"],
    cache: ["cache", "Cached intent"],
  }[data.intent_source] || ["rules", data.intent_source];
  const degraded = (data.degraded || []).length
    ? `<span class="pill" title="${data.degraded.join(", ")}">degraded gracefully</span>` : "";
  const trace = traceId
    ? `<a href="${jaegerURL}/trace/${traceId}" target="_blank" rel="noopener" title="open this exact request in Jaeger">trace ↗</a>` : "";
  $("#meta").innerHTML = `
    <span class="badge ${src[0]}">${src[1]}</span>
    ${intentPills(data.intent || {})}
    <span class="pill">${data.results.length} results · ${data.took_ms} ms</span>
    ${degraded} ${trace}`;
}

/* An out-of-catalogue destination gets said out loud. Quietly widening to the
 * whole world and presenting the result as a near match would be worse than
 * admitting the platform does not go there. */
function renderCoverage(data) {
  const el = $("#coverage");
  if (!el) return;
  if (!data.unsupported) { el.classList.add("hidden"); return; }
  const place = esc(data.unsupported);
  el.innerHTML = `<strong>We have no inventory in ${place}.</strong>
    This catalogue covers Europe only. Nothing below is in ${place};
    these are simply the strongest stays we do have.`;
  el.classList.remove("hidden");
}

function renderRelaxations(relaxations) {
  const el = $("#relaxations");
  if (!relaxations || !relaxations.length) return;
  el.innerHTML = `<strong>No exact match, so these constraints were relaxed:</strong> ${relaxations.join(" · ")}`;
  el.classList.remove("hidden");
}

/* Listing artwork is generated rather than fetched.
 *
 * The seeded inventory has no real photography, and wiring a stock-photo
 * service returns things like a tiger for a beach villa, which reads as
 * broken. Deriving a deterministic abstract landscape from the listing id
 * keeps the art direction consistent, keeps the page inside its
 * same-origin CSP with no external image host, and costs no requests.
 * A production system would swap this for supplier media. */
const SCENERY = {
  beach:       ["#f6d9b0", "#7fc7c4", "#2e6f7e", 62],
  city:        ["#e8d9d2", "#8f7f92", "#3b3550", 20],
  ski:         ["#eef3f7", "#a9c2d6", "#4d6785", 74],
  wellness:    ["#e9efe0", "#a9c39c", "#4a6b55", 55],
  countryside: ["#f4e6c0", "#c2b177", "#6b6a3c", 58],
  adventure:   ["#dbe8e4", "#79a598", "#2f4f4a", 68],
};

function thumb(l) {
  const [sky, mid, land, horizon] = SCENERY[l.category] || SCENERY.countryside;
  // Stable per-listing variation from the id.
  let h = 0;
  for (let i = 0; i < l.id.length; i++) h = (h * 31 + l.id.charCodeAt(i)) >>> 0;
  const sunX = 18 + (h % 64);
  const sunY = horizon - 34 + (h >> 6) % 18;
  const ridge = (h >> 3) % 3;

  const ridges = [
    `M0 ${horizon} L26 ${horizon - 16} L48 ${horizon - 4} L72 ${horizon - 20} L100 ${horizon - 7} L100 100 L0 100 Z`,
    `M0 ${horizon + 3} Q25 ${horizon - 14} 50 ${horizon + 1} T100 ${horizon - 6} L100 100 L0 100 Z`,
    `M0 ${horizon} L20 ${horizon - 22} L38 ${horizon - 6} L60 ${horizon - 26} L80 ${horizon - 8} L100 ${horizon - 18} L100 100 L0 100 Z`,
  ][ridge];

  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" preserveAspectRatio="none">` +
    `<defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1">` +
    `<stop offset="0" stop-color="${sky}"/><stop offset="1" stop-color="${mid}"/>` +
    `</linearGradient></defs>` +
    `<rect width="100" height="100" fill="url(#s)"/>` +
    `<circle cx="${sunX}" cy="${sunY}" r="7" fill="${sky}" opacity=".85"/>` +
    `<path d="${ridges}" fill="${land}" opacity=".92"/>` +
    `</svg>`;

  return `<div class="thumb" role="img" aria-label="Illustration of a ${l.category} destination"
    style="background-image:url('data:image/svg+xml;utf8,${encodeURIComponent(svg)}')"></div>`;
}

function renderResults(results) {
  const grid = $("#results");
  grid.innerHTML = "";
  results.forEach(({ listing: l, reasons, promo }) => {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      ${promo ? `<span class="ribbon">${esc(promo)}</span>` : ""}
      ${thumb(l)}
      <div class="body">
        <h3>${esc(l.name)}</h3>
        <div class="where">${esc(l.destination)} · ${esc(l.country)}</div>
        <div class="rowline">
          <span class="stars">${l.rating.toFixed(1)} <small>(${l.review_count})</small></span>
          <span class="price">€${Math.round(l.price_per_night_cents / 100)}<span> /night</span></span>
        </div>
        <div class="tagrow">${(l.amenities || []).slice(0, 4).map((a) => `<span class="tag">${esc(a)}</span>`).join("")}</div>
        ${reasons && reasons.length ? `<div class="whyrow">${reasons.map((r) => `<span class="why">${esc(r)}</span>`).join("")}</div>` : ""}
        <div class="actions"><button class="book">Book this trip</button></div>
      </div>`;

    const attrs = { listing_id: l.id, category: l.category, price_cents: l.price_per_night_cents, amenities: l.amenities, vibe_tags: l.vibe_tags };
    card.addEventListener("click", () => signal({ type: "click", ...attrs }).then(refreshProfile));
    let dwellTimer;
    card.addEventListener("mouseenter", () => { dwellTimer = setTimeout(() => signal({ type: "dwell", ...attrs }).then(refreshProfile), 2500); });
    card.addEventListener("mouseleave", () => clearTimeout(dwellTimer));
    card.querySelector(".book").addEventListener("click", (e) => {
      e.stopPropagation();
      signal({ type: "book", ...attrs }).then(refreshProfile);
      e.target.textContent = "✓ Noted, the engine learned";
      e.target.classList.add("booked");
    });
    grid.appendChild(card);
  });
}

/* ---------- profile panel ---------- */
async function refreshProfile() {
  try {
    const { data: p } = await api(`/api/profile/${session}`);
    const body = $("#profile-body");
    if (!p.events) {
      body.innerHTML = '<p class="fine dim">No signals yet. This session ranks cold.</p>';
      return;
    }
    const cats = Object.entries(p.category_affinity || {}).sort((a, b) => b[1] - a[1]).slice(0, 4);
    body.innerHTML = `
      ${cats.map(([c, w]) => `
        <div class="aff"><div class="lbl"><span>${esc(c)}</span><span>${Math.round(w * 100)}%</span></div>
        <div class="bar"><i style="width:${Math.round(w * 100)}%"></i></div></div>`).join("")}
      ${p.avg_price_cents ? `<div class="pstat"><span>usual price</span><b>€${Math.round(p.avg_price_cents / 100)}/night</b></div>` : ""}
      <div class="pstat"><span>signals recorded</span><b>${p.events}</b></div>`;
  } catch { /* panel is decorative; never break search over it */ }
}

$("#reset-session")?.addEventListener("click", () => {
  session = newSession();
  refreshProfile();
  $("#profile-body").innerHTML = '<p class="fine dim">Fresh session. The engine forgot you.</p>';
});

/* ---------- cold against warm ----------
 *
 * The evaluation table below says personalization beats the cold baseline, and
 * those numbers are real, but nobody can see a number reorder a page. In live
 * search you cannot see it either: the SQL filters have already narrowed the
 * candidates before the scorer runs, so most of what personalization would
 * have moved was never on the page to move.
 *
 * So this holds the candidate set still and varies only the profile. Both
 * columns render in the cold order first, then the right column FLIPs into its
 * personalized order. The movement is the explanation. */
(function comparison() {
  const cmp = $("#cmp");
  if (!cmp) return;

  const coldList = $("#cmp-cold"), warmList = $("#cmp-warm");
  const chips = $("#cmp-personas"), form = $("#cmp-form"), input = $("#cmp-q");
  const meta = $("#cmp-meta"), profileLine = $("#cmp-profile");
  const title = $("#cmp-warm-title");
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  let chosen = null, running = false;

  const euros = (cents) => "€" + Math.round(cents / 100);

  function card(r, side) {
    const l = r.listing;
    const delta = side === "warm" && r.delta
      ? `<span class="cmp-delta ${r.delta > 0 ? "up" : "down"}">${r.delta > 0 ? "▲" : "▼"}${Math.abs(r.delta)}</span>`
      : "";
    const why = side === "warm" && r.reasons?.length
      ? `<p class="cmp-why">${r.reasons.map(esc).join(" · ")}</p>` : "";
    return `<li class="cmp-card" data-id="${esc(l.id)}">
      <span class="cmp-rank">${r.rank}</span>
      <div class="cmp-body">
        <strong>${esc(l.name)}</strong>
        <p class="cmp-facts">${esc(l.category)} · ${esc(l.destination)} · ${euros(l.price_per_night_cents)}/night · ${l.rating.toFixed(1)}★</p>
        ${why}
      </div>
      ${delta}
    </li>`;
  }

  /* FLIP: measure where every card is, reorder the DOM, then play each card
   * back from where it used to be. Transforms only, so a list of ten animates
   * without touching layout once. */
  function flip(list, render) {
    const before = new Map();
    [...list.children].forEach((c) => before.set(c.dataset.id, c.getBoundingClientRect().top));

    render();

    if (reduced) return;
    [...list.children].forEach((c) => {
      const was = before.get(c.dataset.id);
      if (was === undefined) return;
      const dy = was - c.getBoundingClientRect().top;
      if (!dy) return;
      c.style.transition = "none";
      c.style.transform = `translateY(${dy}px)`;
      c.classList.add("moving");
    });
    requestAnimationFrame(() => {
      [...list.children].forEach((c) => {
        c.style.transition = "transform .85s cubic-bezier(.2,.85,.25,1)";
        c.style.transform = "";
      });
    });
    setTimeout(() => {
      [...list.children].forEach((c) => {
        c.style.transition = c.style.transform = "";
        c.classList.remove("moving");
      });
    }, 900);
  }

  function describe(p) {
    const band = `${euros(p.price_min_cents)}–${euros(p.price_max_cents)}`;
    return `Declared profile: ${esc(p.category)} · ${p.amenities.map(esc).join(", ")} · ` +
           `${p.vibes.map(esc).join(", ")} · ${band}/night`;
  }

  async function run() {
    if (running || !chosen) return;
    running = true;
    cmp.classList.add("busy");
    meta.textContent = "Ranking the same candidates twice…";
    try {
      const { data } = await api("/api/compare", {
        method: "POST",
        body: JSON.stringify({ query: input.value.trim(), persona: chosen }),
      });
      if (!data.cold.length) {
        meta.textContent = "That query matched nothing to compare. Try a broader one.";
        return;
      }

      title.textContent = data.persona.name;
      profileLine.innerHTML = describe(data.persona);
      coldList.innerHTML = data.cold.map((r) => card(r, "cold")).join("");

      // The warm column opens in the cold order so there is a shared starting
      // point, then moves. Rendering it already sorted would show a different
      // list rather than the same list rearranged.
      const warmByID = new Map(data.warm.map((r) => [r.listing.id, r]));
      warmList.innerHTML = data.cold
        .map((r) => card({ ...warmByID.get(r.listing.id), rank: r.rank, delta: 0, reasons: [] }, "warm"))
        .join("");

      setTimeout(() => {
        flip(warmList, () => {
          warmList.innerHTML = data.warm.map((r) => card(r, "warm")).join("");
        });
      }, reduced ? 0 : 420);

      const pct = data.compared ? Math.round((data.moved / data.compared) * 100) : 0;
      meta.textContent =
        `${data.moved} of ${data.compared} candidates changed position (${pct}%), ` +
        `showing the top ${data.cold.length} of each. Ranked in ${data.took_ms} ms.` +
        (data.relaxations?.length ? ` Ladder applied: ${data.relaxations.join("; ")}` : "");
    } catch (err) {
      meta.textContent = "Comparison unavailable: " + err.message;
    } finally {
      running = false;
      cmp.classList.remove("busy");
    }
  }

  api("/api/compare/personas").then(({ data }) => {
    chips.innerHTML = data.personas
      .map((p, i) => `<button type="button" class="chip${i ? "" : " on"}" data-persona="${esc(p.name)}">${esc(p.name)}</button>`)
      .join("");
    chosen = data.personas[0]?.name || null;
    run();
  }).catch(() => {
    chips.innerHTML = '<p class="fine">Persona list unavailable. Is the ranking service up?</p>';
  });

  chips.addEventListener("click", (e) => {
    const b = e.target.closest("[data-persona]");
    if (!b) return;
    $$("#cmp-personas .chip").forEach((c) => c.classList.toggle("on", c === b));
    chosen = b.dataset.persona;
    run();
  });

  form.addEventListener("submit", (e) => { e.preventDefault(); run(); });
})();

/* ---------- offline evaluation numbers ----------
 * Read from the report committed by tools/rankeval, so the page can never
 * show a figure that is not reproducible from the repository. */
(function evaluation() {
  const tiles = $("#eval-tiles");
  if (!tiles) return;

  fetch("/eval.json").then((r) => r.json()).then((d) => {
    const p = d.personalized, b = d.baseline;
    const lift = (a, c) => (c > 0 ? (a / c).toFixed(1) + "x" : "n/a");
    const row = (label, val, sub) =>
      `<div class="tile"><div class="n">${val}</div><div class="l">${label}</div>
       <p class="fine" style="margin:8px 0 0">${sub}</p></div>`;

    tiles.innerHTML =
      row("NDCG@10", p.ndcg_at_10.toFixed(3), `baseline ${b.ndcg_at_10.toFixed(3)} · ${lift(p.ndcg_at_10, b.ndcg_at_10)}`) +
      row("Precision@10", p.precision_at_10.toFixed(3), `baseline ${b.precision_at_10.toFixed(3)} · ${lift(p.precision_at_10, b.precision_at_10)}`) +
      row("Recall@10", p.recall_at_10.toFixed(3), `baseline ${b.recall_at_10.toFixed(3)} · ${lift(p.recall_at_10, b.recall_at_10)}`) +
      row("MAP@10", p.map.toFixed(3), `baseline ${b.map.toFixed(3)} · ${lift(p.map, b.map)}`) +
      row("Coverage", Math.round(p.catalogue_coverage * 100) + "%", `of a ${Math.round((d.personas * 10 / d.catalogue_size) * 100)}% ceiling`) +
      row("Diversity@10", p.diversity_at_10.toFixed(2), `baseline ${b.diversity_at_10.toFixed(2)} · by design`);

    const meta = $("#eval-meta");
    if (meta) {
      meta.textContent = `Computed over ${d.catalogue_size} listings and ${d.personas} declared personas` +
        (d.generated_at ? `, generated ${d.generated_at.slice(0, 10)}` : "") +
        `. Regenerate with: go run ./tools/rankeval`;
    }
  }).catch(() => {
    tiles.innerHTML = '<p class="fine">Evaluation report unavailable. Run <code>go run ./tools/rankeval</code> to generate it.</p>';
  });
})();

/* ---------- real before and after ----------
 * Pulled from the audit trail of a listing the pipeline actually repaired,
 * so the comparison is evidence rather than an illustration. */
(function comparison() {
  if (!$("#compare")) return;

  const chips = (csv) => {
    let list = [];
    try { list = JSON.parse(csv); } catch { list = String(csv || "").split(",").filter(Boolean); }
    if (!list.length) return '<span class="fine">None listed</span>';
    return list.map((a) => `<span class="amenity">${esc(a)}</span>`).join("");
  };

  api("/api/enrich/listings?status=enriched&limit=8").then(async ({ data }) => {
    const listings = data.listings || [];
    for (const l of listings) {
      const { data: a } = await api(`/api/enrich/listings/${l.id}/audit`);
      const desc = (a.audit || []).find((e) => e.field === "description");
      const amen = (a.audit || []).find((e) => e.field === "amenities");
      // Prefer an example whose description genuinely started empty.
      if (!desc || (desc.before || "").trim().length > 40) continue;

      $("#before-id").textContent = `${l.name} · ${l.destination}, ${l.country}`;
      $("#before-desc").innerHTML = (desc.before || "").trim()
        ? esc(desc.before) : '<em>Empty. Nothing supplied by the feed.</em>';
      $("#before-amen").innerHTML = amen ? chips(amen.before) : '<span class="fine">None listed</span>';
      $("#after-desc").textContent = desc.after;
      $("#after-amen").innerHTML = amen ? chips(amen.after) : chips(JSON.stringify(l.amenities));
      $("#after-src").textContent = desc.source === "ai"
        ? `Written by ${desc.model || "the model"}` : "Written by the template path, no model used";
      $("#compare-note").textContent =
        `Listing ${l.id}, read live from the enrichment audit trail. Not an illustration.`;
      return;
    }
    $("#compare-note").textContent =
      "No repaired listing with an empty starting description is in the catalogue right now. Press “Break some listings again”, run a scan, then reload.";
  }).catch(() => {
    $("#compare-note").textContent = "Audit trail unavailable; is the stack running?";
  });
})();

/* ================= OPS =================
 * The dashboard now lives inline on the page rather than behind a tab, so
 * polling starts when it scrolls into view and stops when it leaves. There is
 * no reason to hold a 3-second timer open against a section nobody is
 * looking at. */
let opsTimer = null;
function startOps() { if (opsTimer) return; pollOps(); opsTimer = setInterval(pollOps, 3000); }
function stopOps() { clearInterval(opsTimer); opsTimer = null; }

(function opsVisibility() {
  const tiles = $("#stat-tiles");
  if (!tiles) return;
  if (!("IntersectionObserver" in window)) { startOps(); return; }
  new IntersectionObserver((entries) => {
    entries.forEach((e) => (e.isIntersecting ? startOps() : stopOps()));
  }, { rootMargin: "200px 0px" }).observe(tiles);
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) stopOps();
  });
})();

async function pollOps() {
  try {
    const { data: s } = await api("/api/enrich/stats");
    const inv = s.inventory || {};
    const q = s.queue || {};
    $("#stat-tiles").innerHTML = `
      <div class="tile"><div class="n">${s.total}</div><div class="l">listings</div></div>
      <div class="tile good"><div class="n">${Math.round((s.completeness || 0) * 100)}%</div><div class="l">content complete</div></div>
      <div class="tile hot"><div class="n">${inv.needs_enrichment || 0}</div><div class="l">needs enrichment</div></div>
      <div class="tile"><div class="n">${(q.pending || 0) + (q.active || 0)}</div><div class="l">in queue</div></div>
      <div class="tile good"><div class="n">${inv.enriched || 0}</div><div class="l">enriched by AI</div></div>
      <div class="tile"><div class="n">${q.failed_total || 0}</div><div class="l">failures (retried)</div></div>`;

    const [gaps, done] = await Promise.all([
      api("/api/enrich/listings?status=needs_enrichment&limit=8"),
      api("/api/enrich/listings?status=enriched&limit=8"),
    ]);
    $("#gap-count").textContent = `(${inv.needs_enrichment || 0})`;
    renderOpsRows($("#gap-list"), gaps.data.listings, false);
    renderOpsRows($("#enriched-list"), done.data.listings, true);
  } catch { /* stack warming up */ }
}

function renderOpsRows(el, listings, withAudit) {
  el.innerHTML = "";
  (listings || []).forEach((l) => {
    const row = document.createElement("div");
    row.className = "rowc";
    row.innerHTML = `
      <div class="t"><span>${esc(l.name)} · ${esc(l.destination)}</span>
        <span class="status ${esc(l.content_status)}">${esc(l.content_status.replace("_", " "))}</span></div>
      <div class="d">${l.description ? esc(l.description.slice(0, 110)) + (l.description.length > 110 ? "…" : "") : "<em>no description</em>"}</div>`;
    if (withAudit) {
      row.title = "Click to see before/after";
      row.addEventListener("click", async () => {
        if (row.querySelector(".diff")) { row.querySelector(".diff").remove(); return; }
        const { data } = await api(`/api/enrich/listings/${l.id}/audit`);
        const desc = (data.audit || []).find((a) => a.field === "description");
        if (!desc) return;
        const d = document.createElement("div");
        d.className = "diff";
        d.innerHTML = `
          <div class="before">${desc.before ? esc(desc.before) : "<em>(empty)</em>"}</div>
          <div class="after">${esc(desc.after)}</div>
          <div class="src">source: ${esc(desc.source)}${desc.model ? " · " + esc(desc.model) : ""}</div>`;
        row.appendChild(d);
      });
    }
    el.appendChild(row);
  });
  if (!listings || !listings.length) {
    el.innerHTML = '<div class="rowc"><div class="d">Nothing here. The pipeline caught up; run a scan or wait for the next sweep.</div></div>';
  }
}

/* One click, no token. Breaking the catalogue then immediately scanning it is
 * the whole demonstration, so the two actions are chained: a visitor should
 * not have to work out that they need to press a second button. */
$("#reset-demo")?.addEventListener("click", async (e) => {
  const btn = e.currentTarget;
  const original = btn.textContent;
  btn.disabled = true;
  btn.textContent = "Breaking listings…";
  try {
    const res = await fetch("/api/enrich/demo-reset", { method: "POST" });
    if (res.status === 429) {
      btn.textContent = "Still draining, try shortly";
    } else if (!res.ok) {
      btn.textContent = "Failed, retry";
    } else {
      const data = await res.json();
      btn.textContent = `${data.reset} broken, queueing…`;
      pollOps();
      // Queue them straight away so the tiles start moving without a second
      // click, then follow the drain closely for a while.
      await fetch("/api/enrich/scan", { method: "POST" }).catch(() => {});
      btn.textContent = `${data.reset} queued, watch them repair`;
      followDrain();
    }
  } catch {
    btn.textContent = "Failed, retry";
  }
  setTimeout(() => { btn.disabled = false; btn.textContent = original; }, 4000);
});

/* Poll faster than the idle interval while a batch is draining, so the
 * numbers visibly move instead of stepping every few seconds. */
function followDrain() {
  let ticks = 0;
  const fast = setInterval(() => {
    pollOps();
    if (++ticks > 40) clearInterval(fast);
  }, 900);
}

$("#scan-btn")?.addEventListener("click", async (e) => {
  e.target.disabled = true;
  e.target.textContent = "Scanning…";
  try {
    const { data } = await api("/api/enrich/scan", { method: "POST" });
    // Saying "queued 0" reads as a broken button. It is not: there is simply
    // nothing outstanding, and the honest response points at what to press.
    e.target.textContent = data.enqueued > 0
      ? `Queued ${data.enqueued} listings`
      : "Nothing outstanding, break some first";
    if (data.enqueued > 0) followDrain();
  } catch {
    e.target.textContent = "Scan failed, retry";
  }
  setTimeout(() => { e.target.disabled = false; e.target.textContent = "Run scan now"; }, 2500);
});

refreshProfile();
