# Architecture

This document describes how CleanCuts is structured, how data flows through it,
and how to extend it. The *why* lives in [design-decisions.md](design-decisions.md);
this is the *what* and *how*.

## The pipeline

CleanCuts is a pipeline of **composable, independently testable stages** (ADR-008).
Every stage has a narrow input/output contract and communicates through the
shared value types in `internal/kernel` and `internal/moves`.

```
   config (tool / machine JSON)
        │                          ▲                        ▲
        ▼                          │                        │
 part ─► features ─► PLANNING ─► GENERATION ─► MOVES ─► POST-PROCESS ─► G-code
 stock           (operation,      (emits      (format)   (generic +
 tool            strategy,         moves       │          machine-
                       mode,       in the      │          specific)
                       direction,  planned)
                       pass list)
                          ▲                        │
                          │                        │
                    (cutter compensation composes here, phase 2)
```

Read it as data flowing left to right. The tool and machine JSON config feeds the
planner (which picks operation, strategy, milling mode, direction, and passes) and
the post-processor (which applies the dialect). Cutter compensation composes in
front of generation (phase 2). Nothing in the pipeline carries mutable global
state.

### Stage 1 — Geometry & features

- **`internal/kernel`** is the dependency-free geometry and math core. It defines
  `Vec2/Point2`, `Vec3/Point3`, `Line2`, `Arc2`, intersection predicates, and the
  tolerance model (`kernel.AbsTol`). It has **no dependencies** and **no
  knowledge of CAM** — it is a pure, tested geometry library.
- **Feature planning** (detecting pockets, profiles, faces; deciding which
  operation applies where) is a stage that will sit on top of the kernel plus the
  imported part/stock geometry. In v0.1 the "feature" is implicit (the caller
  supplies the contour), so this stage is not yet a separate package. See the
  roadmap for when it becomes one.

### Stage 1.5 — Planning (operation, strategy, milling mode, direction, passes)

The **planner** runs *before* generation and makes the per-feature decisions that
generation then executes (ADR-009):

- **Operation & strategy:** which feature gets which strategy (profile, pocket,
  face, adaptive/trochoidal, 3D finishing).
- **Milling mode:** climb or conventional per feature (default + overrides), and
  — crucially — the **traversal direction / contour orientation** that achieves
  it. This is decided *here*, because it is an input to generation, not a label on
  the output (ADR-005, ADR-009).
- **Pass structure:** rough → (semi-finish) → finish, depth-of-cut per pass,
  finishing allowance, and the per-region strategy for 3D (see
  [surface-quality.md](surface-quality.md)).
- **Feed/speed intent:** resolve the tool's `feeds[]` + `max_rpm` into target
  spindle/feed, to be clamped by the machine limits (ADR-010).

**Phase 2 (in progress):** the dedicated planner package
**`internal/planning`** implements the milling-mode half of this stage: it
derives the climb/conventional decision from feature kind + spindle rotation
(`spindle_rotation` in the machine config), orients the contour to achieve it,
and picks a start vertex outside the keep-out zones (hard constraint). Its
model is in [planning.md](planning.md). Still to come: strategy selection,
pass structure, feed/speed resolution, and entry/approach vectors.

### Stage 2 — Toolpath generation

- **`internal/toolpath`** turns geometry into `moves.Path`. **`Loop`** is the
  core generator: any `kernel.Chain2` (lines *and* arcs) → plunge, one analytic
  move per edge (arcs stay analytic; the emitter decides G2/G3 vs. tessellation),
  retract. **`Profile`** is `Loop` over a polygon chain. Later phases add
  pocket, facing, adaptive/trochoidal, and 3D strategies, each as its own
  function or module producing `moves.Path`.
