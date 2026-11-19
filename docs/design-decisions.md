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
