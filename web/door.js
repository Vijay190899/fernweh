/* The portal sequence.
 *
 * One tall container with a pinned stage inside it. How far you have scrolled
 * through the container is the whole timeline:
 *
 *   0.00 - 0.45   the panel swings open
 *   0.40 - 1.00   the scene rushes past the camera, so the doorway grows
 *                 around the viewer and the products behind it come forward
 *
 * The two phases overlap deliberately: the push through begins while the door
 * is still finishing its swing, which is what makes it feel like moving
 * rather than like two animations played in sequence.
 *
 * Every frame writes custom properties on two elements, so the browser is
 * only ever compositing transforms and opacity.
 */
"use strict";

(function () {
  // Rows reveal themselves as they are reached. Independent of the portal, so
  // it still runs if the door sequence is absent or reduced-motion is on.
  const rows = [...document.querySelectorAll(".reveal-on-scroll")];
  if (rows.length) {
    if (!("IntersectionObserver" in window) ||
        window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      rows.forEach((r) => r.classList.add("in"));
    } else {
      const obs = new IntersectionObserver((entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) { e.target.classList.add("in"); obs.unobserve(e.target); }
        });
      }, { rootMargin: "0px 0px -12% 0px" });
      rows.forEach((r) => obs.observe(r));
    }
  }

  const portal = document.getElementById("portal");
  const scene = document.getElementById("scene");
  const panel = document.getElementById("door-panel");
  if (!portal || !scene || !panel) return;

  const clamp01 = (v) => (v < 0 ? 0 : v > 1 ? 1 : v);
  const easeOut = (t) => 1 - Math.pow(1 - t, 3);
  const easeInOut = (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);

  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    // No travel: show the door open, skip the journey through it.
    panel.style.setProperty("--door", "0.85");
    portal.style.height = "100vh";
    document.body.classList.add("scrolled");
    return;
  }

  let ticking = false;

  function update() {
    ticking = false;

    const travel = portal.offsetHeight - window.innerHeight;
    const progress = clamp01((window.scrollY - portal.offsetTop) / (travel || 1));

    // Phase one: the panel swings, and quickly. The door is an invitation,
    // not the content, so it should not cost several screens of scrolling.
    const swing = easeInOut(clamp01(progress / 0.30));
    panel.style.setProperty("--door", swing.toFixed(4));

    // Phase two overlaps phase one heavily and completes well before the
    // container ends, so the product list is arriving while the door is still
    // opening rather than after a stretch of empty scroll.
    const push = easeOut(clamp01((progress - 0.20) / 0.55));
    // Scale is exponential so it accelerates the way approaching a doorway
    // actually looks, rather than creeping linearly.
    scene.style.setProperty("--zoom", (1 + push * push * 7).toFixed(4));
    scene.style.setProperty("--scene-opacity", (1 - clamp01(push * 1.35)).toFixed(4));

    // The header sits on photography during the sequence and on pale paper
    // afterwards, so it has to change colour rather than pick one and be
    // unreadable half the time.
    document.body.classList.toggle("scrolled", push > 0.6);
  }

  const onScroll = () => {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(update);
  };

  window.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", onScroll, { passive: true });
  update();
})();
