// Package moves defines CleanCuts's machine-independent toolpath
// representation — the "intermediate format" that sits between toolpath
// generation and machine-specific post-processing.
//
// A Path is an ordered, continuous list of Moves. Each Move is a rapid, a
// linear feed, or a circular arc, expressed entirely in the canonical WCS
// (Z-up, millimetres). This is the single source of truth for *what the tool
// does*, independent of any particular controller: it is what users inspect
// and override, what the (future) GUI renders, and what the generic and
// machine-specific post-processors consume.
//
// Design notes (see docs/design-decisions.md, ADR-003):
//   - Curves are analytic (lines and arcs), never tessellated polylines.
//     Tessellation, if a controller needs it, is a post-processing concern.
//   - A Path is one continuous chain. Multiple chains (e.g. several pockets)
//     are represented as separate Paths grouped by a higher-level program.
//   - All fields are plain data with explicit invariants enforced by
//     Validate, keeping the type easy to reason about, serialise, and diff.
package moves

import (
	"fmt"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// Kind classifies a Move by its motion type.
type Kind string

const (
	// KindRapid is a non-cutting positioning move (G0).
	KindRapid Kind = "rapid"
	// KindLinear is a feed move along a straight line (G1).
	KindLinear Kind = "linear"
	// KindArcCCW is a counter-clockwise circular arc (G3).
	KindArcCCW Kind = "arc_ccw"
	// KindArcCW is a clockwise circular arc (G2).
	KindArcCW Kind = "arc_cw"
)

// Milling records whether a cutting move is performed as climb (down) or
// conventional (up) milling. It is informational metadata used by the
// post-processor and the GUI; it does not change the geometry. See
// docs/design-decisions.md (ADR-005) for how it is derived and overridden.
type Milling string

const (
	MillingClimb        Milling = "climb"
	MillingConventional Milling = "conventional"
)

// Coolant is the coolant state requested by a move.
type Coolant string

const (
	CoolantOff     Coolant = "off"
	CoolantFlood   Coolant = "flood"
	CoolantMist    Coolant = "mist"
	CoolantThrough Coolant = "through"
)

// Move is a single motion in a toolpath.
//
// From and To are always populated. Center and CW are populated only for arc
// moves (KindArcCCW / KindArcCW); for 2.5D arcs in the XY plane, Center.Z is
// equal to From.Z and To.Z. Feed is mm/min; a rapid move uses Feed == 0.
type Move struct {
	Kind Kind          `json:"kind"`
	From kernel.Point3 `json:"from"`
	To   kernel.Point3 `json:"to"`
	// Center and CW apply to arc moves only; they are zero/false for other
	// kinds. Center is always emitted so the on-disk format stays explicit and
	// unambiguous regardless of kind.
	Center kernel.Point3 `json:"center"`
	CW     bool          `json:"cw"`
	Feed   float64       `json:"feed"`
	// Spindle is the requested spindle speed in RPM. 0 means "no change".
	Spindle float64 `json:"spindle"`
	// Coolant is the requested coolant state.
	Coolant Coolant `json:"coolant"`
	// Milling records climb vs conventional for cutting moves.
	Milling Milling `json:"milling"`
	// Comment is optional, human-readable context.
	Comment string `json:"comment"`
}

// Validate enforces the invariants of a Move and returns a descriptive error.
func (m Move) Validate() error {
	if !m.From.Finite() || !m.To.Finite() {
		return fmt.Errorf("moves: move has non-finite coordinates (from %v, to %v)", m.From, m.To)
	}
	switch m.Kind {
	case KindRapid:
		if m.Feed > 0 {
			return fmt.Errorf("moves: rapid move has feed %g; rapid must use feed 0", m.Feed)
		}
	case KindLinear:
		if m.Feed <= 0 {
			return fmt.Errorf("moves: linear move %q requires feed > 0 (got %g)", m.Comment, m.Feed)
		}
		if m.From.Eq(m.To, kernel.AbsTol) {
			return fmt.Errorf("moves: linear move %q has zero length", m.Comment)
		}
	case KindArcCCW, KindArcCW:
		if m.Feed <= 0 {
			return fmt.Errorf("moves: arc move requires feed > 0 (got %g)", m.Feed)
		}
		if m.CW != (m.Kind == KindArcCW) {
			return fmt.Errorf("moves: arc kind %q is inconsistent with CW flag %v", m.Kind, m.CW)
		}
		if !m.Center.Finite() {
			return fmt.Errorf("moves: arc move has non-finite centre %v", m.Center)
		}
		rStart := m.Center.Dist(m.From)
		rEnd := m.Center.Dist(m.To)
		if !kernel.EqualTol(rStart, rEnd) {
			return fmt.Errorf("moves: arc centre %v not equidistant from endpoints (start r=%g, end r=%g)",
				m.Center, rStart, rEnd)
		}
		if rStart <= 0 {
			return fmt.Errorf("moves: arc centre coincides with an endpoint (radius %g)", rStart)
		}
	default:
		return fmt.Errorf("moves: unknown move kind %q", m.Kind)
	}
	return nil
}
