// Package kernel is CleanCuts's dependency-free geometry and math core.
//
// All coordinates are expressed in the canonical work coordinate system
// (WCS): right-handed, Z-up, X to the right, Y forward, measured in
// millimetres. A single canonical system is used throughout the entire
// application to avoid the class of bugs caused by re-interpreting and
// re-interpolating coordinates between layers. See
// docs/design-decisions.md (ADR-001, ADR-006) for the full rationale and the
// invariants every package in the repository must uphold.
//
// The package intentionally has no external dependencies so that it can be
// embedded anywhere the Go core can (including a future WASM build) and so
// that its behaviour is fully determined by this source.
package kernel

import (
	"math"
)

// AbsTol is the default absolute tolerance (mm for lengths, radians for
// angles) used by geometric comparisons. Callers that need tighter or looser
// behaviour should pass an explicit epsilon to the comparison helpers instead
// of relying on this constant.
const AbsTol = 1e-9

// Equal reports whether a and b are equal within an absolute tolerance.
func Equal(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

// EqualTol reports whether a and b are equal within AbsTol.
func EqualTol(a, b float64) bool { return Equal(a, b, AbsTol) }

// Clamp returns v constrained to the closed interval [lo, hi].
func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// NormalizeAngle wraps a to the half-open range [0, 2π).
func NormalizeAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}

// NormalizePi wraps a to the closed range [-π, π].
func NormalizePi(a float64) float64 {
	a = NormalizeAngle(a)
	if a > math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

// IsFinite reports whether v is a finite number (not NaN and not ±Inf).
func IsFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
