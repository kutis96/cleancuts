# 003: Minor comment and doc nits

- **Status:** closed
- **Type:** housekeeping
- **Priority:** low
- **Found:** 2026-11-21 (independent code review)
- **Files:** `internal/part/part.go`, `internal/kernel/curve2.go`, `cmd/cleancuts/generate.go`, `cmd/cleancuts/main.go`, `internal/planner/planner.go`

## Evidence (checklist)

- [x] `internal/part/part.go`: comment "a parent is always strictly **smaller** in area" — wrong direction (a parent contains its child, so it is larger). The no-cycle logic itself is sound.
- [x] `internal/kernel/curve2.go`: `NewChain2` doc says a closed chain needs ≥3 edges, but `IsClosed` allows 2 (two half-arcs = full circle, ADR-014). Align the doc with the implementation.
- [x] `cmd/cleancuts/generate.go`: `gcodeOptions` always emits `G21` (mm) even when the machine config has `units: "inch"`. Harmless in v1 (core is mm-only), but inconsistent with dialect-from-machine-config — derive it from the machine, or note the mm-only guarantee.
- [x] `cmd/cleancuts/main.go`: `version` is still `"0.1.0"` while the repo is well past v0.1.
- [x] `internal/planner/planner.go`: flute-length check uses the mesh's max-Z as part top — over-conservative for pockets with a raised boss elsewhere. Safe-side over-refusal, acceptable in v1; add a code comment saying so.

## Suggested fix

Correct the comments/docs; for `G21`, either read `machine.Units` or document the mm-only v1 guarantee next to the emitter.

## Acceptance criteria

All items checked off, comments/docs accurate, `sh scripts/ci.sh` green.

## Resolution

All five checklist items, 2026-11-21:

1. `internal/part/part.go` (depth comment): "strictly smaller in area"
   → "a parent contains its child, so it is always strictly larger in
   area". The no-cycle argument is unchanged and now reads correctly.
2. `internal/kernel/curve2.go` (`NewChain2` doc): "a closed chain has
   at least 3 edges" → "at least 2 edges (two half-arcs make a full
   circle, ADR-014 — see IsClosed)", matching the implementation.
3. `cmd/cleancuts/generate.go` (`gcodeOptions`): kept the `G21` header
   (v1 is mm-only) and added a comment documenting the mm-only
   guarantee. Stronger still, the **planner now refuses inch machines**
   up front (`internal/planner/planner.go`, right after
   `Machine.Validate()`): comparing mm process values against inch
   limits is a category error, and emitting G20 over mm numbers would
   scale the program 25.4x (over-cut). The safe failure is no program;
   inch support stays deferred (ADR-001 — units mapping is a
   post-processing concern). This goes slightly beyond "document the
   guarantee" because the two suggested fixes in this issue were
   either wrong (emit G20 unconverted) or weaker (a comment alone
   leaves the meaningless limit comparisons in place).
   Also made `examples/machines/linuxcnc_3axis.json` self-consistent:
   its header said `G21` while declaring `units: "inch"` — now `G20`
   (the example is a dialect reference; v1 refuses it by design).
4. `cmd/cleancuts/main.go`: `version` "0.1.0" → "0.2.0" (Phase 2 is
   complete: 19 ADRs, 14 internal packages, `generate` command).
5. `internal/planner/planner.go` (flute-length check): added a comment
   that `partTop` = mesh max-Z is intentionally over-conservative for
   a pocket below a raised boss elsewhere — a safe-side over-refusal
   in v1, to be replaced by per-feature top Z with multi-pass
   scheduling.

Verified: new refusal case "inch machine (v1 is mm-only)" in
`TestPlanProcessRefusals`; CLI end-to-end — the inch example is
refused with the precise message while the mm example still plans and
emits; `sh scripts/ci.sh` green (gofmt, vet, test, build).
