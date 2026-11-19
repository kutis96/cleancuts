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