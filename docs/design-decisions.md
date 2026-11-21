# Design decisions (ADR log)

This is an **append-only log** of architectural decisions (ADRs). Each entry
records the context, the decision, and the consequences. New decisions get a
new number; settled decisions are not edited, only superseded by a new entry
that references the old one.

> **For agents:** read this before proposing or changing core behaviour. If a
> decision here conflicts with your plan, either follow the decision or write a
> new ADR explaining why the situation has changed. Do not silently reverse a
> settled decision.

Status legend: **Accepted** (in force) · **Proposed** (under discussion) ·
**Superseded by ADR-NNN** (no longer in force).

---

## ADR-001 — Single canonical coordinate system (Accepted)

**Context.** The most common source of silent, distributed bugs in CAM tools is
coordinate-system confusion: geometry defined in the model's frame, toolpaths
computed in a "local" frame, and post-processing that re-frames into the
machine frame. Each re-interpretation is a place to lose a sign, a units factor,
or an origin offset. These errors are expensive because they surface far from
where they were introduced.

**Decision.** The entire core uses exactly one coordinate system:

- **Right-handed Cartesian.**
- **Z-up:** Z is the spindle axis; positive Z is "up" (away from the workpiece).
- **Units: millimetres** throughout the core.
- The origin is the **work-coordinate origin (WCS)** as defined by the part
  model / operator, i.e. the frame in which the part is described.

All packages — kernel, moves, toolpath, gcode, tool, machine — read and write
coordinates in this one system. The mapping to a specific machine's axes,
origin, and units happens **exactly once**, in the post-processor, and is driven
by the machine JSON config.

**Consequences.**

