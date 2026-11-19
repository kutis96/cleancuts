# Handover — read this first after a context reset

> **You are resuming CleanCuts after a compaction.** This is the orientation map
> for a future you (or any agent). It is a **snapshot of session state**, not a
> replacement for the durable docs it points to. Where this doc and the ADRs/roadmap
> disagree, **the ADRs and roadmap win** (they are the source of truth).
>
> Last updated: **2026-11-19** · branch `main` · working tree clean · `make ci` green.

## TL;DR — where we are

- **What:** a CAM core for 2.5D (eventually full 3D) CNC milling. Go, **zero
  dependencies in `internal/`**, built for human *and* agent maintenance.
- **Repo:** `github.com/kutis96/cleancuts` (local dir `cleancuts/`). Module path
  matches. MIT licensed. CI = GitHub Actions **and** a portable local script.
- **Built + tested (v0.1 core):** `kernel` (geometry/math), `moves` (toolpath
  format), `toolpath` (2D profile), `gcode` (emitter + arc tessellation), `tool`,
  `machine`, `cmd/cleancuts` (CLI: `version`, `demo`). All green.
- **Designed (not yet coded):** the big design surface is essentially complete —
  **ADR-001…013**, 10 docs, 5 JSON schemas. Phase 2's first slice (2D offset /
  cutter compensation) is **fully specified** (`docs/cutter-compensation.md`) but
  **not implemented**.
- **Immediate next:** implement `comp` slice 1 (`Edge.Offset`) **or** design the
  stock model first — **awaiting the user's choice** (see "Pick up here").

## Verify the repo is healthy — do this first

```sh
cd cleancuts
make ci            # or:  sh scripts/ci.sh
```
Expect `==> OK: gofmt, vet, test, build all pass`. If it's red, that's your first
job — don't build on a broken base.

## Where things live (one-line map)

| Path | What it is |
|---|---|
| `README.md` | What CleanCuts is, principles, quick start, license. |
| `AGENTS.md` | The **laws** + working mode (design‑first, git commit‑as‑you‑go). Read before changing code. |
| `HANDOVER.md` | **This file** — session state snapshot. |
| `docs/design-decisions.md` | **ADR-001…013** — the *why*. Read before changing any core behaviour. |
| `docs/architecture.md` | The pipeline, package contracts, how to extend, design‑for‑3D. |
| `docs/roadmap.md` | Phases, **resolved decisions**, deferred (B‑rep). |
| `docs/cutter-compensation.md` | **Phase 2 spec** (the next implementation) — model + build order. |
| `docs/workholding.md` | Keep‑out zones (clamps/vise) — ADR‑012. |
| `docs/surface-quality.md` | 3D finish model: scallop↔stepover, pass structure, the config knobs. |
| `docs/tool-parameters.md` | Tool model + feed/speed model + the tip envelope `r(z)`. |
| `docs/operations.md` | Strategies (trochoidal/adaptive), climb/conventional, smoothing. |
| `docs/gcode.md` | The `moves` format spec + post‑processing + feed/rapid clamping. |
| `docs/agentic-workflow.md` | How to drive agent work: slices, handoffs, parallel work. |
| `schemas/` | JSON contracts: `tool`, `tool_library`, `machine`, `moves`, `fixture`. |
| `examples/` | Tool crib, 2 machines, a fixture (clamps), a sample toolpath. |
| `scripts/ci.sh` | The portable health check (local + CI run this). |

## The durable decisions (don't re‑litigate; read the ADR for detail)

- **001** one coordinate system everywhere (right‑handed, **Z‑up**, **mm**).
- **002** analytic geometry (lines/arcs are values; no tessellation in the core).
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

## Who we're building for (context that isn't in the code)

- The **user is a software developer (not a machinist)** building this to help a
  **machinist friend with a CNC mill**. The friend wants: trochoidal/adaptive
  clearing, constant chipload, smooth low‑jerk paths, climb/conventional done
  correctly, and "G1 is enough but G2/G3 would be nice."
