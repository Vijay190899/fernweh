/* Drives the demo the way a hiring manager would: load every page, click every
 * control, and fail on anything the browser complains about. */
const puppeteer = require("puppeteer-core");
const fs = require("fs");

const BASE = process.env.BASE || "http://localhost:8080";
const CHROME = process.env.CHROME_PATH ||
  (process.platform === "win32" ? "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
   : process.platform === "darwin" ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
   : "/usr/bin/google-chrome");
const OUT = process.env.SHOT_DIR || __dirname + "/shots";
fs.mkdirSync(OUT, { recursive: true });

const problems = [];
const note = (page, kind, detail) => {
  problems.push({ page, kind, detail });
  console.log(`  ! [${kind}] ${detail}`);
};

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function watch(page, label) {
  page.on("pageerror", (e) => note(label, "js-error", e.message));
  page.on("console", (m) => {
    if (m.type() === "error") note(label, "console-error", m.text());
    if (m.type() === "warning" && /deprecat|violation/i.test(m.text()))
      note(label, "console-warn", m.text());
  });
  page.on("requestfailed", (r) => {
    const u = r.url();
    if (u.startsWith("data:")) return;
    note(label, "request-failed", `${u} ${r.failure()?.errorText || ""}`);
  });
  page.on("response", (r) => {
    if (r.status() >= 400) note(label, `http-${r.status()}`, r.url());
  });
}

async function textOf(page, sel) {
  return page.$eval(sel, (e) => e.textContent.trim()).catch(() => null);
}

