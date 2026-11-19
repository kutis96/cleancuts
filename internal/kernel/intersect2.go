package kernel

import "math"

// inArc reports whether point p lies on the arc (within AbsTol of the
// swept span), assuming p is at the arc's radius.
func (a Arc2) inArc(p Point2) bool {
	theta := NormalizeAngle(math.Atan2(p.Y-a.Center.Y, p.X-a.Center.X))
	s := a.Sweep()
	if a.CW {
		return NormalizeAngle(a.A0-theta) <= s+AbsTol
	}
	return NormalizeAngle(theta-a.A0) <= s+AbsTol
}

// dedupe returns ps without points that coincide within AbsTol.
func dedupe(ps []Point2) []Point2 {
	out := ps[:0]
	for _, p := range ps {
		dup := false
		for _, q := range out {
			if p.Eq(q, AbsTol) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

// LineCircleIntersect returns the points at which the infinite line
// containing l meets the full circle (centre c, radius r) (0, 1, or 2
// points). Join resolution in comp (ADR-013) uses the supporting curve, not
// the finite segment, because a mitre point lies beyond the segment's
// endpoints. Tangency returns the single point, not a duplicate.
func LineCircleIntersect(l Line2, c Point2, r float64) []Point2 {
	d := l.Vec()
	f := l.Start.Sub(c)
	aa := d.Dot(d)
	if !IsFinite(aa) || aa < AbsTol {
		return nil // degenerate segment
	}
	bb := 2 * f.Dot(d)
	cc := f.Dot(f) - r*r
	disc := bb*bb - 4*aa*cc
	if disc < -AbsTol {
		return nil
	}
	if disc < 0 {
		disc = 0
	}
	sq := math.Sqrt(disc)
	var hits []Point2
	for _, t := range []float64{(-bb - sq) / (2 * aa), (-bb + sq) / (2 * aa)} {
		if !IsFinite(t) {
			continue
		}
		hits = append(hits, l.Start.Add(d.Scale(t)))
	}
	return dedupe(hits)
}

// CircleCircleIntersect returns the points at which the two full circles
// meet (0, 1, or 2 points). Concentric or nested circles return nil.
func CircleCircleIntersect(c1 Point2, r1 float64, c2 Point2, r2 float64) []Point2 {
	dist := c2.Dist(c1)
	if !IsFinite(dist) || dist < AbsTol {
		return nil // concentric (or coincident centres)
	}
	if dist > r1+r2+AbsTol {
		return nil // circles too far apart
	}
	if dist < math.Abs(r1-r2)-AbsTol {
		return nil // one circle inside the other
	}
	u := c2.Sub(c1).Norm()
	a := (r1*r1 - r2*r2 + dist*dist) / (2 * dist)
	h2 := r1*r1 - a*a
	if h2 < -AbsTol {
		return nil
	}
	if h2 < 0 {
		h2 = 0
	}
	h := math.Sqrt(h2)
	base := c1.Add(u.Scale(a))
	return dedupe([]Point2{
		base.Add(u.LeftPerp().Scale(h)),
		base.Add(u.LeftPerp().Scale(-h)),
	})
}

// LineArcIntersect returns the points at which the finite segment l meets the
// arc a (0, 1, or 2 points). It is LineCircleIntersect filtered to the
// segment range and the arc's swept span, so the result is exact for the
// finite shapes (no supporting-line artefacts).
func LineArcIntersect(l Line2, a Arc2) []Point2 {
	var out []Point2
	for _, p := range LineCircleIntersect(l, a.Center, a.Radius) {
		if !onSegment(l.Start, l.End, p) {
			continue
		}
		if a.inArc(p) {
			out = append(out, p)
		}
	}
	return out
}

// ArcArcIntersect returns the points at which the two arcs meet (0, 1, or
// 2). It is CircleCircleIntersect filtered to both swept spans. Concentric
// arcs never meet transversely and return nil.
func ArcArcIntersect(a, b Arc2) []Point2 {
	var out []Point2
	for _, p := range CircleCircleIntersect(a.Center, a.Radius, b.Center, b.Radius) {
		if a.inArc(p) && b.inArc(p) {
			out = append(out, p)
		}
	}
	return out
}

// CurveIntersect returns the intersection points of two edges (any mix of
// line and arc). It is the single entry point comp's join resolution and
// self-intersection detection use.
func CurveIntersect(a, b Edge2) []Point2 {
	switch {
	case a.IsLine() && b.IsLine():
		l1, _ := a.AsLine()
		l2, _ := b.AsLine()
		p, ok := LineLineIntersect(l1, l2)
		if !ok {
			return nil
		}
		if !onSegment(l1.Start, l1.End, p) || !onSegment(l2.Start, l2.End, p) {
			return nil
		}
		return []Point2{p}
	case a.IsLine() && b.IsArc():
		l, _ := a.AsLine()
		ar, _ := b.AsArc()
		return LineArcIntersect(l, ar)
	case a.IsArc() && b.IsLine():
		ar, _ := a.AsArc()
		l, _ := b.AsLine()
		return LineArcIntersect(l, ar)
	default:
		ar1, _ := a.AsArc()
		ar2, _ := b.AsArc()
		return ArcArcIntersect(ar1, ar2)
	}
}
