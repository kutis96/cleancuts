package comp

import (
	"fmt"
	"math"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// ErrTooLarge indicates the tool radius exceeds a feature, making the offset
// degenerate (a non-positive arc radius, or a vanished edge). Offset wraps it
// with the offending edge; the safe resolution is to refuse the contour (and
// let the planner retry with a smaller tool), never to guess.
var ErrTooLarge = fmt.Errorf("comp: tool too large for feature")

// offsetEdge offsets one edge by r on the requested side.
//
// ccw is the orientation of the containing contour (true = counter-
// clockwise), which decides which side of an edge is "outside": the interior
// of the contour is on the left of travel for a CCW contour and on the right
// for a CW one. Offsetting therefore depends on the contour orientation, not
// on the edge alone — passing the wrong ccw flips the cut to the other side
// of the boundary, the classic over-cut bug, so Offset derives ccw from
// SignedArea and never trusts input order.
func offsetEdge(e kernel.Edge2, r float64, dir Direction, ccw bool) (kernel.Edge2, error) {
	if e.IsLine() {
		l, _ := e.AsLine()
		d := l.Vec().Norm()
		shift := d.LeftPerp().Scale(r) // interior side for a CCW contour
		if !ccw {
			shift = d.RightPerp().Scale(r)
		}
		if dir == Outside {
			shift = shift.Scale(-1)
		}
		ne, err := kernel.NewLineEdge(l.Start.Add(shift), l.End.Add(shift))
		if err != nil {
			return kernel.Edge2{}, fmt.Errorf("comp: %w", err)
		}
		return ne, nil
	}

	a, _ := e.AsArc()
	// An arc sweeping in the same sense as the contour has the interior
	// toward its centre (a convex bump); an arc sweeping the other way has
	// the interior away from its centre (a concave notch). So:
	//   same sense  -> outside adds r, inside subtracts r
	//   opposite    -> outside subtracts r, inside adds r
	arcCCW := !a.CW
	same := arcCCW == ccw
	k := 1.0
	if (dir == Outside) != same {
		k = -1.0
	}
	nr := a.Radius + k*r
	if nr <= kernel.AbsTol {
		return kernel.Edge2{}, fmt.Errorf("%w: arc radius %g offset by %g gives %g (need > 0); use a smaller tool for this feature",
			ErrTooLarge, a.Radius, r, nr)
	}
	// Same centre and span; rebuild start/end at the new radius (NewArcEdge
	// derives the radius from centre and start).
	ns := kernel.P2(a.Center.X+nr*math.Cos(a.A0), a.Center.Y+nr*math.Sin(a.A0))
	ne := kernel.P2(a.Center.X+nr*math.Cos(a.A1), a.Center.Y+nr*math.Sin(a.A1))
	edge, err := kernel.NewArcEdge(a.Center, ns, ne, a.CW)
	if err != nil {
		return kernel.Edge2{}, fmt.Errorf("comp: %w", err)
	}
	return edge, nil
}
