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
  const portal = document.getElementById("portal");
  const scene = document.getElementById("scene");
  const panel = document.getElementById("door-panel");
  const reveal = document.getElementById("reveal");
  if (!portal || !scene || !panel || !reveal) return;

  const clamp01 = (v) => (v < 0 ? 0 : v > 1 ? 1 : v);
  const easeOut = (t) => 1 - Math.pow(1 - t, 3);
  const easeInOut = (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);

  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    // No travel: show the destination, skip the journey.
    panel.style.setProperty("--door", "1");
    scene.style.setProperty("--scene-opacity", "0");
    reveal.style.setProperty("--reveal-opacity", "1");
    reveal.style.setProperty("--reveal-scale", "1");
    reveal.dataset.open = "1";
    portal.style.height = "100vh";
    return;
  }

  let ticking = false;

  function update() {
    ticking = false;

    const travel = portal.offsetHeight - window.innerHeight;
    const progress = clamp01((window.scrollY - portal.offsetTop) / (travel || 1));

    // Phase one: the panel swings.
    const swing = easeInOut(clamp01(progress / 0.45));
    panel.style.setProperty("--door", swing.toFixed(4));

    // Phase two: push through the opening.
    const push = easeOut(clamp01((progress - 0.4) / 0.6));
    // Scale is exponential so it accelerates the way approaching a doorway
    // actually looks, rather than creeping linearly.
    scene.style.setProperty("--zoom", (1 + push * push * 7).toFixed(4));
    scene.style.setProperty("--scene-opacity", (1 - clamp01(push * 1.45)).toFixed(4));

    reveal.style.setProperty("--reveal-opacity", clamp01((push - 0.18) / 0.42).toFixed(4));
    reveal.style.setProperty("--reveal-scale", (0.92 + 0.08 * push).toFixed(4));
    // Only accept clicks once the cards are actually readable.
    reveal.dataset.open = push > 0.55 ? "1" : "0";

    // The header sits on photography during the sequence and on pale paper
    // afterwards, so it has to change colour rather than pick one and be
    // unreadable half the time.
    document.body.classList.toggle("scrolled", progress > 0.92);
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
