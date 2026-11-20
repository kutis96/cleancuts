package kernel

import "fmt"

// Point3 is a position in the canonical XYZ space (mm), right-handed and
// Z-up (ADR-001).
//
// The JSON form is the canonical lowercase {"x":…, "y":…, "z":…} spelling
// used by every CleanCuts schema (ADR-016).
type Point3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// P3 is a convenience constructor for Point3.
func P3(x, y, z float64) Point3 { return Point3{X: x, Y: y, Z: z} }

// ToVec interprets the point as a vector from the origin.
func (p Point3) ToVec() Vec3 { return Vec3{X: p.X, Y: p.Y, Z: p.Z} }

// Add returns the point displaced by vector v.
func (p Point3) Add(v Vec3) Point3 {
	return Point3{X: p.X + v.X, Y: p.Y + v.Y, Z: p.Z + v.Z}
}

// Sub returns the vector from o to p.
func (p Point3) Sub(o Point3) Vec3 {
	return Vec3{X: p.X - o.X, Y: p.Y - o.Y, Z: p.Z - o.Z}
}

// Dist returns the Euclidean distance to o.
func (p Point3) Dist(o Point3) float64 { return p.Sub(o).Len() }

// Eq reports whether p and o coincide within an absolute tolerance.
func (p Point3) Eq(o Point3, eps float64) bool {
	return Equal(p.X, o.X, eps) && Equal(p.Y, o.Y, eps) && Equal(p.Z, o.Z, eps)
}

// Finite reports whether all coordinates are finite numbers.
func (p Point3) Finite() bool { return IsFinite(p.X) && IsFinite(p.Y) && IsFinite(p.Z) }

// String returns a human-readable form for logs and error messages.
func (p Point3) String() string {
	return fmt.Sprintf("Point3(%g, %g, %g)", p.X, p.Y, p.Z)
}
