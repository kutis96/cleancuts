# CleanCuts

CleanCuts is a CAM (computer-aided manufacturing) core for 2.5D — and eventually
full 3D — machining on CNC mills. It takes a part model (mesh or B-rep), a
material block, and a tool, and produces verified G-code.

This repository is intentionally built as a **small, dependency-free, heavily
tested core** that can later be wrapped by a CLI (today) and a WASM-based GUI
(later). The goal is a CAM tool that is accurate, maintainable, and easy to
hand off to both humans and AI agents — the two things that have historically
lacked in the open-source CAM space.

> **Status:** v0.1 — foundation scaffold. A working, tested core (geometry
> kernel, toolpath format, toolpath generator, generic G-code emitter, tool and
> machine config) plus the architecture and design decisions. Real part
> import, cutter compensation, and adaptive/trochoidal strategies arrive in
> later phases (see [Roadmap](docs/roadmap.md)).

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
| [`internal/kernel`](internal/kernel) | Dependency-free geometry and math: vectors, points, lines, arcs, intersections. The single source of truth for coordinates. |
| [`internal/moves`](internal/moves) | The machine-independent toolpath format (Move / Path) and its invariants. |
| [`internal/toolpath`](internal/toolpath) | Generates `moves.Path` from geometry (v0.1: 2D profile). |
| [`internal/gcode`](internal/gcode) | Generic G-code emitter and arc tessellation. |
| [`internal/tool`](internal/tool) | Portable tool model + library (JSON). |
| [`internal/machine`](internal/machine) | Portable machine + post-processor config (JSON). |
| [`cmd/cleancuts`](cmd/cleancuts) | The CLI entry point (version, demo). |

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

Or use the Makefile:

```sh
make build test vet fmt
```

---

## Project layout

```
cleancuts/
├── cmd/cleancuts/          # CLI entry point
├── internal/
│   ├── kernel/             # geometry + math core (no external deps)
│   ├── moves/              # machine-independent toolpath format
│   ├── toolpath/           # toolpath generation (2D profile in v0.1)
│   ├── gcode/              # generic G-code emitter + arc tessellation
│   ├── tool/               # portable tool model + library (JSON)
│   └── machine/            # portable machine config (JSON)
├── schemas/                # JSON schemas for tool / machine / moves
├── examples/               # example tools, machines, and toolpaths
├── docs/                   # architecture, ADRs, parameters, roadmap
├── AGENTS.md               # working agreement for AI agents (and humans)
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

See [AGENTS.md](AGENTS.md) for how to work in this codebase, and
[docs/roadmap.md](docs/roadmap.md) for where we're headed.