- Importers that use a different convention (e.g. STL's Y-up) perform a single,
  documented transform **at the boundary** and then discard the source frame.
  The transform is the only place a frame conversion exists.
- If we later need a different convention, the change is confined to the import
  boundary and the post-processor — not threaded through the core. This is the
  "easy to swap the coordinate system" property we want.
- `moves.Move` and `kernel.Point*` carry no frame metadata; the frame is
  *implicit and singular*. This is deliberate: there is nothing to get wrong.

**Related.** ADR-006 (precision), ADR-003 (intermediate format).

---

## ADR-002 — Analytic geometry, not tessellation (Accepted)

**Context.** Many CAM tools represent toolpaths as tessellated polylines and
re-sample them at fixed intervals in different subsystems. This is the
"re-interpolation onto fixed-point grids" failure mode: each resample loses a
little accuracy and accumulates error, and the result depends on the (arbitrary)
sampling.

**Decision.** Curves are represented **analytically** as first-class values:

- Straight segments (`kernel.Line2`) and circular arcs (`kernel.Arc2`, and arcs
  in `moves.Move`) are exact.
- An arc is fully determined by its centre, radius, and direction, so it can be
  reconstructed unambiguously from its two endpoints and centre.
- The toolpath format (`moves`) stores analytic moves. **Tessellation is a
  post-processing concern** (e.g. `gcode` can emit G2/G3, or approximate an arc
  with G1 segments at a configurable angular tolerance for controllers without
  reliable arc support).

**Consequences.**

- The core never introduces sampling error; accuracy is bounded only by the
  controller's arc/line support, which is explicit and configurable.
- Adding a new curve type (B-spline) is a local change to `kernel` + `moves`,
  not a re-architecture.
- Consumers that *need* polylines (the future GUI) tessellate on demand with a
  tolerance they control, rather than inheriting someone else's sampling.

**Related.** ADR-001.

---

## ADR-003 — Machine-independent intermediate toolpath format (Accepted)

**Context.** We need (a) a place where a user can inspect and **override**
per-feature / per-tool decisions (e.g. climb vs. conventional, feed, strategy),
(b) a stable contract between toolpath generation and post-processing, and (c) a
renderable model for the future GUI. These are best served by one explicit
interchange format rather than by letting the generator feed the post-processor
directly.

**Decision.** `internal/moves` defines the interchange format:

- A `Move` is one of: rapid (G0), linear (G1), arc CCW (G3), arc CW (G2), each
  with start/end (and centre for arcs), feed, spindle, coolant, and the
  **milling mode (climb / conventional)**.
- A `Path` is one **continuous** chain of moves (one pass of one feature), with
  an operation tag, tool id, and a free-form `metadata` bag for strategy
  context.
- The format is plain data with explicit invariants enforced by `Validate()`,
  and it serialises to stable, lowercase snake_case JSON
  (see `schemas/moves.schema.json`).

**Consequences.**

- Users can load a generated `.moves.json`, edit feeds / milling mode / strategy
  metadata, and re-post-process without re-running generation.
- The generic and machine-specific post-processors both consume the same format,
  so they can be compared and swapped.
- The format is the natural GUI data model (render moves, highlight features).
- Multiple passes / features are modelled as multiple `Path`s (later grouped by a
  program type), keeping each path a simple continuous chain.

**Related.** ADR-001, ADR-002, ADR-005.

---

## ADR-004 — Zero external dependencies in the core (Accepted)

**Context.** Dependencies increase build fragility, licensing surface, and the
difficulty of embedding the core (e.g. in WASM). Most of what a CAM core needs
(math, geometry, JSON) is well within the Go standard library.

**Decision.** The `internal/` core uses **only** the Go standard library. No
third-party modules appear in `go.mod` for core functionality.

**Consequences.**

- The core can be embedded in a WASM build and any other host without a
  dependency-resolution step.
- Behaviour is fully determined by this source; there is no transitive surprise.
- If the CLI or GUI later need heavier tooling (arg parsing, HTTP, rendering),
  those dependencies live in the edge (`cmd/`, `ui/`), never in `internal/`.
- We deliberately build our own numeric/geometry core rather than depending on a
  general math library, so that precision behaviour is ours to test and own.

**Exception policy.** A new dependency may be added only with an ADR entry and a
clear statement of why the standard library cannot serve the need.

---

## ADR-005 — Conventional vs. climb milling is explicit data (Accepted)

**Context.** Climb (down) and conventional (up) milling differ in the direction
the tool travels relative to its rotation and to the material being removed.
They have different force profiles, finish quality, and tool-wear behaviour. A
CAM tool should *choose* the correct one per feature, but also let the operator
override that choice.

**Decision.**

- The **milling mode is recorded on every cutting move** as `milling`
  (`"climb"` or `"conventional"`), per ADR-003.
- The *derivation* of the mode (from contour winding + cutter rotation + which
  side holds material) lives in toolpath generation, not in the format. The
  format only stores the result, so it stays simple and stable.
- A global default and per-operation overrides are applied at generation time;
  the resulting mode is visible and editable in the intermediate format.
- The post-processor does not need to *decide* the mode; it only needs to know
  it (e.g. to apply backlash compensation for conventional milling on a machine
  that requires it).

**Consequences.**

- Decisions are inspectable and overridable in the intermediate format.
- The "correct mode for the surface" logic is a well-scoped, testable piece of
  toolpath generation (see [operations.md](operations.md)), not a global flag.

**Related.** ADR-003.

---

## ADR-006 — Precision and robustness strategy (Accepted)

**Context.** The user's explicit concern: floating-point precision and
re-interpolation errors are pervasive in other CAM tools and hard to isolate.
We want the core to be *solid, well testable*, and to make precision bugs local
rather than systemic.

**Decision.**

- **`float64` everywhere** in the core (Go's default). No fixed-point grids, no
  integer resampling of coordinates.
- **Analytic representations** (ADR-002) minimise the number of derived values,
  which is where error accumulates.
- A **single, explicit tolerance** (`kernel.AbsTol = 1e-9`, mm/rad) is the
  default for geometric comparisons, and comparison helpers accept an explicit
  epsilon so callers can tighten or loosen deliberately rather than relying on
  magic constants scattered through the code.
- **No accumulated transforms.** Coordinates are stored as absolute values in the
  canonical frame; a transform, when needed, is an explicit operation producing
  new values, not a chain of implicit state.
- **Robust predicates** for the geometric primitives that will carry load
  (intersection, on-segment, winding) are implemented with a documented tolerance
  band and are individually tested, including the degenerate cases that occur in
  practice (collinear, touching endpoints).
- **Invariants are checked, not assumed.** Every public type has `Validate()`,
  and generators validate their output before returning it, so an invalid state
  fails fast at the boundary where it was created — with a message that names the
  offending value.

**Consequences.**

- Precision bugs are local: they are properties of a specific, tested primitive,
  not a global frame.
- Tests can assert analytic results to tight tolerance (e.g. arc length to 1e-6)
  because we are not comparing against a resampled polyline.
- The tolerance model is one documented constant with an override path, not a
  zoo of epsilons.

**Open refinement (Proposed).** For the cutter-compensation and adaptive modules
we will likely introduce a small, dedicated "robust geometry" layer (oriented
matroid / adaptive-precision predicates) for the truly degenerate cases. That is
a *new module* with its own ADR, not a change to this strategy.

---

## ADR-007 — Tools and machines are portable JSON (Accepted)

**Context.** A key usability goal: a user should be able to keep a library of
tool and machine config files, and move them between machines and operators, the
same way a shop keeps a physical tool crib and a machine crib.

**Decision.**

- **Tools** (`internal/tool`) and **machines** (`internal/machine`) are plain
  JSON documents with a stable, documented schema (see `schemas/`).
- Both are data-only, dependency-free Go types with `Validate()`, so a file that
  parses must also *make sense*.
- Tool libraries can hold many tools; a tool references its geometry (diameter,
  flutes, corner radius, taper) **and** its recommended process parameters
  (feed-per-tooth and/or cutting speed per material, max RPM).
- Machine config carries dialect + axis mapping/travel + spindle/coolant M-codes
  + WCS + arc format, i.e. everything a post-processor needs that is
  machine-specific.
- JSON keys are stable, lowercase snake_case. The schemas in `schemas/` are the
  contract.

**Consequences.**

- Config is version-controllable, diffable, and shareable as files.
- The core never hard-codes a machine or a tool; it reads config.
- Adding a new machine or tool is a data change, not a code change.

**Related.** ADR-004.

---

## ADR-008 — Composable pipeline stages (Accepted)

**Context.** We want each capability (profile, pocket, adaptive, facing, 3D) to
be independently testable and replaceable, and we want post-processing to be
swappable without touching generation.

**Decision.** The core is a pipeline of narrow, composable stages:

```
geometry ─► features ─► toolpath generation ─► [cutter compensation] ─► moves ─► post-processing
```

- Each stage takes well-defined inputs and returns well-defined outputs.
- A stage may be omitted or swapped (e.g. no compensation for a profile whose
  contour already is the tool-centre path; a different post-processor per
  machine).
- Stages communicate through the shared value types in `kernel` and `moves`, not
  through shared mutable state.

**Consequences.**

- New strategies are new generator modules that produce `moves.Path`; they do
  not modify the format, the kernel, or the post-processor.
- Failure is localised to the stage that produced it, and is reported through
  that stage's error type.

**Related.** ADR-003, ADR-004.

---

## ADR-009 — Climb/conventional is decided in planning, before generation (Accepted)

**Context.** ADR-005 established that the milling mode is explicit data on each
cutting move. It left *where the decision is made* underspecified. In practice
the choice is not a label you tack on after the fact: **climb vs. conventional
determines the direction the tool must travel** along a contour, given the cutter
rotation and which side holds the material. The direction is a fundamental input
to toolpath generation. Deciding it after generation (or trying to flip it by
reversing a finished path) is fragile and error-prone.

**Decision.** The pipeline gains an explicit **planning** stage that runs *before*
generation and is responsible for:

- selecting the operation and strategy per feature,
- **choosing the milling mode** (climb/conventional) per feature, applying the
  global default and any per-operation / per-path override, and
- **fixing the contour orientation / traversal direction** that achieves the
  chosen mode (rotation + travel direction + material side, per ADR-005).

Generation then produces the `moves.Path` *in that orientation* and records the
resulting mode on each cutting move (as data, per ADR-005). In short:

```
features ─► PLANNING (operation, strategy, milling mode, traversal direction, passes)
        ─► GENERATION (emits moves in the planned orientation, mode recorded)
        ─► moves
```

**Consequences.**

- "Use the correct milling on the correct surface" is a *planning* decision with
  a single, testable policy (see [operations.md](operations.md)), not a
  post-hoc relabel.
- The traversal direction is decided once, up front, and is a visible input to
  generation — overridable in the intermediate format (flip the orientation and
  regenerate, or edit the resulting path).
- ADR-005 stands (the mode is still explicit data on the move); ADR-009 fixes
  where the decision lives.

**Related.** ADR-005, ADR-008.

---

## ADR-010 — Motion smoothing is clearance-preserving; feed and rapid limits are respected (Accepted)

**Context.** Smooth, low-jerk motion is a goal, but the *paramount* constraint is
never cutting into material that is designated to be kept. Naive corner
filleting or lookahead deceleration can round a tool-centre path the wrong way
and drive the cutter across the part — destroying the part or the tool. Smoothness
must be subordinate to correctness. Separately, both cutting (G1) and non-cutting
(G0) moves must respect the machine's feed/rapid limits.

**Decision.**

1. **Clearance is preserved.** Any smoothing (fillets, arc insertion, lookahead
   re-shaping) is applied to the *tool-centre path*, and only in the direction
   that **increases or holds** the minimum distance between the tool's cutting
   envelope and the kept material. The safe failure mode is leaving a small
   amount of extra stock (a residual scallop that finishing removes), **never**
   removing material that should stay. Over-cutting is unrecoverable;
   under-cutting is recoverable, so we always bias to the latter.
2. **The tool envelope, not a point, is what is kept clear.** Clearance checks use
   the actual tool tip surface (see ADR-011), not just the centre, so a ball-nose
   or bull-nose is respected, not only its radius.
3. **Feed and rapid limits are enforced.** Cutting moves are clamped to
   `[min_feed, max_feed]`; rapid moves to `[min_rapid, max_rapid]`; per-axis
   motion is bounded by the axis `max_feed`/`max_rapid`. These limits live in the
   machine config and are applied by the feed/rapid scheduler (see
   [gcode.md](gcode.md)). A rapid is never silently turned into a slow creep, and
   a cutting move is never allowed to exceed the machine's max feed.
4. **Lookahead decelerates, it does not re-route, into corners** for the v1 model:
   it lowers feed into a join and recovers out, rather than geometrically
   re-shaping the path (which is where over-cutting risk lives). Geometric
   fillets, where used, follow rule 1.

**Consequences.**

- Correctness dominates smoothness, by construction: the optimisation domain for
  smoothing is the clearance-safe region only.
- Feed/rapid clamping is a well-defined, testable contract (given a machine
  config and a move, the clamped value is a pure function).
- "Smooth but safe" is a stated invariant, not an aspiration.

**Related.** ADR-002 (analytic arcs), ADR-006 (precision), ADR-011 (tool envelope).

---

## ADR-011 — The core is designed 3D-capable from the start; the tool is a tip envelope (Accepted)

**Context.** The immediate goal is 2.5D, but the friend's work and the long-term
value are in 3D milling of organic shapes with varied end mills (flat, ball-nose,
bull-nose, tapered) and flute counts. We want to build 2.5D now *without*
architecting a wall we must later tear down. The user is not a machinist and wants
the software to expose the *right knobs* for surface quality (passes, fineness)
rather than guess.

**Decision.**

1. **The spine is 3D-ready.** The pipeline (geometry → features → planning →
   generation → moves → post-processing) is the same for 2.5D and 3D. 3D adds
   *modules* (surface offset, collision, adaptive stepover), it does not change
   the spine. 2.5D is the degenerate case (Z constant) of 3D, so building 2.5D
   on the 3D shape is free.
2. **The tool is modelled as a tip envelope** — a surface of revolution about the
   spindle axis, described by a radius-as-a-function-of-height `r(z)`. Every
   tip family is a case of this one abstraction:
   - flat end mill: `r(z) = D/2` over the cutting length, else 0;
   - ball-nose: a spherical cap of radius `D/2` + a cylinder;
   - bull-nose: a corner-radius cap + a cylinder;
   - tapered: a cone (`r(z) = r_tip + z·tanθ`).
   The current `Tool` fields (`kind`, `diameter`, `corner_radius`, `taper_angle`)
   already *encode* this envelope; the `tipRadius(z)` helper is a pure, testable
   addition. 2D offset uses `r` at a fixed Z; 3D compensation offsets the surface.
3. **The moves format is 3D-ready.** `Move` already carries 3D points. An arc is
   fully defined by centre + start + end + direction for *any* plane, so the
   format needs no new arc type for 3D. The only 2.5D assumption that exists is
   in the *helpers* (the generic emitter's arc angle math and `Path.Length`),
   which assume the XY plane; generalising them means computing the arc plane from
   the three points. For 3D arcs we will add an explicit plane normal (or a
   stated convention) so CW/CCW is unambiguous — this is a format *extension*, not
   a break.
4. **Surface quality is configuration, not a constant.** The planner exposes the
   machinist's knobs — target scallop height (or stepover), depth-of-cut per pass,
   number of passes, finishing allowance, per-region strategy, steep-angle
   threshold, surface tolerance — rather than hard-coding "how fine" (see
   [surface-quality.md](surface-quality.md)).

**Consequences.**

- 2.5D work is not throwaway; it exercises the same spine, format, and tool model
   that 3D will use.
- "How fine should it go" becomes a defined, testable relationship (e.g. scallop
  height ↔ stepover) instead of a fudge factor.
- Adding 3D is "new modules + a plane field on arcs," not a re-architecture.

**Related.** ADR-001 (canonical frame), ADR-002 (analytic curves), ADR-003 (format).

---

## ADR-012 — Keep-out zones (workholding) are hard safety volumes (Accepted)

**Context.** In a real setup the part sits in stock, held by clamps, a vise,
tabs, or a fixture. The tool must never touch those — a collision with an edge
clamp or a vise jaw destroys the tool, the clamp, and usually the part, and can be
a safety hazard. This is a distinct, first-class concern from "don't cut the
part": it is *additional* geometry the tool must avoid, positioned relative to the
stock/setup. We want to declare such no-go regions once, portably, and have every
downstream stage respect them.

**Decision.**

1. **A keep-out zone is a volume in the canonical WCS** (relative to the
   stock/setup origin) that the **tool envelope** must never enter. Volumes are
   simple, portable solids: box (clamps, vise jaws), cylinder, prism, or a solid
   mesh (complex fixtures). They are declared in a **fixture/workholding config**
   (portable JSON), not hard-coded.
2. **It is a hard constraint, not a soft preference.** A violation is a
   program-level error (the program is rejected / flagged), not a warning. This
   generalises ADR-010's "never over-cut the part" to "never touch any keep-out
   volume (part, fixture, or an explicit no-go region)."
3. **It is enforced in two places:**
   - **Planning** routes around keep-out volumes: safe entry/exit points, retract
     heights above the fixture, adaptive paths that don't dive into a clamp region.
   - **Collision check** (phase 4) verifies the tool envelope at every point of
     the whole program never intersects any keep-out volume or the kept part
     volume. This is a dry-run gate before emission.
4. **The setup is a portable recipe.** A complete, shareable setup is
   `part + stock + fixture(keep-out) + tool + machine`, all portable JSON. This
   is the unit a shop keeps and moves, and the unit the planner and collision
   check consume. The tool and machine configs already exist; the fixture config
   is the new piece.

**Consequences.**

- "Don't hit the clamp" is a declared, portable, verifiable property — not a
  hope.
- The collision check is a well-defined predicate (tool envelope vs. a set of
  volumes) and is independently testable (a toolpath that clips a keep-out box is
  rejected; one that clears it is accepted).
- Adding a clamp to the setup is a data change (a new keep-out volume), not a code
  change (ADR-007).

**Related.** ADR-001 (canonical frame), ADR-010 (never over-cut), ADR-011 (tool
envelope), ADR-007 (config as data).

---

## ADR-013 — 2D offset / cutter compensation is a dedicated, composable, safe-by-construction module (Accepted)

**Context.** Cutter compensation — offsetting a contour by the tool radius to get
the tool-centre path — is the highest-risk, highest-value piece of 2D CAM, and the
place where most CAM 2D bugs live: self-intersecting offsets, degenerate corners,
and (worst) offsets that over-cut the part. It must be isolated, heavily tested,
and *composable* so that "profile with compensation" and "profile without
compensation" are the same generator with different inputs, and so the safe
behaviour is guaranteed by the module, not by every caller.

**Decision.**

1. **A dedicated `comp` module** composes *in front of* the toolpath generators
   (ADR-008). Contract: `(contour, tool radius r, offset direction, join policy)
   → (new contour, diagnostics)`. It is pure geometry on `kernel` types (lines,
   arcs); it does **not** emit moves. The generator consumes the offset contour.
2. **The tool-centre path is the boundary offset by the tool radius `r`.** `r` is
   the tool's tip-envelope radius **at the working Z** (ADR-011): `D/2` for a flat
   end mill, the corner-radius cap for a bull-nose, the spherical cap for a
   ball-nose (in 2D, at a fixed Z this is just a number).
3. **Offset direction** is *outside* (tool-centre path outside the kept boundary
   → **profiling**) or *inside* (tool-centre path inside the boundary → **pocket
   walls**). This is the single most common 2D mistake, so it is an explicit, named
   input — not inferred.
4. **Corner joins:** a sharp boundary corner yields a **mitre** in the centre path
   (the tool is a circle, tangent to both offset edges, leaving a small **cusp** in
   the cut — the standard, correct result). A **mitre limit** caps how far the mitre
   point may extend; beyond it the join becomes a **chamfer**. A **fillet** (an arc
   of radius `r`) is used where the boundary corner is rounded or a rounded centre
   path is desired. Join type is per-operation config with a sensible default.
5. **Safe-by-construction (the core guarantee):** any offset that would
   **self-intersect** or **over-cut** is resolved by **trimming to the
   tool-reachable region — always leaving material, never taking it** (ADR-010).
   A feature the tool cannot reach (too large for the notch/corner) is **reported**
   in the diagnostics ("feature unreachable at this tool radius"), not silently
   over-cut. The module's failure mode is *under-cut* (a residual a smaller tool /
   finish pass removes), never *over-cut*.
6. **Analytic, tested:** offsets of known shapes (offset a square, offset a
   circle, offset a tight notch) have exact analytic results and are asserted to
   tight tolerance. Degenerate cases (collinear edges, zero-length features,
   tool-radius-larger-than-feature) have explicit tests and the safe resolution.

**Consequences.**

- The hardest 2D geometry is one isolated, heavily-tested module with a defined
  safe-failure mode, not a risk smeared across every generator.
- Generators stay simple: they take a contour (already compensated or not) and
  emit moves.
- "Don't over-cut at the corner" is a property of the `comp` module, verified by
  test, independent of which operation uses it.

**Related.** ADR-002 (analytic curves), ADR-006 (robustness), ADR-010 (never
over-cut), ADR-011 (tip envelope), ADR-008 (composable stages).

---

## ADR-014 — Additive kernel/moves/gcode primitives for Phase 2 (Accepted)

**Context.** ADR-013's `comp` module, ADR-012's safety gate, and the Phase 2
planner all need shared 2D curve machinery that the v0.1 kernel does not yet
have: line–arc and arc–arc intersections (the join resolution ADR-013 refers
to as "kernel intersection primitives"), point-in-contour, point-to-curve
distance, and a shared representation of a contour as a chain of lines and
arcs (so import, compensation, and generation consume *one* contour model).
The working agreement says spine changes need an ADR first; this records them
as **additive**.

**Decision.**

1. `kernel` gains (additions only — no existing type, method, or invariant
   changes):
   - `Edge2` — a line segment or a circular arc (a value, not an interface);
   - `Chain2` — an ordered chain of `Edge2`, open or closed, with
     `SignedArea`, `Reversed`, `Len`, `Validate`;
   - `LineArcIntersect`, `ArcArcIntersect` (exact, analytic, finite-domain);
   - `PointInContour` (even-odd; a point on the boundary reports *outside*);
   - `PointToSegmentDist`, `PointToArcDist`, `PointToChainDist`.
2. **An arc is not a full circle.** `Arc2` sweep is in the open interval
   `(0, 2π)`; a full circle is represented as two or more arcs (two halves or
   four quarters). This keeps CW/CCW and start/end unambiguous for every
   consumer (comp, gcode, GUI). Full-circle special cases are a known
   limitation, documented, not a silent branch.
3. `moves` gains `Program` — an ordered list of `Path`s, the unit of a job the
   CLI writes to disk and the GUI will render. Additive only.
4. `gcode` gains `EmitProgram` — render a `Program` as one G-code file with
   per-path setup/teardown and a single program end. Additive only; `Emit`
   (single path) is unchanged.
5. The safety gate for keep-out volumes (ADR-012) is a **consumer package**
   (`internal/safety`), not a spine change: it tests tool-envelope shapes
   (disc / capsule / annular sector) against the fixture's volumes using
   convex-separating-axis tests — analytic, no tessellation (ADR-002).

**Consequences.**

- `comp`, `safety`, `plan`, import, and generation share one contour model, so
  there is exactly one place to get "line vs arc" wrong (kernel), and it is
  tested once.
- The spine diff for Phase 2 is small and additive; ADR-001…013 are untouched.
- The full-circle rule is explicit and testable; consumers never branch on
  "is this almost 2π?".

**Related.** ADR-002 (analytic curves), ADR-006 (robustness), ADR-012 (keep-out
safety), ADR-013 (comp), ADR-003 (moves format).
---

## ADR-015 — Stock is data; its XY section at a height is a pure primitive (Accepted)

**Context.** The part is milled *from* something: a block of material with a
finite top surface. Planning and safety need to know (a) what material exists
at the cutting depth, (b) how high the stock top is at a given XY point (the
plunge must start above it), and (c) that the part actually fits in the stock.
Earlier CAM tools bake the stock into the strategies; that makes strategies
less testable and the stock non-portable. The setup recipe is
`part + stock + fixture + tool + machine` (ADR-012) — stock is the missing
JSON citizen.

**Decision.**

1. **Stock is config data** (Law 6): a portable JSON document describing one
   solid: a **box** (opposite corners in WCS), a **vertical cylinder**
   (centre XY + radius + Z range), or a **prism** (a `kernel.Chain2` footprint
   extruded over a Z range). These are the common cases; a mesh stock is a
   later extension behind the same interface (the `mesh` slice's
   `SectionAt`).
2. **`SectionAt(z)` is a pure, tested primitive:** it returns the closed XY
   chain of the stock's cross-section at height `z` (a box gives a rectangle,
   a cylinder a circle as two half-arcs, a prism its footprint), or an error
   when `z` is outside the stock's Z range. Consumers (planning, safety,
   later roughing) call this one function instead of re-deriving geometry —
   the same single-source-of-truth rule as the kernel contour primitives
   (ADR-014).