- **`internal/comp`** (cutter compensation) offsets a contour by the tool
  radius with correct inside/outside semantics, corner joins (mitre / chamfer /
  fillet), and safe resolution of too-narrow features (drop + report, or refuse
  with `ErrTooLarge` — never over-cut; ADR-013). It composes in front of the
  generators as a separate, tested module — specified in
  [cutter-compensation.md](cutter-compensation.md). Keeping it separate means
  "profile with offset" and "profile without offset" are the same generator
  with different inputs.

### Stage 3 — The interchange format

- **`internal/moves`** is the machine-independent toolpath format (ADR-003):
  `Move` (rapid / linear / arc, with feed, spindle, coolant, milling mode) and
  `Path` (one continuous chain). It enforces its own invariants (`Validate`) and
  serialises to stable JSON. This is the **seam** between "what to cut" and "how
  to tell a machine to cut it."

### Stage 4 — Post-processing & emission

- **`internal/gcode`** is the generic, dialect-neutral G-code emitter (G0/G1/G2/G3
  + S/M codes), with optional arc tessellation for controllers without reliable
  G2/G3.
- **Machine-specific** post-processors (GRBL, LinuxCNC, Fanuc, …) will read the
  **`internal/machine`** config and vary only the dialect/framing — never the
  geometry. They compose over the generic emitter rather than replacing it.

### Configuration

- **`internal/tool`** — portable tool model + library (JSON). See
  [tool-parameters.md](tool-parameters.md).
- **`internal/machine`** — portable machine + post-processor config (JSON). See
  [gcode.md](gcode.md).
- **fixture / workholding** — keep-out zones (clamps, vise jaws, no-go regions)
  the tool must never touch (ADR-012). See [workholding.md](workholding.md).

### Safety gate

- **`internal/safety`** verifies the tool *envelope* (tip-disc sweep: capsule
  for lines, annular sector for arcs) against the keep-out footprints, with
  exact analytic distances. `CheckChain` gates a planned tool-centre chain;
  `CheckPath` gates a finished `moves.Path` (rapids and plunges included).
  A touch is a collision; the safe failure is a precise refusal. See
  [workholding.md](workholding.md).

A complete, shareable **setup is a recipe**: `part + stock + fixture + tool +
machine`, all portable JSON. That recipe is the unit the planner and the
collision check consume, and the unit a shop keeps and moves.

## Data flow, concretely

A v0.1 program does the following, and each step is independently testable:

1. The caller defines a part contour (`[]kernel.Point2`) and a `toolpath.ProfileParams`.
2. `toolpath.Profile` builds a `moves.Path`: one rapid plunge, one linear move
   per edge (all at `CutZ`), one rapid retract. It validates the result before
   returning.
3. `gcode.Emit` reads the `moves.Path`, validates it again (defence in depth), and
   renders G-code: spindle/coolant setup, the moves (G2/G3 arcs or tessellated G1),
   and program-end codes.

The same `moves.Path` could instead be:
- serialised to JSON and inspected/edited by a user, or
- fed to a machine-specific post-processor, or
- rendered by the (future) GUI.

That single fact — *one format, many consumers* — is the architectural centrepiece.

## Package contracts

Each package exposes a small, deliberate public surface. The rule: **a package
depends only on `kernel` and, at most, the formats it produces/consumes.** There
are no cycles and no "god packages."

```
        ┌─────────────────────────────┐
        │          kernel             │   (no internal deps)
        └─────────────────────────────┘
              ▲      ▲      ▲
              │      │      │
        ┌─────────┐ ┌────────┐ ┌────────────┐
        │  moves  │ │ tool   │ │  machine   │   (depend on kernel only)
        └─────────┘ └────────┘ └────────────┘
              ▲
              │
        ┌──────────────────────────┐
        │  toolpath                │   (produces moves)
        └──────────────────────────┘
              ▲
              │
        ┌──────────────────────────┐
        │  gcode                   │   (consumes moves)
        └──────────────────────────┘
              ▲
              │
        ┌──────────────────────────┐
        │  cmd/cleancuts           │   (CLI)
        └──────────────────────────┘
```

