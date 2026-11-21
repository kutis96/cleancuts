# 001: PointInContour undercounts parity when a ray hits one arc edge twice

- **Status:** closed
- **Type:** bug
- **Priority:** high
- **Found:** 2026-11-21 (independent code review; confirmed by execution in a sandbox copy outside the repo)
- **Files:** `internal/kernel/contain2.go`, `internal/kernel/contain2_test.go`, `internal/stock/stock.go`, `internal/comp/join.go`

## Summary

`rayCrossesArc` ([contain2.go](../internal/kernel/contain2.go)) returns a single `bool` per edge, so a ray that crosses the *same* arc edge at two distinct points is counted as **one** crossing. The even-odd parity then flips, and a point **outside** the contour is classified **inside**.

## Evidence

Reproduced in a sandbox copy (not committed to this repo):

1. Circle as 220° + 140° arcs; point (−1.866, −0.5) — provably outside the unit circle — → `PointInContour` reports **inside**. FAIL.
2. Circle as two 180° arcs — the exact chain shape `stock.SectionAt` emits for cylinder stock — outside point (−1.632, 0.143) → reported **inside**. FAIL.

The existing `TestPointInContourCircle` passes only because its test points sit where the 30° ray crosses two *different* arcs; the "lens" region beyond an arc's bulge is untested.

Reachability in v1:

- `stock.inFootprint` / `Fits` on **cylinder stock** (semicircle section) and **prism stock with arc edges** (rounded-rectangle examples): a part vertex or edge midpoint in the lens region just beyond an arc can make the "does the part fit" gate wrongly pass.
- `comp/join.go` `mitreAcceptable` calls `PointInContour(p, orig)` on the *original* contour — a misclassification flips mitre acceptance for arc-containing originals.
- Not affected in v1: `part.Detect` (mesh-slice loops are line-only) and the ADR-012 safety gate (exact distances, no point-in-contour).

## Suggested fix

Count crossings per hit, not per edge:

```go
// was: func rayCrossesArc(c, start, end kernel.Point2) bool { ... }
func rayArcCrossings(c, start, end kernel.Point2) int {
	if c == start || c == end {
		return 0
	}
	t1, t2, ok := kernel.RayCircleIntersection(c, kernel.Vector2{X: 1, Y: 0}, start, end)
	if !ok {
		return 0
	}
	n := 0
	if kernel.RayPointOnArc(c, kernel.Vector2{X: t1, Y: 0}, start, end) {
		n++
	}
	if t2 > t1+1e-12 && kernel.RayPointOnArc(c, kernel.Vector2{X: t2, Y: 0}, start, end) {
		n++
	}
	return n
}
// PointInContour: crossings += rayArcCrossings(c, e.Start, e.End)
```

Then add a regression test in `contain2_test.go` for the lens region (either case above), e.g. a 220°+140° arc pair with the outside point (−1.866, −0.5) expected `false`.

## Acceptance criteria

- New table case(s) with lens-region outside points for a >180° arc pair **and** two 180° arcs; fails before the fix, passes after.
- `PointInContour` doc comment notes arcs are counted per distinct crossing point.
- `sh scripts/ci.sh` green.

## Resolution

Fixed in `internal/kernel/contain2.go`:

- `rayCrossesArc` (returned one `bool` per edge) was replaced by
  `rayArcCrossings`, which counts **per hit point**: each distinct root
  `t` of the ray–circle quadratic that lands on the arc's swept span is
  one crossing. A ray meeting a >180° arc at two points now contributes
  2 (even), restoring correct parity. At tangency the two roots
  coincide and both count — 2 is even, so a touch flips nothing
  (algebraic multiplicity, the standard even-odd convention).
- `PointInContour` now does `crossings += rayArcCrossings(...)` and its
  doc comment states the per-hit counting rule (per the acceptance
  criteria).

No other files needed changes: `stock.inFootprint`/`Fits`,
`comp/join.go` `mitreAcceptable`, `comp/offset_assemble.go`, and
`part.Detect` all call `PointInContour` and inherit the fix.

Verified:

- New regression test `TestPointInContourArcDoubleHit` in
  `internal/kernel/contain2_test.go`: unit circle as a 220°+140° arc
  pair and as two 180° arcs (the `stock.SectionAt` cylinder shape);
  outside points in the lens region (|p| = 1.93 and 1.64, both > 1)
  report outside, inside points report inside.
- Confirmed the test **fails against the pre-fix code** (both lens
  cases reported inside) and passes after — a true regression pin.
- Full suite: `sh scripts/ci.sh` → gofmt, vet, test, build all pass
  (2026-11-21).

Date: 2026-11-21 · Commit: (this commit).
