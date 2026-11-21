package kernel

import "math"

// PointInContour reports whether p lies strictly inside the closed contour c
// (even-odd rule). A point on the boundary (within AbsTol of it) reports
// false — "on the line" is not "inside the part", which is the safe reading
// for both planning and the safety gate (ADR-010).
//
// The test casts a ray from p at a fixed 30° direction and counts crossings
// of the ray with each edge, analytically. Crossings are counted **per
// distinct hit point, not per edge**: a ray can meet one arc (a >180° span)
// at two points, and each flips the parity (issue 001). A tangent double-hit
// counts 2 — even — so it flips nothing, matching algebraic multiplicity.
//
// The 30° direction is deliberate: it avoids the vertex-degeneracies an
// axis-aligned ray would hit on axis-aligned (the common) test and part
// data.
func PointInContour(p Point2, c Chain2) bool {
	if !c.IsClosed() {
		return false
	}
	if PointToChainDist(p, c) < AbsTol {
		return false // on the boundary: treat as outside (safe side)
	}
	// Unit ray direction at 30° above +X: a fixed, non-axis-aligned angle that
	// avoids the vertex degeneracies an axis-aligned ray would hit on the
	// axis-aligned data that is common in tests and parts.
	dx := math.Sqrt(3) / 2
	dy := 0.5
	crossings := 0
	for _, e := range c.Edges {
		if e.IsLine() {
			l, _ := e.AsLine()
			if rayCrossesLine(p, dx, dy, l) {
				crossings++
			}
		} else {
			a, _ := e.AsArc()
			crossings += rayArcCrossings(p, dx, dy, a)
		}
	}
	return crossings%2 == 1
}

// rayCrossesLine reports whether the ray p + t·d (t > AbsTol) crosses the
// finite segment l.
func rayCrossesLine(p Point2, dx, dy float64, l Line2) bool {
	// Solve p + t·d = A + u·v for the ray parameter t and the segment
	// parameter u, by Cramer's rule on
	//   t·dx − u·vx = Ax − px
	//   t·dy − u·vy = Ay − py
	v := l.Vec()
	f := l.Start.Sub(p)
	det := v.X*dy - v.Y*dx
	if math.Abs(det) < AbsTol {
		return false // parallel (or collinear: a collinear overlap is a
	} // measure-zero case the fixed ray direction avoids)
	t := (v.X*f.Y - v.Y*f.X) / det
	u := (dx*f.Y - dy*f.X) / det
	return t > AbsTol && u >= -AbsTol && u <= 1+AbsTol
}

// rayArcCrossings counts the distinct hit points of the ray p + t·d
// (t > AbsTol) on the arc a. A ray can meet a >180° arc twice — each hit
// is one crossing for the even-odd parity, so this returns a count, not a
// bool (issue 001). At tangency the two roots coincide and both are
// counted: 2 is even, so a touch flips nothing (algebraic multiplicity).
func rayArcCrossings(p Point2, dx, dy float64, a Arc2) int {
	f := p.Sub(a.Center)
	bb := 2 * f.Dot(Vec2{X: dx, Y: dy})
	cc := f.Dot(f) - a.Radius*a.Radius
	disc := bb*bb - 4*cc
	if disc < -AbsTol {
		return 0
	}
	if disc < 0 {
		disc = 0
	}
	sq := math.Sqrt(disc)
	n := 0
	for _, t := range []float64{(-bb - sq) / 2, (-bb + sq) / 2} {
		if t <= AbsTol {
			continue
		}
		hit := p.Add(Vec2{X: dx, Y: dy}.Scale(t))
		if a.inArc(hit) {
			n++
		}
	}
	return n
}
