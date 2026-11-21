# 004: docs/part.md had the climb direction backwards

- **Status:** closed
- **Type:** docs (bug)
- **Priority:** (was) high — user-facing correctness
- **Found:** 2026-11-21 (independent code review)
- **Files:** `docs/part.md`

## Summary

The committed text said "CCW spindle → profiles are milled CCW, pockets CW (climb)". That is the reverse of the code and of the standard convention for a **right-handed** spindle: profiles are milled **CW**, pockets **CCW** — both climb. The planning code and the `planning.md` direction table were already correct; only `part.md` was wrong.

## Evidence

- `planning.go`/`planner.go` set the climb direction per ADR-016; `planning.md`'s table matches.
- Vector derivation: for a right-handed spindle cutting a profile on the right, material is on the left of the travel direction → CW profile / CCW pocket.

## Resolution

Fixed in the working tree on 2026-11-21 (uncommitted as of that date).
**Commit it as its own doc-fix commit** rather than bundling it silently
into the slice-L commit, so the fix is greppable in history.
