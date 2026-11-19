# 2D offset & cutter compensation

The core 2D geometry problem, specified before it is built (ADR-013). This is
the highest-risk, highest-value piece of 2D CAM and the one most likely to
over-cut the part — so it is specified as a **dedicated, composable,
safe-by-construction module** with a defined safe-failure mode.

## The core idea

Given a **contour** (a closed chain of lines and arcs) and a **tool radius `r`**,
2D offset produces a **new contour**: the path the **tool centre** must follow so
that the tool's cutting edge traces the original contour.

```
tool-centre path  =  boundary offset by r, on the tool-centre side
```

That's the whole thing. Everything else is (a) getting the *direction* right,
(b) joining the offset edges cleanly, and (c) **never letting it over-cut.**

### The tool radius is the tip envelope at the working Z (ADR-011)

`r` is the tool's tip-envelope radius **at the working Z**:

| Tool | `r` at a fixed Z |
|---|---|
| flat end mill | `D/2` |
| bull-nose | the corner-radius cap radius (constant over the cap) |
| ball-nose | the spherical-cap radius (a number at the working Z) |

In 2.5D (Z constant) `r` is just a number, so 2D offset is straightforward. In 3D
the offset becomes a *surface* offset (the tool envelope) — a later module
(ADR-011) — but it is the same idea with `r` varying.

## Outside vs. inside — the most common 2D mistake

The offset direction is **not** inferred; it is an explicit, named input:

- **Outside** — the tool-centre path lies *outside* the kept boundary. Used for
  **profiling**: cutting a part's outline, where the tool is in the stock being
  removed, at distance `r` from the part edge.
- **Inside** — the tool-centre path lies *inside* the boundary. Used for
  **pocket walls**: the material is outside the wall, so the tool centre is
  inside the pocket, at distance `r` from the wall.

Getting this backwards is the classic "cut the wrong side / cut into the part"
bug. Making it an explicit parameter (with the generator asserting the operation
matches the direction) is what prevents it.

## The algorithm

Offset is a two-step process: offset each edge, then resolve the joins.

### 1. Offset each edge (pure, per-edge, testable)

- **Line** `A→B`: shift both endpoints by `±r·n`, where `n` is the unit normal in
  the offset direction. Result: a parallel line.
- **Arc** (centre `C`, radius `R`, span `θ0→θ1`): **same centre**, radius
  **`R + r`** (outside) or **`R − r`** (inside), same span. If `R − r ≤ 0`, the
  arc degenerates — the tool is too large for that concave arc → safe resolution
  (below).

Each of these is a pure function of `(edge, r, direction)` with an exact analytic
result — trivially testable.

### 2. Resolve the join at each vertex

At a vertex `V` between two edges, the two offset edges don't meet cleanly. The
join is one of:

| Join | Construction | When |
|---|---|---|
| **Mitre** | intersect the two offset edges at point `M`; the centre path turns at `M`. | default for a sharp corner |
| **Chamfer** | truncate the two offset edges at a bounded distance from `V`, join with a straight segment. | when the mitre would be too long (see mitre limit) |
| **Fillet** | an arc of radius `r` centred at `V`, between the tangent points on the two offset edges. | when the boundary corner is rounded, or a rounded centre path is wanted |

- **Mitre limit:** a sharp *acute* corner pushes the mitre point far out. Cap it —
  if the mitre point would extend beyond `mitre_limit · r` from `V`, switch to a
  **chamfer**. `mitre_limit` is config (typical default ≈ 5).
- **Line↔arc and arc↔arc joins** are resolved by the corresponding intersection
  (line–arc, arc–arc) from the kernel, choosing the point on the offset side.
  These are exactly the `kernel` intersection primitives the design exists to
  provide.

**The circular-tool cusp (why a mitre is correct, not a bug).** For a circular
tool at a sharp corner, the tool centre reaches the mitre point where it is
tangent to *both* offset edges, and the cut leaves a small **cusp** at the true
corner — a round tool simply cannot cut a sharp point. This is standard and
**safe** (it *leaves* material). Finishing the cusp needs a smaller tool or a
chamfer; it is never over-cut.

### 3. Assemble + safe resolution (the guarantee)

After building the offset contour:

1. **Detect self-intersections** (segment–segment, segment–arc, arc–arc — kernel
   primitives).
2. **If it self-intersects** (a tight concave notch narrower than the tool, or a
   tool radius larger than the feature): **trim to the tool-reachable region.**
   The tool stops at the limit and **leaves material** in the unreachable tight
   spot. This is *under-cut* — a residual a smaller tool or a finish pass removes.
3. **Report unreachable features** in the diagnostics (`"feature unreachable at
   tool radius r"`), so the planner can pick a smaller tool or flag it — it is
   never silently over-cut.

**The safe invariant the module owns:** the offset contour must (a) stay at
distance `≥ r` from the kept boundary (never over-cut) and (b) be a valid loop. If
(a) is geometrically impossible (feature too tight for the tool), the module
*leaves material* and *reports* — it does not over-cut. This is ADR-010 applied
to 2D, and it is **a property of this one module**, so every generator that uses
it inherits it for free.

## The output and how it composes

`comp` returns a **new contour** (a chain of lines and arcs in `kernel`) plus
diagnostics. It does **not** emit moves. The generators consume it:

- **Profile:** outside-offset the part contour → emit the loop as moves (plunge,
  cutting loop, retract).
- **Pocket:** inside-offset the pocket boundary per pass → emit the wall loop.

So "profile with compensation" and "profile without compensation" are the *same
generator* with and without a `comp` step in front — the composable-stages design
(ADR-008) in action.

## What is testable (the acceptance model)

Analytic offsets with exact results, asserted to tight tolerance:

- **Offset a circle** radius `R` by `r` → a circle of radius `R + r` (outside) /
  `R − r` (inside). The cleanest case.
- **Offset a square** side `L` by `r` → a larger square with mitre (or chamfer /
  fillet) corners at the expected analytic distances.
- **Offset a tight notch** narrower than `2r` → the offset trims and reports the
  feature unreachable; it never over-cuts.
- **Degenerate inputs:** collinear edges, zero-length features, `r` larger than
  the feature → explicit, safe behaviour + diagnostics.

## Build order (each slice independently testable)

1. `comp.Edge` (a line or arc) + `Edge.Offset(r, direction)` — per-edge offset.
   *Test:* line offset; arc offset `R±r`; `R−r ≤ 0` degenerate.
2. `comp.Join` (mitre / chamfer / fillet) — join two offset edges at a vertex,
   incl. line–arc and arc–arc via `kernel` intersections. *Test:* each join type;
   mitre-limit → chamfer.
3. `comp.Offset(contour, r, direction, joins)` — assemble + self-intersection
   detection + safe trim. *Test:* square, circle, tight notch.
4. **Diagnostics** (unreachable features, degenerate edges). *Test:* reported,
   not over-cut.
5. **Compose** with `toolpath.Profile` (outside) and a `toolpath.Pocket`
   (inside): offset contour → moves. *Test:* the emitted moves trace the offset
   contour and stay `≥ r` from the kept boundary.

## Status

**v0.1:** design only (this doc + ADR-013). The `comp` module is the first
*implementation* slice of Phase 2 — built in the order above, each step tested
against the analytic cases here. Nothing here changes the spine, the `moves`
format, or the kernel (beyond using the intersection primitives the kernel already
provides).
