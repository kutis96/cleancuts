# 002: README.md, architecture.md, and HANDOVER.md still describe the v0.1 state

- **Status:** closed
- **Type:** docs
- **Priority:** medium
- **Found:** 2026-11-21 (independent code review)
- **Files:** `README.md`, `docs/architecture.md`, `HANDOVER.md`

## Summary

Phase 2 is nearly complete in the working tree, but the three main docs still describe the v0.1 scaffold.

## Evidence (checklist)

- [x] `README.md`: status line ("v0.1 — foundation scaffold … Real part import, cutter compensation … arrive in later phases") is no longer true; the package table and project tree list only the 7 v0.1 packages — missing `comp`, `planning`, `planner`, `safety`, `stock`, `mesh`, `part`, `fixture`.
- [x] `docs/architecture.md`: the per-stage sections are current (keep them), but the "Package contracts" dependency diagram and the "What is deliberately NOT in the core (yet)" list still claim part import and 2D comp are to come — both are implemented.
- [x] `HANDOVER.md`: snapshot dated 2026-11-19 (comp slice 1 "not implemented", cites only ADR-001…013, "working tree clean"). ADRs now run to 019 and the tree is actively in-flight. Refresh at the next commit point.

## Suggested fix

Update all three to reflect the implemented package set and ADR-001…019. Keep the honest "deliberately deferred" claims (phases 3–6) — only correct the parts that are now done.

## Acceptance criteria

- No "arrive in later phases" / "to come" claim for an implemented feature in any of the three docs.
- Package table, project tree, and dependency diagram list all 14 internal packages + CLI.
- HANDOVER.md snapshot date and ADR range match reality at commit time.

## Resolution

All three docs updated to the implemented v0.2 state, 2026-11-21:

- `README.md`: status line now says v0.2 / 2.5D core complete (the old
  "v0.1 — foundation scaffold … arrive in later phases" claim removed);
  the package table lists all 14 internal packages + CLI (added
  mesh, part, stock, fixture, comp, planning, planner, safety, and
  Program/generate notes); the project tree matches reality (adds
  `issues/`, `HANDOVER.md`); a **`generate` quick-start** was added with
  the full example command and the keep-out behaviour.
- `docs/architecture.md`: Stage 1 now describes `internal/part`
  (recipe + nesting-parity detection, ADR-018) instead of "not yet a
  separate package"; Stage 1.5 says "Phase 2 (done)" and covers
  `planning` + `planner` + `moves.Program` (ADR-018/019); "Data flow,
  concretely" now walks the real `generate` pipeline (5 steps); the
  package dependency diagram was redrawn for all 14 packages + CLI with
  a reading note; "What is deliberately NOT in the core (yet)" no
  longer lists part import or 2D comp as to-come (both done) and adds
  the real deferrals (multi-pass Z, inch machines).
- `HANDOVER.md`: full rewrite — snapshot dated 2026-11-21, v0.2,
  Phase 2 complete (slices F→L ✅), ADR-001…019, all issues closed,
  the post-L hardening thread (coordinate formatting, inch refusal,
  issue 001 kernel fix), and "pick up here" = Phase 3 **awaiting the
  user's go-ahead**.
- Also in the same docs sweep: `docs/roadmap.md` (Phase 2 ✅ + exit
  criterion met + slice line flipped + inch deferral) and
  `docs/gcode.md` (`Program`, `EmitProgram`, `CoordPrecision`, the
  mm-only v1 note) — both were stale for slice L and are now current.

Verified: grep shows no "arrive in later phases / to come" claim for
an implemented feature in any of the docs; package table, project
tree, and dependency diagram list all 14 internal packages + CLI;
HANDOVER date/ADR range match the commit trail; `sh scripts/ci.sh`
green (docs-only change, but the gate was run per AGENTS.md).
