/* Fernweh demo UI, vanilla JS, talks to the gateway API only. */
"use strict";

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => [...document.querySelectorAll(sel)];

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

/* ---------- tabs ---------- */
$$(".tab").forEach((t) =>
  t.addEventListener("click", () => {
    $$(".tab").forEach((x) => x.classList.toggle("active", x === t));
    ["search", "ops", "how"].forEach((v) =>
      $("#view-" + v).classList.toggle("hidden", t.dataset.view !== v)
    );
    if (t.dataset.view === "ops") startOps(); else stopOps();
  })
);

/* ---------- config (Jaeger link) ---------- */
let jaegerURL = "http://localhost:16686";
api("/api-config").then(({ data }) => {
  if (data.jaeger_url) {
    jaegerURL = data.jaeger_url;
    $("#jaeger-link").href = jaegerURL;
  }
}).catch(() => {});

/* ================= SEARCH ================= */
const form = $("#search-form");
const input = $("#q");

$("#examples").addEventListener("click", (e) => {
  if (!e.target.classList.contains("chip")) return;
  input.value = e.target.textContent;
  form.requestSubmit();
});

form.addEventListener("submit", async (e) => {
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
  const add = (label, val) => val && pills.push(`<span class="pill">${label} <b>${val}</b></span>`);
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
  const src = { llm: ["ai", "🤖 AI parsed"], fallback: ["rules", "⚙️ rules parsed"], cache: ["cache", "⚡ cached"] }[data.intent_source] || ["rules", data.intent_source];
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

function renderRelaxations(relaxations) {
  const el = $("#relaxations");
  if (!relaxations || !relaxations.length) return;
  el.innerHTML = `🪜 <strong>No exact matches, so we adjusted:</strong> ${relaxations.join(" · ")}`;
  el.classList.remove("hidden");
}

function renderResults(results) {
  const grid = $("#results");
  grid.innerHTML = "";
  results.forEach(({ listing: l, reasons, promo }) => {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      ${promo ? `<span class="ribbon">${promo}</span>` : ""}
      <img src="${l.image_url}" alt="" loading="lazy">
      <div class="body">
        <h3>${l.name}</h3>
        <div class="where">${l.destination} · ${l.country}</div>
        <div class="rowline">
          <span class="stars">★ ${l.rating.toFixed(1)} <span style="color:var(--ink-soft);font-weight:400">(${l.review_count})</span></span>
          <span class="price">€${Math.round(l.price_per_night_cents / 100)}<span> /night</span></span>
        </div>
        <div class="tagrow">${(l.amenities || []).slice(0, 4).map((a) => `<span class="tag">${a}</span>`).join("")}</div>
        ${reasons && reasons.length ? `<div class="whyrow">${reasons.map((r) => `<span class="why">✨ ${r}</span>`).join("")}</div>` : ""}
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
        <div class="aff"><div class="lbl"><span>${c}</span><span>${Math.round(w * 100)}%</span></div>
        <div class="bar"><i style="width:${Math.round(w * 100)}%"></i></div></div>`).join("")}
      ${p.avg_price_cents ? `<div class="pstat"><span>usual price</span><b>€${Math.round(p.avg_price_cents / 100)}/night</b></div>` : ""}
      <div class="pstat"><span>signals recorded</span><b>${p.events}</b></div>`;
  } catch { /* panel is decorative; never break search over it */ }
}

$("#reset-session").addEventListener("click", () => {
  session = newSession();
  refreshProfile();
  $("#profile-body").innerHTML = '<p class="fine dim">Fresh session. The engine forgot you.</p>';
});

/* ================= OPS ================= */
let opsTimer = null;
function startOps() { pollOps(); opsTimer = setInterval(pollOps, 3000); }
function stopOps() { clearInterval(opsTimer); opsTimer = null; }

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
      <div class="t"><span>${l.name} · ${l.destination}</span>
        <span class="status ${l.content_status}">${l.content_status.replace("_", " ")}</span></div>
      <div class="d">${l.description ? l.description.slice(0, 110) + (l.description.length > 110 ? "…" : "") : "<em>no description</em>"}</div>`;
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
          <div class="before">${desc.before || "<em>(empty)</em>"}</div>
          <div class="after">${desc.after}</div>
          <div class="src">source: ${desc.source}${desc.model ? " · " + desc.model : ""}</div>`;
        row.appendChild(d);
      });
    }
    el.appendChild(row);
  });
  if (!listings || !listings.length) {
    el.innerHTML = '<div class="rowc"><div class="d">Nothing here. The pipeline caught up; run a scan or wait for the next sweep.</div></div>';
  }
}

$("#scan-btn").addEventListener("click", async (e) => {
  e.target.disabled = true;
  e.target.textContent = "Scanning…";
  try {
    const { data } = await api("/api/enrich/scan", { method: "POST" });
    e.target.textContent = `Queued ${data.enqueued} listings ✓`;
  } catch {
    e.target.textContent = "Scan failed, retry";
  }
  setTimeout(() => { e.target.disabled = false; e.target.textContent = "Run scan now"; }, 2500);
});

refreshProfile();
