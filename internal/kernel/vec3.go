package kernel

import (
	"fmt"
	"math"
)

// Vec3 is a direction or displacement in the canonical XYZ space (mm).
// The canonical system is right-handed and Z-up (ADR-001): Z is the spindle
// axis, and positive Z is "up" (away from the workpiece).
type Vec3 struct {
	X, Y, Z float64
}

// V3 is a convenience constructor for Vec3.
func V3(x, y, z float64) Vec3 { return Vec3{X: x, Y: y, Z: z} }

// Add returns v + o.
func (v Vec3) Add(o Vec3) Vec3 { return Vec3{X: v.X + o.X, Y: v.Y + o.Y, Z: v.Z + o.Z} }

// Sub returns v - o.
func (v Vec3) Sub(o Vec3) Vec3 { return Vec3{X: v.X - o.X, Y: v.Y - o.Y, Z: v.Z - o.Z} }

// Scale returns v * s.
func (v Vec3) Scale(s float64) Vec3 { return Vec3{X: v.X * s, Y: v.Y * s, Z: v.Z * s} }

// Dot returns the scalar dot product v · o.
func (v Vec3) Dot(o Vec3) float64 { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }

// Cross returns the vector cross product v × o.
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		X: v.Y*o.Z - v.Z*o.Y,
		Y: v.Z*o.X - v.X*o.Z,
		Z: v.X*o.Y - v.Y*o.X,
	}
}

// Len returns the Euclidean length of v.
func (v Vec3) Len() float64 { return math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z) }

// Norm returns a unit vector in the direction of v. The zero vector
// normalises to itself.
func (v Vec3) Norm() Vec3 {
	l := v.Len()
	if l == 0 {
		return v
	}
	return Vec3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}

// Eq reports whether v and o are equal within an absolute tolerance.
func (v Vec3) Eq(o Vec3, eps float64) bool {
	return Equal(v.X, o.X, eps) && Equal(v.Y, o.Y, eps) && Equal(v.Z, o.Z, eps)
}

// Finite reports whether all coordinates are finite numbers.
func (v Vec3) Finite() bool { return IsFinite(v.X) && IsFinite(v.Y) && IsFinite(v.Z) }

// String returns a human-readable form for logs and error messages.
func (v Vec3) String() string {
	return fmt.Sprintf("Vec3(%g, %g, %g)", v.X, v.Y, v.Z)
}
