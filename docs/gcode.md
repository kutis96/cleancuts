# G-code & post-processing

This document specifies the **intermediate toolpath format** (the thing users can
inspect and override) and the **post-processing** model (how it becomes a
machine-specific G-code program).

## The intermediate format

The intermediate format is `internal/moves`. It is **machine-independent**: it
describes *what the tool does* in the canonical WCS, not *how a particular
controller spells it out*.

### `Move`

One motion. Exactly one of the kinds applies:

| `kind` | G-code | Meaning |
|---|---|---|
| `rapid` | G0 | Non-cutting positioning. `feed` must be `0`. |
| `linear` | G1 | Cutting feed along a straight line. `feed > 0`. |
| `arc_ccw` | G3 | Counter-clockwise circular arc. `feed > 0`, `center` set. |
| `arc_cw` | G2 | Clockwise circular arc. `feed > 0`, `center` set. |

Common fields:

| Field | JSON | Meaning |
|---|---|---|
| `from` / `to` | `from` / `to` | Start and end points, `{x,y,z}` mm, canonical WCS. |
| `center` | `center` | Arc centre (arcs only; zero for others). Always emitted for a stable schema. |
| `cw` | `cw` | Arc direction flag (arcs only). |
| `feed` | `feed` | mm/min. `0` = rapid. |
| `spindle` | `spindle` | Requested RPM; `0` = no change. |
| `coolant` | `coolant` | `off` / `flood` / `mist` / `through`. |
| `milling` | `milling` | `climb` / `conventional` (cutting moves); informational. |
| `comment` | `comment` | Optional human context. |

### `Path`

One **continuous** chain of moves (one pass of one feature). Invariant: each
move's `from` equals the previous move's `to`. Multiple features = multiple
`Path`s, grouped by a `Program`.

| Field | JSON | Meaning |
|---|---|---|
| `id` | `id` | Stable identifier (required). |
| `name` | `name` | Human name. |
| `operation` | `operation` | e.g. `profile`, `pocket`, `face`. |
| `tool_id` | `tool_id` | Which tool this path is for. |
| `pass` | `pass` | Pass number (1-indexed) within the operation. |
| `moves` | `moves` | The ordered moves. |
| `metadata` | `metadata` | Free-form bag: strategy name, per-segment engagement, model version, … |
| `created_at` | `created_at` | Generation timestamp (informational). |

### `Program` (ADR-019)

An **ordered, addressable** list of `Path`s — the unit `cleancuts generate`
writes to `.moves.json` and `gcode.EmitProgram` emits. Invariants:

- `id` required; `paths` non-empty.
- **Path IDs are unique** — each path is addressable (a future GUI can select
  and override one; a post-processor can emit or skip one).
- The inter-feature **link rapids are first-class paths** (their own ID, e.g.
  `link-1`): the planner inserts them between features, *verified against the
  keep-out zones at construction* (a zero-length link is omitted rather than
  emitted). Linking is data, not emitter logic.

| Field | JSON | Meaning |
|---|---|---|
| `id` | `id` | Program identifier (required). |
| `name` | `name` | Human name (emitted as a header comment). |
| `paths` | `paths` | The ordered `Path`s (unique IDs). |

### Why this shape

- **Explicit, stable JSON** (lowercase snake_case) that is easy to diff, edit,
  and version. See `schemas/moves.schema.json`.
- **Analytic moves** (ADR-002): arcs are arcs, not polylines. Tessellation and
  **coordinate formatting are post-processing choices** — the intermediate JSON
  keeps the core's float64 values verbatim (so an editor sees the exact geometry
  and can re-emit at any precision).
- **Decisions are data** (ADR-005): climb/conventional, feed, coolant, and
  strategy context are all *visible and editable* here.
- **Many consumers, one source:** the generic emitter, machine-specific
  post-processors, and the future GUI all read the same format.

### A minimal example

```json
{
  "id": "profile-1",
  "name": "2D profile",
  "operation": "profile",
  "tool_id": "endmill-10mm-4f",
  "pass": 1,
  "moves": [
    { "kind": "rapid",  "from": {"x":0,"y":0,"z":5},   "to": {"x":0,"y":0,"z":-10},
      "center": {"x":0,"y":0,"z":0}, "cw": false, "feed": 0,
      "spindle": 8000, "coolant": "flood", "milling": "", "comment": "plunge" },
    { "kind": "linear", "from": {"x":0,"y":0,"z":-10}, "to": {"x":100,"y":0,"z":-10},
      "center": {"x":0,"y":0,"z":0}, "cw": false, "feed": 600,
      "spindle": 8000, "coolant": "flood", "milling": "climb", "comment": "" },
    { "kind": "arc_ccw","from": {"x":100,"y":0,"z":-10},"to": {"x":100,"y":10,"z":-10},
      "center": {"x":100,"y":10,"z":-10}, "cw": false, "feed": 600,
      "spindle": 8000, "coolant": "flood", "milling": "climb", "comment": "" }
  ],
  "metadata": { "strategy": "profile", "model": "v0.1" }
}
```

## Post-processing model

Post-processing is a **layer over the intermediate format**, not a rewrite of it.
Two concerns, kept separate:

