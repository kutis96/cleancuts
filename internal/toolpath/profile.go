// Package toolpath generates machine-independent toolpaths (moves.Path) from
// part geometry.
//
// Deliberate scope notes:
//   - Cutter compensation (offsetting the contour by the tool radius) is a
//     dedicated, heavily-tested module (comp) that composes in front of these
//     generators, not a side effect of them (ADR-008).
//   - Adaptive/trochoidal strategies are later phases; see
//     docs/operations.md and docs/roadmap.md.
//
// Keeping generation, compensation, and post-processing as separate, composable
// modules is what makes each one independently testable and lets us swap a
// strategy without touching the rest of the pipeline.
package toolpath

import (
	"fmt"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
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
// It is Loop over the polygon's chain (lines only).
func Profile(poly []kernel.Point2, p ProfileParams) (moves.Path, error) {
	if len(poly) < 3 {
		return moves.Path{}, fmt.Errorf("toolpath: profile needs >= 3 vertices (got %d)", len(poly))
	}
	chain, err := polygonChain(poly)
	if err != nil {
		return moves.Path{}, fmt.Errorf("toolpath: profile: %w", err)
	}
	return Loop(chain, LoopParams{
		Operation: "profile",
		Name:      "2D profile",
		Milling:   p.Milling,
		Feed:      p.Feed,
		Spindle:   p.Spindle,
		SafeZ:     p.SafeZ,
		CutZ:      p.CutZ,
		Coolant:   p.Coolant,
		ToolID:    p.ToolID,
	})
}

// polygonChain builds a closed chain of line edges from polygon vertices.
func polygonChain(poly []kernel.Point2) (kernel.Chain2, error) {
	var edges []kernel.Edge2
	for i := range poly {
		e, err := kernel.NewLineEdge(poly[i], poly[(i+1)%len(poly)])
		if err != nil {
			return kernel.Chain2{}, fmt.Errorf("edge %d: %w", i, err)
		}
		edges = append(edges, e)
	}
	return kernel.NewChain2(edges)
}