3. **Stock never re-frames coordinates.** All stock geometry is in the
   canonical WCS (Z-up, mm, ADR-001); `SectionAt` projects to XY without
   changing anything else.
4. **Fits-check is a consumer of these primitives:** "the part's footprint
   fits in the stock's section at the part's Z, with the stock top above the
   part top" is a small pure check over `SectionAt` + `TopAt`, not a hidden
   assumption inside a generator.

**Consequences.**

- Strategies stay pure: they consume a contour + a radius, and ask the stock
  only "what is the section at z?" — no strategy knows what shape the stock is.
- Adding a stock shape (mesh) is an additive extension of the `Stock` model
  with the same `SectionAt`/`TopAt`/`Contains` surface; nothing downstream
  changes.
- The `generate` command (slice L) validates the recipe (part ⊆ stock, stock
  top above part top, entries above the stock) with these primitives, and
  reports precisely.

**Related.** ADR-001 (one coordinate system), ADR-007 (config is data),
ADR-012 (the setup recipe), ADR-014 (contour primitives).

## ADR-016 — Kernel point types carry the canonical lowercase JSON form (Accepted)

**Context.** Every JSON schema and example in this repository spells a point
as `{"x": …, "y": …, "z": …}` (lowercase, Law 6). But `kernel.Point2` /
`kernel.Point3` are plain Go structs with no JSON tags, so `moves.Path` —
which embeds them (`from`, `to`, `center`) — serialises points as
`{"X": …, "Y": …, "Z": …}`. The intermediate format's Go output therefore
disagrees with its own schema, and any new config type containing points
(e.g. the stock prism footprint, ADR-015) would inherit the same split.
Two point representations is exactly the kind of drifting convention the
JSON- Schema discipline (schemas + examples) exists to prevent.

