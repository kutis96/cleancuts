package kernel

import (
	"fmt"
	"math"
)

// Arc2 is a circular arc in the canonical XY plane, defined by its centre,
// radius, and start/end angles (radians). Sweep direction is given by CW.
//
// Angles are measured from the +X axis, increasing counter-clockwise. The
// arc runs from A0 to A1; if CW is true it sweeps clockwise (decreasing
// angle), otherwise counter-clockwise (increasing angle). This parameterisation
// is chosen so that an arc can be reconstructed unambiguously from its
// centre, two endpoints, and direction, which is how cutter compensation and
// arc fitting will construct arcs later.
type Arc2 struct {
	Center Point2
	Radius float64
	A0     float64 // start angle (rad)
	A1     float64 // end angle (rad)
	CW     bool
}

// NewArc2 builds an arc from its centre, start point, end point, and
// direction. The radius is derived from the centre and start point.
func NewArc2(center, start, end Point2, cw bool) Arc2 {
	r := center.Dist(start)
	a0 := NormalizeAngle(math.Atan2(start.Y-center.Y, start.X-center.X))
	a1 := NormalizeAngle(math.Atan2(end.Y-center.Y, end.X-center.X))
	return Arc2{Center: center, Radius: r, A0: a0, A1: a1, CW: cw}
}

// Start returns the arc's start point.
func (a Arc2) Start() Point2 { return a.PointAtAngle(a.A0) }

// End returns the arc's end point.
func (a Arc2) End() Point2 { return a.PointAtAngle(a.A1) }

// PointAtAngle returns the point on the arc at the given absolute angle.
func (a Arc2) PointAtAngle(theta float64) Point2 {
	return Point2{
		X: a.Center.X + a.Radius*math.Cos(theta),
		Y: a.Center.Y + a.Radius*math.Sin(theta),
	}
}

// signedSweep returns the swept angle, positive for CCW and negative for CW.
func (a Arc2) signedSweep() float64 {
	if a.CW {
		return -NormalizeAngle(a.A0 - a.A1)
	}
	return NormalizeAngle(a.A1 - a.A0)
}

// Sweep returns the absolute swept angle in radians, in [0, 2π].
func (a Arc2) Sweep() float64 {
	s := a.signedSweep()
	if s < 0 {
		return -s
	}
	return s
}

// Len returns the arc length in mm.
func (a Arc2) Len() float64 { return a.Radius * a.Sweep() }

// PointAt returns the point at the fractional position t in [0, 1] along the
// arc, in the direction of travel.
func (a Arc2) PointAt(t float64) Point2 {
	t = Clamp(t, 0, 1)
	return a.PointAtAngle(a.A0 + t*a.signedSweep())
}

// String returns a human-readable form for logs and error messages.
func (a Arc2) String() string {
	dir := "CCW"
	if a.CW {
		dir = "CW"
	}
	return fmt.Sprintf("Arc2(%s r=%g %s)", a.Center, a.Radius, dir)
}
