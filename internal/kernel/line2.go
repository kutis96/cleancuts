package kernel

import "fmt"

// Line2 is a finite line segment in the canonical XY plane.
type Line2 struct {
	Start, End Point2
}

// NewLine2 builds a segment between a and b.
func NewLine2(a, b Point2) Line2 { return Line2{Start: a, End: b} }

// Vec returns the direction vector Start -> End.
func (l Line2) Vec() Vec2 { return l.End.Sub(l.Start) }

// Len returns the length of the segment.
func (l Line2) Len() float64 { return l.Vec().Len() }

// Midpoint returns the point at t = 0.5.
func (l Line2) Midpoint() Point2 {
	return Point2{X: (l.Start.X + l.End.X) / 2, Y: (l.Start.Y + l.End.Y) / 2}
}

// PointAt returns the point at the fractional position t along the segment,
// clamping t to [0, 1].
func (l Line2) PointAt(t float64) Point2 {
	t = Clamp(t, 0, 1)
	return l.Start.Add(l.Vec().Scale(t))
}

// String returns a human-readable form for logs and error messages.
func (l Line2) String() string {
	return fmt.Sprintf("Line2(%s -> %s)", l.Start, l.End)
}
