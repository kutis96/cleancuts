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

// LineArcIntersect returns the points at which the finite segment l meets the
// arc a (0, 1, or 2 points).
//
// It solves the quadratic |A + t·d − C|² = R² for t ∈ [0, 1] and keeps the
// hits whose angle lies on the arc's swept span, so the result is exact for
// the finite shapes (no supporting-line artefacts). Tangency returns the
// single point, not a duplicate.
func LineArcIntersect(l Line2, a Arc2) []Point2 {
	d := l.Vec()
	f := l.Start.Sub(a.Center)
	aa := d.Dot(d)
	if !IsFinite(aa) || aa < AbsTol {
		return nil // degenerate segment
	}
	bb := 2 * f.Dot(d)
	cc := f.Dot(f) - a.Radius*a.Radius
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
		if !IsFinite(t) || t < -AbsTol || t > 1+AbsTol {
			continue
		}
		p := l.PointAt(Clamp(t, 0, 1))
		if a.inArc(p) {
			hits = append(hits, p)
		}
	}
	return dedupe(hits)
}

// ArcArcIntersect returns the points at which the two arcs meet (0, 1, or 2).
//
// It solves the circle–circle intersection and keeps the candidates that lie
// on both swept spans. Concentric arcs never meet transversely and return
// nil. Tangency returns the single point, not a duplicate.
func ArcArcIntersect(a, b Arc2) []Point2 {
	d := b.Center.Sub(a.Center)
	dist := d.Len()
	if !IsFinite(dist) || dist < AbsTol {
		return nil // concentric (or coincident centres)
	}
	if dist > a.Radius+b.Radius+AbsTol {
		return nil // circles too far apart
	}
	if dist < math.Abs(a.Radius-b.Radius)-AbsTol {
		return nil // one circle inside the other
	}
	u := d.Norm()
	// Distance from a's centre to the radical line.
	aa := (a.Radius*a.Radius - b.Radius*b.Radius + dist*dist) / (2 * dist)
	h2 := a.Radius*a.Radius - aa*aa
	if h2 < -AbsTol {
		return nil
	}
	if h2 < 0 {
		h2 = 0
	}
	h := math.Sqrt(h2)
	base := a.Center.Add(u.Scale(aa))
	var hits []Point2
	for _, p := range []Point2{
		base.Add(u.LeftPerp().Scale(h)),
		base.Add(u.LeftPerp().Scale(-h)),
	} {
		if a.inArc(p) && b.inArc(p) {
			hits = append(hits, p)
		}
	}
	return dedupe(hits)
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