## Error-handling contract

- Errors are **precise and non-misleading**: they name the package, the object,
  and the offending value, e.g.
  `tool "t1": corner radius 6 exceeds radius 5`.
- Generators **validate their output** before returning, so an invalid state
  fails at the point it is created, not deep in a consumer.
- Consumers re-validate their inputs (defence in depth) so a bad value is caught
  at the boundary of the package that would otherwise misbehave.
- Errors wrap (`%w`) so the full chain is available when needed, but the message
  a human reads is the outermost, most specific one.

## How to add a feature

Adding a new toolpath strategy (e.g. a 2D pocket):

1. **Create** `internal/toolpath/pocket.go` (or a subpackage). It takes geometry
   + params and returns `moves.Path` (+ `error`). It must call
   `Path.Validate()` before returning.
2. **Test** it table-driven: valid inputs → expected moves; invalid inputs →
   precise errors; boundary cases (zero depth, single pass, etc.).
3. **Document** it in [operations.md](operations.md) and add it to the roadmap.
4. **Do not** change `kernel`, `moves`, or `gcode` to fit your feature. If you
   find you need to, write an ADR first — that is a signal the abstraction
   boundary is in the wrong place.

Adding a new machine post-processor:

1. Read the **`internal/machine`** config.
2. Compose over `gcode.Emit` (vary dialect/framing from the config).
3. Add an example machine JSON under `examples/machines/`.
4. Add a round-trip + emission test.

## Testing strategy

- **Table-driven, self-documenting tests** in the same package (white-box where
  it helps, black-box for public contracts).
- **Analytic assertions** where possible (arc length, intersection points) to
  tight tolerance, since we do not resample (ADR-002, ADR-006).
- **Round-trip tests** for every JSON-serialised type (marshal → unmarshal →
  equal + `Validate()` passes).
- **Property-style checks** where a relation must hold (e.g. a closed profile's
  cutting length equals the contour perimeter).

## Design-for-3D (ADR-011)

The immediate target is 2.5D, but the spine is shaped so 3D is *addition*, not
*re-architecture*:

- **Same pipeline.** geometry → features → planning → generation → moves →
  post-processing. 2.5D is the degenerate case (Z constant) of 3D, so building it
  now exercises the exact path 3D will use.
- **3D-ready format.** `Move` already carries 3D points; an arc is defined by
  centre + start + end + direction for any plane, so no new arc type is needed.
  The only 2.5D assumption is in the *helpers* (the emitter's arc-angle math and
  `Path.Length`), which assume the XY plane — generalising them means computing
  the arc plane from the three points and, for 3D arcs, storing an explicit plane
  normal so CW/CCW is unambiguous. That is an extension, not a break.
- **Tool as a tip envelope.** Every tip family (flat / ball / bull / taper) is a
  surface of revolution described by `r(z)`. The current `Tool` fields already
  encode it; the `tipRadius(z)` helper is a pure, testable addition. 2D offset
  uses `r` at fixed Z; 3D offsets the surface.
- **3D adds modules, not a new spine:** surface offset / tool placement, volume
  collision, and adaptive (curvature-driven) stepover — each a separate,
  heavily-tested package.

See [surface-quality.md](surface-quality.md) for the surface-finish model and
[operations.md](operations.md) for the 3D strategy list.

## What is deliberately NOT in the core (yet)

- Part import (STL/3MF/B-rep) — a boundary module that normalises into the
  canonical frame (ADR-001) and hands geometry to the pipeline. Roadmap phase 2.
- Full 2D offset / cutter compensation with robust corner joins. Phase 2.
- Adaptive/trochoidal, constant-chip-load, and jerk-limited feed scheduling.
  Phases 3–4 (see [operations.md](operations.md)).
- The WASM GUI. A separate effort that consumes the `moves` format.

These are all *additions to the pipeline*, not changes to its spine.
