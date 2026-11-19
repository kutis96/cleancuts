// Package toolpath generates machine-independent toolpaths (moves.Path) from
// part geometry. In v0.1 it provides a 2D profile generator for closed,
// planar contours.
//
// Deliberate scope notes:
//   - Cutter compensation (offsetting the contour by the tool radius) is a
//     dedicated, heavily-tested module that composes in front of these
//     generators, not a side effect of them.
//   - Fillet (arc) corner joins and adaptive/trochoidal strategies are phase
//     2; see docs/operations.md and docs/roadmap.md.
//
// Keeping generation, compensation, and post-processing as separate, composable
// modules is what makes each one independently testable and lets us swap a
// strategy without touching the rest of the pipeline.
package toolpath

import (
	"fmt"

	"github.com/spacecat/cleancuts/internal/kernel"
	"github.com/spacecat/cleancuts/internal/moves"
)

// ProfileParams configures a 2D profile (contour) toolpath.
type ProfileParams struct {
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

// Profile generates a closed 2D profile toolpath for the given polygon.
//
// The polygon is used directly as the tool-centre path. A single plunge at
// the first vertex and a single retract back to SafeZ frame the cutting loop.
// Corners are joined with sharp mitres; fillet joins are a phase-2 concern.
func Profile(poly []kernel.Point2, p ProfileParams) (moves.Path, error) {
	n := len(poly)
	if n < 3 {
		return moves.Path{}, fmt.Errorf("toolpath: profile needs >= 3 vertices (got %d)", n)
	}
	if p.Feed <= 0 {
		return moves.Path{}, fmt.Errorf("toolpath: profile feed must be > 0 (got %g)", p.Feed)
	}
	if p.CutZ >= p.SafeZ {
		return moves.Path{}, fmt.Errorf("toolpath: CutZ (%g) must be strictly below SafeZ (%g)", p.CutZ, p.SafeZ)
	}
	for i := 0; i < n; i++ {
		if !poly[i].Finite() {
			return moves.Path{}, fmt.Errorf("toolpath: profile vertex %d is not finite: %v", i, poly[i])
		}
		if poly[i].Eq(poly[(i+1)%n], kernel.AbsTol) {
			return moves.Path{}, fmt.Errorf("toolpath: profile has a zero-length edge between vertices %d and %d", i, (i+1)%n)
		}
	}

	at := func(v kernel.Point2, z float64) kernel.Point3 {
		return kernel.Point3{X: v.X, Y: v.Y, Z: z}
	}

	out := moves.Path{
		ID:        "profile",
		Name:      "2D profile",
		Operation: "profile",
		ToolID:    p.ToolID,
		Pass:      1,
	}

	// Plunge.
	out.Moves = append(out.Moves, moves.Move{
		Kind:    moves.KindRapid,
		From:    at(poly[0], p.SafeZ),
		To:      at(poly[0], p.CutZ),
		Spindle: p.Spindle,
		Coolant: p.Coolant,
		Comment: "plunge",
	})
	// Cutting loop.
	for i := 1; i <= n; i++ {
		out.Moves = append(out.Moves, moves.Move{
			Kind:    moves.KindLinear,
			From:    at(poly[(i-1)%n], p.CutZ),
			To:      at(poly[i%n], p.CutZ),
			Feed:    p.Feed,
			Spindle: p.Spindle,
			Coolant: p.Coolant,
			Milling: p.Milling,
		})
	}
	// Retract.
	out.Moves = append(out.Moves, moves.Move{
		Kind:    moves.KindRapid,
		From:    at(poly[0], p.CutZ),
		To:      at(poly[0], p.SafeZ),
		Comment: "retract",
	})

	if err := out.Validate(); err != nil {
		return moves.Path{}, fmt.Errorf("toolpath: profile produced invalid path: %w", err)
	}
	return out, nil
}
