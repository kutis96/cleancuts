package kernel

import (
	"fmt"
	"math"
)

// Vec2 is a direction or displacement in the canonical XY plane (mm).
type Vec2 struct {
	X, Y float64
}

// V2 is a convenience constructor for Vec2.
func V2(x, y float64) Vec2 { return Vec2{X: x, Y: y} }

// Add returns v + o.
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{X: v.X + o.X, Y: v.Y + o.Y} }

// Sub returns v - o.
func (v Vec2) Sub(o Vec2) Vec2 { return Vec2{X: v.X - o.X, Y: v.Y - o.Y} }

// Scale returns v * s.
func (v Vec2) Scale(s float64) Vec2 { return Vec2{X: v.X * s, Y: v.Y * s} }

// Dot returns the scalar dot product v · o.
func (v Vec2) Dot(o Vec2) float64 { return v.X*o.X + v.Y*o.Y }

// Cross returns the z-component of the 3D cross product v × o. A positive
// result means o is counter-clockwise from v.
func (v Vec2) Cross(o Vec2) float64 { return v.X*o.Y - v.Y*o.X }

// Len returns the Euclidean length of v.
func (v Vec2) Len() float64 { return math.Hypot(v.X, v.Y) }

// Norm returns a unit vector in the direction of v. The zero vector
// normalises to itself.
func (v Vec2) Norm() Vec2 {
	l := v.Len()
	if l == 0 {
		return v
	}
	return Vec2{X: v.X / l, Y: v.Y / l}
}

// LeftPerp returns v rotated 90 degrees counter-clockwise.
func (v Vec2) LeftPerp() Vec2 { return Vec2{X: -v.Y, Y: v.X} }

// RightPerp returns v rotated 90 degrees clockwise.
func (v Vec2) RightPerp() Vec2 { return Vec2{X: v.Y, Y: -v.X} }

// Angle returns the direction of v in radians in [0, 2π).
func (v Vec2) Angle() float64 { return NormalizeAngle(math.Atan2(v.Y, v.X)) }

// Rotate returns v rotated by theta radians counter-clockwise.
func (v Vec2) Rotate(theta float64) Vec2 {
	c, s := math.Cos(theta), math.Sin(theta)
	return Vec2{X: c*v.X - s*v.Y, Y: s*v.X + c*v.Y}
}

// Eq reports whether v and o are equal within an absolute tolerance.
func (v Vec2) Eq(o Vec2, eps float64) bool {
	return Equal(v.X, o.X, eps) && Equal(v.Y, o.Y, eps)
}

// Finite reports whether both coordinates are finite numbers.
func (v Vec2) Finite() bool { return IsFinite(v.X) && IsFinite(v.Y) }

// String returns a human-readable form for logs and error messages.
func (v Vec2) String() string { return fmt.Sprintf("Vec2(%g, %g)", v.X, v.Y) }
