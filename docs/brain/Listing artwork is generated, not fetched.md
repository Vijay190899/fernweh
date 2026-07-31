---
title: Listing artwork is generated, not fetched
tags: [decision, frontend, security]
status: accepted
date: 2026-07-31
---

# Listing artwork is generated, not fetched

**Decision.** Each listing renders a deterministic abstract landscape derived from its identifier, instead of a photograph from an image service.

The seeded inventory has no real photography, and the placeholder service returned a photograph of a tiger for a villa on the Costa Brava. Random imagery on a travel product does not read as a placeholder, it reads as broken.

Each listing derives a stable hash from its id, which selects a horizon height, a ridge silhouette and a light position within a palette chosen by category. Beaches get warm sand and turquoise water, ski destinations cool blue and white. The same listing always produces the same artwork, and the result is coherent because it is designed rather than sampled.

The security consequence was the better one. Removing the image host removed the last external origin from the page, so the content security policy tightened to self and data URIs, with framing denied. Fonts, scripts, styles and imagery are now all first-party.

A production system would use supplier media. The point is that the demo does not need to borrow credibility from stock photography to look finished.

Related: [[Raw WebGL over a 3D library]]
