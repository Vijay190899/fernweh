/* Fernweh scene director.
 *
 * One persistent WebGL canvas sits behind the whole document. The DOM does
 * not contain the visuals: sections declare which beat they belong to via
 * data-scene, and scroll position drives a continuous timeline that morphs a
 * single particle system between formations. Each formation visualises one
 * service in the platform:
 *
 *   0  globe    inventory distributed across Europe, routes between it
 *   1  converge a natural-language query collapsing scattered supply
 *   2  rank     the same points sorted into a ranked column
 *   3  grid     content gaps filling in, row by row
 *
 * Written against raw WebGL with no libraries, because the whole page ships
 * inside the Go binary under a same-origin CSP and stays under 20KB.
 */
"use strict";

(function () {
  const canvas = document.getElementById("scene");
  if (!canvas) return;

  const gl = canvas.getContext("webgl", {
    alpha: true, antialias: true, powerPreference: "high-performance",
  });
  if (!gl) { document.documentElement.classList.add("no-webgl"); return; }

  const N = 12000;                      // particle count
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ---------------- geography ---------------- */

  // The real coordinates of the seeded destinations. The globe formation is
  // built from these rather than from noise, so the opening frame shows where
  // the inventory actually is instead of an abstract sphere.
  const places = {
    algarve: [37.1, -8.3], mallorca: [39.6, 2.9], crete: [35.2, 24.9],
    santorini: [36.4, 25.4], amalfi: [40.6, 14.6], costabrava: [41.9, 3.2],
    dubrovnik: [42.6, 18.1], canaries: [28.3, -16.6], sardinia: [40.1, 9.0],
    madeira: [32.7, -17.0], cyprus: [35.1, 33.4], berlin: [52.5, 13.4],
    paris: [48.9, 2.4], barcelona: [41.4, 2.2], rome: [41.9, 12.5],
    prague: [50.1, 14.4], vienna: [48.2, 16.4], amsterdam: [52.4, 4.9],
    lisbon: [38.7, -9.1], budapest: [47.5, 19.0], copenhagen: [55.7, 12.6],
    zermatt: [46.0, 7.7], innsbruck: [47.3, 11.4], chamonix: [45.9, 6.9],
    livigno: [46.5, 10.1], badenbaden: [48.8, 8.2], bled: [46.4, 14.1],
    tuscany: [43.4, 11.2], provence: [43.9, 5.8], douro: [41.2, -7.8],
    azores: [37.7, -25.7], fjords: [61.0, 7.0], highlands: [57.1, -4.5],
  };
  const toVec = ([lat, lon]) => {
    const la = (lat * Math.PI) / 180, lo = (lon * Math.PI) / 180;
    return [Math.cos(la) * Math.cos(lo), Math.sin(la) * 0.92, Math.cos(la) * Math.sin(lo)];
  };
  const placeVecs = Object.values(places).map(toVec);

  /* ---------------- formations ---------------- */

  const globe = new Float32Array(N * 3);
  const converge = new Float32Array(N * 3);
  const rank = new Float32Array(N * 3);
  const grid = new Float32Array(N * 3);
  const seeds = new Float32Array(N * 3);

  // Deterministic pseudo-random so the composition is identical every load.
  let s = 1;
  const rnd = () => (s = (s * 16807) % 2147483647) / 2147483647;

  const GOLD = Math.PI * (3 - Math.sqrt(5));
  // The shell has to dominate. Crowding most of the cloud onto European
  // coordinates buries the sphere under one dense patch, which is exactly
  // what made earlier versions read as a blob. Places are accents on a body.
  const CLUSTERED = Math.floor(N * 0.16);

  for (let i = 0; i < N; i++) {
    const i3 = i * 3;

    if (i < CLUSTERED) {
      // Gaussian-ish scatter around a real destination, tighter for most
      // points so each place reads as a dense knot rather than a blur.
      const p = placeVecs[i % placeVecs.length];
      const spread = 0.055 + Math.pow(rnd(), 2.2) * 0.16;
      const a = rnd() * Math.PI * 2;
      const b = Math.acos(2 * rnd() - 1);
      const dx = Math.sin(b) * Math.cos(a) * spread;
      const dy = Math.sin(b) * Math.sin(a) * spread;
      const dz = Math.cos(b) * spread;
      // Renormalise back onto the surface so clusters hug the sphere.
      const x = p[0] + dx, y = p[1] + dy, z = p[2] + dz;
      const len = Math.sqrt(x * x + y * y + z * z) || 1;
      globe[i3] = x / len;
      globe[i3 + 1] = (y / len) * 0.92;
      globe[i3 + 2] = z / len;
    } else {
      const y = 1 - ((i - CLUSTERED) / (N - CLUSTERED - 1)) * 2;
      const r = Math.sqrt(Math.max(0, 1 - y * y));
      const th = GOLD * i;
      globe[i3] = Math.cos(th) * r;
      globe[i3 + 1] = y * 0.92;
      globe[i3 + 2] = Math.sin(th) * r;
    }

    // Converge: a wide scatter that funnels toward a focal point, the shape
    // of many candidates resolving into one answer.
    const a = rnd() * Math.PI * 2;
    const rad = 0.25 + Math.pow(rnd(), 0.6) * 1.9;
    const depth = (rnd() - 0.5) * 2.2;
    const funnel = 1 - Math.min(1, rad / 2.2) * 0.55;
    converge[i3] = Math.cos(a) * rad * funnel;
    converge[i3 + 1] = Math.sin(a) * rad * funnel * 0.62;
    converge[i3 + 2] = depth;

    // Rank: sorted columns. Position in the column encodes score, so the
    // formation reads as a leaderboard seen edge-on.
    const col = i % 12;
    const row = Math.floor(i / 12);
    const rows = Math.ceil(N / 12);
    const score = 1 - row / rows;
    rank[i3] = (col - 5.5) * 0.17 + (rnd() - 0.5) * 0.05;
    rank[i3 + 1] = (score - 0.5) * 2.3;
    rank[i3 + 2] = (rnd() - 0.5) * 0.5 - score * 0.35;

    // Grid: a catalogue. Roughly 40% sit behind the plane as gaps that the
    // enrichment beat pulls forward into place.
    const gc = i % 60;
    const gr = Math.floor(i / 60);
    const grows = Math.ceil(N / 60);
    const gap = rnd() < 0.4 ? 1 : 0;
    grid[i3] = (gc / 59 - 0.5) * 3.4;
    grid[i3 + 1] = (gr / grows - 0.5) * 2.0;
    grid[i3 + 2] = gap ? -0.85 - rnd() * 0.7 : 0;

    seeds[i * 3] = rnd();               // per-particle phase
    seeds[i * 3 + 1] = gap;             // content gap, for the enrichment beat
    // Destination points are marked so they can read as bright places on a
    // dim sphere. Without this the clusters and the sphere look identical
    // and the whole thing reads as a blob.
    seeds[i * 3 + 2] = i < CLUSTERED ? 1 : 0;
  }

  /* ---------------- travel routes (globe beat) ---------------- */

  const routes = [
    ["berlin", "mallorca"], ["paris", "crete"], ["vienna", "lisbon"],
    ["berlin", "zermatt"], ["lisbon", "azores"], ["paris", "dubrovnik"],
    ["vienna", "madeira"], ["berlin", "algarve"], ["copenhagen", "canaries"],
    ["amsterdam", "santorini"], ["prague", "sardinia"], ["barcelona", "highlands"],
  ];
  const slerp = (a, b, t) => {
    const d = Math.max(-1, Math.min(1, a[0] * b[0] + a[1] * b[1] + a[2] * b[2]));
    const om = Math.acos(d), so = Math.sin(om) || 1e-6;
    const ka = Math.sin((1 - t) * om) / so, kb = Math.sin(t * om) / so;
    return [ka * a[0] + kb * b[0], ka * a[1] + kb * b[1], ka * a[2] + kb * b[2]];
  };

  // Arcs are drawn as line strips, one draw call each. Sampling them as
  // separate points, which is what this did first, reads as scattered dust
  // rather than as a route between two places.
  const SEG = 72;
  const arcPos = [], arcMeta = [];
  routes.forEach(([f, t], idx) => {
    const A = toVec(places[f]), B = toVec(places[t]);
    for (let i = 0; i <= SEG; i++) {
      const u = i / SEG;
      const p = slerp(A, B, u);
      const lift = 1 + Math.sin(u * Math.PI) * 0.22;
      arcPos.push(p[0] * lift, p[1] * lift, p[2] * lift);
      arcMeta.push(u, idx / routes.length);
    }
  });
  const ARC_VERTS = SEG + 1;

  /* ---------------- shaders ---------------- */

  const VERT = `
    attribute vec3 aGlobe, aConverge, aRank, aGrid;
    attribute vec3 aSeed;
    uniform mediump float uBeat;     // 0..3 continuous
    uniform mediump float uFill;     // enrichment progress, 0..1
    uniform float uTime, uRot, uScale, uAspect, uDPR, uDrift, uShift, uLift;
    varying mediump float vDepth;
    varying mediump vec3 vSeed;
    varying mediump float vBeat;

    vec3 formation() {
      vec3 p = mix(aGlobe, aConverge, clamp(uBeat, 0.0, 1.0));
      p = mix(p, aRank, clamp(uBeat - 1.0, 0.0, 1.0));
      vec3 g = aGrid;
      g.z = mix(aGrid.z, 0.0, uFill * aSeed.y) ;
      p = mix(p, g, clamp(uBeat - 2.0, 0.0, 1.0));
      return p;
    }

    void main() {
      vec3 p = formation();

      // Idle drift keeps the composition alive without implying motion.
      float ph = aSeed.x * 6.2831;
      p += vec3(sin(uTime * 0.25 + ph), cos(uTime * 0.22 + ph * 1.3),
                sin(uTime * 0.19 + ph * 0.7)) * uDrift;

      // Y-rotation, strongest on the globe beat and easing off after it.
      float spin = uRot * (1.0 - clamp(uBeat, 0.0, 1.0) * 0.82);
      float c = cos(spin), sn = sin(spin);
      p = vec3(p.x * c + p.z * sn, p.y, -p.x * sn + p.z * c);

      // Fixed tilt, then a weak perspective divide.
      float ct = cos(0.38), st = sin(0.38);
      p = vec3(p.x, p.y * ct - p.z * st, p.y * st + p.z * ct);

      float d = 3.05;
      vec2 xy = vec2(p.x, p.y * uAspect) * uScale / (d - p.z);
      // The composition sits right of the text column and rides higher on
      // the hero, so type always has clean space beneath it.
      xy.x += uShift;
      xy.y += uLift;
      gl_Position = vec4(xy, 0.0, 1.0);

      vDepth = (p.z + 1.4) / 2.8;
      vSeed = aSeed;
      vBeat = uBeat;
      // Near points are noticeably larger than far ones. Too narrow a range
      // reads as uniform dust instead of a body with depth.
      float near = pow(clamp(vDepth, 0.0, 1.0), 1.6);
      float place = aSeed.z * (1.0 - smoothstep(0.0, 0.8, uBeat));
      gl_PointSize = mix(1.5, 5.4, near) * (1.0 + place * 0.5) * uDPR;
    }`;

  const FRAG = `
    precision mediump float;
    uniform vec3 uInk, uRose, uGold;
    uniform mediump float uFill, uOpacity;
    varying mediump float vDepth;
    varying mediump vec3 vSeed;
    varying mediump float vBeat;

    void main() {
      vec2 c = gl_PointCoord - 0.5;
      float d2 = dot(c, c);
      if (d2 > 0.25) discard;
      // Tight falloff. A wide one turns every point into a smudge and the
      // whole field reads as haze instead of as discrete marks.
      float soft = smoothstep(0.25, 0.13, d2);

      float depth = clamp(vDepth, 0.0, 1.0);

      // On the globe beat, destinations read as lit places and the rest of
      // the sphere is a dim shell behind them. That contrast is what makes
      // the form legible as a planet rather than a cloud of dust.
      float onGlobe = 1.0 - smoothstep(0.0, 0.8, vBeat);
      float isPlace = vSeed.z;
      float placeLit = isPlace * onGlobe;

      // Gaps read gold while unresolved, then settle into the base ink.
      float gapPhase = smoothstep(2.0, 2.6, vBeat) * vSeed.y * (1.0 - uFill);

      vec3 col = mix(uInk, uRose, placeLit * 0.9);
      col = mix(col, uGold, gapPhase);
      // Away from the globe a minority carry the accent, denser when ranking.
      float accent = step(0.86 - smoothstep(1.0, 2.0, vBeat) * 0.22, vSeed.x);
      col = mix(col, uRose, accent * 0.85 * (1.0 - placeLit));

      float quiet = 1.0 - smoothstep(0.1, 1.0, vBeat) * 0.25;
      float base = mix(0.16, 1.0, pow(depth, 1.4));
      // Shell points dim right down on the globe beat; places brighten.
      float shell = mix(1.0, 0.78, onGlobe * (1.0 - isPlace));
      float alpha = base * soft * quiet * uOpacity * shell;
      alpha = mix(alpha, alpha * 1.7, placeLit);
      alpha = mix(alpha, alpha * 1.6, gapPhase);

      // Recover the camera-facing component (vDepth packs it) and, on the
      // globe beat, cull the far hemisphere. Drawing front and back at equal
      // weight fills the disc evenly, which is why it read as a cloud rather
      // than a body. Points near the silhouette get a rim lift, so the
      // circular edge is what the eye lands on.
      float z = vDepth * 2.8 - 1.4;
      float facing = smoothstep(-0.20, 0.30, z);
      float rim = smoothstep(0.75, 0.02, abs(z));
      alpha *= mix(1.0, mix(0.05, 1.0, facing) + rim * 0.35, onGlobe);

      gl_FragColor = vec4(col, alpha);
    }`;

  const ARC_VERT = `
    attribute vec3 aPos;
    attribute vec2 aMeta;
    uniform float uTime, uRot, uScale, uAspect, uDPR, uShift, uLift;
    uniform mediump float uBeat;
    varying mediump float vDepth;
    varying mediump vec2 vMeta;
    void main() {
      vec3 p = aPos;
      float spin = uRot;
      float c = cos(spin), sn = sin(spin);
      p = vec3(p.x * c + p.z * sn, p.y, -p.x * sn + p.z * c);
      float ct = cos(0.38), st = sin(0.38);
      p = vec3(p.x, p.y * ct - p.z * st, p.y * st + p.z * ct);
      float d = 3.05;
      vec2 xy = vec2(p.x, p.y * uAspect) * uScale / (d - p.z);
      xy.x += uShift;
      gl_Position = vec4(xy, 0.0, 1.0);
      vDepth = (p.z + 1.4) / 2.8;
      vMeta = aMeta;
    }`;

  const ARC_FRAG = `
    precision mediump float;
    uniform float uTime;
    uniform mediump float uBeat;
    uniform vec3 uRose, uGold;
    varying mediump float vDepth;
    varying mediump vec2 vMeta;
    void main() {
      // Arcs belong to the first beat and fade as it ends.
      float life = 1.0 - smoothstep(0.15, 1.0, uBeat);
      if (life <= 0.001) discard;

      // A comet travels the route: a bright head with a trailing tail, over
      // a faint constant line so the path itself stays legible.
      float head = fract(uTime * 0.11 + vMeta.y * 1.7);
      float d = head - vMeta.x;
      if (d < -0.5) d += 1.0;
      if (d > 0.5) d -= 1.0;
      float comet = smoothstep(0.30, 0.0, abs(d)) * step(-0.02, d);
      float spark = smoothstep(0.04, 0.0, abs(d));

      vec3 col = mix(uRose, uGold, comet * 0.8);
      float alpha = (0.14 + 0.75 * comet + 0.5 * spark) * mix(0.35, 1.0, vDepth) * life;
      gl_FragColor = vec4(col, alpha);
    }`;

  function program(vs, fs) {
    const compile = (type, src) => {
      const sh = gl.createShader(type);
      gl.shaderSource(sh, src);
      gl.compileShader(sh);
      if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
        throw new Error(gl.getShaderInfoLog(sh));
      }
      return sh;
    };
    const p = gl.createProgram();
    gl.attachShader(p, compile(gl.VERTEX_SHADER, vs));
    gl.attachShader(p, compile(gl.FRAGMENT_SHADER, fs));
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) {
      throw new Error(gl.getProgramInfoLog(p));
    }
    return p;
  }

  let pts, arcs;
  try {
    pts = program(VERT, FRAG);
    arcs = program(ARC_VERT, ARC_FRAG);
  } catch (e) {
    document.documentElement.classList.add("no-webgl");
    canvas.remove();
    return;
  }

  const buf = (data) => {
    const b = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, b);
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
    return b;
  };
  const B = {
    globe: buf(globe), converge: buf(converge), rank: buf(rank),
    grid: buf(grid), seed: buf(seeds),
    arcPos: buf(new Float32Array(arcPos)), arcMeta: buf(new Float32Array(arcMeta)),
  };

  const uni = (p, names) => {
    const o = {};
    names.forEach((n) => (o[n] = gl.getUniformLocation(p, n)));
    return o;
  };
  const U = uni(pts, ["uBeat", "uFill", "uTime", "uRot", "uScale", "uAspect",
    "uDPR", "uDrift", "uShift", "uLift", "uOpacity", "uInk", "uRose", "uGold"]);
  const UA = uni(arcs, ["uTime", "uRot", "uScale", "uAspect", "uDPR", "uBeat",
    "uShift", "uLift", "uRose", "uGold"]);

  const attr = (p, name) => gl.getAttribLocation(p, name);
  const A = {
    globe: attr(pts, "aGlobe"), converge: attr(pts, "aConverge"),
    rank: attr(pts, "aRank"), grid: attr(pts, "aGrid"), seed: attr(pts, "aSeed"),
  };
  const AA = { pos: attr(arcs, "aPos"), meta: attr(arcs, "aMeta") };

  const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);

  gl.enable(gl.BLEND);
  gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);

  /* ---------------- scroll timeline ---------------- */

  const beats = [...document.querySelectorAll("[data-scene]")];
  let beat = 0, targetBeat = 0, fill = 0, targetFill = 0;

  function readScroll() {
    // The beat index is a continuous value derived from which marker section
    // currently occupies the middle of the viewport, so the morph is driven
    // by reading position rather than by a scroll-distance guess.
    const mid = window.innerHeight * 0.5;
    let value = 0;
    for (let i = 0; i < beats.length; i++) {
      const r = beats[i].getBoundingClientRect();
      if (r.top <= mid) {
        const span = Math.max(1, r.height);
        const progress = Math.min(1, (mid - r.top) / span);
        value = i + progress;
      }
    }
    targetBeat = Math.max(0, Math.min(beats.length - 1, value));

    const ops = document.getElementById("beat-enrich");
    if (ops) {
      const r = ops.getBoundingClientRect();
      targetFill = Math.max(0, Math.min(1,
        (window.innerHeight - r.top) / (window.innerHeight * 0.9)));
    }
  }

  let ticking = false;
  const onScroll = () => {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => { readScroll(); ticking = false; });
  };
  window.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", onScroll, { passive: true });
  readScroll();

  let px = 0, targetPx = 0;
  window.addEventListener("pointermove", (e) => {
    targetPx = (e.clientX / window.innerWidth - 0.5) * 0.3;
  }, { passive: true });

  function resize() {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const w = canvas.clientWidth, h = canvas.clientHeight;
    if (canvas.width !== (w * dpr | 0) || canvas.height !== (h * dpr | 0)) {
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      gl.viewport(0, 0, canvas.width, canvas.height);
    }
    return { aspect: w / h, dpr };
  }

  function bind(p, loc, buffer, size) {
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
    gl.enableVertexAttribArray(loc);
    gl.vertexAttribPointer(loc, size, gl.FLOAT, false, 0, 0);
  }

  const dark = window.matchMedia("(prefers-color-scheme: dark)");
  // Light mode needs darker particles and more of them showing, because a
  // pale mark on cream carries far less than a pale mark on near-black.
  const palette = () => (dark.matches
    ? { ink: "#8d8377", rose: "#e08fa4", gold: "#d8a44b", opacity: 1.0 }
    : { ink: "#6f6659", rose: "#b0536c", gold: "#a2761f", opacity: 1.5 });

  let start = performance.now();

  function frame(now) {
    const { aspect, dpr } = resize();
    const t = (now - start) / 1000;

    // Critically damped follow, so scrolling feels weighted rather than
    // locked to the wheel.
    beat += (targetBeat - beat) * 0.075;
    fill += (targetFill - fill) * 0.06;
    px += (targetPx - px) * 0.04;

    const pal = palette();
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);

    const rot = t * 0.045 + px + 2.4;

    // The globe has to be small enough to read as a body. Filling the
    // viewport with it just produces a dust cloud with no silhouette.
    const wide = canvas.clientWidth > 900 ? 1 : 0;
    const scale = (wide ? 0.62 : 0.52) - Math.min(1, beat) * 0.03;

    // Composition sits right of the text column throughout, so the headline
    // and body copy always have clean paper under them. Shared by both
    // programs so the arcs travel with the globe.
    const s01 = Math.max(0, Math.min(1, (beat - 0.15) / 0.85));
    const settle = s01 * s01 * (3 - 2 * s01);
    const shift = wide ? 0.46 + settle * 0.06 : 0;
    const lift = wide ? 0.30 - settle * 0.30 : 0.34;

    gl.useProgram(arcs);
    gl.uniform1f(UA.uTime, t);
    gl.uniform1f(UA.uRot, rot);
    gl.uniform1f(UA.uScale, scale);
    gl.uniform1f(UA.uAspect, aspect);
    gl.uniform1f(UA.uDPR, dpr);
    gl.uniform1f(UA.uBeat, beat);
    gl.uniform3fv(UA.uRose, hex(pal.rose));
    gl.uniform3fv(UA.uGold, hex(pal.gold));
    gl.uniform1f(UA.uShift, shift);
    gl.uniform1f(UA.uLift, lift);
    bind(arcs, AA.pos, B.arcPos, 3);
    bind(arcs, AA.meta, B.arcMeta, 2);
    // One strip per route. A single call would join the end of each arc to
    // the start of the next with a stray line across the globe.
    for (let r = 0; r < routes.length; r++) {
      gl.drawArrays(gl.LINE_STRIP, r * ARC_VERTS, ARC_VERTS);
    }

    gl.useProgram(pts);
    gl.uniform1f(U.uBeat, beat);
    gl.uniform1f(U.uFill, fill);
    gl.uniform1f(U.uTime, t);
    gl.uniform1f(U.uRot, rot);
    gl.uniform1f(U.uScale, scale);
    gl.uniform1f(U.uAspect, aspect);
    gl.uniform1f(U.uDPR, dpr);
    gl.uniform1f(U.uDrift, reduced ? 0 : 0.012);
    gl.uniform1f(U.uOpacity, pal.opacity);
    gl.uniform1f(U.uShift, shift);
    gl.uniform1f(U.uLift, lift);
    gl.uniform3fv(U.uInk, hex(pal.ink));
    gl.uniform3fv(U.uRose, hex(pal.rose));
    gl.uniform3fv(U.uGold, hex(pal.gold));
    bind(pts, A.globe, B.globe, 3);
    bind(pts, A.converge, B.converge, 3);
    bind(pts, A.rank, B.rank, 3);
    bind(pts, A.grid, B.grid, 3);
    bind(pts, A.seed, B.seed, 3);
    gl.drawArrays(gl.POINTS, 0, N);
  }

  if (reduced) {
    // One settled frame: the composition still communicates, nothing moves.
    beat = targetBeat; fill = targetFill;
    requestAnimationFrame(frame);
    window.addEventListener("scroll", () => {
      readScroll();
      beat = targetBeat; fill = targetFill;
      requestAnimationFrame(frame);
    }, { passive: true });
  } else {
    const loop = (now) => { frame(now); requestAnimationFrame(loop); };
    requestAnimationFrame(loop);
  }

  document.documentElement.classList.add("has-webgl");
})();
