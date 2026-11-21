# 003: Minor comment and doc nits

- **Status:** open
- **Type:** housekeeping
- **Priority:** low
- **Found:** 2026-11-21 (independent code review)
- **Files:** `internal/part/part.go`, `internal/kernel/curve2.go`, `cmd/cleancuts/generate.go`, `cmd/cleancuts/main.go`, `internal/planner/planner.go`

## Evidence (checklist)

- [ ] `internal/part/part.go`: comment "a parent is always strictly **smaller** in area" — wrong direction (a parent contains its child, so it is larger). The no-cycle logic itself is sound.
- [ ] `internal/kernel/curve2.go`: `NewChain2` doc says a closed chain needs ≥3 edges, but `IsClosed` allows 2 (two half-arcs = full circle, ADR-014). Align the doc with the implementation.
- [ ] `cmd/cleancuts/generate.go`: `gcodeOptions` always emits `G21` (mm) even when the machine config has `units: "inch"`. Harmless in v1 (core is mm-only), but inconsistent with dialect-from-machine-config — derive it from the machine, or note the mm-only guarantee.
- [ ] `cmd/cleancuts/main.go`: `version` is still `"0.1.0"` while the repo is well past v0.1.
- [ ] `internal/planner/planner.go`: flute-length check uses the mesh's max-Z as part top — over-conservative for pockets with a raised boss elsewhere. Safe-side over-refusal, acceptable in v1; add a code comment saying so.

## Suggested fix

Correct the comments/docs; for `G21`, either read `machine.Units` or document the mm-only v1 guarantee next to the emitter.

## Acceptance criteria

All items checked off, comments/docs accurate, `sh scripts/ci.sh` green.