**Decision.** `kernel.Point2` and `kernel.Point3` get JSON tags for
lowercase `x`, `y`, `z`. This is an additive, non-breaking change to the Go
API (same fields, same values); it only pins the on-disk spelling to the
already-decided schema convention. All other packages inherit the fix:
`moves.Path` output now conforms to `schemas/moves.schema.json`, and
point-bearing config types (stock, later mesh) match their schemas.

**Consequences.**

- One JSON point form repository-wide, enforced by the schemas, exemplified
  by `examples/`, and now produced by the Go core itself.
- Round-trip tests remain valid (they are self-consistent); no code reads or
  writes these JSON keys directly outside `encoding/json`.
- `kernel` is touched deliberately, with this ADR, per AGENTS.md's rule that
  spine changes are decided, not incidental.

**Related.** ADR-007 (config is data), ADR-015 (stock as data).

## ADR-017 — Mesh import is a boundary module; Slice(z) is the clean-case section primitive (Accepted)

**Context.** Phase 2's exit needs real part geometry: STL and 3MF importers
that yield 2D contours at heights (roadmap, phase 2). A *general* robust
planar section of a triangle mesh — coplanar faces, T-junctions,
non-manifold input, sliver triangles — is a 2D polygon-boolean problem, the
exact class of subsystem where open-source CAM tools accumulate their worst
bugs (and where our "floating-point precision" complaints come from). v1
parts are 2.5D, and their contours are obtained exactly by slicing the solid
*between* face heights — the same way a slicer takes 2.5D contours. We want
that case to be exact, tested, and honest about what it does not do.

