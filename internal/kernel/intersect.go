package kernel

import "math"

// LineLineIntersect returns the intersection of the two infinite lines that
// contain a and b, and whether the lines are non-parallel. The result is the
// intersection of the supporting lines, not necessarily of the finite
// segments; use SegmentsIntersect for the finite-segment case.
func LineLineIntersect(a, b Line2) (Point2, bool) {
	da := a.Vec()
	db := b.Vec()
	denom := da.Cross(db)
	if !IsFinite(denom) || math.Abs(denom) < AbsTol {
		return Point2{}, false // parallel or coincident
	}
	ab := b.Start.Sub(a.Start)
	t := ab.Cross(db) / denom
	return a.Start.Add(da.Scale(t)), true
}

// SegmentsIntersect reports whether the closed segments a and b intersect,
// including the cases where they share an endpoint or overlap collinearly.
//
// The test is the standard orientation test augmented with collinear
// on-segment checks, using AbsTol for the "collinear" band. It is exact for
// non-collinear inputs and robust for the degenerate ones that occur in
// practice.
func SegmentsIntersect(a, b Line2) bool {
	o1 := crossSign(a.Start, a.End, b.Start)
	o2 := crossSign(a.Start, a.End, b.End)
	o3 := crossSign(b.Start, b.End, a.Start)
	o4 := crossSign(b.Start, b.End, a.End)

	if o1 != o2 && o3 != o4 {
		return true
	}
	if o1 == 0 && onSegment(a.Start, a.End, b.Start) {
		return true
	}
	if o2 == 0 && onSegment(a.Start, a.End, b.End) {
		return true
	}
	if o3 == 0 && onSegment(b.Start, b.End, a.Start) {
		return true
	}
	if o4 == 0 && onSegment(b.Start, b.End, a.End) {
		return true
	}
	return false
}

// crossSign returns -1, 0, or +1 for the sign of (v - p) × (w - p), treating
// |value| < AbsTol as collinear (0).
func crossSign(p, v, w Point2) int {
	c := (v.X-p.X)*(w.Y-p.Y) - (v.Y-p.Y)*(w.X-p.X)
	switch {
	case math.Abs(c) < AbsTol:
		return 0
	case c > 0:
		return 1
	default:
		return -1
	}
}

// onSegment reports whether q lies on the closed segment p..r, assuming q is
// collinear with p and r.
func onSegment(p, r, q Point2) bool {
	return math.Min(p.X, r.X)-AbsTol <= q.X && q.X <= math.Max(p.X, r.X)+AbsTol &&
		math.Min(p.Y, r.Y)-AbsTol <= q.Y && q.Y <= math.Max(p.Y, r.Y)+AbsTol
}
