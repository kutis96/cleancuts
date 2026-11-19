package kernel

import "fmt"

// Point2 is a position in the canonical XY plane (mm).
type Point2 struct {
	X, Y float64
}

// P2 is a convenience constructor for Point2.
func P2(x, y float64) Point2 { return Point2{X: x, Y: y} }

// ToVec interprets the point as a vector from the origin.
func (p Point2) ToVec() Vec2 { return Vec2{X: p.X, Y: p.Y} }

// Add returns the point displaced by vector v.
func (p Point2) Add(v Vec2) Point2 { return Point2{X: p.X + v.X, Y: p.Y + v.Y} }

// Sub returns the vector from o to p.
func (p Point2) Sub(o Point2) Vec2 { return Vec2{X: p.X - o.X, Y: p.Y - o.Y} }

// Dist returns the Euclidean distance to o.
func (p Point2) Dist(o Point2) float64 { return p.Sub(o).Len() }

// Eq reports whether p and o coincide within an absolute tolerance.
func (p Point2) Eq(o Point2, eps float64) bool {
	return Equal(p.X, o.X, eps) && Equal(p.Y, o.Y, eps)
}

// Finite reports whether both coordinates are finite numbers.
func (p Point2) Finite() bool { return IsFinite(p.X) && IsFinite(p.Y) }

// String returns a human-readable form for logs and error messages.
func (p Point2) String() string { return fmt.Sprintf("Point2(%g, %g)", p.X, p.Y) }