**Decision.**

1. **`internal/mesh` is a boundary/import module** (Law 1, Law 4): pure
   functions `ParseSTL(data []byte)` and `Parse3MF(data []byte)` over
   in-memory bytes (the CLI does the file I/O, keeping `internal/` pure),
   returning a `Mesh` value already in the canonical WCS (mm). The 3MF
   `unit` attribute is honoured exactly once, at import (millimeter is the
   default; inch/centimeter convert; other units are rejected with a precise
   error). Parsing uses the standard library only (`encoding/xml`,
   `archive/zip` for 3MF) — no new dependencies (ADR-004).
2. **`Mesh` is a triangle soup:** `Vertices []kernel.Point3` + index
   triples, with `Validate()` (finite, in-range, non-degenerate triangles),
   `VertexZs()` (sorted distinct vertex heights), `Bounds()`, and
   `IsClosedMesh()` (each undirected edge in exactly two triangles — the
   watertightness diagnostic the CLI reports).
3. **`Slice(z)` returns the closed intersection loops** of the solid with
   the horizontal plane z, as `[]kernel.Chain2` of line segments, in the
   mesh's own winding (consumers measure `SignedArea`, never assume — ADR-001).
   It is defined for the **clean case**: every triangle is fully above, fully
   below, or straddles the plane with exactly one crossing segment. The
   intersection graph is then 2-regular (each crossing point joins exactly
   two segments) and stitches into closed loops deterministically.
4. **Unsafe input gets a precise, actionable error, never a silent
   contour:** a plane through a vertex or along an edge (within `AbsTol`)
   reports the offending vertex and suggests a valid z from
   `VertexZs()` (e.g. the midpoint of adjacent levels — the documented rule
   for 2.5D contour extraction); a dangling crossing point (non-manifold
   intersection) names the point and the segment count. This is Law 7 in
   import form: the safe failure is refusing to extract, not extracting
   wrong.
5. **Coplanar (face-height) sectioning, T-junctions, and non-manifold
   stitching are v1 limitations**, documented with their precise errors.
   They are a later refinement (or a B-rep import) — not a silent v1
   behaviour.

**Consequences.**

- No polygon-boolean engine in v1: the hard, bug-prone part is deferred
  with a *specified* contract and precise errors instead of hidden wrongness.
- The planner/CLI rule for 2.5D contours is small and testable: *slice at a
  z strictly between adjacent `VertexZs()` levels* (midpoint rule), which is
  exact for prismatic parts and what real slicers do.
- Import is a swappable boundary module: STL/3MF/STEP can come and go
  without touching the spine; `Mesh` and `Slice` are the stable seam for
  the part/planner composition (slice K) and for a future mesh stock behind
  the same surface (ADR-015).
- 3MF's `unit` handling pins the one-legal-place-for-units rule (ADR-001):
  conversion at import, never later.

**Related.** ADR-001 (one coordinate system, units once at the boundary),
ADR-004 (no deps in internal/), ADR-015 (mesh stock behind the same
surface), ADR-016 (point JSON form), roadmap phase 2 (part import).

## ADR-018 — The part is a recipe over a mesh; feature kind follows nesting parity; the first-cut planner is a thin composition (Accepted)

**Context.** Phase 2's exit criterion is: import a real STL part, plan its 2D
features, offset with a real tool, apply the correct milling mode, route
around keep-out volumes, and emit G-code. The geometric pieces now exist and
are tested: `mesh.Slice(z)` (ADR-017), `stock` (ADR-015), `comp.Offset`
(ADR-013), `planning.PlanFeature` (ADR-009), `toolpath.Loop`, and
`safety.CheckPath` (ADR-012). What is missing is (a) the *part* model — what
to cut and where, (b) the classification of the section loops into features,
(c) the composition of those pieces into verified per-feature paths with one
coherent Z model, and (d) the contract the program layer (slice L) will build
on.

**Decision.**

1. **`Part` is a portable JSON recipe over an in-memory mesh** (Law 6): `id`,
   `name`, `mesh` (the mesh file — resolved by the CLI; the core never does
   I/O), `slice_z` (optional; default: midpoint of the two topmost distinct
   `VertexZs` levels — the documented 2.5D slicing rule, ADR-017), and
   `cut_z` (optional; default: the part's minimum Z, i.e. a cut through the
   part). A recipe of just `id` + `mesh` is complete for a prismatic part.
2. **Feature kind follows nesting parity.** The section loops at `slice_z`
   nest by containment: a loop's parent is the innermost loop that contains
   it; outermost loops have no parent. A loop's kind is set by the parity of
   its nesting depth: **even depth (0, 2, …) is a material contour**
   (`planning.Profile`: kept material inside, tool outside, `comp.Outside`),
   **odd depth (1, 3, …) is a void contour** (`planning.Pocket`: kept material
   outside, tool inside, `comp.Inside`). One rule covers profiles, holes, and
   islands uniformly, and it agrees with the ADR-009 climb predicate (both
   kinds mill in the requested mode with the matching traversal direction).
   Loops whose edges intersect are refused: the section of a valid solid is a
   disjoint, nested set, and overlapping loops make the kept material
   ambiguous — milling both would over-cut (Law 7).
3. **The first-cut planner is a thin composition.** `planner.Plan` maps
   `part + mesh + stock + fixture + tool + machine + process` to an ordered,
   verified list of paths, and per feature runs exactly: `comp.Offset`
   (direction by kind) → per chain `planning.PlanFeature` (mode + entry) →
   `toolpath.Loop` → `safety.CheckPath`. The planner re-derives no geometry:
   it owns the Z model, the feature order, and process validation, and
   delegates every geometric decision to its tested module (ADR-008).
4. **The Z model (v1, single-depth).** The cut is at `cut_z` (default: part
   bottom); the tool's flute covers the depth — checked against
   `flute_length` when specified (a short flute would leave the top of the
   wall uncut: a precise refusal, not a guess). Rapids are at
   `safe_z = stock top + safe_clearance` (default 5 mm). The plunge column —
   the entry XY through `[cut_z, safe_z]` — must clear every keep-out active
   anywhere in that range: `planning` gains `PickStartBetween(zLo, zHi)`
   (Law 7 at the entry: a clamp floating above the cutting plane is a real
   collision, and the old single-height check missed it). Linking rapids
   between features are a **program-layer** concern (slice L: `moves.Program`
   materializes them and verifies them with `safety`); every planner path is
   self-contained (plunge → cutting loop → retract) and verified on its own.
5. **Process values are explicit, checked, and refused — not clamped.**
   Feed, spindle, mode, and coolant are planner inputs (the resolved process;
   the CLI resolves them from the tool crib and flags). Feed must lie in the
   machine's `[min_feed, max_feed]`, and spindle must not exceed
   `spindle_max_rpm` or the tool's `max_rpm` (where non-zero): out of range is
   a precise refusal. Feed clamping/scheduling is phase 4 (ADR-010) and will
   replace these checks with the scheduler, not contradict them.
6. **A fully unreachable feature is a hard refusal; a partial drop is a
   diagnostic.** If `comp.Offset` drops a feature entirely (tool too large),
   `Plan` refuses, naming the feature, the tool, and the radius — the part
cannot be made with this tool. If it drops part of a feature (a narrow
corner), the remaining chains are milled and the drop is surfaced as a
diagnostic: residual material is the safe side (ADR-013).

**Consequences.**

- The part contract is small and stable: a recipe over a mesh plus two Z
  values with documented defaults. Multi-depth roughing (phase 3) is added as
  a pass structure over the same features — the part/feature contract does
  not change.
