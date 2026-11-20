package comp

import (
	"fmt"
	"math"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// fracTol is the tolerance (in edge-fraction units) for "at an endpoint"
// tests along supporting curves.
const fracTol = 1e-6

// joinResult is the resolved corner: the trimmed tail of the previous offset
// edge, the edge(s) inserted between the two (none for a mitre, one line for
// a chamfer, one arc for a fillet), and the trimmed head of the next offset
// edge.
type joinResult struct {
	tail kernel.Edge2
	mid  []kernel.Edge2
	head kernel.Edge2
}

// resolveJoin resolves the corner at vertex v between the offset edge prev
// (ending at the offset of v) and the offset edge next (starting at the
// offset of v), per the configured join policy (ADR-013).
//
// A mitre that would extend beyond the mitre limit falls back to a chamfer,
// so the centre path never shoots off into stock on a sharp corner.
func resolveJoin(prev, next kernel.Edge2, v kernel.Point2, o Options, orig kernel.Chain2) (joinResult, error) {
	if tangentAt(prev, next) {
		// The boundary corner is rounded (or the edges are collinear): the
		// offset edges already meet tangentially at the shared point.
		return joinResult{tail: prev, head: next}, nil
	}
	switch o.Join {
	case JoinChamfer:
		return chamferJoin(prev, next, o.mitreLimitMM())
	case JoinFillet:
		return filletJoin(prev, next, v, o.filletRadius())
	default: // JoinMitre
		m, err := mitrePoint(prev, next, v, o, orig)
		if err != nil {
			return joinResult{}, err
		}
		if m.Dist(v) > o.mitreLimitMM() {
			return chamferJoin(prev, next, o.mitreLimitMM())
		}
		tail, err := trimTo(prev, m)
		if err != nil {
			return joinResult{}, fmt.Errorf("comp: mitre at corner %v: %w", v, err)
		}
		head, err := trimFrom(next, m)
		if err != nil {
			return joinResult{}, fmt.Errorf("comp: mitre at corner %v: %w", v, err)
		}
		return joinResult{tail: tail, head: head}, nil
	}
}

// tangentAt reports whether prev and next meet tangentially at their shared
// vertex (collinear lines, a line tangent to an arc, or continuous arcs), in
// which case the offset edges meet at a point and no join edge is needed.
func tangentAt(prev, next kernel.Edge2) bool {
	switch {
	case prev.IsLine() && next.IsLine():
		l1, _ := prev.AsLine()
		l2, _ := next.AsLine()
		return math.Abs(l1.Vec().Norm().Cross(l2.Vec().Norm())) < 1e-9
	case prev.IsLine() && next.IsArc():
		// The line is tangent to the arc at the shared vertex iff the radius
		// to that point is perpendicular to the line (dot ≈ 0).
		l, _ := prev.AsLine()
		a, _ := next.AsArc()
		rad := next.Start().Sub(a.Center)
		if rad.Len() < kernel.AbsTol {
			return false
		}
		return math.Abs(l.Vec().Norm().Dot(rad.Norm())) < 1e-9
	case prev.IsArc() && next.IsLine():
		l, _ := next.AsLine()
		a, _ := prev.AsArc()
		rad := prev.End().Sub(a.Center)
		if rad.Len() < kernel.AbsTol {
			return false
		}
		return math.Abs(l.Vec().Norm().Dot(rad.Norm())) < 1e-9
	default:
		a1, _ := prev.AsArc()
		a2, _ := next.AsArc()
		return a1.Center.Eq(a2.Center, 1e-6) && kernel.Equal(a1.Radius, a2.Radius, 1e-6)
	}
}

// mitrePoint returns the intersection of the two offset edges' supporting
// curves that lies on the tool side of the original contour, closest to the
// vertex (pickMitre).
func mitrePoint(prev, next kernel.Edge2, v kernel.Point2, o Options, orig kernel.Chain2) (kernel.Point2, error) {
	var cands []kernel.Point2
	switch {
	case prev.IsLine() && next.IsLine():
		l1, _ := prev.AsLine()
		l2, _ := next.AsLine()
		p, ok := kernel.LineLineIntersect(l1, l2)
		if !ok {
			return kernel.Point2{}, fmt.Errorf("comp: no mitre at corner %v: offset edges are parallel (degenerate corner)", v)
		}
		cands = []kernel.Point2{p}
	case prev.IsLine() && next.IsArc():
		l, _ := prev.AsLine()
		a, _ := next.AsArc()
		cands = kernel.LineCircleIntersect(l, a.Center, a.Radius)
	case prev.IsArc() && next.IsLine():
		a, _ := prev.AsArc()
		l, _ := next.AsLine()
		cands = kernel.LineCircleIntersect(l, a.Center, a.Radius)
	default:
		a1, _ := prev.AsArc()
		a2, _ := next.AsArc()
		cands = kernel.CircleCircleIntersect(a1.Center, a1.Radius, a2.Center, a2.Radius)
	}
	return pickMitre(cands, prev, next, v, o, orig)
}

// pickMitre chooses the candidate that lies on the tool side of the original
// contour — the side the tool centre must stay on (Law 7), exactly the test
// isReachable applies to the assembled offset. If two candidates qualify
// (line–circle, circle–circle), the one closest to the vertex wins: the true
// mitre sits at the corner, while the other intersection is on the far side
// of the supporting circle.
func pickMitre(cands []kernel.Point2, prev, next kernel.Edge2, v kernel.Point2, o Options, orig kernel.Chain2) (kernel.Point2, error) {
	wantInside := o.Direction == Inside
	best := kernel.Point2{}
	bestDist := math.Inf(1)
	found := false
	for _, p := range cands {
		if kernel.PointInContour(p, orig) != wantInside {
			continue
		}
		if !mitreReachable(prev, p, true) || !mitreReachable(next, p, false) {
			continue // degenerate corner: the offset edges cross (feature too narrow)
		}
		if d := p.Dist(v); d < bestDist {
			best, bestDist, found = p, d, true
		}
	}
	if !found {
		// The offset edges' supporting curves do not meet on the tool side at
		// a resolvable point: the feature is narrower than the tool. Safe
		// resolution: refuse the contour (the planner may retry with a smaller
		// tool) — never guess (Law 7).
		return kernel.Point2{}, fmt.Errorf("%w: corner %v — the feature is too narrow for this tool radius; use a smaller tool", ErrTooLarge, v)
	}
	return best, nil
}

// mitreReachable reports whether the mitre point p is reachable from e's
// join-side endpoint without passing through e's other endpoint — i.e. the
// trim to p is non-degenerate and does not fold back through e. For the
// previous edge that excludes p at/before the start; for the next edge, p
// at/beyond the end. On an arc, the complement of the span beyond the far
// endpoint is a natural continuation (for a line it would be a fold-back),
// so only the start-side complement is excluded. A candidate failing this
// makes the corner degenerate: the offset edges cross because the feature
// is narrower than the tool.
func mitreReachable(e kernel.Edge2, p kernel.Point2, isPrev bool) bool {
	const tol = 1e-9
	if e.IsLine() {
		f := fracAlongLine(e, p)
		if isPrev {
			return f >= tol
		}
		return f <= 1-tol
	}
	a, _ := e.AsArc()
	d := arcDelta(e, p)
	s := a.Sweep()
	if isPrev {
		return d >= tol && d <= 2*math.Pi-s+tol
	}
	return d <= s-tol || d > s+tol
}

// fracAlongLine returns the position of p on the line through e as a
// fraction of e's length: 0 at the start, 1 at the end. Values < 0 or > 1
// mean p is on the supporting line beyond an endpoint.
func fracAlongLine(e kernel.Edge2, p kernel.Point2) float64 {
	l, _ := e.AsLine()
	d := l.Vec()
	l2 := d.Dot(d)
	if l2 < kernel.AbsTol {
		return 0
	}
	return (p.Sub(l.Start)).Dot(d) / l2
}

// arcDelta returns the angular distance (rad) from e's start to p, measured
// in e's direction of travel, in [0, 2π).
func arcDelta(e kernel.Edge2, p kernel.Point2) float64 {
	a, _ := e.AsArc()
	theta := kernel.NormalizeAngle(math.Atan2(p.Y-a.Center.Y, p.X-a.Center.X))
	if a.CW {
		return kernel.NormalizeAngle(a.A0 - theta)
	}
	return kernel.NormalizeAngle(theta - a.A0)
}

// onEdge reports whether p lies on the finite edge e.
func onEdge(e kernel.Edge2, p kernel.Point2) bool {
	if e.IsLine() {
		f := fracAlongLine(e, p)
		return f >= -fracTol && f <= 1+fracTol
	}
	a, _ := e.AsArc()
	return arcDelta(e, p) <= a.Sweep()+fracTol
}

// chamferJoin truncates both offset edges a bounded distance from the vertex
// and joins the cut points with a straight segment.
func chamferJoin(prev, next kernel.Edge2, limit float64) (joinResult, error) {
	fp := 1 - limit/prev.Len()
	fn := limit / next.Len()
	if fp < -fracTol || fn < -fracTol {
		return joinResult{}, fmt.Errorf("comp: chamfer limit %g exceeds an offset edge; the corner is degenerate at this tool radius", limit)
	}
	p1 := prev.PointAt(math.Max(0, fp))
	p2 := next.PointAt(math.Min(1, fn))
	seg, err := kernel.NewLineEdge(p1, p2)
	if err != nil {
		return joinResult{}, fmt.Errorf("comp: chamfer segment: %w", err)
	}
	tail, err := trimTo(prev, p1)
	if err != nil {
		return joinResult{}, fmt.Errorf("comp: chamfer: %w", err)
	}
	head, err := trimFrom(next, p2)
	if err != nil {
		return joinResult{}, fmt.Errorf("comp: chamfer: %w", err)
	}
	return joinResult{tail: tail, mid: []kernel.Edge2{seg}, head: head}, nil
}

// filletJoin rounds the corner with an arc of radius rf tangent to both
// offset edges, centred on the vertex side. The centre is the intersection
// of the two offset edges' rf-offsets toward the vertex — a line–line,
// line–circle, or circle–circle solve, all analytic kernel primitives.
func filletJoin(prev, next kernel.Edge2, v kernel.Point2, rf float64) (joinResult, error) {
	if rf <= 0 {
		rf = kernel.AbsTol
	}
	var center kernel.Point2
	var cands []kernel.Point2
	switch {
	case prev.IsLine() && next.IsLine():
		l1, _ := prev.AsLine()
		l2, _ := next.AsLine()
		o1, err := offsetLineToward(l1, rf, v)
		if err != nil {
			return joinResult{}, fmt.Errorf("comp: fillet at corner %v: %w", v, err)
		}
		o2, err := offsetLineToward(l2, rf, v)
		if err != nil {
			return joinResult{}, fmt.Errorf("comp: fillet at corner %v: %w", v, err)
		}
		p, ok := kernel.LineLineIntersect(o1, o2)
		if !ok {
			return joinResult{}, fmt.Errorf("comp: no fillet centre at corner %v: offset edges are parallel", v)
		}
		cands = []kernel.Point2{p}
	case prev.IsLine() && next.IsArc(), prev.IsArc() && next.IsLine():
		var line kernel.Line2
		var arc kernel.Arc2
		if prev.IsLine() {
			line, _ = prev.AsLine()
			arc, _ = next.AsArc()
		} else {
			arc, _ = prev.AsArc()
			line, _ = next.AsLine()
		}
		ol, err := offsetLineToward(line, rf, v)
		if err != nil {
			return joinResult{}, fmt.Errorf("comp: fillet at corner %v: %w", v, err)
		}
		or := offsetArcRadiusToward(arc, rf, v)
		if or <= 0 {
			return joinResult{}, fmt.Errorf("comp: fillet at corner %v: radius %g leaves no arc (fillet too large)", v, rf)
		}
		cands = kernel.LineCircleIntersect(ol, arc.Center, or)
	default:
		a1, _ := prev.AsArc()
		a2, _ := next.AsArc()
		r1 := offsetArcRadiusToward(a1, rf, v)
		r2 := offsetArcRadiusToward(a2, rf, v)
		if r1 <= 0 || r2 <= 0 {
			return joinResult{}, fmt.Errorf("comp: fillet at corner %v: radius %g leaves no arc (fillet too large)", v, rf)
		}
		cands = kernel.CircleCircleIntersect(a1.Center, r1, a2.Center, r2)
	}
	if len(cands) == 0 {
		return joinResult{}, fmt.Errorf("comp: no fillet centre at corner %v: the offset edges do not meet within radius %g", v, rf)
	}
	best := cands[0]
	for _, p := range cands[1:] {
		if p.Dist(v) < best.Dist(v) {
			best = p
		}
	}
	center = best

	// Tangent points: the foot of the perpendicular from the fillet centre to
	// a line edge; the radial point on an arc edge.
	t1 := tangentPoint(prev, center)
	t2 := tangentPoint(next, center)

	// The tangent points must lie on the finite offset edges, otherwise the
	// fillet would run past them into the neighbouring corners.
	if !onEdge(prev, t1) || !onEdge(next, t2) {
		return joinResult{}, fmt.Errorf("comp: fillet radius %g is too large for the edges at corner %v", rf, v)
	}
	cross := t1.Sub(center).Cross(t2.Sub(center))
	fe, err := kernel.NewArcEdge(center, t1, t2, cross < 0)
	if err != nil {
		return joinResult{}, fmt.Errorf("comp: fillet arc at corner %v: %w", v, err)
	}
	tail, err := trimTo(prev, t1)
	if err != nil {
		return joinResult{}, fmt.Errorf("comp: fillet at corner %v: %w", v, err)
	}
	head, err := trimFrom(next, t2)
	if err != nil {
		return joinResult{}, fmt.Errorf("comp: fillet at corner %v: %w", v, err)
	}
	return joinResult{tail: tail, mid: []kernel.Edge2{fe}, head: head}, nil
}

// tangentPoint returns the point on e at which a fillet circle centred at c
// touches e: the perpendicular foot for a line, the radial point for an arc.
func tangentPoint(e kernel.Edge2, c kernel.Point2) kernel.Point2 {
	if e.IsLine() {
		l, _ := e.AsLine()
		v := l.Vec()
		l2 := v.Dot(v)
		if l2 < kernel.AbsTol {
			return l.Start
		}
		t := (c.Sub(l.Start)).Dot(v) / l2
		return l.Start.Add(v.Scale(t))
	}
	a, _ := e.AsArc()
	rad := c.Sub(a.Center)
	if rad.Len() < kernel.AbsTol {
		return a.Start()
	}
	return a.Center.Add(rad.Norm().Scale(a.Radius))
}

// offsetLineToward returns l shifted by d on the side of q.
func offsetLineToward(l kernel.Line2, d float64, q kernel.Point2) (kernel.Line2, error) {
	v := l.Vec()
	cross := v.Cross(q.Sub(l.Start))
	if cross == 0 {
		return kernel.Line2{}, fmt.Errorf("point %v lies on the line; cannot offset toward it", q)
	}
	n := v.Norm().LeftPerp()
	if cross < 0 {
		n = n.Scale(-1)
	}
	shift := n.Scale(d)
	return kernel.Line2{Start: l.Start.Add(shift), End: l.End.Add(shift)}, nil
}

// offsetArcRadiusToward returns the radius of the arc's rf-offset on the
// side of q: shrink if q is toward the centre, grow otherwise.
func offsetArcRadiusToward(a kernel.Arc2, d float64, q kernel.Point2) float64 {
	if q.Dist(a.Center) < a.Radius {
		return a.Radius - d
	}
	return a.Radius + d
}

// trimTo returns the part of e from its start up to p (p on the supporting
// curve at or beyond the end).
func trimTo(e kernel.Edge2, p kernel.Point2) (kernel.Edge2, error) {
	if e.IsLine() {
		l, _ := e.AsLine()
		ne, err := kernel.NewLineEdge(l.Start, p)
		if err != nil {
			return kernel.Edge2{}, err
		}
		return ne, nil
	}
	a, _ := e.AsArc()
	ne, err := kernel.NewArcEdge(a.Center, a.Start(), p, a.CW)
	if err != nil {
		return kernel.Edge2{}, err
	}
	return ne, nil
}

// trimFrom returns the part of e from p (on the supporting curve at or
// before the start) to its end.
func trimFrom(e kernel.Edge2, p kernel.Point2) (kernel.Edge2, error) {
	if e.IsLine() {
		l, _ := e.AsLine()
		ne, err := kernel.NewLineEdge(p, l.End)
		if err != nil {
			return kernel.Edge2{}, err
		}
		return ne, nil
	}
	a, _ := e.AsArc()
	ne, err := kernel.NewArcEdge(a.Center, p, a.End(), a.CW)
	if err != nil {
		return kernel.Edge2{}, err
	}
	return ne, nil
}
