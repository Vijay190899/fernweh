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

  const N = 3600;                       // particle count
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ---------------- formations ---------------- */

  const globe = new Float32Array(N * 3);
  const converge = new Float32Array(N * 3);
  const rank = new Float32Array(N * 3);
  const grid = new Float32Array(N * 3);
  const seeds = new Float32Array(N * 2);

  // Deterministic pseudo-random so the composition is identical every load.
  let s = 1;
  const rnd = () => (s = (s * 16807) % 2147483647) / 2147483647;

  const GOLD = Math.PI * (3 - Math.sqrt(5));
  for (let i = 0; i < N; i++) {
    const i3 = i * 3;

    // Globe: Fibonacci sphere, slightly flattened for a cinematic profile.
    const y = 1 - (i / (N - 1)) * 2;
    const r = Math.sqrt(Math.max(0, 1 - y * y));
    const th = GOLD * i;
    globe[i3] = Math.cos(th) * r;
    globe[i3 + 1] = y * 0.92;
    globe[i3 + 2] = Math.sin(th) * r;

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

    seeds[i * 2] = rnd();        // per-particle phase
    seeds[i * 2 + 1] = gap;      // 1 when this particle is a content gap
  }

  /* ---------------- travel routes (globe beat) ---------------- */

  const places = {
    berlin: [52.5, 13.4], lisbon: [38.7, -9.1], mallorca: [39.6, 2.9],
    crete: [35.2, 24.9], zermatt: [46.0, 7.7], vienna: [48.2, 16.4],
    azores: [37.7, -25.7], dubrovnik: [42.6, 18.1], paris: [48.9, 2.4],
    madeira: [32.7, -17.0], algarve: [37.1, -8.3],
  };
  const routes = [
    ["berlin", "mallorca"], ["paris", "crete"], ["vienna", "lisbon"],
    ["berlin", "zermatt"], ["lisbon", "azores"], ["paris", "dubrovnik"],
    ["vienna", "madeira"], ["berlin", "algarve"],
  ];
  const toVec = ([lat, lon]) => {
    const la = (lat * Math.PI) / 180, lo = (lon * Math.PI) / 180;
    return [Math.cos(la) * Math.cos(lo), Math.sin(la) * 0.92, Math.cos(la) * Math.sin(lo)];
  };
  const slerp = (a, b, t) => {
    const d = Math.max(-1, Math.min(1, a[0] * b[0] + a[1] * b[1] + a[2] * b[2]));
    const om = Math.acos(d), so = Math.sin(om) || 1e-6;
    const ka = Math.sin((1 - t) * om) / so, kb = Math.sin(t * om) / so;
    return [ka * a[0] + kb * b[0], ka * a[1] + kb * b[1], ka * a[2] + kb * b[2]];
  };

  const SEG = 64;
  const arcPos = [], arcMeta = [];
  routes.forEach(([f, t], idx) => {
    const A = toVec(places[f]), B = toVec(places[t]);
    for (let i = 0; i <= SEG; i++) {
      const u = i / SEG;
      const p = slerp(A, B, u);
      const lift = 1 + Math.sin(u * Math.PI) * 0.19;
      arcPos.push(p[0] * lift, p[1] * lift, p[2] * lift);
      arcMeta.push(u, idx / routes.length);
    }
  });

  /* ---------------- shaders ---------------- */

  const VERT = `
    attribute vec3 aGlobe, aConverge, aRank, aGrid;
    attribute vec2 aSeed;
    uniform mediump float uBeat;     // 0..3 continuous
    uniform mediump float uFill;     // enrichment progress, 0..1
    uniform float uTime, uRot, uScale, uAspect, uDPR, uDrift, uShift;
    varying mediump float vDepth;
    varying mediump vec2 vSeed;
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
      // On the story beats the composition slides into the right half so the
      // left column stays clean for text. Centred only on the hero.
      xy.x += uShift;
      gl_Position = vec4(xy, 0.0, 1.0);

      vDepth = (p.z + 1.4) / 2.8;
      vSeed = aSeed;
      vBeat = uBeat;
      gl_PointSize = mix(1.5, 3.4, clamp(vDepth, 0.0, 1.0)) * uDPR;
    }`;

  const FRAG = `
    precision mediump float;
    uniform vec3 uInk, uRose, uGold;
    uniform mediump float uFill, uOpacity;
    varying mediump float vDepth;
    varying mediump vec2 vSeed;
    varying mediump float vBeat;

    void main() {
      vec2 c = gl_PointCoord - 0.5;
      float d2 = dot(c, c);
      if (d2 > 0.25) discard;
      float soft = smoothstep(0.25, 0.02, d2);

      // Gaps read gold while unresolved, then settle into the base ink.
      float isGap = vSeed.y;
      float gapPhase = smoothstep(2.0, 2.6, vBeat) * isGap * (1.0 - uFill);
      vec3 col = mix(uInk, uGold, gapPhase);

      // A minority of points carry the accent, denser on the ranking beat.
      float accent = step(0.86 - smoothstep(1.0, 2.0, vBeat) * 0.22, vSeed.x);
      col = mix(col, uRose, accent * 0.85);

      // Quieter once the page is reading as an article rather than a hero.
      float quiet = 1.0 - smoothstep(0.1, 1.0, vBeat) * 0.42;
      float alpha = mix(0.10, 0.72, clamp(vDepth, 0.0, 1.0)) * soft * quiet * uOpacity;
      alpha = mix(alpha, alpha * 1.6, gapPhase);
      gl_FragColor = vec4(col, alpha);
    }`;

  const ARC_VERT = `
    attribute vec3 aPos;
    attribute vec2 aMeta;
    uniform float uTime, uRot, uScale, uAspect, uDPR;
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
      gl_Position = vec4(vec2(p.x, p.y * uAspect) * uScale / (d - p.z), 0.0, 1.0);
      vDepth = (p.z + 1.4) / 2.8;
      vMeta = aMeta;
      gl_PointSize = 2.4 * uDPR;
    }`;

  const ARC_FRAG = `
    precision mediump float;
    uniform float uTime;
    uniform mediump float uBeat;
    uniform vec3 uRose, uGold;
    varying mediump float vDepth;
    varying mediump vec2 vMeta;
    void main() {
      vec2 c = gl_PointCoord - 0.5;
      if (dot(c, c) > 0.25) discard;
      // Arcs only exist during the first beat and fade out as it ends.
      float life = 1.0 - smoothstep(0.0, 0.85, uBeat);
      if (life <= 0.001) discard;
      float head = fract(uTime * 0.13 + vMeta.y * 1.7);
      float glow = smoothstep(0.14, 0.0, abs(vMeta.x - head));
      vec3 col = mix(uRose, uGold, vMeta.x);
      float alpha = (0.05 + 0.9 * glow) * mix(0.2, 1.0, vDepth) * life;
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
    "uDPR", "uDrift", "uShift", "uOpacity", "uInk", "uRose", "uGold"]);
  const UA = uni(arcs, ["uTime", "uRot", "uScale", "uAspect", "uDPR", "uBeat",
    "uRose", "uGold"]);

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
    const scale = 1.32 - Math.min(1, beat) * 0.06;

    gl.useProgram(arcs);
    gl.uniform1f(UA.uTime, t);
    gl.uniform1f(UA.uRot, rot);
    gl.uniform1f(UA.uScale, scale);
    gl.uniform1f(UA.uAspect, aspect);
    gl.uniform1f(UA.uDPR, dpr);
    gl.uniform1f(UA.uBeat, beat);
    gl.uniform3fv(UA.uRose, hex(pal.rose));
    gl.uniform3fv(UA.uGold, hex(pal.gold));
    bind(arcs, AA.pos, B.arcPos, 3);
    bind(arcs, AA.meta, B.arcMeta, 2);
    gl.drawArrays(gl.POINTS, 0, arcPos.length / 3);

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
    // Narrow viewports have no room for a side-by-side split, so the shift
    // only applies once there is a left column worth protecting.
    const wide = canvas.clientWidth > 900 ? 1 : 0;
    const s01 = Math.max(0, Math.min(1, (beat - 0.15) / 0.85));
    gl.uniform1f(U.uShift, s01 * s01 * (3 - 2 * s01) * 0.46 * wide);
    gl.uniform3fv(U.uInk, hex(pal.ink));
    gl.uniform3fv(U.uRose, hex(pal.rose));
    gl.uniform3fv(U.uGold, hex(pal.gold));
    bind(pts, A.globe, B.globe, 3);
    bind(pts, A.converge, B.converge, 3);
    bind(pts, A.rank, B.rank, 3);
    bind(pts, A.grid, B.grid, 3);
    bind(pts, A.seed, B.seed, 2);
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