(async () => {
  const browser = await puppeteer.launch({
    executablePath: CHROME,
    headless: "new",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader", "--use-gl=swiftshader"],
  });

  /* ---------- landing ---------- */
  {
    const page = await browser.newPage();
    await page.setViewport({ width: 1440, height: 900 });
    watch(page, "landing");
    await page.goto(BASE + "/", { waitUntil: "domcontentloaded", timeout: 45000 });
    await sleep(1500);

    const loader = await page.$eval("#loader", (e) =>
      getComputedStyle(e).display + "/" + getComputedStyle(e).opacity).catch(() => "absent");
    if (loader !== "absent" && !/none|\/0/.test(loader))
      note("landing", "stuck-loader", `loader still visible: ${loader}`);

    if (await page.$(".facts")) note("landing", "leftover", "metrics block still on catalogue page");

    const rows = await page.$$eval(".prow, .reveal-on-scroll", (n) => n.length).catch(() => 0);
    if (rows < 3) note("landing", "content", `expected 3 product rows, found ${rows}`);

    // Walk the portal so the door sequence actually runs.
    for (let y = 0; y < 4000; y += 400) {
      await page.evaluate((v) => window.scrollTo(0, v), y);
      await sleep(120);
    }
    await page.screenshot({ path: OUT + "/landing.png", fullPage: false });

    // Every internal link must resolve.
    const links = await page.$$eval("a[href]", (as) =>
      as.map((a) => a.getAttribute("href")).filter((h) => h && !h.startsWith("http") && !h.startsWith("#")));
    for (const href of [...new Set(links)]) {
      const res = await page.evaluate(async (h) => {
        try { const r = await fetch(h, { method: "GET" }); return r.status; } catch (e) { return -1; }
      }, href);
      if (res !== 200) note("landing", "dead-link", `${href} -> ${res}`);
    }
    await page.close();
  }

  /* ---------- search ---------- */
  {
    const page = await browser.newPage();
    await page.setViewport({ width: 1440, height: 900 });
    watch(page, "search");
    await page.goto(BASE + "/search/", { waitUntil: "domcontentloaded", timeout: 45000 });
    await sleep(800);

    if (!(await page.$(".page-hero"))) note("search", "layout", "hero band missing");

    // The search box must be reachable without scrolling.
    const boxTop = await page.$eval("#q", (e) => e.getBoundingClientRect().top);
    if (boxTop > 900) note("search", "layout", `search box below the fold at ${Math.round(boxTop)}px`);

    const chips = await page.$$(".chips .chip");
    if (chips.length < 3) note("search", "content", `only ${chips.length} example chips`);

    for (let i = 0; i < chips.length; i++) {
      const label = await (await page.$$(".chips .chip"))[i].evaluate((e) => e.textContent.trim());
      await (await page.$$(".chips .chip"))[i].click();
      await page.waitForFunction(
        () => document.querySelectorAll("#results .card").length > 0 && document.querySelector("#meta").textContent.trim().length > 0,
        { timeout: 20000 }).catch(() => note("search", "no-results", `chip "${label}" produced nothing`));
      await sleep(600);
      const n = await page.$$eval("#results .card", (e) => e.length).catch(() => 0);
      const meta = (await textOf(page, "#meta")) || "";
      console.log(`    chip "${label}" -> ${n} results | ${meta.replace(/\s+/g, " ").slice(0, 90)}`);
      if (n < 3) note("search", "thin-page", `chip "${label}" returned ${n} results`);
      if (i === 0) await page.screenshot({ path: OUT + "/search-results.png" });
    }

    // Free-text query.
    await page.click("#q", { clickCount: 3 });
    await page.type("#q", "quiet wellness retreat in Austria under 200 a night");
    await page.click("#go");
    await sleep(2500);
    const n2 = await page.$$eval("#results .card", (e) => e.length).catch(() => 0);
    if (n2 < 1) note("search", "no-results", "typed query returned nothing");
    console.log(`    typed query -> ${n2} results`);

    // Session profile panel.
    const cards = await page.$$("#results .card");
    if (cards.length) { await cards[0].click(); await sleep(900); }
    await page.close();
  }

  /* ---------- recommendations ---------- */
  {
    const page = await browser.newPage();
    await page.setViewport({ width: 1440, height: 1100 });
    watch(page, "recommendations");
    await page.goto(BASE + "/recommendations/", { waitUntil: "domcontentloaded", timeout: 45000 });

    await page.waitForSelector("#cmp-personas .chip", { timeout: 20000 })
      .catch(() => note("recommendations", "compare", "persona chips never rendered"));
    await sleep(2500);

    const cold = await page.$$eval("#cmp-cold .cmp-card", (n) => n.length).catch(() => 0);
    const warm = await page.$$eval("#cmp-warm .cmp-card:not(.leaving)", (n) => n.length).catch(() => 0);
    console.log(`    initial compare: cold=${cold} warm=${warm}`);
    if (cold < 5) note("recommendations", "compare", `cold column has ${cold} cards`);
    if (warm < 5) note("recommendations", "compare", `warm column has ${warm} cards`);

    const meta0 = await textOf(page, "#cmp-meta");
    if (/unavailable|undefined|NaN|Error/i.test(meta0 || ""))
      note("recommendations", "compare", `meta line reads: ${meta0}`);
    console.log(`    meta: ${meta0}`);

    // Every persona, clicked in turn, is where the last bug lived.
    const names = await page.$$eval("#cmp-personas .chip", (n) => n.map((e) => e.textContent.trim()));
    for (const name of names) {
      await page.evaluate((t) => {
        [...document.querySelectorAll("#cmp-personas .chip")].find((c) => c.textContent.trim() === t).click();
      }, name);
      await sleep(2400);
      const c = await page.$$eval("#cmp-cold .cmp-card", (n) => n.length).catch(() => 0);
      const w = await page.$$eval("#cmp-warm .cmp-card:not(.leaving)", (n) => n.length).catch(() => 0);
      const m = await textOf(page, "#cmp-meta");
      const title = await textOf(page, "#cmp-warm-title");
      const badges = await page.$$eval("#cmp-warm .cmp-delta", (n) => n.length).catch(() => 0);
      const stray = await page.$$eval("#cmp-warm .cmp-card", (n) =>
        n.filter((e) => /undefined|NaN/.test(e.textContent)).length).catch(() => 0);
      console.log(`    ${name}: cold=${c} warm=${w} badges=${badges} title="${title}"`);
      if (c !== 10 || w !== 10) note("recommendations", "compare", `${name}: cold=${c} warm=${w}, want 10/10`);
      if (badges !== w) note("recommendations", "compare", `${name}: ${badges} badges for ${w} cards`);
      if (stray) note("recommendations", "compare", `${name}: ${stray} cards render undefined/NaN`);
      if (title !== name) note("recommendations", "compare", `${name}: warm title says "${title}"`);
      if (/unavailable|undefined|NaN/i.test(m || "")) note("recommendations", "compare", `${name}: meta "${m}"`);
    }
    await page.screenshot({ path: OUT + "/compare.png" });

    // Custom query through the form.
    await page.click("#cmp-q", { clickCount: 3 });
    await page.type("#cmp-q", "ski chalet in the Alps");
    await page.click("#cmp-form .btn");
    await sleep(2600);
    const c2 = await page.$$eval("#cmp-cold .cmp-card", (n) => n.length).catch(() => 0);
    console.log(`    custom query -> cold=${c2}`);
    if (c2 < 1) note("recommendations", "compare", "custom query rendered nothing");

    // Rapid persona switching: the transition timers must not collide.
    for (const name of names.slice(0, 4)) {
      await page.evaluate((t) => {
        [...document.querySelectorAll("#cmp-personas .chip")].find((c) => c.textContent.trim() === t).click();
      }, name);
      await sleep(250);
    }
    await sleep(3000);
    const cR = await page.$$eval("#cmp-cold .cmp-card", (n) => n.length).catch(() => 0);
    const wR = await page.$$eval("#cmp-warm .cmp-card", (n) => n.length).catch(() => 0);
    console.log(`    after rapid switching: cold=${cR} warm=${wR}`);
    if (cR !== 10 || wR !== 10)
      note("recommendations", "compare", `rapid switching left cold=${cR} warm=${wR}, want 10/10`);

    // Evaluation tiles.
    const tiles = await page.$$eval("#eval-tiles .tile", (n) => n.length).catch(() => 0);
    if (tiles < 6) note("recommendations", "eval", `${tiles} evaluation tiles, expected 6`);
    const evalMeta = await textOf(page, "#eval-meta");
    if (!evalMeta) note("recommendations", "eval", "evaluation meta line empty");

    // Flow diagram source links.
    const srcs = await page.$$eval(".stage .src, .flow a", (a) => a.map((e) => e.href));
    for (const u of srcs) if (!/github\.com/.test(u)) note("recommendations", "link", `odd source link ${u}`);
    await page.close();
  }

  /* ---------- content enrichment ---------- */
  {
    const page = await browser.newPage();
    await page.setViewport({ width: 1440, height: 1100 });
    watch(page, "content");
    await page.goto(BASE + "/content/", { waitUntil: "domcontentloaded", timeout: 45000 });
    await sleep(2500);

    if (!(await page.$(".page-hero"))) note("content", "layout", "hero band missing");

    const before = await textOf(page, "#ops-stats, .tiles");
    console.log(`    ops on arrival: ${(before || "").replace(/\s+/g, " ").slice(0, 120)}`);

    // The two buttons a hiring manager will press.
    for (const sel of ["#reset-demo", "#scan-btn"]) {
      const btn = await page.$(sel);
      if (!btn) { note("content", "missing", `button ${sel} not on page`); continue; }
      const label0 = await btn.evaluate((e) => e.textContent.trim());
      await btn.click();
      await sleep(3500);
      const label1 = await btn.evaluate((e) => e.textContent.trim());
      console.log(`    ${sel}: "${label0}" -> "${label1}"`);
      if (label1 === label0) note("content", "dead-button", `${sel} did not change state`);
      if (/token|unavailable|failed|error/i.test(label1))
        note("content", "dead-button", `${sel} reports: ${label1}`);
    }

    await sleep(6000);
    const audits = await page.$$eval(".audit-row, .rowc, #audit-list > *", (n) => n.length).catch(() => 0);
    console.log(`    before/after rows: ${audits}`);
    if (audits < 1) note("content", "empty", "no before/after audit rows rendered");
    await page.screenshot({ path: OUT + "/content.png" });
    await page.close();
  }

  /* ---------- mobile pass ---------- */
  for (const path of ["/", "/search/", "/recommendations/", "/content/"]) {
    const page = await browser.newPage();
    await page.setViewport({ width: 390, height: 844, isMobile: true });
    watch(page, "mobile" + path);
    await page.goto(BASE + path, { waitUntil: "domcontentloaded", timeout: 45000 });
    await sleep(2000);
    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth - document.documentElement.clientWidth);
    if (overflow > 2) note("mobile" + path, "overflow", `page scrolls ${overflow}px horizontally`);
    await page.close();
  }

  await browser.close();

  console.log("\n================ SUMMARY ================");
  if (!problems.length) {
    console.log("clean: no errors, no dead links, no dead buttons");
  } else {
    const grouped = {};
    problems.forEach((p) => { (grouped[p.page] ||= []).push(p); });
    for (const [pg, list] of Object.entries(grouped)) {
      console.log(`\n${pg} (${list.length})`);
      const seen = new Set();
      list.forEach((p) => {
        const k = p.kind + p.detail;
        if (seen.has(k)) return;
        seen.add(k);
        console.log(`  ${p.kind}: ${p.detail}`);
      });
    }
  }
  process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error("AUDIT CRASHED:", e); process.exit(2); });
