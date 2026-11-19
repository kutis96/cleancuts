package kernel

import "math"

// PointToSegmentDist returns the minimum distance (mm) from p to the finite
// segment l.
func PointToSegmentDist(p Point2, l Line2) float64 {
	d := l.Vec()
	l2 := d.Dot(d)
	if l2 < AbsTol {
		return p.Dist(l.Start)
	}
	t := Clamp(p.Sub(l.Start).Dot(d)/l2, 0, 1)
	return p.Dist(l.PointAt(t))
}

// PointToArcDist returns the minimum distance (mm) from p to the arc a: the
// radial gap if p's angle is on the swept span, else the distance to the
// nearer endpoint.
func PointToArcDist(p Point2, a Arc2) float64 {
	if a.inArc(p) {
		return math.Abs(a.Radius - p.Dist(a.Center))
	}
	return math.Min(p.Dist(a.Start()), p.Dist(a.End()))
}

// PointToEdgeDist returns the minimum distance from p to edge e.
func PointToEdgeDist(p Point2, e Edge2) float64 {
	if e.IsLine() {
		l, _ := e.AsLine()
		return PointToSegmentDist(p, l)
	}
	a, _ := e.AsArc()
	return PointToArcDist(p, a)
}

// PointToChainDist returns the minimum distance from p to any edge of c.
// An empty chain returns +Inf. This is the clearance test comp uses to
// verify "the tool centre stays at least r from the kept boundary"
// (ADR-013), and the safety gate uses it against kept material (ADR-010).
func PointToChainDist(p Point2, c Chain2) float64 {
	min := math.Inf(1)
	for _, e := range c.Edges {
		if d := PointToEdgeDist(p, e); d < min {
			min = d
		}
	}
	return min
}