- Islands (depth-2 material contours) work in v1 with zero special-casing,
  and the parity rule extends to any nesting depth, so the 3D era does not
  invalidate it.
- `planner` is the one package that fans in over the pipeline; it is small,
  stateless, and its output is a value (`Result`) that both the program layer
  (slice L) and the future GUI can consume.
- The entry-safety fix (plunge column vs. keep-out Z range) closes a real
  gap in the ADR-012 gate: a clamp floating above the cutting plane no longer
  survives a plunge through it.

**Related.** ADR-008 (composable stages), ADR-009 (climb predicate),
ADR-012 (keep-outs), ADR-013 (comp), ADR-015 (stock), ADR-017 (mesh/Slice),
roadmap phase 2 (operation planner, first cut).

## ADR-019 — The program layer: linking rapids are first-class paths; fixture mapping is data (Accepted)

**Context.** Slice L completes phase 2: `moves.Program` + `gcode.EmitProgram`
(anticipated in ADR-014) and the `generate` CLI. The planner's paths are
self-contained (plunge → cutting loop → retract, ADR-018 §4). What is left
to decide: how paths group into one program, how the machine travels between
features, how the fixture JSON (schema already exists) becomes the planner's
keep-outs, and what the CLI surface looks like.

**Decision.**

1. **`moves.Program` is an ordered group of paths:** `id`, `name`, `paths`,
   `metadata`. `Validate()` (Law 5): non-empty ID, at least one path, each
   path valid, and **unique path IDs** — paths are addressable objects the
   user inspects and overrides (ADR-003) and the future GUI selects.
2. **The linking rapid between consecutive features is a standalone
   one-move path** (a *link* path), inserted between the two feature paths
   by `planner.ToProgram`. The program is therefore a flat, uniform list of
   paths, and the emitter simply concatenates them in order — no special
   linking logic in `gcode`. The link is visible in `.moves.json` (and
   overridable there, like any move) and is verified against the keep-outs
   at construction time with `safety` — the safe failure is a refusal naming
   the zones, never a rapid through a clamp. A zero-length link (consecutive
   features sharing an entry) is omitted, not emitted.
3. **`internal/fixture` is the portable workholding model** (the existing
   `fixture.schema.json` is its contract, Law 6) with
   `KeepOuts() → []planning.KeepOut`. Boxes map directly. Cylinders map iff
   vertical (axis ±Z, base centre + length → Z range). Anything else — a
   non-vertical axis, a mesh volume — is a precise refusal: planning against
   a zone the gate cannot verify is a Law 7 violation, and the safe side is
   to say so and stop.
4. **`gcode.EmitProgram` is one header + the paths in order + one footer:**
   the header carries the program's spindle (the max across paths) and
   coolant (if any path wants it); each path emits with a comment; the
   footer turns coolant and spindle off and ends the program. `Emit`
   (single path) stays for the v0.1 surface.
5. **The CLI `generate`** is the one place I/O lives (Law 4):
   `--part --stock --fixture --tool [--tool-id] --machine [--feed 500
   --spindle 8000 --coolant off --milling climb] [--out … --nc …]`. It
   loads the JSON (part recipe, mesh file, stock, fixture, tool library or
   single tool, machine), runs `planner.Plan`, builds and verifies the
   program, and writes `.moves.json` (indented, the inspectable
   intermediate format) plus `.nc` (the machine's post-processor codes from
   its JSON). It prints a short summary — the Z model, the features, any
   diagnostics — and warns (without failing) when the part mesh is not a
   closed 2-manifold. Feed and spindle are explicit in v1; resolving them
   from the tool's `feeds[]` per material (the ProcessTable) is phase 3.

**Consequences.**

- The program is data the user can see end to end: every move, including
  the travels between features, is a first-class, overridable object in
  `.moves.json` — the "override per-feature decisions" request (ADR-003)
  is satisfied at the program level now, without a new format.
- The emitter stays trivially reviewable: header, paths in order, footer.
  Machine dialect lives only in the Options (from the machine JSON).
- Fixture mapping is a small, schema-backed, fully tested data function —
  adding a zone shape to v1 is a mapping change, not a pipeline change.
- The CLI is the only place that reads or writes a file in the project.

**Related.** ADR-003 (the seam), ADR-007 (config is data), ADR-012
(keep-outs), ADR-014 (program primitives), ADR-018 §4 (linking rapids are
the program layer's job), roadmap phase 2 (CLI generate, slice L).

---

## ADR-020 — Program start: the emitter pre-positions at safe Z before the first move (Accepted)

**Context.** A program's first move is a plunge (a vertical move from safe Z
to cut Z at the entry point). But the machine's position at program start is
*unknown* — the operator homed it, the core does not model home. The current
emitter just emits the plunge, so the machine rapids diagonally from wherever
it is (possibly over the part or a clamp) straight down to cut depth. That is
exactly the "ramping in over the part" failure the user observed. The fix
must benefit every producer (the planner, the `demo` command, and a user-edited
`.moves.json`) in one place.

**Decision.** The machine position at program start is the operator's
responsibility, and the emitter makes the first move safe: **before emitting
the first move of a program, the emitter emits a rapid to that move's
`From`** (skipped when `From == To`). Producers set a plunge's `From` to the
entry point at safe Z, so this pre-positioning is a safe-Z move; the plunge
then descends vertically. This is program *framing* (like the header and
footer), so it lives in the emitter, not in each producer, and the `moves`
format is unchanged. The producer contract — *the first move of the first
path is a plunge whose `From` is at safe Z* — is documented in gcode.md.

**Consequences.**

- One place owns program start; the planner, `demo`, and hand-edited programs
  all get the safe pre-positioning automatically.
- The intermediate format stays minimal (no synthetic move added); the
  pre-positioning is a rendering concern, consistent with ADR-002's
  "formatting is post-processing" stance.
- A GUI replaying `moves` applies the same rule (rapid to the first move's
  `From` first) so the preview matches the emitted G-code.

**Related.** ADR-002 (post-processing concerns), ADR-019 (program framing),
ADR-010 (safety), gcode.md (emission model).

---

## ADR-021 — Feature kind follows the region's material Z, not just nesting parity (Supersedes part of ADR-018 §2)

**Context.** ADR-018 classifies a section loop by nesting-parity: even depth
is a material contour (profile), odd depth a void (pocket). That is wrong for
a *raised boss*: a cylinder standing on a base, sectioned at the base's top,
yields the base square (depth 0) and the cylinder's circle (depth 1). Parity
calls the circle a *pocket to cut* — but it is *kept material* (the cylinder
rises above the slice plane). Milling it would destroy the part. The correct
signal is the **Z relationship between the region's material and the slice
plane**: a region with material *above* the slice plane is a boss (kept); a
region with no material above it is a void (cuttable). Parity is a special
case of this rule (a through-hole has no material above, so it is a void).

**Decision.** `part.Detect` classifies each section loop as:

- **Profile** — the outermost loop (depth 0): the part's boundary, milled on
  the outside.
- **Boss** — a nested loop (depth > 0) whose region has material *above* the
  slice plane (region top Z > slice Z). It is **kept**; it is never milled and
  is a hard obstacle (keep-out) within its parent pocket region.
- **Pocket** — a nested loop (depth > 0) with no material above the slice
  plane. It is a void: cuttable in profile mode, or part of the cleared
  region in pocket mode.

The "material above the slice plane" test is the new pure primitive
`mesh.RegionTopZ(outer, inners) (topZ, has, err)`: the highest material Z
within the region (outer minus the inners), over the mesh triangles whose
centroid lies in the region. `has` is false when the region is a void. A
region is a boss iff `has && topZ > sliceZ + tol`.

**Consequences.**

