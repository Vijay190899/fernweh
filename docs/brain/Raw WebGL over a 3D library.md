---
title: Raw WebGL over a 3D library
tags: [decision, frontend, performance]
status: accepted
date: 2026-07-31
---

# Raw WebGL over a 3D library

**Decision.** The scroll-driven scene is written directly against WebGL rather than importing a 3D library.

The reference site this borrows its structure from runs Three.js with custom shaders. The architecture worth copying was not the library, it was the arrangement: one persistent canvas behind the document, sections acting as timeline markers rather than containers, and scroll position directing a single continuous scene instead of many separate animations.

That architecture does not require the library. What this project needs is a point cloud that morphs between four formations, which is a handful of attribute buffers and about eighty lines of shader.

Three reasons to write it directly. The page ships inside the Go binary under a same-origin policy that permits no external origins, so a CDN was never available. The result is under 20KB against roughly 600KB for the library. And the morph targets live in vertex attributes, so transitioning between formations costs one draw call regardless of scroll position.

The cost is real: no scene graph, no camera helpers, and a precision mismatch between shader stages that a library would have hidden. That bug is recorded in [[Bugs the AI wrote]].

Each formation visualises an actual service rather than decorating: inventory across Europe, a query converging on an answer, results in ranked columns, and a content grid whose gaps fill in as the enrichment section scrolls past.

Related: [[Listing artwork is generated, not fetched]], [[No web framework]]
