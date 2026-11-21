# Operations & strategies

This document specifies the toolpath strategies CleanCuts will support, in
rough order of delivery, and — importantly — the *models* each one uses, so that
each is a well-defined, testable piece of math rather than a black box.

## 2D / 2.5D operations (phases 2–3)

### Profile (2D contour) — *v0.1*

Cuts the outline of a part (material inside the contour) or the boundary of a
pocket (material outside). v0.1 implements the geometry: a closed contour →
plunge, cutting loop, retract. See `internal/toolpath/profile.go`.

**Phase 2 (in progress):**
- **Analytic loops:** the contour is a `kernel.Chain2` (lines *and* arcs). The
  generator `toolpath.Loop` emits each edge as an analytic move (line → G1-
  capable, arc → G2/G3-capable `moves` arc move) — no tessellation in the core
  (ADR-002). `Profile` is `Loop` over a polygon chain.
- **Cutter compensation:** offset the contour by the tool radius, with correct
  *inside vs. outside* semantics (profile cuts offset outward; pocket walls
  offset inward) and corner joins (mitre/chamfer/fillet — see
  [cutter-compensation.md](cutter-compensation.md)). `comp` is done; composing
  it in front of `Loop` is the next step.
- **Climb vs. conventional** per contour (see below).

### Pocket (2D) — *phase 3* (ADR-021, ADR-022, ADR-026)

Removes the material in a **region** down to `cut_z`.

- **The region** (ADR-021): `outer` (the depth-0 boundary) minus the
  **bosses** — nested loops whose material rises *above* the slice plane
  (kept, e.g. a cylinder standing on a base). Void loops are air (part of
  the cleared region). The tool centre stays inside `outer`, outside every
  boss.

**The path model** (ADR-026 — the full, testable spec; `toolpath.Pocket`):

1. **Offset first** (the planner composes `comp`): the tool-centre domain is
   `outer2 = Offset(outer, inside, R)` and, per boss,
   `boss2 = Offset(boss, outside, R)`. Every tool-centre position must be
   inside `outer2` and outside every `boss2` — the primary clearance
   (Law 7), by construction.
2. **Rows:** horizontal lines `y = yMin + s/2 + i·s` across `outer2`'s
   bounding box, where `s` is the stepover (default `R`; must satisfy
   `0 < s ≤ 2R` — coverage — else a precise refusal).
3. **Clearable intervals:** row ∩ `outer2`, minus the row ∩ each `boss2` —
   analytic line/edge intersection (kernel) + interval subtraction; the
   result is a sorted list of disjoint intervals. Intervals shorter than
   0.1 mm are dropped with a diagnostic (residual — the safe side, ADR-013).
4. **Traversal:** rows bottom → top; row *i* runs left→right for even *i*,
   right→left for odd *i*. A row with several intervals (a boss split it)
   traverses them in that direction's order, with a **lift** between
   consecutive intervals of the same row (retract → rapid → plunge): the gap
   is over kept material or outside the region and is never cut through.
