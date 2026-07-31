/* Fernweh hero globe. Hand-written WebGL, no libraries: a point-cloud globe
   with animated great-circle travel routes between the seeded destinations.
   Falls back to a plain hero when WebGL is unavailable and renders a single
   static frame when the visitor prefers reduced motion. */
"use strict";

(function () {
  const canvas = document.getElementById("globe");
  if (!canvas) return;
  const gl = canvas.getContext("webgl", { alpha: true, antialias: true });
  if (!gl) { canvas.remove(); return; }

  /* ---------- geometry ---------- */

  const DOTS = 1500;
  const dotPositions = new Float32Array(DOTS * 3);
  // Fibonacci sphere: near-uniform point distribution.
  const golden = Math.PI * (3 - Math.sqrt(5));
  for (let i = 0; i < DOTS; i++) {
    const y = 1 - (i / (DOTS - 1)) * 2;
    const r = Math.sqrt(1 - y * y);
    const th = golden * i;
    dotPositions.set([Math.cos(th) * r, y, Math.sin(th) * r], i * 3);
  }

  // Destination coordinates (lat, lon) from the seeded inventory.
  const places = {
    berlin: [52.5, 13.4], lisbon: [38.7, -9.1], mallorca: [39.6, 2.9],
    crete: [35.2, 24.9], zermatt: [46.0, 7.7], vienna: [48.2, 16.4],
    azores: [37.7, -25.7], dubrovnik: [42.6, 18.1], paris: [48.9, 2.4],
    madeira: [32.7, -17.0],
  };
  const routes = [
    ["berlin", "mallorca"], ["paris", "crete"], ["vienna", "lisbon"],
    ["berlin", "zermatt"], ["lisbon", "azores"], ["paris", "dubrovnik"],
    ["vienna", "madeira"],
  ];

  const toVec = ([lat, lon]) => {
    const la = (lat * Math.PI) / 180, lo = (lon * Math.PI) / 180;
    return [Math.cos(la) * Math.cos(lo), Math.sin(la), Math.cos(la) * Math.sin(lo)];
  };
  const slerp = (a, b, t) => {
    const dot = Math.max(-1, Math.min(1, a[0] * b[0] + a[1] * b[1] + a[2] * b[2]));
    const om = Math.acos(dot), so = Math.sin(om) || 1e-6;
    const ka = Math.sin((1 - t) * om) / so, kb = Math.sin(t * om) / so;
    return [ka * a[0] + kb * b[0], ka * a[1] + kb * b[1], ka * a[2] + kb * b[2]];
  };

  const SEG = 56;
  const arcVerts = [];
  const arcMeta = []; // per point: t along arc, per-arc phase
  routes.forEach(([from, to], idx) => {
    const a = toVec(places[from]), b = toVec(places[to]);
    for (let i = 0; i <= SEG; i++) {
      const t = i / SEG;
      const p = slerp(a, b, t);
      const lift = 1 + Math.sin(t * Math.PI) * 0.16; // arc altitude
      arcVerts.push(p[0] * lift, p[1] * lift, p[2] * lift);
      arcMeta.push(t, idx / routes.length);
    }
  });

  /* ---------- shaders ---------- */

  // uMode is read by both stages; its precision must match the fragment
  // shader's mediump or strict drivers refuse to link the program.
  const vsrc = `
    attribute vec3 aPos;
    attribute vec2 aMeta;
    uniform float uRot, uTilt, uScale, uAspect, uDPR;
    uniform mediump float uMode;
    varying float vDepth; varying vec2 vMeta;
    void main() {
      float cy = cos(uRot), sy = sin(uRot);
      vec3 p = vec3(aPos.x * cy + aPos.z * sy, aPos.y, -aPos.x * sy + aPos.z * cy);
      float cx = cos(uTilt), sx = sin(uTilt);
      p = vec3(p.x, p.y * cx - p.z * sx, p.y * sx + p.z * cx);
      float d = 2.6;
      vec2 xy = vec2(p.x, p.y * uAspect) * uScale / (d - p.z);
      gl_Position = vec4(xy, 0.0, 1.0);
      vDepth = (p.z + 1.0) * 0.5;
      vMeta = aMeta;
      float base = uMode < 0.5 ? mix(1.4, 2.8, vDepth) : mix(1.8, 3.4, vDepth);
      gl_PointSize = base * uDPR;
    }`;
  const fsrc = `
    precision mediump float;
    uniform float uMode, uTime;
    uniform vec3 uInk, uRoseA, uRoseB;
    varying float vDepth; varying vec2 vMeta;
    void main() {
      vec2 c = gl_PointCoord - 0.5;
      if (dot(c, c) > 0.25) discard;
      if (uMode < 0.5) {
        float alpha = mix(0.07, 0.5, vDepth * vDepth);
        gl_FragColor = vec4(uInk, alpha);
      } else {
        float head = fract(uTime * 0.14 + vMeta.y * 1.7);
        float dist = abs(vMeta.x - head);
        float glow = smoothstep(0.16, 0.0, dist);
        float alpha = (0.05 + 0.85 * glow) * mix(0.25, 1.0, vDepth);
        vec3 col = mix(uRoseA, uRoseB, vMeta.x);
        gl_FragColor = vec4(col, alpha);
      }
    }`;

  function compile(type, src) {
    const s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
      throw new Error(gl.getShaderInfoLog(s));
    }
    return s;
  }
  const prog = gl.createProgram();
  gl.attachShader(prog, compile(gl.VERTEX_SHADER, vsrc));
  gl.attachShader(prog, compile(gl.FRAGMENT_SHADER, fsrc));
  gl.linkProgram(prog);
  if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) { canvas.remove(); return; }
  gl.useProgram(prog);

  const U = {};
  ["uRot", "uTilt", "uScale", "uAspect", "uDPR", "uMode", "uTime", "uInk", "uRoseA", "uRoseB"]
    .forEach((n) => (U[n] = gl.getUniformLocation(prog, n)));
  const aPos = gl.getAttribLocation(prog, "aPos");
  const aMeta = gl.getAttribLocation(prog, "aMeta");

  function buffer(data) {
    const b = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, b);
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
    return b;
  }
  const dotBuf = buffer(dotPositions);
  const dotMetaBuf = buffer(new Float32Array(DOTS * 2)); // zeros
  const arcBuf = buffer(new Float32Array(arcVerts));
  const arcMetaBuf = buffer(new Float32Array(arcMeta));

  const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  const darkMode = window.matchMedia("(prefers-color-scheme: dark)");
  const setPalette = () => {
    gl.uniform3fv(U.uInk, hex(darkMode.matches ? "#e8e2d9" : "#1c1917"));
    gl.uniform3fv(U.uRoseA, hex(darkMode.matches ? "#e096a6" : "#d17588"));
    gl.uniform3fv(U.uRoseB, hex("#e879b9"));
  };
  setPalette();
  darkMode.addEventListener("change", setPalette);
  gl.enable(gl.BLEND);
  gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);

  /* ---------- render loop ---------- */

  let parallaxX = 0, targetX = 0;
  window.addEventListener("pointermove", (e) => {
    targetX = (e.clientX / window.innerWidth - 0.5) * 0.25;
  }, { passive: true });

  function resize() {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const w = canvas.clientWidth, h = canvas.clientHeight;
    if (canvas.width !== w * dpr || canvas.height !== h * dpr) {
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      gl.viewport(0, 0, canvas.width, canvas.height);
      gl.uniform1f(U.uAspect, w / h);
      gl.uniform1f(U.uDPR, dpr);
    }
  }

  function drawSet(posBuf, metaBuf, count, mode, time) {
    gl.uniform1f(U.uMode, mode);
    gl.uniform1f(U.uTime, time);
    gl.bindBuffer(gl.ARRAY_BUFFER, posBuf);
    gl.enableVertexAttribArray(aPos);
    gl.vertexAttribPointer(aPos, 3, gl.FLOAT, false, 0, 0);
    gl.bindBuffer(gl.ARRAY_BUFFER, metaBuf);
    gl.enableVertexAttribArray(aMeta);
    gl.vertexAttribPointer(aMeta, 2, gl.FLOAT, false, 0, 0);
    gl.drawArrays(gl.POINTS, 0, count);
  }

  function frame(ms) {
    resize();
    const t = ms / 1000;
    parallaxX += (targetX - parallaxX) * 0.04;
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.uniform1f(U.uRot, t * 0.07 + parallaxX + 2.2);
    gl.uniform1f(U.uTilt, 0.42);
    gl.uniform1f(U.uScale, 1.25);
    drawSet(dotBuf, dotMetaBuf, DOTS, 0, t);
    drawSet(arcBuf, arcMetaBuf, arcVerts.length / 3, 1, t);
  }

  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (reduced) {
    requestAnimationFrame(frame); // one static frame
  } else {
    const loop = (ms) => { frame(ms); requestAnimationFrame(loop); };
    requestAnimationFrame(loop);
  }
})();
