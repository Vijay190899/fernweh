/* Ambient background for the product pages.
 *
 * A single full-viewport WebGL canvas running one fragment shader: layered
 * flow noise pushed through a warm palette, drifting slowly and leaning
 * toward the pointer. It sits behind the content at low contrast so type
 * stays readable, and it is the same field on every product page so the three
 * of them read as one product rather than three documents.
 *
 * No libraries, no textures, ~4KB. Falls back to plain paper if WebGL is
 * unavailable, and renders one still frame under reduced-motion.
 */
"use strict";

(function () {
  const canvas = document.getElementById("bg");
  if (!canvas) return;

  const gl = canvas.getContext("webgl", { alpha: true, antialias: false, depth: false });
  if (!gl) return;

  const VERT = `
    attribute vec2 aPos;
    void main() { gl_Position = vec4(aPos, 0.0, 1.0); }`;

  const FRAG = `
    precision highp float;
    uniform vec2  uRes;
    uniform float uTime;
    uniform vec2  uPointer;
    uniform vec3  uA, uB, uC;
    uniform float uAlpha;

    // Value noise. Cheap, and smooth enough that banding never shows at the
    // low contrast this is drawn with.
    float hash(vec2 p) {
      return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453123);
    }
    float noise(vec2 p) {
      vec2 i = floor(p), f = fract(p);
      vec2 u = f * f * (3.0 - 2.0 * f);
      return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), u.x),
                 mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x), u.y);
    }
    float fbm(vec2 p) {
      float v = 0.0, a = 0.5;
      for (int i = 0; i < 5; i++) {
        v += a * noise(p);
        p = p * 2.03 + vec2(11.7, 5.3);
        a *= 0.5;
      }
      return v;
    }

    void main() {
      vec2 uv = gl_FragCoord.xy / uRes;
      vec2 p  = uv * vec2(uRes.x / uRes.y, 1.0);

      float t = uTime * 0.028;
      vec2 drift = (uPointer - 0.5) * 0.22;

      // Domain warping: noise displacing the lookup of more noise, which is
      // what gives the slow liquid movement rather than a shimmer.
      vec2 q = vec2(fbm(p * 1.6 + vec2(0.0, t)),
                    fbm(p * 1.6 + vec2(4.3, -t) + drift));
      vec2 r = vec2(fbm(p * 1.6 + 3.2 * q + vec2(1.7, 9.2) + t * 0.6),
                    fbm(p * 1.6 + 3.2 * q + vec2(8.3, 2.8) - t * 0.4));
      float f = fbm(p * 1.4 + 3.6 * r);

      // Keep the three hues in distinct regions. Mixing all of them toward
      // each other at roughly half strength averages to grey, which is what
      // the first version did: vivid inputs, neutral output.
      vec3 col = mix(uA, uB, smoothstep(0.32, 0.78, f));
      col = mix(col, uC, smoothstep(0.62, 1.05, length(r)) * 0.85);
      // Lift saturation back up after blending.
      float lum = dot(col, vec3(0.299, 0.587, 0.114));
      col = mix(vec3(lum), col, 1.45);

      // Vignette toward the page colour so the field never fights the header
      // or the footer, and edges stay clean.
      float vig = smoothstep(1.18, 0.28, length(uv - 0.5));
      gl_FragColor = vec4(col, uAlpha * vig);
    }`;

  function compile(type, src) {
    const s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s));
    return s;
  }

  let prog;
  try {
    prog = gl.createProgram();
    gl.attachShader(prog, compile(gl.VERTEX_SHADER, VERT));
    gl.attachShader(prog, compile(gl.FRAGMENT_SHADER, FRAG));
    gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(prog));
  } catch (e) {
    canvas.remove();
    return;
  }
  gl.useProgram(prog);

  // One full-screen triangle: fewer vertices than a quad and no seam.
  const buf = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buf);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  const aPos = gl.getAttribLocation(prog, "aPos");
  gl.enableVertexAttribArray(aPos);
  gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);

  const U = {};
  ["uRes", "uTime", "uPointer", "uA", "uB", "uC", "uAlpha"].forEach(
    (n) => (U[n] = gl.getUniformLocation(prog, n)));

  const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);

  // Each page tints the same field, so the set feels related without any two
  // pages looking identical. Set via data-bg on the canvas.
  const THEMES = {
    search:  { a: "#ffe9c9", b: "#f08fae", c: "#6fb6e8" },
    ranking: { a: "#ffe2f0", b: "#a86fe0", c: "#f5a15c" },
    content: { a: "#ddf3e6", b: "#4fb894", c: "#f0c65a" },
  };
  const theme = THEMES[canvas.dataset.bg] || THEMES.search;

  // One palette, because the page has one appearance. This used to dim itself
  // to 30% under prefers-color-scheme: dark, left over from a dark theme that
  // was never built. The stylesheet has no dark mode, so the result was dimmed
  // pastels drawn at low alpha over light paper: a flat grey field, on the
  // default setting of a good share of the machines this will be opened on.
  const palette = () => [hex(theme.a), hex(theme.b), hex(theme.c)];

  let px = 0.5, py = 0.42, tx = 0.5, ty = 0.42;
  window.addEventListener("pointermove", (e) => {
    tx = e.clientX / window.innerWidth;
    ty = 1 - e.clientY / window.innerHeight;
  }, { passive: true });

  function resize() {
    // Half resolution: this is an out-of-focus background, and the noise is
    // smooth, so nobody can tell while the fill rate halves.
    const dpr = Math.min(window.devicePixelRatio || 1, 1.5) * 0.5;
    const w = Math.max(1, (window.innerWidth * dpr) | 0);
    const h = Math.max(1, (window.innerHeight * dpr) | 0);
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
      gl.viewport(0, 0, w, h);
    }
  }

  gl.enable(gl.BLEND);
  gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);

  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const start = performance.now();

  function frame(now) {
    resize();
    px += (tx - px) * 0.05;
    py += (ty - py) * 0.05;
    const [a, b, c] = palette();
    gl.uniform2f(U.uRes, canvas.width, canvas.height);
    gl.uniform1f(U.uTime, reduced ? 12 : (now - start) / 1000);
    gl.uniform2f(U.uPointer, px, py);
    gl.uniform3fv(U.uA, a);
    gl.uniform3fv(U.uB, b);
    gl.uniform3fv(U.uC, c);
    // Strong enough to read as a moving field rather than a flat page, low
    // enough that ink on it still clears contrast comfortably.
    gl.uniform1f(U.uAlpha, 0.80);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  }

  if (reduced) {
    requestAnimationFrame(frame);
  } else {
    const loop = (n) => { frame(n); requestAnimationFrame(loop); };
    requestAnimationFrame(loop);
  }
  document.documentElement.classList.add("has-bg");
})();