- The cylinder-on-base is classified correctly: square = Profile, circle =
  Boss. The planner never mills the boss; it clears *around* it.
- ADR-018's parity rule is preserved as the special case (a through-hole has
  no material above the slice plane → Pocket), so the plate example is
  unchanged.
- `planning.FeatureKind` gains `Boss`; the planner treats a boss as a keep-out
  obstacle, not a feature to cut (Law 7: never over-cut).

**Related.** ADR-018 (refined), ADR-012 (keep-outs), ADR-017 (mesh boundary),
`mesh.RegionTopZ`, part.md (the part model).

---

## ADR-022 — Pocket clearing: a region, clipped rows, constant engagement, constant chip-load feed (Accepted)

**Context.** "Mill out all that isn't the cylinder on the top layer" is an
*area* operation: remove the material in a **region** (the base square minus
the cylinder) down to a depth. The existing `toolpath.Loop` only mills a
*contour*. Phase 3 adds the area operation with the friend's
"trochoidal / adaptive / constant chip-load / constant engagement" requests
made concrete and testable (operations.md, tool-parameters.md).

**Decision.** A **pocket** is a region cleared down to `cut_z`:

- **The region** is `outer` (the Profile boundary) minus the `bosses` (the
  kept loops, ADR-021). Void loops are part of the cleared region. The tool
  centre must stay inside `outer` and outside every boss.
- **The path** is a **serpentine (trochoidal) raster**: rows at a constant
  *stepover* `s`, clipped to the region, traversed in alternating directions,
  joined by **semicircular end arcs** of radius `s/2` (the trochoidal
  element — arcs, not polyline corners, so the path is smooth by
  construction, ADR-002).
- **Clipping** is analytic: a row (a straight line) is intersected with the
  outer loop and with each boss (kernel `LineLineIntersect` /
  `LineArcIntersect`), the x-extents are collected as intervals, and the
  clearable intervals are *outer minus bosses* (interval subtraction). No
  tessellation, no general polygon booleans.
- **Constant engagement:** the radial engagement of a row at stepover `s` is
  the exact lens width `a_e = R − sqrt(R² − (s/2)²)` (≈ s²/2R for small s).
  A constant stepover gives a constant `a_e` — the "constant cutter
  engagement" request.
- **Constant chip-load feed:** each cutting move's feed is
  `F(a_e) = F_full · sqrt(a_e / D)` (the tool-parameters.md model, D = 2R).
  The geometry module emits the path *and* the per-move `a_e` (in the move's
  metadata); the feed is derived from it. Both halves are independently
  tested.
- **Safety:** the whole path is run through `safety.CheckPath` against the
  bosses (as keep-outs) and the fixture keep-outs. A boss the tool would
  touch is a refusal, not a collision.

**Consequences.**

- The cylinder-on-base mills: the annulus is cleared, the cylinder is kept,
  the outer square is the limit — with constant engagement and chip-load.
- The engagement and feed models are pure, tested functions (the
  "testable pieces of math" principle), separate from the path geometry.
- A straight-row raster with arc ends is the v1 trochoid; a true epi/hypo-
  cycloid is a refinement that reuses the same engagement + feed model.

**Related.** ADR-021 (the region), ADR-013 (comp/safety), ADR-002 (arcs),
operations.md (the strategies), tool-parameters.md (the feed model).

---

## ADR-023 — Lead-in / lead-out is a user-definable entry segment, safely bounded (Accepted)

**Context.** The friend wants "a user-definable lead-in on the entry to the
cutting paths, so the cutter can start at a specified XY coordinate outside
the actual area to be cut. (If the lead-in isn't specified, pick a safe
default.)" A lead-in lets the tool enter at full speed and build chip load
gradually, instead of starting the cut dead-on at the first corner.

**Decision.** A **lead-in** is an extra entry segment before the first
cutting move, on the *material-removal side* of the entry edge (so it never
cuts kept material). It is a per-part setting (portable JSON, the part
recipe) with a CLI override:

- **Kind:** `straight` (extend the entry edge's line backward by the length
  — the friend's "start at a specified XY outside the area") or `tangent`
  (an arc tangent to the entry edge, bulging onto the removal side).
- **Length:** the lead-in length in mm.
- **Default (unspecified):** a **tangent** lead-in of length
  `min(tool radius, 3 mm)` — short enough to be safe in a tight feature,
  long enough to smooth the entry.
- **Safe fallback:** the lead-in is checked against the other loops (kept
  material and other voids). If the requested kind/length does not fit, it
  degrades `tangent → straight → none`, and a diagnostic is recorded. It
  never over-cuts (Law 7); the safe outcome is a shorter or absent lead-in,
  said so.

The lead-in is a *generation* concern (it changes the tool-centre path
before the first edge), so it is applied in the toolpath layer, driven by a
planning parameter. It is a cutting move (it carries the milling mode and
feed), not a rapid.

**Consequences.**

- The entry is visible and overridable in the recipe and the intermediate
  format (ADR-003/005), and the safe default means a part with no lead-in set
  still gets a smooth, safe entry.
- The fallback chain is a safe-side degradation with a diagnostic, consistent
  with ADR-013's "residual + report" philosophy.

**Related.** ADR-003 (decisions are data), ADR-005 (milling mode), ADR-013
(safe degradation), part.md (the recipe), toolpath (application).

---

## ADR-024 — Stock is optional (a virtual bounding-box default) and feed/speed resolve from the tool's feeds[] (Accepted)

**Context.** Two requests. (1) "It would be nice to be able to run without
specifying the stock (especially for quick test runs)." Today `--stock` is
required. (2) ADR-019 §5 left feed/spindle as explicit CLI values; the
roadmap's Phase 3 resolves them from the tool's `feeds[]` (the
ProcessTable, tool-parameters.md).

**Decision.**

- **Optional stock:** when no stock is given, the planner uses a **virtual
  stock = the part's bounding box expanded by a 5 mm margin in XY** (Z from
  the part's min to max Z). This keeps the safety machinery intact (the fit
  gate is trivially satisfied, `safe_z` = part max Z + clearance, the Z-floor
  is the part min Z) while removing the stock file from a quick run. The
  virtual stock is a *convenience default*, not a claim about the real
  billet; a real stock file still overrides it and is required for
  production runs where the stock top/footprint matters. The planner says in
  its summary when the virtual stock is in effect.
