# Handover — read this first after a context reset

> **You are resuming CleanCuts after a compaction.** This is the orientation map
> for a future you (or any agent). It is a **snapshot of session state**, not a
> replacement for the durable docs it points to. Where this doc and the ADRs/roadmap
> disagree, **the ADRs and roadmap win** (they are the source of truth).
>
> Last updated: **2026-11-21** · branch `main` · working tree clean · `make ci` green · **v0.2**.

## TL;DR — where we are

- **What:** a CAM core for 2.5D (eventually full 3D) CNC milling. Go, **zero
  dependencies in `internal/`**, built for human *and* agent maintenance.
- **Repo:** `github.com/kutis96/cleancuts` (local dir `cleancuts/`). MIT licensed.
  CI = GitHub Actions **and** a portable local script (`sh scripts/ci.sh`).
- **Built + tested (v0.2):** the **entire 2.5D first-cut pipeline**: `kernel`,
  `moves` (Move/Path/**Program**), `mesh` (STL ASCII+binary, 3MF, `Slice(z)`),
  `part` (recipe + nesting-parity feature detection), `stock`, `fixture`,
  `comp` (2D offset, safe-by-construction), `planning`, `planner` (first-cut),
  `safety`, `toolpath`, `gcode` (path + program emission, coordinate
  formatting), `tool`, `machine` — and the CLI: `version`, `demo`, **`generate`**
  (JSON configs → `.moves.json` + `.nc`). All green.
- **Phase 2 is DONE** (slices F→L all ✅, roadmap exit criterion met).
  **Phase 3 (adaptive & trochoidal clearing) is the natural next step — but
  has NOT been started and needs the user's go-ahead.**
- **All known issues are closed** (`issues/open/` should be empty or contain
  only items you opened; read `issues/open/` at session start per AGENTS.md).

## Verify the repo is healthy — do this first

```sh
cd cleancuts
make ci            # or:  sh scripts/ci.sh
```
Expect `==> OK: gofmt, vet, test, build all pass`. If it's red, that's your
first job — don't build on a broken base. Then sanity-check the end-to-end
workflow (it is the Phase 2 exit criterion):

```sh
go run ./cmd/cleancuts generate \
  --part examples/parts/plate_60x40x10_hole20.json \
  --stock examples/stock/block_120x80x20.json \
  --tool examples/tools/library.json --tool-id endmill-10mm-4f \
  --machine examples/machines/grbl_3axis.json \
  --coolant flood --feed 500 --spindle 8000 --out /tmp/p.moves.json --nc /tmp/p.nc
```
Expect: profile CW + pocket CCW (both climb), a verified rapid link between,
clean coordinates (`X-5`, never `X-5.000000000000002`), footer `M9 M5 M2`.

## Where things live (one-line map)

| Path | What it is |
|---|---|
| `README.md` | What CleanCuts is, principles, quick start (incl. `generate`), license. |
| `AGENTS.md` | The **laws** + working mode (design‑first, issues, git commit‑as‑you‑go). Read before changing code. |
| `HANDOVER.md` | **This file** — session state snapshot. |
| `docs/design-decisions.md` | **ADR-001…019** — the *why*. Read before changing any core behaviour. |
| `docs/architecture.md` | The pipeline, package contracts (14 packages + CLI), how to extend, design‑for‑3D. |
| `docs/roadmap.md` | Phases; **Phase 2 ✅ (v0.2)**, Phase 3+ planned; resolved + deferred decisions. |
| `docs/part.md` | The part recipe + feature-detection model (ADR-018). |
| `docs/cutter-compensation.md` | The `comp` spec (ADR-013). |
| `docs/workholding.md` | Keep‑out zones (clamps/vise) — ADR-012. |
| `docs/stock.md` | The stock model (ADR-015). |
| `docs/planning.md` | Per-feature planning: milling mode, orientation, entry (ADR-009/016/018). |
| `docs/surface-quality.md` | 3D finish model: scallop↔stepover, pass structure. |
| `docs/tool-parameters.md` | Tool model + feed/speed model + tip envelope `r(z)`. |
| `docs/operations.md` | Strategies (trochoidal/adaptive), climb/conventional, smoothing. |
| `docs/gcode.md` | The `moves` format spec (Move/Path/**Program**) + post‑processing + `CoordPrecision`. |
| `docs/agentic-workflow.md` | How to drive agent work: slices, handoffs, parallel work. |
| `schemas/` | JSON contracts: `tool`, `tool_library`, `machine`, `moves`, `part`, `stock`, `fixture`. |
| `examples/` | Example part (plate + hole, JSON recipe + STL), stock, tool crib, 2 machines, a fixture (clamps). |
| `issues/` | Bug + doc-drift tracker: `open/` and `closed/` (conventions in `issues/README.md`). |
| `scripts/ci.sh` | The portable health check (local + CI run this). |

## The durable decisions (don't re‑litigate; read the ADR for detail)

- **001** one coordinate system everywhere (right‑handed, **Z‑up**, **mm**);
  units mapping happens exactly once, in post-processing. **v1 is mm-only:
  the planner refuses `units: "inch"` machines.**
- **002** analytic geometry (lines/arcs are values; no tessellation in the core;
  **coordinate formatting is also a post-processing concern** — `gcode`
  `CoordPrecision` absorbs the float64 ulps).
- **003** machine‑independent `moves` format = the seam between generation and post.
- **004** zero external dependencies in `internal/`.
- **005** climb/conventional is explicit data on each cutting move.
- **006** precision/robustness strategy (float64, one tolerance, invariants checked).
- **007** tools/machines/fixtures are portable JSON with `Validate()`.
- **008** composable pipeline stages (no god packages, no cycles).
- **009** climb/conventional is decided in **planning, before generation** (it sets
  traversal direction).
- **010** **never over‑cut**; motion smoothing is clearance‑safe; feed/rapid min/max
  are enforced.
- **011** the core is 3D‑ready; the tool is a **tip envelope `r(z)`**; surface
  quality is configuration.
- **012** **keep‑out zones** (clamps, vise jaws) are **hard safety volumes** the tool
  must never touch.
- **013** 2D offset/comp is a dedicated, composable, **safe‑by‑construction** module
  (self‑intersection trimmed, never over‑cut).
- **014** additive kernel/moves/gcode primitives for Phase 2 (full circle = arcs).
- **015** stock is data; `SectionAt(z)` is a pure primitive.
- **016** kernel point types carry the canonical lowercase JSON form.
- **017** mesh import is a boundary module; `Slice(z)` is the clean-case section
  primitive (coplanar case refused, not silently booleaned).
- **018** the part is a **recipe over a mesh**; feature kind follows **nesting
  parity** (even = profile, odd = pocket); the first‑cut planner is a thin
  composition; process values are **refused, not clamped**; v1 is single-depth Z.
- **019** the program layer: **link rapids are first-class paths** (verified at
  construction); fixture mapping is data; `moves.Program` has unique path IDs.

## Who we're building for (context that isn't in the code)

- The **user is a software developer (not a machinist)** building this to help a
  **machinist friend with a CNC mill**. The friend wants: trochoidal/adaptive
  clearing, constant chipload, smooth low‑jerk paths, climb/conventional done
  correctly, and "G1 is enough but G2/G3 would be nice." **These are Phase 3–4.**
- **What the user values:** design‑first (ADRs/docs **before** code), strong
  testability, **safety** (never over‑cut the part; keep‑out zones for clamps),
  **portable JSON + JSON Schema** (they are explicitly tired of "well‑formatted"
  unvalidated JSON — they came from XML/XSD), a Go core with a **plain‑JS WASM GUI
  later** (deferred), agent‑maintainability, and **continuity under compaction**
  (this doc exists for that).
- **Non‑negotiables:** one coordinate system end‑to‑end; analytic geometry; never
  destroy the part or the tool; keep it maintainable by agents; commit as we go.

## Phase 2 thread (what was built, most recent last)

1. Foundation (v0.1): `kernel`, `moves`, `toolpath`, `gcode`, `tool`, `machine`,
   CLI `demo`; ADR-001…013; doc set; JSON schemas.
2. **F** `toolpath.Loop` (arcs stay analytic) · **G** `planning` (milling mode,
   orientation, keep-out entry) · **H** `safety` (envelope gate, analytic).
3. **I** `stock` (box/cylinder/prism, `SectionAt`/`TopAt`/`Contains`/`Fits`,
   ADR-015) · **J** `mesh` (STL ASCII+binary, 3MF, `Slice(z)`, ADR-017).
4. **K** `part` recipe + nesting-parity feature detection (ADR-018) +
   `planning.PickStartBetween` (entry clears keep-outs over the whole plunge
   column) + the **first-cut planner** (Z model, refuse-not-clamp process
   validation, `ToProgram`).
5. **L** `moves.Program` (unique path IDs, links as first-class paths) +
   `planner.ToProgram` + `gcode.EmitProgram` + `internal/fixture` (workholding →
   `KeepOut`) + **CLI `generate`** (ADR-019).
6. **Post-L hardening (2026-11-21):** `gcode` coordinate formatting
   (`CoordPrecision`, trailing-zero trim, `-0`→`0`) — the
   `X-5.000000000000002` artifact is gone; planner coolant-kind validation;
   **issue 001 fixed** — `PointInContour` counted a ray meeting one >180° arc
   twice as one crossing (kernel; regression test pins it); **issue 003 fixed**
   — comment/doc nits + **planner now refuses inch machines up front** (v1 is
   mm-only; emitting G20 over mm numbers would over-cut 25.4×); **issue 004**
   (part.md climb direction) corrected; **issue 002** (this doc + README +
   architecture + roadmap + gcode.md) refreshed to v0.2.
7. An **issues/ tracker** (`issues/open/`, `issues/closed/`) now holds known
   bugs and doc drift; AGENTS.md requires reading `issues/open/` at session
   start, recording new findings under the next free number, and closing with a
   self-contained resolution note (date + commit + what/why/verified).

## Pick up here (the immediate next action)

**Phase 3 — Adaptive & trochoidal clearing** is the natural next step per
`docs/roadmap.md` (engagement model → trochoidal generator → constant-chipload
feed → pocket/face on clearing → feed/speed resolution from `feeds[]`).
**Do NOT start it without the user's explicit go-ahead** — Phase 2 was just
finished and the user may want to review, dry-run on the friend's machine, or
re-prioritise (e.g. multi-pass Z earlier, or the WASM GUI).

If the user says "proceed": follow the standing working mode —
**design first** (the engagement model and trochoidal path math get their ADR +
doc *before* code), then implement slice by slice with tests, `make ci` green
at every commit, and keep this file honest at each phase boundary.

## Conventions that matter most (full list in AGENTS.md)

1. **Design first:** ADR + doc before code. A decision is "done" once it's in an
   ADR, even before the code exists.
2. **Git — commit as you go:** one focused slice per commit, **only at a green
   checkpoint** (`make ci` passes), `bionic/…` branches for risky work, keep
   `main` shippable. Never commit a broken state.
3. **Verify before every commit:** `make ci` (gofmt, vet, test, build).
4. **Safety is a law:** never over‑cut (ADR-010/012/013); respect feed/rapid
   limits; keep-outs are hard; refuse, don't clamp.
5. **Keep it portable:** zero deps in `internal/` (ADR-004); config is JSON
   (ADR-007); v1 plans in mm only.
6. **Issues:** read `issues/open/` first; new findings get the next free number;
   closings are self-contained.

## Open questions still awaiting the user

- **Go-ahead for Phase 3** (or a re-prioritisation).
- Real-world validation: a dry-run of the example plate program on the friend's
  machine or a simulator (the remaining check on the Phase 2 exit criterion).
- 3D implementation is **Phase 5**; the design (ADR-011, `surface-quality.md`)
  is ready, so 2.5D work now is not throwaway.
- B‑rep: settled as **future, integrate‑don't‑build** — no action now.
- GUI (plain JS + WASM): **deferred** until the 2.5D core is solid (Phase 6).

## Quick re‑orientation commands

```sh
cd cleancuts
git log --oneline -12       # see the trail of committed slices
make ci                     # confirm green
go run ./cmd/cleancuts version
go run ./cmd/cleancuts demo # the short pipeline
# generate: see "Verify the repo is healthy" above
```
