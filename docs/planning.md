# Planning (the stage before generation)

ADR-009 puts milling-mode and traversal-direction decisions in an explicit
**planning** stage that runs *before* generation, because climb vs.
conventional determines the *direction the tool must travel* — an input to
generation, not a label on the output. This document specifies that stage's
model so it is testable math, not a black box.

## What the planner decides per feature

Given a closed contour, its **kind**, the spindle rotation, and the keep-out
zones, the planner produces:

1. **The milling mode** — climb or conventional. Default is **climb**
   (better finish, less tool wear); a per-feature override is respected.
2. **The traversal direction** — the contour is oriented (kept or reversed)
   so that the chosen mode is actually achieved (below).
3. **The start point** — the vertex where the cut begins, chosen so the entry
   never lies in a keep-out zone (clamps, jaws). This is a hard safety
   constraint, not a preference.

Generation then emits `moves.Path` in that orientation with the mode recorded
on each cutting move (ADR-005), exactly as the planner fixed it.

## Feature kinds (which side holds the kept material)

| Kind | Kept material | Tool runs |
|---|---|---|
| **Profile** | *inside* the contour (the part) | outside the contour |
| **Pocket** | *outside* the contour (the wall) | inside the contour |

The kind is explicit input — never guessed — because it flips the geometry
(inside vs. outside offset, climb vs. conventional) and guessing it wrong is
the classic "cut into the part" bug.

## Deriving the milling mode

Viewed from above (looking down −Z), the cutter is a disc rotating with
angular velocity `ω`. At the engagement point (radius vector `r` toward the
kept material), the cutter surface moves with `v = ω × r`. **Climb milling
is the case where that surface velocity points along the tool's feed
direction** (chip thickness maximal at entry, tapering to zero);
conventional is the opposite.

For a spindle rotating **CCW viewed from above** (the common "right-handed"
setup, `spindle_rotation: "ccw"`), `ω × r` points 90° counter-clockwise from
`r`, i.e. toward the **left of the material side** — equivalently, the
surface moves along the feed when the **kept material is on the tool's right
of travel**. A CW spindle flips every entry.

| Kind | Spindle | Travel that achieves **climb** | Travel that achieves **conventional** |
|---|---|---|---|
| Profile (material inside) | CCW | clockwise (signed area < 0) | counter-clockwise (signed area > 0) |
| Pocket (material outside) | CCW | counter-clockwise (signed area > 0) | clockwise (signed area < 0) |
| Profile | CW | counter-clockwise (signed area > 0) | clockwise (signed area < 0) |
| Pocket | CW | clockwise (signed area < 0) | counter-clockwise (signed area > 0) |

As one predicate, with `ccw = SignedArea > 0` (measured, never assumed —
ADR-001):

```
climbIsAchieved = ((kind == Pocket) == (spindle == CCW)) == ccw
```

`planning.Plan` orients the contour (or reverses it) until the *requested*
mode is achieved, and records the resulting mode on the plan. Orientation is
a **measured** property of the oriented chain, so the generator can trust it.

## Start point and keep-out zones

Keep-out zones (fixture jaws, edge clamps — see
[workholding.md](workholding.md)) are hard constraints: **the tool centre
must never be commanded into them.** At planning time they enter as
`planning.KeepOut` values — boxes (min/max in WCS) and cylinders (centre XY +
radius + Z range). A zone applies at the cutting depth iff its Z range covers
the cut Z; its XY footprint is the box projection or the cylinder disc.

`PickStart` walks the chain's vertices in order and returns the first one
whose point is **not inside any applicable footprint**. If every vertex lies
in a keep-out, planning **refuses** (a precise error naming the zone) rather
than routing the entry through the clamp — the safe failure is "no plan",
never a path into the workholding. (Choosing a *better* entry — a safe
approach vector, an in-and-out lead-in — is a later refinement; v1 enters
vertically at the chosen vertex.)

`Rotate` cyclically reorders the chain's edges so the cut begins at the
chosen vertex; the edge sequence and orientation (and therefore the milling
mode) are preserved.

## Errors (precise and kind, per AGENTS.md)

- an **open** contour cannot be planned (a closed loop is required);
- a **degenerate** contour (≈ zero area) cannot be planned;
- **all vertices in a keep-out** → refusal naming the zone;
- an unknown **spindle rotation** value is rejected by validation.

## What planning is NOT

- Not a geometry changer: it orients and starts the path; it never re-routes
  it (that is smoothing's job, and it is bound by the clearance rule in
  [operations.md](operations.md)).
- Not a feed scheduler: speeds and feeds are set at generation/post time
  (ADR-010).
- Not collision checking against the *part* itself: that is the safety gate
  in `comp` (never over-cut) and, later, the Phase-4 collision check.

## Status

**v0.1 (this slice):** implemented in `internal/planning` — mode derivation
(table + predicate), orientation, `pickStart`/`Rotate` with box + cylinder
keep-outs, precise errors; the machine config carries `spindle_rotation`
(`ccw`/`cw`, default `ccw`) as the data source for this stage.
**Later:** approach vectors / lead-in, per-pass start for multi-pass
pockets, and the `generate` command wiring machine + fixture JSON into this
stage (slice L).
