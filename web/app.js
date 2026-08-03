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

/* Fire and forget, but the response still has to be drained. Signals answer
 * 202 with an empty body; dropping the Response without reading it makes
 * Chrome tear the request down and log ERR_ABORTED, so a write that succeeded
 * shows up red in devtools. The write was always landing, but anyone opening
 * the console while trying the demo would reasonably conclude otherwise. */
const signal = (payload) =>
  fetch("/api/signals", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ session_id: session, ...payload }),
    keepalive: true,
  }).then((r) => r.arrayBuffer()).catch(() => {});

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
 * So this holds the candidate set still and varies only the profile, and shows
 * the right column becoming personalized: it opens as a copy of the cold page,
 * then listings that lost their place slide out, survivors move to their new
 * ranks, and listings promoted from deeper in the candidate set drop in.
 *
 * The two columns are windows onto thirty candidates, not the whole ranking,
 * so they hold different listings. That is the reason each card carries where
 * it came from: an arrival at #2 that was #23 is the point, and a column that
 * silently swapped its contents would not show it. */
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

  function card(r, side, opts = {}) {
    const l = r.listing;
    const li = document.createElement("li");
    li.className = "cmp-card";
    li.dataset.id = l.id;

    let badge = "";
    if (side === "warm" && !opts.plain) {
      if (r.was > 10) {
        badge = `<span class="cmp-delta up">▲${r.delta}<em>from #${r.was}</em></span>`;
      } else if (r.delta > 0) {
        badge = `<span class="cmp-delta up">▲${r.delta}</span>`;
      } else if (r.delta < 0) {
        badge = `<span class="cmp-delta down">▼${Math.abs(r.delta)}</span>`;
      } else {
        badge = `<span class="cmp-delta flat">held</span>`;
      }
    }
    const why = side === "warm" && !opts.plain && r.reasons?.length
      ? `<p class="cmp-why">${r.reasons.map(esc).join(" · ")}</p>` : "";

    li.innerHTML =
      `<span class="cmp-rank">${opts.plain ? opts.rank : r.rank}</span>
       <div class="cmp-body">
         <strong>${esc(l.name)}</strong>
         <p class="cmp-facts">${esc(l.category)} · ${esc(l.destination)} · ${euros(l.price_per_night_cents)}/night · ${l.rating.toFixed(1)}★</p>
         ${why}
       </div>
       ${badge}`;
    return li;
  }

  /* The transition, in one beat.
   *
   * Departing cards are lifted out of flow first, pinned at the position they
   * already occupied, so the rest of the column can be measured and rebuilt
   * without their heights moving the numbers. Then survivors play back from
   * where they were, and arrivals fade up in place. */
  function transition(list, targets) {
    if (reduced) {
      list.replaceChildren(...targets.map((r) => card(r, "warm")));
      return;
    }

    // Measure everything before anything moves. Pinning one card out of flow
    // shifts the ones below it, so reading offsetTop mid-loop reads positions
    // that have already changed.
    const before = [...list.children].map((n) => ({
      node: n, id: n.dataset.id, top: n.offsetTop, height: n.offsetHeight,
    }));
    const keep = new Set(targets.map((r) => r.listing.id));

    list.style.position = "relative";
    const leavers = [];
    before.forEach((m) => {
      if (keep.has(m.id)) return;
      m.node.style.cssText =
        `position:absolute;left:0;right:0;top:${m.top}px;height:${m.height}px;`;
      leavers.push(m.node);
    });

    // Swap the flow contents, keeping the pinned departures on top of it.
    before.forEach((m) => { if (keep.has(m.id)) m.node.remove(); });
    const arrived = targets.map((r) => card(r, "warm"));
    list.prepend(...arrived);

    const firstTop = new Map(before.map((m) => [m.id, m.top]));
    const moving = [];
    arrived.forEach((n) => {
      const was = firstTop.get(n.dataset.id);
      if (was === undefined) { n.classList.add("arriving"); return; }
      const dy = was - n.offsetTop;
      if (!dy) return;
      n.style.transform = `translateY(${dy}px)`;
      n.classList.add("moving");
      moving.push(n);
    });

    // One forced reflow so the pinned and translated start states are real
    // before the transitions are attached, otherwise the browser is free to
    // coalesce both states and animate nothing.
    void list.offsetHeight;
    requestAnimationFrame(() => {
      leavers.forEach((n) => n.classList.add("leaving"));
      moving.forEach((n) => {
        n.style.transition = "transform .8s cubic-bezier(.2,.85,.25,1)";
        n.style.transform = "";
      });
    });

    setTimeout(() => {
      leavers.forEach((n) => n.remove());
      moving.forEach((n) => {
        n.style.transition = n.style.transform = "";
        n.classList.remove("moving");
      });
      list.style.position = "";
    }, 900);
  }

  function describe(p) {
    const band = `${euros(p.price_min_cents)}–${euros(p.price_max_cents)}`;
    return `Declared profile: ${esc(p.category)} · ${p.amenities.map(esc).join(", ")} · ` +
           `${p.vibes.map(esc).join(", ")} · ${band}/night`;
  }

  let pending = null;

  async function run() {
    if (running || !chosen) return;
    running = true;
    clearTimeout(pending);
    cmp.classList.add("busy");
    meta.textContent = "Ranking the same candidates twice…";
    try {
      const { data } = await api("/api/compare", {
        method: "POST",
        body: JSON.stringify({ query: input.value.trim(), persona: chosen }),
      });
      if (!data.cold.length) {
        meta.textContent = "That query matched no inventory to compare. Try a broader one.";
        return;
      }

      title.textContent = data.persona.name;
      profileLine.textContent = describe(data.persona);
      coldList.replaceChildren(...data.cold.map((r) => card(r, "cold")));

      // The warm column opens as a copy of the cold page, so there is a shared
      // starting point to move away from. Rendering it already personalized
      // would show two lists rather than one becoming the other.
      warmList.replaceChildren(
        ...data.cold.map((r, i) => card(r, "warm", { plain: true, rank: i + 1 })));

      pending = setTimeout(() => transition(warmList, data.warm), reduced ? 0 : 500);

      const arrivals = data.warm.filter((r) => r.was > data.cold.length).length;
      const pct = data.compared ? Math.round((data.moved / data.compared) * 100) : 0;
      meta.textContent =
        `${data.moved} of ${data.compared} candidates changed position (${pct}%). ` +
        (arrivals
          ? `${arrivals} of the personalized top ${data.warm.length} were not on the cold page at all. `
          : "") +
        `Both rankings computed in ${data.took_ms} ms.` +
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

    // Samples ride along with the counters. Fetching them separately meant
    // three round trips for every tick of a poll that runs for half a minute.
    const samples = s.samples || {};
    $("#gap-count").textContent = `(${inv.needs_enrichment || 0})`;
    renderOpsRows($("#gap-list"), samples.needs_enrichment, false);
    renderOpsRows($("#enriched-list"), samples.enriched, true);
    return (q.pending || 0) + (q.active || 0);
  } catch { /* stack warming up */ }
  return 0;
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

/* Poll faster than the idle interval while a batch is draining, so the numbers
 * visibly move instead of stepping every few seconds.
 *
 * The idle timer is stopped for the duration rather than left running
 * alongside, and the fast poll stops as soon as the queue is empty. Two timers
 * racing on the same endpoint spent most of a per-IP rate limit on watching
 * one thing finish, and would have started answering the visitor 429 if they
 * so much as opened a second tab. */
let draining = null;
function followDrain() {
  if (draining) return;
  stopOps();
  let ticks = 0, empty = 0;
  draining = setInterval(async () => {
    const inFlight = await pollOps();
    // One empty reading can land between a queue draining and the next batch
    // being enqueued, so wait for two before declaring it finished.
    if (inFlight === 0 && ++empty >= 2) ticks = 999;
    else if (inFlight > 0) empty = 0;
    if (++ticks > 30) {
      clearInterval(draining);
      draining = null;
      startOps();
    }
  }, 1200);
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