- **What the user values:** design‑first (ADRs/docs **before** code), strong
  testability, **safety** (never over‑cut the part; keep‑out zones for clamps),
  **portable JSON + JSON Schema** (they are explicitly tired of "well‑formatted"
  unvalidated JSON — they came from XML/XSD), a Go core with a **plain‑JS WASM GUI
  later** (deferred), agent‑maintainability, and **continuity under compaction**
  (this doc exists for that).
- **Non‑negotiables:** one coordinate system end‑to‑end; analytic geometry; never
  destroy the part or the tool; keep it maintainable by agents; commit as we go.

## This session's thread (most recent last)

1. Built the v0.1 core + full doc set + JSON schemas (committed `3e94401`).
2. Repointed module to `kutis96` (committed `22da3c5`).
3. User feedback → **ADR‑009** (climb/conventional in planning), **ADR‑010**
   (safety‑first smoothing + feed/rapid limits), **ADR‑011** (3D‑ready + tip
   envelope), `surface-quality.md`, and machine `min/max` feed+rapid (committed
   `d1eedc1`).
4. **MIT** license, **portable local CI** (`scripts/ci.sh`), **mesh‑first**
   (B‑rep = future "integrate a kernel, don't build one") (committed `1c82191`).
5. **ADR‑012** keep‑out zones + `fixture` schema + `workholding.md` (committed
   `ae90eb3`).
6. **ADR‑013** + `cutter-compensation.md` = the **Phase 2 comp spec**, design done,
   code not started (committed `ec23d05`).

## Pick up here (the immediate next action)

The user **greenlit "proceed with Phase 2."** The design for the first slice is
**done** (`docs/cutter-compensation.md`, with a test‑per‑step build order). We were
awaiting the user's choice between:

- **(a)** implement `comp` **slice 1** — `Edge.Offset(r, direction)`: line offset
  (shift by `±r·n`) and arc offset (radius `R±r`, same centre/span; `R−r ≤ 0` is the
  degenerate "tool too big" case), **with analytic tests** — the documented next
  step, testable in isolation; **or**
- **(b)** design the **stock model** first (the other Phase 2 input; so far only
  sketched as "box + common solids" in the roadmap).

**If there is no answer yet, default to (a)** — it's the documented next step and
isolates cleanly. Follow `AGENTS.md`: the design for `comp` is already satisfied
(ADR‑013 + doc), so write the tests first, implement to pass them, run `make ci`,
and commit a small green slice. Keep the `comp` module dependency‑free (use
`kernel` for lines/arcs/intersections).

## Conventions that matter most (full list in AGENTS.md)

1. **Design first:** ADR + doc before code. A decision is "done" once it's in an
   ADR, even before the code exists.
2. **Git — commit as you go:** one focused slice per commit, **only at a green
   checkpoint** (`make ci` passes), `bionic/…` branches for risky work, keep `main`
   shippable. Never commit a broken state.
3. **Verify before every commit:** `make ci` (gofmt, vet, test, build).
4. **Safety is a law:** never over‑cut (ADR‑010/012/013); respect feed/rapid limits.
5. **Keep it portable:** zero deps in `internal/` (ADR‑004); config is JSON (ADR‑007).

## Open questions still awaiting the user

- **comp slice 1 vs. stock‑model‑design‑first** — their call (default: comp slice 1).
- 3D implementation is **Phase 5**; the design (ADR‑011, `surface-quality.md`) is
  ready, so 2.5D work now is not throwaway.
- B‑rep: settled as **future, integrate‑don't‑build** — no action now.
- GUI (plain JS + WASM): **deferred** until the 2.5D core is solid (Phase 6).

## Quick re‑orientation commands

```sh
cd cleancuts
git log --oneline -8          # see the trail of committed slices
make ci                        # confirm green
go run ./cmd/cleancuts demo    # see the core pipeline emit real G-code
```