### 1. Generic emission (`internal/gcode`)

The default post-processor turns a `moves.Program` (or a single `moves.Path`
via `Emit`) into dialect-neutral G-code:

- **One header:** spindle = the max across all paths (`S…`, `M3`), coolant on
  if any path wants it (`M8`).
- **The paths in order**, each under a `; name` comment; a link path emits like
  any other (its moves are just moves).
- Each move → `G0`/`G1`/`G2`/`G3` with `X Y Z`, `F`, and (arcs) `I J` (centre
  offsets relative to the arc start — the most widely supported arc format).
- **One footer:** coolant off (`M9`), spindle off (`M5`), program end (`M2`).

Configurable via `gcode.Options`:
- `use_arms` — emit G2/G3 (default) or tessellate to G1.
- `arc_tol_deg` — max segment angle when tessellating.
- **`coord_precision`** — decimal places for coordinates (default 4 = 1 µm),
  trailing zeros trimmed, `-0` normalised to `0`. This is where the core's
  float64 ulps are absorbed: an offset corner that is `-5.000000000000002`
in the intermediate format emits as `X-5`. G-code is a decimal text protocol
  for machines with finite resolution, so formatting is a post-processing
  concern (ADR-002), not a core-geometry change.
- `comment_prefix`, and the M-codes (`spindle_start/stop`, `program_end`,
  `coolant_on/off`).

### 2. Machine-specific dialect (`internal/machine`)

A machine JSON carries everything dialect-specific, so a machine post-processor
varies *only* the dialect and framing, never the geometry:

- `dialect` (`grbl`, `linuxcnc`, `fanuc`, `generic`).
- `units` (`mm` / `inch`) — the *only* place units conversion happens (ADR-001).
  **v1 note:** the core plans in millimetres only, and the planner refuses
  `units: "inch"` machines up front — emitting G20 over mm numbers would scale
  the program 25.4× (over-cut), and comparing mm process values against inch
  limits is a category error. Inch support is deferred (roadmap).
- `axes` — letters, travel, max feed/rapid (for bounds checks and lookahead).
- `spindle_rotation` — `ccw` (default) or `cw`, viewed from above; planning uses
  it to derive climb/conventional traversal (ADR-005, ADR-009).
- `wcs` — the work coordinate word (e.g. `G54`).
- `post_processor` — M-codes, arc format (`ij` / `absolute`), header/footer lines.

**The rule:** a machine post-processor reads `machine.Machine`, composes over
`gcode.Emit`, and applies the dialect. It never re-derives geometry or re-frames
coordinates beyond the single, config-driven axis/units mapping. This is what
keeps coordinate bugs (ADR-001, ADR-006) out of the core.

### 3. Feed & rapid limits (ADR-010)

Post-processing enforces the machine's feed and rapid bounds so that neither
cutting nor rapid motion overruns the machine:

- **Cutting moves (G1)** clamp to `[min_feed, max_feed]`.
- **Rapid moves (G0)** clamp to `[min_rapid, max_rapid]`.
- **Per-axis motion** is bounded by the axis `max_feed` / `max_rapid` (the
  machine config carries per-axis `max_feed`/`max_rapid` plus the machine-level
  `min/max_feed` and `min/max_rapid`).
- A limit of `0` means "no limit/minimum" in that direction; a non-zero `min`
  prevents a cutting move from becoming a stall-prone creep or a rapid from
  becoming an unintended slow move.

Clamping is a **pure function** of (machine config, move) and is independently
testable. It runs after the feed is resolved (from the tool's `feeds[]` and any
constant-chip-load scaling) and before emission, so the emitted `F` is always
within the machine's envelope. The lookahead scheduler (phase 4) layers
acceleration/jerk limits on top of these bounds, never replacing them.

### Arc format note

- **`ij` (relative offsets):** `G3 X… Y… I… J…`, where `I`/`J` are the centre
  offset from the arc *start* point. Widely supported (GRBL, Fanuc, most
  controllers). This is the default.
- **`absolute` (centre):** some dialects prefer an explicit centre word. The
  machine config selects which; the generic emitter defaults to `ij`.

### What post-processing will add (phases 4+)

- **Feed scheduling / lookahead** for acceleration & jerk limits (see
  [operations.md](operations.md)).
- **Backlash compensation** for conventional milling on machines that need it.
- **Tool change handling** (ATC vs. manual, `T`/`M6` ordering, offsets).
- **Dry-run bounds checks** against `axes` travel before emission.

## Emission example (v0.1)

`go run ./cmd/cleancuts demo` produces (abridged):

```gcode
% CleanCuts demo — 2D profile of a 100x60 rectangle
; Generated by CleanCuts (generic post-processor)
S8000
M3
M8
; plunge
G0 X0 Y0 Z-10
G1 X100 Y0 Z-10 F600
G1 X100 Y60 Z-10 F600
G1 X0 Y60 Z-10 F600
G1 X0 Y0 Z-10 F600
; retract
G0 X0 Y0 Z5
M9
M5
M2
```

The same program, with `--no-arms` and an arc in it, would show the arc
tessellated into `G1` segments no wider than the configured angle.
