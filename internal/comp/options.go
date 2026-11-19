// Package comp is CleanCuts's 2D cutter compensation (ADR-013): it offsets a
// closed contour by the tool radius to produce the tool-centre path(s), with
// explicit outside/inside semantics, corner joins (mitre / chamfer / fillet),
// and a safe-by-construction guarantee — regions the tool cannot reach are
// dropped and reported, never over-cut.
//
// comp is pure geometry on kernel types: it consumes a closed kernel.Chain2
// and produces open or closed kernel.Chain2 tool-centre paths plus
// diagnostics. It emits no moves — generators (toolpath) consume the result
// (ADR-008), so "profile with compensation" and "without" are the same
// generator with different inputs.
package comp

import (
	"fmt"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// Direction names which side of the boundary the tool centre stays on. It is
// an explicit, named input — never inferred — because getting it backwards is
// the classic "cut into the part" 2D bug (ADR-013).
type Direction int

const (
	// Outside: the tool-centre path lies outside the kept boundary —
	// profiling, where the tool is in the stock being removed.
	Outside Direction = iota
	// Inside: the tool-centre path lies inside the boundary — pocket walls,
	// where the kept material is outside the wall.
	Inside
)

// String returns a human-readable name for logs and errors.
func (d Direction) String() string {
	if d == Outside {
		return "outside"
	}
	return "inside"
}

// JoinType selects the corner join in the tool-centre path.
type JoinType int

const (
	// JoinMitre: the centre path turns at the intersection of the two offset
	// edges. A circular tool is tangent to both offset edges, leaving a small
	// cusp at a sharp corner — the standard, correct result (under-cut, not
	// over-cut).
	JoinMitre JoinType = iota
	// JoinChamfer: the offset edges are truncated and joined by a straight
	// segment. Also the automatic fallback when a mitre exceeds the limit.
	JoinChamfer
	// JoinFillet: the corner is rounded by an arc of radius FilletRadius
	// tangent to both offset edges.
	JoinFillet
)

// DefaultMitreLimit is the default mitre limit, in units of the tool radius
// (a typical value is 5).
const DefaultMitreLimit = 5.0

// Options configure an Offset call.
type Options struct {
	// Radius is the tool tip-envelope radius at the working Z (ADR-011);
	// must be > 0. For a flat end mill this is D/2.
	Radius float64
	// Direction is Outside (profile) or Inside (pocket wall).
	Direction Direction
	// Join selects the corner join (default JoinMitre).
	Join JoinType
	// MitreLimit bounds how far a mitre point may extend from the corner, in
	// units of Radius; beyond it the join becomes a chamfer. <= 0 uses
	// DefaultMitreLimit.
	MitreLimit float64
	// FilletRadius is the fillet arc radius for JoinFillet; <= 0 uses Radius.
	FilletRadius float64
}

// validate enforces the invariants of Options.
func (o Options) validate() error {
	if !kernel.IsFinite(o.Radius) || o.Radius <= 0 {
		return fmt.Errorf("comp: radius must be > 0 (got %g)", o.Radius)
	}
	if o.Direction < Outside || o.Direction > Inside {
		return fmt.Errorf("comp: unknown direction %d (want outside or inside)", int(o.Direction))
	}
	switch o.Join {
	case JoinMitre, JoinChamfer, JoinFillet:
	default:
		return fmt.Errorf("comp: unknown join type %d (want mitre, chamfer, or fillet)", int(o.Join))
	}
	if o.MitreLimit < 0 {
		return fmt.Errorf("comp: mitre limit must be >= 0 (got %g)", o.MitreLimit)
	}
	if o.FilletRadius < 0 {
		return fmt.Errorf("comp: fillet radius must be >= 0 (got %g)", o.FilletRadius)
	}
	return nil
}

// mitreLimitMM returns the mitre limit in millimetres.
func (o Options) mitreLimitMM() float64 {
	l := o.MitreLimit
	if l <= 0 {
		l = DefaultMitreLimit
	}
	return l * o.Radius
}

// filletRadius returns the effective fillet radius in millimetres.
func (o Options) filletRadius() float64 {
	if o.FilletRadius <= 0 {
		return o.Radius
	}
	return o.FilletRadius
}

// Diagnostic is a note about a safe resolution the offset took. Diagnostics
// are informational: Offset never fails by over-cutting; it fails by leaving
// material and saying so.
type Diagnostic struct {
	Level   string `json:"level"` // "info" | "warning"
	Message string `json:"message"`
}

// Result is the output of Offset: the tool-centre path(s) and any notes.
//
// Chains may be closed (loops) or open (in-and-out paths, e.g. a slot the
// tool enters and exits). An empty Chains with a diagnostic means the whole
// feature is unreachable at this tool radius — the safe outcome.
type Result struct {
	Chains      []kernel.Chain2 `json:"chains"`
	Diagnostics []Diagnostic    `json:"diagnostics"`
}

// Unreachable reports whether the offset dropped the whole feature (or any
// region) because the tool cannot reach it at this radius.
func (r Result) Unreachable() bool {
	for _, d := range r.Diagnostics {
		if d.Level == "warning" {
			return true
		}
	}
	return false
}
