package toolpath

import (
	"fmt"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

// LoopParams configures an analytic loop toolpath.
type LoopParams struct {
	// Operation is the operation name recorded on the path (e.g. "profile",
	// "pocket"); empty defaults to "loop".
	Operation string
	// Name is the display name; empty defaults to "2.5D loop".
	Name string
	// Milling is the climb/conventional mode recorded on cutting moves.
	Milling moves.Milling
	// Feed is the cutting feed rate (mm/min); must be > 0.
	Feed float64
	// Spindle is the spindle speed (RPM).
	Spindle float64
	// SafeZ is the rapid safe height (mm); must be above CutZ.
	SafeZ float64
	// CutZ is the cutting depth (mm).
	CutZ float64
	// Coolant is the coolant state during cutting.
	Coolant moves.Coolant
	// ToolID is recorded on the path for post-processing and inspection.
	ToolID string
}

// validate enforces the invariants of LoopParams.
func (p LoopParams) validate() error {
	if p.Feed <= 0 {
		return fmt.Errorf("toolpath: feed must be > 0 (got %g)", p.Feed)
	}
	if p.CutZ >= p.SafeZ {
		return fmt.Errorf("toolpath: CutZ (%g) must be strictly below SafeZ (%g)", p.CutZ, p.SafeZ)
	}
	return nil
}

// Loop generates a toolpath that traverses the chain edge by edge: a plunge
// at the start, one analytic cutting move per edge (line → linear, arc →
// arc move), and a retract at the end. The chain is used as-is as the
// tool-centre path — cutter compensation (comp) composes in front of it as a
// separate, tested module (ADR-008), and arcs stay analytic in the core
// (ADR-002).
//
// The plunge is a vertical rapid at the chain's start point. Choosing a
// better entry (a safe, non-cutting spot, or an in-and-out approach) is the
// planner's job, not the generator's: the planner picks the start and the
// traversal direction (climb vs. conventional, ADR-009) and calls Loop with
// the oriented chain.
func Loop(c kernel.Chain2, p LoopParams) (moves.Path, error) {
	if err := c.Validate(); err != nil {
		return moves.Path{}, fmt.Errorf("toolpath: loop contour: %w", err)
	}
	if err := p.validate(); err != nil {
		return moves.Path{}, err
	}

	operation := p.Operation
	if operation == "" {
		operation = "loop"
	}
	name := p.Name
	if name == "" {
		name = "2.5D loop"
	}

	out := moves.Path{
		ID:        "loop",
		Name:      name,
		Operation: operation,
		ToolID:    p.ToolID,
		Pass:      1,
	}

	// Plunge: vertical rapid at the chain start.
	out.Moves = append(out.Moves, moves.Move{
		Kind:    moves.KindRapid,
		From:    atZ(c.Start(), p.SafeZ),
		To:      atZ(c.Start(), p.CutZ),
		Spindle: p.Spindle,
		Coolant: p.Coolant,
		Comment: "plunge",
	})
	for i, e := range c.Edges {
		mv, err := edgeMove(e, p)
		if err != nil {
			return moves.Path{}, fmt.Errorf("toolpath: loop edge %d: %w", i, err)
		}
		out.Moves = append(out.Moves, mv)
	}
	// Retract: vertical rapid at the chain end.
	out.Moves = append(out.Moves, moves.Move{
		Kind:    moves.KindRapid,
		From:    atZ(c.End(), p.CutZ),
		To:      atZ(c.End(), p.SafeZ),
		Comment: "retract",
	})

	if err := out.Validate(); err != nil {
		return moves.Path{}, fmt.Errorf("toolpath: loop produced invalid path: %w", err)
	}
	return out, nil
}

// edgeMove maps one contour edge to its cutting move at the cutting Z: a
// line to a linear move, an arc to an arc move carrying the analytic centre
// (so post-processing can emit G2/G3 or tessellate, its choice).
func edgeMove(e kernel.Edge2, p LoopParams) (moves.Move, error) {
	if e.IsLine() {
		l, _ := e.AsLine()
		return moves.Move{
			Kind:    moves.KindLinear,
			From:    atZ(l.Start, p.CutZ),
			To:      atZ(l.End, p.CutZ),
			Feed:    p.Feed,
			Spindle: p.Spindle,
			Coolant: p.Coolant,
			Milling: p.Milling,
		}, nil
	}
	a, _ := e.AsArc()
	kind := moves.KindArcCCW
	if a.CW {
		kind = moves.KindArcCW
	}
	return moves.Move{
		Kind:    kind,
		From:    atZ(a.Start(), p.CutZ),
		To:      atZ(a.End(), p.CutZ),
		Center:  atZ(a.Center, p.CutZ),
		CW:      a.CW,
		Feed:    p.Feed,
		Spindle: p.Spindle,
		Coolant: p.Coolant,
		Milling: p.Milling,
	}, nil
}

// atZ lifts a 2D point to the canonical WCS at height z.
func atZ(v kernel.Point2, z float64) kernel.Point3 {
	return kernel.Point3{X: v.X, Y: v.Y, Z: z}
}
