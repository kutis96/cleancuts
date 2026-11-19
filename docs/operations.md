# Operations & strategies

This document specifies the toolpath strategies CleanCuts will support, in
rough order of delivery, and — importantly — the *models* each one uses, so that
each is a well-defined, testable piece of math rather than a black box.

## 2D / 2.5D operations (phases 2–3)

### Profile (2D contour) — *v0.1*

Cuts the outline of a part (material inside the contour) or the boundary of a
pocket (material outside). v0.1 implements the geometry: a closed contour →
plunge, cutting loop, retract. See `internal/toolpath/profile.go`.

**Open for phase 2:**
- **Cutter compensation:** offset the contour by the tool radius, with correct
  *inside vs. outside* semantics (profile cuts offset outward; pocket walls
  offset inward) and corner joins (mitre now, fillet/arc later).
- **Climb vs. conventional** per contour (see below).

### Pocket (2D) — *phase 3*

Removes material from inside a boundary down to a depth, in one or more passes.

- **Passes:** multiple Z depths, each a full contour (or a stepped depth).
- **Strategy per pass:**
  - *Slotting → raster/offset* for shallow, wide pockets.
  - *Adaptive clearing* (below) for deep, wide pockets to control load.
- **Entry/plunge:** ramp or vertical plunge at a safe, non-cutting spot.

### Facing — *phase 3*

Removes material from the top of the stock to a uniform Z. Uses a face mill or
end mill in a trochoidal/fan pattern to keep a constant engagement width.

## The friend's requests, as first-class strategies

> "trochoidal machining, adaptive machining, constant cutter engagement, constant
> chipload, smooth paths without too much acceleration/jerk."

These are all *one family*: **engagement-controlled clearing**. The unifying idea
is to keep the tool's **cutting load** (≈ chip volume removed per tooth) roughly
constant by controlling **how much of the cutter is engaged** at any instant.

### Trochoidal (epi-/hypo-cycloid) milling

Instead of a straight raster, the tool follows a **curved, looping path** so that
the engaged arc length stays small and constant even at full depth of cut. The
path is generated from the tool's own radius: a series of arcs that weave through
the material, each pass re-entering the cut at a controlled radial engagement
`a_e`.

- **Why it works:** the cutting edge only ever engages a small fraction of the
  circle, so the chip thickness and radial/axial loads stay low and steady — even
  at depths of cut that would load a straight pass heavily.
- **Model (v1):** generate a family of trochoid arcs with a fixed tooth
  advance `s` and radial engagement `a_e`; the toolpath is the union of these
  arcs clipped to the pocket boundary. We will start with the well-understood
  *hypo-trochoid* for clearing and refine from measured behaviour.
- **Deliverable:** a `toolpath.AdaptivePocket(boundary, depth, tool, params)`
  that emits a `moves.Path` of arcs + lines, plus the per-segment engagement `a_e`
  in metadata so the feed scheduler (below) can scale feed.

### Adaptive / constant-chip-load clearing

Adaptive clearing is trochoidal clearing with the added rule that **feed is scaled
to engagement** so the *chip load per tooth* stays constant (see the constant
chip-load model in [tool-parameters.md](tool-parameters.md)):

```
F(a_e) ≈ F_full · sqrt( a_e / D )
```

- The geometry module produces the path **and** the per-segment `a_e`.
- The feed module turns `a_e` into a per-move `feed`.
- Both are visible in the intermediate format (`moves.Move.Feed` and
  `moves.Path.Metadata`), so a user can inspect *why* a given feed was chosen and
  override it.

**This is the "constant chipload" and "constant cutter engagement" request, made
concrete and testable.** Each of the two halves (geometry → `a_e`; `a_e` → feed)
has its own tests.

### Smoothing & jerk control

> "smooth paths without too much acceleration/jerk."

Smoothness is a **post-processing** concern, not a generation one, and it is
separated deliberately:

1. **Analytic arcs where they exist** (G2/G3). A trochoidal path is *already*
   mostly arcs, which are smooth by construction. Prefer emitting them as arcs
   over tessellating them (ADR-002).
2. **Corner handling:** at joins between segments, insert a fillet (an arc) of a
   configurable radius rather than a sharp mitre, or emit a short deceleration.
   This removes the velocity discontinuity that causes jerk.
3. **Lookahead feed scheduling (phase 4):** a small planner walks the move list
   and computes a feed schedule that respects the machine's acceleration and jerk
   limits, decelerating into corners and re-accelerating out. Inputs are the
   machine config (max feed/accels) and the move geometry. Output is a per-move
   `feed` (and, if the dialect supports it, feed-override words).
4. **G1 fallback:** for controllers without reliable G2/G3, the same moves are
   tessellated at a configurable angular tolerance (`gcode.Options.ArcTolDeg`),
   and the fillet/lookahead rules still apply to the resulting segments.

**Principle:** the *shape* is analytic and owned by generation; the *velocity
profile* is owned by post-processing. Neither reaches into the other.

## Climb vs. conventional: how the choice is made

The milling mode of a cutting move is determined by three things (ADR-005):

1. **Cutter rotation direction** (from the tool / machine convention — most mills
   run the spindle CCW as viewed from above, i.e. "right-handed").
2. **Direction of tool travel** along the contour.
3. **Which side holds the material** (inside vs. outside the contour).

Given the rotation and the travel direction, the material is either on the
tool's *left* or *right*:

- Material on the **left** of travel (for a CCW spindle) → **climb (down) milling**:
  the cutter pulls the work into it; chip thickness is maximum at entry and tapers
  to zero at exit. Generally preferred — smoother finish, less tool wear — but it
  pulls the work and needs a machine without significant backlash.
- Material on the **right** of travel → **conventional (up) milling**: the cutter
  pushes away; chip thickness starts at zero and builds. Used when there is
  backlash, on manual machines, or to push a flexible workpiece against a stop.

**Decision policy (v1):**
- **Default to climb** for external profiles and internal pocket walls (the common
  case for a rigid machine), and record the mode on each cutting move.
- Allow a **global override** and a **per-operation override** (in the operation
  planner), and a **per-path override** directly in the intermediate format.
- The post-processor may apply **backlash compensation** when the mode is
  conventional and the machine config flags it as needed.

This makes "use the correct one on the correct surface" a **visible, overridable,
per-feature decision** rather than a hidden global flag — exactly the
inspectability the intermediate format is for.

## 3D operations (later)

- **Z-level / parallel:** horizontal slices at fixed Z; good for general relief.
- **Scallop / constant stepover:** stepover chosen from tool radius and a target
  scallop height; natural for ball-nose finishing.
- **Flow / spiral:** radial or flow patterns for finishing.
- **Pencil / contour-parallel:** for steep walls.

3D needs the kernel to grow (B-splines, surface/edge intersection, Z-height
functions) and the planner to become a real feature detector. It is a **larger**
addition to the pipeline, not a change to its spine.

## Strategy selection (planner)

A future **operation planner** maps part features + stock + tool to a sequence of
operations and picks a strategy per feature. Its inputs are the part, stock,
tool, machine, and a user intent ("rough this pocket", "finish this face"). Its
output is an ordered list of operations, each producing `moves.Path`. This is the
stage that turns "a part" into "a program," and it is the natural home for the
climb/conventional policy and the strategy selection above.

**Roadmap status:** profile geometry (v0.1) → compensation + climb/conventional
(phase 2) → pocket/facing + adaptive geometry (phase 3) → feed scheduling /
smoothing (phase 4) → 3D (phase 5+). See [roadmap.md](roadmap.md).
