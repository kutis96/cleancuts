# Roadmap

CleanCuts is built **from the core up, in testable slices.** Each phase has a
concrete, verifiable *exit criterion* so "done" is unambiguous for both humans
and agents. The spine (kernel → moves → generation → post-processing) is fixed;
each phase *adds a stage or a capability* to it rather than re-architecting.

> Status: **v0.1** is complete (foundation scaffold). Everything below is
> planned. Phase boundaries may be re-scoped as we learn, but the *order* —
> precise geometry before strategies, strategies before smoothing, 2.5D before
> 3D — is deliberate and should be preserved.

---

## v0.1 — Foundation scaffold ✅ (this release)

A working, tested spine and the architecture to build on.

- [x] Dependency-free geometry kernel (vectors, points, lines, arcs,
      intersections, tolerance model) with table-driven tests.
- [x] Machine-independent toolpath format (`Move`/`Path`), invariants, and
      stable JSON round-trip.
- [x] A 2D profile toolpath generator (plunge / cutting loop / retract).
- [x] A generic G-code emitter with G2/G3 arcs and a G1 tessellation fallback.
- [x] Portable tool and machine config models (JSON) with `Validate()`.
- [x] A CLI (`version`, `demo`) proving the pipeline end to end.
- [x] Architecture, ADR log, tool-parameter and operations docs, JSON schemas,
      example configs, Makefile, and an agent-facing working agreement.

**Exit criterion (met):** `go build ./...`, `go vet ./...`, and `go test ./...`
all pass with zero external dependencies, and `demo` emits valid, inspectable
G-code.

---

## Phase 2 — Real 2D: import, compensation, climb/conventional

Goal: machine a real part's 2D features correctly.

- [ ] **Part import:** STL and 3MF (triangle mesh) importers that normalise into
      the canonical frame (ADR-001) and extract 2D contours (planar face
      detection). B-rep import is a separate, higher-effort effort — defer to a
      later phase and treat mesh as the v1 input.
- [ ] **Stock:** box + common solids (cylinder, prism) as the material block;
      stock-vs-part boolean to find what to remove.
- [ ] **Cutter compensation:** robust 2D offset (inside/outside), with mitre and
      fillet corner joins. This is the highest-risk, highest-value piece — it gets
      its own heavily-tested module (a new ADR if it introduces a robust-geometry
      layer).
- [ ] **Climb/conventional:** derive the milling mode per contour
      (rotation + travel + material side), with global/per-operation/per-path
      overrides (ADR-005).
- [ ] **Operation planner (first cut):** map part + stock + tool to an ordered
      list of operations (profile/pocket/face) and strategies.
- [ ] **Config loading in the CLI:** `cleancuts generate --part … --stock …
      --tool … --machine …` producing `.moves.json` and `.nc`.

**Exit criterion:** import a real STL part, plan its 2D features, offset by a
real tool, apply the correct milling mode, and emit G-code that a simulator
(e.g. a G-code simulator or the friend's machine dry-run) accepts without
crashing — with the intermediate `.moves.json` inspectable.

---

## Phase 3 — Adaptive & trochoidal clearing

Goal: the friend's "trochoidal / adaptive / constant chipload" requests.

- [ ] **Engagement model:** compute per-segment radial engagement `a_e` for a
      trochoid/adaptive pass from geometry + tool radius.
- [ ] **Trochoidal path generator:** emit the arc-based clearing path (a
      `moves.Path`), clipped to the pocket boundary, with `a_e` in metadata.
- [ ] **Constant-chip-load feed:** scale per-move feed by engagement
      (`F ≈ F_full · sqrt(a_e/D)`), visible and overridable.
- [ ] **Pocket & facing operations** built on the clearing strategies.
- [ ] **Feed/speed resolution** from the tool's `feeds[]` + `max_rpm` (the
      `ProcessTable` in tool-parameters.md).

**Exit criterion:** a deep/wide pocket is cleared with a trochoidal/adaptive
path whose per-move feeds are provably engagement-scaled (a test asserts the
scaling), and the resulting G-code is smoother/less loaded than a straight raster
at the same depth.

---

## Phase 4 — Smoothing & motion planning

Goal: "smooth paths without too much acceleration/jerk."

- [ ] **Corner fillets / joins** with a configurable radius (arcs at joins).
- [ ] **Lookahead feed scheduler** respecting machine accel/jerk limits
      (from the machine config), decelerating into corners.
- [ ] **Dry-run bounds checks** against axis travel; clear, early errors if a
      path would exceed travel.
- [ ] **Backlash compensation** for conventional milling where the machine
      config requires it.

**Exit criterion:** for a given machine config, the emitted feed schedule keeps
accelerations and (where modelled) jerk within the configured limits — asserted
by a test that replays the schedule.

---

## Phase 5 — 3D machining

Goal: full 3D on a 3-axis mill.

- [ ] Kernel extensions: B-splines, surface/edge intersection, Z-height
      functions.
- [ ] Strategies: Z-level/parallel, scallop/stepover, flow, contour-parallel,
      steep.
- [ ] 3D tool-tip compensation (ball/bull-nose) and stock-aware collision
      checks.

**Exit criterion:** a 3D relief part is finished with a chosen strategy and the
resulting G-code is collision-free (per a collision test) and meets the target
scallop height.

---

## Phase 6 — GUI (WASM) — deferred

Goal: a nice face on the core.

- The core is already WASM-friendly (zero deps, pure Go — ADR-004). The GUI
  consumes the `moves` format and the config models.
- Plain JS for the UI shell (per preference), with the Go core compiled to WASM
  for the compute. `jsc` is available locally for a JS test harness even before
  the browser target is chosen.
- Out of scope for the core repo until the 2.5D core is solid (phase 4).

---

## Cross-cutting (every phase)

- **Tests first:** every new module ships table-driven tests before it is
  considered done. No feature merges on "it looks right."
- **Docs keep up:** each new capability gets an entry in
  [operations.md](operations.md) (the *model*), a section in
  [architecture.md](architecture.md) if it adds a package, and an ADR if it
  changes a decision.
- **Agent handoff:** [AGENTS.md](../AGENTS.md) stays current with the conventions
  and the current phase's "definition of done."
- **Precision:** any new geometry primitive gets its tolerance model and its
  degenerate-case tests (ADR-006) at the same time it is written.

## Open questions (need a decision, not just work)

1. **Mesh-first vs. B-rep.** v1 is mesh-first (STL/3MF). Is a native B-rep
   (e.g. a CAD kernel) ever a hard requirement, or is high-quality mesh input +
   analytic toolpaths sufficient for the target work?
2. **Module path.** Current Go module path is `github.com/spacecat/cleancuts`.
   Confirm the home (org/user + repo) so the path is final before it spreads.
3. **License.** No license is committed yet. Choose one (MIT is the working
   default) before external contributors show up.
4. **CI host.** A GitHub Actions workflow is included as a default. Confirm the
   host (GitHub / GitLab / other) or we can swap it.