5. **Row join:** between row *i*'s end P₁ and row *i+1*'s start P₂, a
   **semicircle with diameter P₁P₂** (radius |P₁P₂|/2 — ADR-022's `s/2` is
   the Δx = 0 special case). Of the two semicircles, the one whose centre is
   inside `outer2` is preferred; the arc is gate-checked against the bosses
   (no intersection, clearance ≥ R — `safety`'s loop helpers). If neither
   passes, the join degrades to a **lift** (diagnostic recorded).
6. **Plunges:** the first interval and every interval after a lift begin
   with a vertical rapid plunge at its start (SafeZ → CutZ). The path's
   first move is such a plunge — the ADR-020 contract the emitter relies on.
   Every plunge point lies over the face (air above it — the face-section
   property) and outside every boss (interval clipping) — never over kept
   material.
7. **Engagement & feed** (ADR-022): constant stepover → constant radial
   engagement `a_e = R − sqrt(R² − (s/2)²)`; the cutting feed of every
   pocket cutting move is `F · sqrt(a_e/D)` (underfeeding the joins — the
   safe side). Both are pure functions in `toolpath`, independently tested.
8. **Safety:** the whole path is gate-checked against the fixture keep-outs
   (ADR-012, `safety.CheckPath`); the bosses are enforced by steps 1, 4, and
   5 (a boss the tool would touch is a refusal, never a silent clip).

- **Entry:** the lead-in (ADR-023) precedes the first interval's start — the
  cutter starts outside the cut area, plunges there, and cuts into the
  region. See part.md for the geometry.
- **Coverage:** rows spaced `s ≤ 2R` sweep strips of width `2R` that overlap
  — the region is fully covered by the tool disc.
- **Tessellation bound:** a boss from a tessellated mesh (the Testmesh
  cylinder is a 289-gon) is cleared to its *polygon*; the sagitta bound is
  ≤ r·(1 − cos(π/n)) — for the Testmesh < 1 µm (documented, negligible).
- **Passes:** v1 is a single depth `cut_z`; multiple Z depths (rough →
  finish) is phase 4.

### Facing — *phase 3* (ADR-022)

Facing a top face down to a uniform Z is the pocket operation where the region
is the whole top face (a single outer boundary, no bosses): the same trochoidal
raster keeps a constant engagement width. A dedicated face mill is a later
tooling refinement; v1 uses the end mill.

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

**The paramount constraint is safety, not smoothness** (ADR-010). Smoothing must
never drive the cutter into material that is meant to be kept — that would destroy
the part or the tool, and it is unrecoverable. Smoothness is a goal *within* the
set of safe motions, never a reason to leave it.

Smoothness is a **post-processing** concern, not a generation one, and it is
separated deliberately:

0. **Clearance is preserved first.** Any geometric smoothing (fillets, arc
   insertion, re-shaping) is applied to the *tool-centre path* and **only in the
   direction that increases or holds** the minimum clearance between the tool's
   cutting *envelope* (not just its centre) and the kept material. The safe
   failure mode is leaving a small residual that finishing removes — **never**
   over-cutting. So at a corner we round the way that *leaves* material, not the
   way that *takes* it.
1. **Analytic arcs where they exist** (G2/G3). A trochoidal path is *already*
   mostly arcs, which are smooth by construction. Prefer emitting them as arcs
   over tessellating them (ADR-002).
2. **Corner handling (clearance-safe):** at a join, prefer a **deceleration**
   over a geometric fillet. Where a fillet is used it must satisfy rule 0. This
   removes the velocity discontinuity that causes jerk without risking the part.
3. **Lookahead feed scheduling (phase 4):** a planner walks the move list and
   computes a feed schedule that respects the machine's **feed and rapid limits
   and** acceleration/jerk limits, decelerating into corners and re-accelerating
   out. It **decelerates into corners; it does not re-route** them (re-routing is
   where over-cutting risk lives, and rule 0 governs any geometric change).
4. **Feed and rapid limits (ADR-010):** cutting moves clamp to
   `[min_feed, max_feed]`, rapids to `[min_rapid, max_rapid]`, per-axis motion to
   the axis `max_feed`/`max_rapid`. These come from the machine config
   (see [gcode.md](gcode.md)). A rapid is never a slow creep; a cut never exceeds
   max feed.
5. **G1 fallback:** for controllers without reliable G2/G3, the same moves are
   tessellated at a configurable angular tolerance (`gcode.Options.ArcTolDeg`),
   and rules 0, 2, and 3 still apply to the resulting segments.

**Principle:** the *shape* is analytic and owned by generation; the *velocity
profile* is owned by post-processing; and **correctness (clearance) dominates
smoothness** throughout. Neither the shape nor the velocity profile ever reaches
into the other's domain.

## Climb vs. conventional: how the choice is made

> **Where the decision lives (ADR-009).** This is decided in **planning, before
> generation**, because the choice sets the *traversal direction* the tool must
> take — an input to generation, not a label on the output. The planner picks the
> mode (climb/conventional) per feature and fixes the contour orientation that
> achieves it; generation then emits the moves in that orientation and records the
> mode on each cutting move (ADR-005). The derivation below is what the planner
> uses to choose the orientation.

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
