# CleanCuts

> **Resuming after a context reset / compaction?** Start with [HANDOVER.md](HANDOVER.md)
> — the session-state snapshot (where we are, what's next, how to verify).

CleanCuts is a CAM (computer-aided manufacturing) core for 2.5D — and eventually
full 3D — machining on CNC mills. It takes a part model (mesh or B-rep), a
material block, and a tool, and produces verified G-code.

This repository is intentionally built as a **small, dependency-free, heavily
tested core** that can later be wrapped by a CLI (today) and a WASM-based GUI
(later). The goal is a CAM tool that is accurate, maintainable, and easy to
hand off to both humans and AI agents — the two things that have historically
lacked in the open-source CAM space.

> **Status:** v0.2 — the 2.5D core is complete. Import a part (STL/3MF),
> a stock block, a fixture, a tool, and a machine (all portable JSON), and
> `cleancuts generate` plans the 2D features — profile and pocket in the
> correct climb direction, offset by the tool, verified against keep-out
> zones — and emits inspectable `.moves.json` plus G-code. Adaptive/
> trochoidal strategies, smoothing, and 3D arrive in later phases (see
> [Roadmap](docs/roadmap.md)).

---

## Why this exists

Existing open-source CAM projects tend to fail in the same ways:

- **Floating-point and re-interpolation bugs.** Geometry is tessellated into
  polylines, re-sampled onto fixed grids, and passed between subsystems that
  each assume a slightly different coordinate frame. The resulting errors are
  distributed across the whole app and very hard to isolate.
- **Closed, monolithic cores.** The kernel, toolpath generation, and
  post-processing are tangled, so a fix in one place ripples everywhere.
- **Unwelcoming to contribution.** Sparse documentation and weak test coverage
  make external feedback hard to act on.

CleanCuts is designed to avoid each of these, concretely:

1. **One canonical coordinate system, end to end.** Everything is expressed in a
   single right-handed, Z-up, millimetre work-coordinate system. No subsystem
   re-frames or re-interpolates coordinates; the machine mapping happens exactly
   once, in post-processing. (See [ADR-001](docs/design-decisions.md#adr-001).)
2. **Analytic geometry, not polylines.** Lines and arcs are first-class values.
   Tessellation is a post-processing concern, not a core representation.
   (See [ADR-002](docs/design-decisions.md#adr-002).)
3. **A machine-independent intermediate format.** Toolpaths are a list of moves
   that users can inspect and override, and that both the generic and
   machine-specific post-processors consume. (See [ADR-003](docs/design-decisions.md#adr-003).)
4. **Zero external dependencies in the core.** The math and pipeline use only
   the Go standard library, so they can be embedded anywhere (including WASM)
   and their behaviour is fully determined by source. (See [ADR-004](docs/design-decisions.md#adr-004).)
5. **Portable JSON configuration.** Tools and machines are plain JSON files you
   keep in a library, exactly like a physical tool crib. (See
   [ADR-007](docs/design-decisions.md#adr-007).)
6. **Built for agent handoff.** Every package has a clear contract, table-driven
   tests, precise error messages, and a decision log so future agents don't
   re-litigate settled choices. (See [AGENTS.md](AGENTS.md).)

---

## Architecture at a glance

```
                ┌───────────────────────────────────────────────────────────┐
                │                       cleanCuts core                      │
  part model     │                                                             │
  (.stl/.3mf/    │   part ──► features ──► toolpath ──► moves ──► gcode       │
   brep)         │  (geometry)  (planning)  (generation)  (format)  (post)    │
  tool (json) ───┤                                                             │
  stock (json)   │            ┌───────────────┐      ┌──────────────┐          │
  machine(json)──┘            │  kernel       │      │  moves       │          │
                              │  geometry+math│─────►│  toolpath    │          │
                              └───────────────┘      └──────────────┘          │
                                        ▲                     │                 │
                                        │                     ▼                 │
                              ┌────────────────────┐   ┌───────────────────┐   │
                              │  tool / machine    │   │  generic gcode    │   │
                              │  config (json)     │   │  + machine post   │   │
                              └────────────────────┘   └───────────────────┘   │
                └───────────────────────────────────────────────────────────┘
                                              │
                              ┌───────────────┴───────────────┐
                              ▼                                ▼
                          CLI (today)                   WASM GUI (later)
```

The core is a pipeline of **composable, independently testable** stages. Each
stage has a narrow contract. Swapping a strategy (e.g. profile → trochoidal)
or a post-processor (generic → GRBL) never forces a change in the others.

| Package | Responsibility |
|---|---|
| [`internal/kernel`](internal/kernel) | Dependency-free geometry and math: vectors, points, lines, arcs, intersections, containment. The single source of truth for coordinates. |
| [`internal/moves`](internal/moves) | The machine-independent toolpath format (Move / Path / **Program**) and its invariants. |
| [`internal/mesh`](internal/mesh) | STL (ASCII + binary) and 3MF import; `Slice(z)` section extraction (ADR-017). |
| [`internal/part`](internal/part) | The part recipe (JSON) and feature detection by nesting parity (profile/pocket, ADR-018). |
| [`internal/stock`](internal/stock) | The stock model (box, cylinder, prism): `SectionAt`/`TopAt`/`Contains`/`Fits` (ADR-015). |
| [`internal/fixture`](internal/fixture) | Portable workholding (keep-out zones) mapped to planning (ADR-019 §3). |
| [`internal/comp`](internal/comp) | 2D offset / cutter compensation: joins, assemble+trim, safe-by-construction (ADR-013). |
| [`internal/planning`](internal/planning) | Per-feature planning: milling mode, orientation, keep-out entry (ADR-009, ADR-016/018). |
| [`internal/planner`](internal/planner) | The first-cut planner: part+stock+fixture+tool+machine → ordered, verified paths (ADR-018/019). |
| [`internal/safety`](internal/safety) | The tool-envelope safety gate against keep-out zones (ADR-012). |
| [`internal/toolpath`](internal/toolpath) | Generates `moves.Path` from geometry (2D profile loop). |
| [`internal/gcode`](internal/gcode) | Generic G-code emitter (path + program) with coordinate formatting and arc tessellation. |
| [`internal/tool`](internal/tool) | Portable tool model + library (JSON). |
| [`internal/machine`](internal/machine) | Portable machine + post-processor config (JSON). |
| [`cmd/cleancuts`](cmd/cleancuts) | The CLI entry point (`version`, `demo`, **`generate`**). |

Full design rationale: [Architecture](docs/architecture.md) ·
[Design decisions (ADRs)](docs/design-decisions.md) ·
[Tool parameters](docs/tool-parameters.md) ·
[Operations](docs/operations.md) ·
[G-code & post-processing](docs/gcode.md) ·
[Roadmap](docs/roadmap.md).

---

## Quick start

Requires Go 1.22+ (developed against 1.26). No external dependencies.

```sh
cd cleancuts
go build ./...          # build the whole module
go test ./...           # run the full test suite
go vet ./...            # static checks
go run ./cmd/cleancuts demo        # generate a sample 2D profile as G-code
go run ./cmd/cleancuts demo --no-arms   # same, but arcs tessellated to G1
```

**Plan a real part** (the Phase 2 workflow) — part, stock, tool, and machine
are all portable JSON (see `schemas/` and `examples/`):

```sh
go run ./cmd/cleancuts generate \
  --part examples/parts/plate_60x40x10_hole20.json \
  --stock examples/stock/block_120x80x20.json \
  --tool examples/tools/library.json --tool-id endmill-10mm-4f \
  --machine examples/machines/grbl_3axis.json \
  --coolant flood --feed 500 --spindle 8000 \
  --out plate.moves.json --nc plate.nc
```

That prints a short plan summary, writes `plate.moves.json` (the inspectable
machine-independent intermediate — the seam you can edit and re-post), and
`plate.nc` (generic G-code). Add `--fixture examples/fixture/clamp_setup.json`
to declare keep-out zones; the planner refuses to route through them.

Or use the Makefile:

```sh
make build test vet fmt
```

---

## Project layout

```
cleancuts/
├── cmd/cleancuts/          # CLI entry point (version, demo, generate)
├── internal/               # 14 packages, all stdlib-only
│   ├── kernel/             # geometry + math core (no external deps)
│   ├── moves/              # machine-independent toolpath format (Move/Path/Program)
│   ├── mesh/               # STL/3MF import + Slice(z)
│   ├── part/               # part recipe + feature detection
│   ├── stock/              # stock model (box/cylinder/prism)
│   ├── fixture/            # workholding → keep-out zones
│   ├── comp/               # 2D offset / cutter compensation
│   ├── planning/           # milling mode, orientation, keep-out entry
│   ├── planner/            # first-cut planner (the composition)
│   ├── safety/             # tool-envelope gate vs. keep-outs
│   ├── toolpath/           # toolpath generation (2D profile loop)
│   ├── gcode/              # generic G-code emitter (path + program)
│   ├── tool/               # portable tool model + library (JSON)
│   └── machine/            # portable machine config (JSON)
├── schemas/                # JSON schemas (tool, machine, moves, part, stock, fixture)
├── examples/               # example parts, stock, tools, machines, fixtures
├── docs/                   # architecture, ADRs, parameters, roadmap
├── issues/                 # bug + doc-drift tracker (open/ and closed/)
├── AGENTS.md               # working agreement for AI agents (and humans)
├── HANDOVER.md             # session-state snapshot for context resets
├── Makefile
└── go.mod
```

---

## Design principles (summary)

- **One coordinate system.** Canonical WCS: right-handed, Z-up, millimetres.
- **Analytic over approximate.** Lines and arcs are values; don't tessellate early.
- **Composable stages.** Generation, compensation, and post-processing are separate.
- **Explicit invariants.** Every public type has a `Validate()` with precise errors.
- **Portable config.** Tools and machines are JSON; the core has no third-party deps.
- **Never over-cut.** Smoothing and motion are clearance-safe: the safe failure is
  leaving stock, never cutting into the part; feed/rapid limits are always respected.
- **3D-ready by design.** The spine, format, and tool model are the same 3D will
  use; the tool is a tip envelope `r(z)`; surface quality is configuration.
- **Design first, tested and documented.** Decisions live in an ADR log and docs
  before they're implemented; table-driven tests pin the behaviour. Built for
  handoff to both humans and agents.

This repository lives at **`github.com/kutis96/cleancuts`**.

## Verify (locally or CI)

The full health check is a single portable script — no Make dependency, and it's
the same thing GitHub Actions runs:

```sh
make ci            # or:
sh scripts/ci.sh   # gofmt check + go vet + go test + go build
```

## License

MIT — see [LICENSE](LICENSE).

---

See [AGENTS.md](AGENTS.md) for how to work in this codebase, and
[docs/roadmap.md](docs/roadmap.md) for where we're headed.
