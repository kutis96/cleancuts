# 002: README.md, architecture.md, and HANDOVER.md still describe the v0.1 state

- **Status:** open
- **Type:** docs
- **Priority:** medium
- **Found:** 2026-11-21 (independent code review)
- **Files:** `README.md`, `docs/architecture.md`, `HANDOVER.md`

## Summary

Phase 2 is nearly complete in the working tree, but the three main docs still describe the v0.1 scaffold.

## Evidence (checklist)

- [ ] `README.md`: status line ("v0.1 — foundation scaffold … Real part import, cutter compensation … arrive in later phases") is no longer true; the package table and project tree list only the 7 v0.1 packages — missing `comp`, `planning`, `planner`, `safety`, `stock`, `mesh`, `part`, `fixture`.
- [ ] `docs/architecture.md`: the per-stage sections are current (keep them), but the "Package contracts" dependency diagram and the "What is deliberately NOT in the core (yet)" list still claim part import and 2D comp are to come — both are implemented.
- [ ] `HANDOVER.md`: snapshot dated 2026-11-19 (comp slice 1 "not implemented", cites only ADR-001…013, "working tree clean"). ADRs now run to 019 and the tree is actively in-flight. Refresh at the next commit point.

## Suggested fix

Update all three to reflect the implemented package set and ADR-001…019. Keep the honest "deliberately deferred" claims (phases 3–6) — only correct the parts that are now done.

## Acceptance criteria

- No "arrive in later phases" / "to come" claim for an implemented feature in any of the three docs.
- Package table, project tree, and dependency diagram list all 14 internal packages + CLI.
- HANDOVER.md snapshot date and ADR range match reality at commit time.

## Resolution

(open — when closing, document here: which file(s)/sections changed,
what and why, how it was verified, and date + commit. The record must
stand on its own without reading the commit.)