- **Feed/speed resolution:** the planner resolves the cutting feed and
  spindle from the tool's `feeds[]` for the workpiece material, using the
  tool-parameters.md model (`n = vc·1000/(π·D)`, `F = n·flutes·fz`, clamped
  to `n ≤ max_rpm` and the machine's feed/spindle limits). An explicit CLI
  `--feed` / `--spindle` overrides the resolved values. When neither a CLI
  value nor a matching `feeds[]` entry exists, the planner **refuses** with a
  precise message (the safe failure is no program, not a guessed feed).

**Consequences.**

- A quick test run is `cleancuts generate --part … --tool … --machine …`
  (no `--stock`, no `--feed`/`--spindle` when the tool has `feeds[]`).
- Feed/speed become *visible, overridable, material-aware* (ADR-003/005) and
  the "refuse, don't clamp" stance (ADR-018 §5) is preserved.

**Related.** ADR-007 (config is data), ADR-018 §5 (refuse not clamp),
ADR-019 §5 (the CLI), tool-parameters.md (the feed model).

---

## ADR-025 — Face sections: a slice on a face height selects that face's boundary (Refines ADR-017 §5, ADR-018 §2)

**Context.** ADR-017 makes `Slice(z)` exact for the *clean case* — a plane
strictly between vertex heights — and refuses coplanar planes with a precise
error. That defers the *face section* (a plane coinciding with a face), which
is exactly the section a pocket needs: the Testmesh (a Ø25 cylinder on a
50×50 base) has vertex heights −20, −10, 0; the band sections are the square
only (z∈(−20,−10)) and the Ø25 circle only (z∈(−10,0)). The section that
sees *both* loops — the base-top face at z=−10 (a square with the circle as a
hole) — is coplanar, and therefore refused today. "Mill out everything on the
base top except the cylinder" (the friend's part) is unexpressible without it.
The concept is coherent: the section at height h is the boundary of
(plane h ∩ solid) — for a band height that is the cross-section boundary
(what `Slice` returns); for a face height it is the boundary of the face
itself. ADR-017 deferred the *general* coplanar case (arbitrary polygon
unions); the **prismatic** case (vertical walls — boxes, prisms, cylinders,
the common 2.5D parts) needs no boolean engine: the face's boundary is the
set of its edges that belong to exactly one face triangle.

**Decision.** `mesh` gains a second, sibling section primitive:

- **`FaceLoops(h) ([]Chain2, error)`** — the boundary loops of the union of
  the solid's **upward-facing horizontal faces at height h**. A face triangle
  is a triangle whose three vertices all lie at z=h (within `AbsTol`) and
  whose normal points +Z (a horizontal triangle's normal is exactly ±Z, so
  the test is exact). The face boundary is extracted by **edge pairing**:
  an undirected edge (exact float key, as in `Slice`) that belongs to
  exactly one face triangle is a boundary edge; each boundary vertex joins
  exactly two boundary edges and the edges stitch into closed loops. A
  vertex joining a different number of boundary edges, or a dangling
  boundary edge, is a **precise refusal** (non-manifold face complex — the
  ADR-017 stance: never extract wrong). The result loops are in the mesh's
  own winding (consumers measure `SignedArea`; never assume).
- **Dispatch lives in `part`:** `slice_z` strictly between adjacent vertex
  heights → band section (`Slice`, unchanged); on a vertex height → face
  section (`FaceLoops`). `EffectiveSliceZ` accepts a vertex height now
  (it still refuses a height outside the part's Z range). The midpoint
  default rule is unchanged.
- **Downward-facing-only heights are refused:** a height where the solid has
  no upward-facing face (e.g. the Testmesh bottom, z=−20) yields an empty
  face set → a precise error naming the height and the vertex levels.

**Consequences.**

- The Testmesh section at z=−10 is {50×50 square, Ø25 289-gon}; the circle
  classifies as a **Boss** (ADR-021: material rises above the section) and
  the annulus is the clearable region (ADR-022) — the friend's part becomes
  millable.
- `Slice` and its ADR-017 contract are untouched; the two primitives sit
  side by side (`band` vs. `face`), and the general coplanar case (sloped
  faces, T-junctions at the face, non-manifold input) is still the
  documented refusal — not a silent v1 behaviour.
- Face sections are pure, testable, and exact for clean meshes (Law 4, Law 5,
  Law 7): no new dependencies, no resampling, safe failure by refusal.
- A face section with a *single* loop (the Testmesh top, z=0 → the Ø25
  circle) is a valid section: profile mode mills it as a contour; pocket
  mode clears it (a facing operation — see operations.md).

**Related.** ADR-017 (refined — the coplanar deferral now has a prismatic
case), ADR-018 §2 (the section model), ADR-021 (the classification that
consumes it), mesh.md (the primitive's spec), part.md (the dispatch rule).

---

## ADR-026 — Pocket traversal: rows, disjoint intervals, lifts, and the row-join arc (Refines ADR-022)

**Context.** ADR-022 decides the pocket is a serpentine raster of rows at
stepover `s`, clipped to the region, with "semicircular end arcs of radius
`s/2`". Two refinements fall out of implementing it against real regions
(clipped rows have different extents; a boss can split a row into two
disjoint intervals):

1. A semicircle of radius `s/2` fits a join only when the chord between
   row i's end and row i+1's start is exactly `s` — i.e. only when the two
   rows have identical extents. Clipped rows do not.
2. A boss (kept material) in the middle of a row leaves two disjoint
   clearable intervals; the gap between them is *over kept material* (or
   outside the region), so no cutting move may bridge it.

**Decision.** The pocket path (one per region — the depth-0 loop minus its
bosses) is built as:

- **Rows:** horizontal lines at `y = yMin + s/2 + i·s` across the offset
  region's bounding box. A row's **clearable intervals** are
  (row ∩ offsetOuter) − (row ∩ ⋃ offsetBoss), as sorted disjoint intervals
  (analytic line/edge intersection + interval subtraction — ADR-022).
  Intervals shorter than a sliver threshold (0.1 mm) are dropped with a
  diagnostic (residual material — the safe side, ADR-013).
- **Traversal:** rows are visited bottom to top; row *i* is traversed
  left→right for even *i*, right→left for odd *i* (the classic serpentine).
  A row with several intervals traverses them in that direction's order,
  with a **lift** between consecutive intervals of the same row (retract to
  SafeZ, rapid across, plunge at the next interval's start) — the gap is
  never cut through (Law 7).
- **Row join:** between row *i*'s last interval end P₁ and row *i+1*'s
  first interval start P₂ (when they are on the same side of the region —
  i.e. the previous end and the next start are the *nearest* interval ends),
  the join is a **semicircle with diameter P₁P₂** (radius |P₁P₂|/2 — this
  reduces to `s/2` when the two rows have equal extent, ADR-022's original
  form). Of the two semicircles on P₁P₂, the one whose centre lies inside
  the offset region is preferred; the chosen arc is gate-checked (no
  intersection with any boss loop, clearance ≥ tool radius — the `safety`
  loop-clearance helpers). If no join arc passes the gate, the join
  degrades to a **lift** (the safe fallback, with a diagnostic).
- **Plunges:** every interval that starts after a lift (and the first
  interval of the path) begins with a vertical rapid plunge at its start
  (SafeZ → CutZ). The first move of the whole path is such a plunge — the
  ADR-020 producer contract (the emitter pre-positions there). A pocket
  entry point always sits over the face (air above it, by the face-section
  property), never over a boss (intervals are clipped outside the boss
  offset) — so every plunge is a plunge into the cut face, never into kept
  material.
- **Engagement & feed (unchanged from ADR-022, now fully specified):**
  `a_e = R − √(R² − (s/2)²)` for every cutting move (rows and joins); the
  cutting feed is `F · √(a_e/D)` (underfeeding the joins — the safe side).
  Both are pure functions in `toolpath`, independently tested.

**Consequences.**

- The Testmesh annulus (one boss splitting the middle rows into two
  intervals) yields one coherent path: a serpentine that lifts across the
  boss where the boss is present and joins with arcs where the region is
  open — smooth (arcs, ADR-002), safe (gate-checked, Law 7), and fully
  analytic.
- "Facing" needs no separate operation: facing a face is a pocket of a face
  region with no bosses (the Testmesh top at z=0 is exactly that).
- The planner's per-feature composition (ADR-018 §3) gains one branch:
  `operation: pocket` maps a depth-0 feature to
  `comp.Offset(outer, inside, R)` + `comp.Offset(boss, outside, R)` per boss
  + `toolpath.Pocket` + the fixture keep-out gate. Profile mode is
  unchanged except that **Boss features are never milled** (ADR-021) and a
  void loop nested inside a boss is skipped (kept material — Law 7).

**Related.** ADR-022 (refined), ADR-021 (the boss keep-out), ADR-020 (the
plunge contract), ADR-013 (safe degradation), ADR-012 (the fixture gate),
operations.md (the full model), toolpath (the implementation).
