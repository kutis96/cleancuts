package comp

import (
	"fmt"
	"math"
	"sort"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// clearanceTol is the tolerance (mm) for the "at least r from the kept
// boundary" clearance test: a point exactly at distance r (tangent) is safe.
const clearanceTol = 1e-6

// Offset computes the tool-centre path(s) for a closed contour, offset by the
// tool radius on the requested side, with corners resolved per the join
// policy (ADR-013).
//
// Safety (the core guarantee): Offset never over-cuts. If the offset
// self-intersects (a feature narrower than the tool), the contour is split at
// the intersections and only the tool-reachable chains are kept; the rest is
// dropped and reported in Diagnostics. The failure mode is a residual
// (under-cut) that a smaller tool or a finish pass removes — never material
// that should have been kept being taken.
func Offset(c kernel.Chain2, o Options) (Result, error) {
	if err := o.validate(); err != nil {
		return Result{}, err
	}
	if !c.IsClosed() {
		return Result{}, fmt.Errorf("comp: Offset requires a closed contour (got an open chain)")
	}
	if err := c.Validate(); err != nil {
		return Result{}, err
	}
	if math.Abs(c.SignedArea()) < 1e-6 {
		return Result{}, fmt.Errorf("comp: contour is degenerate (area %g)", c.SignedArea())
	}
	ccw := c.SignedArea() > 0

	// The input must be a well-formed (simple) contour; otherwise the kept
	// region is ambiguous and no safe offset exists.
	if p, i, j, ok := firstSelfIntersection(c.Edges); ok {
		return Result{}, fmt.Errorf("comp: input contour self-intersects at edges %d and %d (%v); a well-formed contour is required", i, j, p)
	}

	// 1. Per-edge offset (pure, analytic).
	var off []kernel.Edge2
	for i, e := range c.Edges {
		oe, err := offsetEdge(e, o.Radius, o.Direction, ccw)
		if err != nil {
			return Result{}, fmt.Errorf("comp: edge %d: %w", i, err)
		}
		off = append(off, oe)
	}

	// 2. Resolve the join at each vertex.
	assembled, err := assemble(c, off, o)
	if err != nil {
		return Result{}, err
	}

	// 3. Detect self-intersections and resolve them safely (trim to the
	//    tool-reachable region; never over-cut).
	chains, diags := safeResolve(assembled, c, o)
	return Result{Chains: chains, Diagnostics: diags}, nil
}

// firstSelfIntersection returns the first intersection point between two
// non-adjacent edges of a closed chain, or ok=false if the chain is simple.
func firstSelfIntersection(edges []kernel.Edge2) (kernel.Point2, int, int, bool) {
	n := len(edges)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if j == i+1 || (i == 0 && j == n-1) {
				continue // adjacent edges share a vertex
			}
			if ps := kernel.CurveIntersect(edges[i], edges[j]); len(ps) > 0 {
				return ps[0], i, j, true
			}
		}
	}
	return kernel.Point2{}, 0, 0, false
}

// assemble resolves the join at every vertex and builds the final offset
// chain. The order is: trimmed edge i, then the join at vertex i+1, ...
// (a cyclic order that NewChain2 validates as closed). orig is the original
// contour: joins resolve on the tool side of it (pickMitre, Law 7).
func assemble(orig kernel.Chain2, off []kernel.Edge2, o Options) (kernel.Chain2, error) {
	n := len(off)
	type joinAt struct {
		tailEnd   kernel.Point2 // point on off[i-1] where the join ends
		mid       []kernel.Edge2
		headStart kernel.Point2 // point on off[i] where the join begins
	}
	joins := make([]joinAt, n)
	for i := 0; i < n; i++ {
		prevIdx := (i - 1 + n) % n
		v := orig.Edges[prevIdx].End() // the vertex between edge i-1 and edge i
		jr, err := resolveJoin(off[prevIdx], off[i], v, o, orig)
		if err != nil {
			return kernel.Chain2{}, fmt.Errorf("comp: join between edges %d and %d (vertex %v): %w", prevIdx, i, v, err)
		}
		joins[i] = joinAt{tailEnd: jr.tail.End(), mid: jr.mid, headStart: jr.head.Start()}
	}

	var edges []kernel.Edge2
	for i := 0; i < n; i++ {
		head := joins[i].headStart
		tail := joins[(i+1)%n].tailEnd
		e := off[i]
		te, err := trimTo(e, tail)
		if err != nil {
			return kernel.Chain2{}, fmt.Errorf("comp: edge %d: %w", i, err)
		}
		trimmed, err := trimFrom(te, head)
		if err != nil {
			return kernel.Chain2{}, fmt.Errorf("comp: edge %d: %w", i, err)
		}
		edges = append(edges, trimmed)
		edges = append(edges, joins[(i+1)%n].mid...)
	}
	chain, err := kernel.NewChain2(edges)
	if err != nil {
		return kernel.Chain2{}, fmt.Errorf("comp: assembled offset contour is invalid: %w", err)
	}
	return chain, nil
}

type hit struct {
	i int
	p kernel.Point2
}

type atomicEdge struct {
	edge     kernel.Edge2
	isHitEnd bool // the edge's END is a self-intersection (a chain boundary)
}

// safeResolve detects self-intersections in the assembled offset contour and
// resolves them safely: the contour is split at the intersections and only
// the tool-reachable chains are kept. Dropped chains leave material
// (under-cut) — never over-cut.
func safeResolve(chain kernel.Chain2, original kernel.Chain2, o Options) ([]kernel.Chain2, []Diagnostic) {
	edges := chain.Edges
	n := len(edges)

	// 1. Intersections between non-adjacent edges (exact, kernel primitives).
	// The curve passes through each intersection point twice — once on each
	// edge — so the hit is recorded on BOTH edges: the loop decomposition
	// cuts the curve at both occurrences.
	var hits []hit
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if j == i+1 || (i == 0 && j == n-1) {
				continue
			}
			for _, p := range kernel.CurveIntersect(edges[i], edges[j]) {
				hits = append(hits, hit{i, p}, hit{j, p})
			}
		}
	}

	// 2. No self-intersection: the whole contour is one chain; keep it only
	//    if it lies on the tool side of the original boundary and clears it.
	if len(hits) == 0 {
		if isReachable(chain, original, o) {
			return []kernel.Chain2{chain}, nil
		}
		return nil, []Diagnostic{{
			Level: "warning",
			Message: fmt.Sprintf(
				"comp: feature unreachable at tool radius %g: the offset lies on the wrong side of the boundary or the tool is larger than the feature; no cut generated, material left in place", o.Radius),
		}}
	}

	// 3. Split at the hit points; keep the reachable chains.
	atomic := splitAtHits(edges, hits)
	var terms []int
	for i, ae := range atomic {
		if ae.isHitEnd {
			terms = append(terms, i)
		}
	}
	if len(terms) == 0 {
		// Intersections only at shared endpoints: topologically one loop.
		if isReachable(chain, original, o) {
			return []kernel.Chain2{chain}, nil
		}
		return nil, []Diagnostic{{
			Level:   "warning",
			Message: fmt.Sprintf("comp: feature unreachable at tool radius %g; no cut generated, material left in place", o.Radius),
		}}
	}

	var kept []kernel.Chain2
	dropped := 0
	for k, term := range terms {
		next := terms[(k+1)%len(terms)]
		var chainEdges []kernel.Edge2
		idx := (term + 1) % len(atomic)
		for {
			chainEdges = append(chainEdges, atomic[idx].edge)
			if idx == next {
				break
			}
			idx = (idx + 1) % len(atomic)
		}
		ch, err := kernel.NewChain2(chainEdges)
		if err == nil && isReachable(ch, original, o) {
			kept = append(kept, ch)
		} else {
			dropped++
		}
	}
	if dropped > 0 {
		return kept, []Diagnostic{{
			Level: "warning",
			Message: fmt.Sprintf(
				"comp: %d offset region(s) dropped: unreachable at tool radius %g (self-intersecting offset); material left in place — use a smaller tool to finish", dropped, o.Radius),
		}}
	}
	return kept, nil
}

// isReachable reports whether every sampled point of ch lies on the tool
// side of the original boundary AND at least r from it — the condition
// under which the tool disc of radius r clears the kept material (Law 7).
func isReachable(ch, original kernel.Chain2, o Options) bool {
	for _, e := range ch.Edges {
		for _, t := range []float64{0, 0.25, 0.5, 0.75, 1} {
			p := e.PointAt(t)
			inside := kernel.PointInContour(p, original)
			if o.Direction == Outside && inside {
				return false
			}
			if o.Direction == Inside && !inside {
				return false
			}
			if kernel.PointToChainDist(p, original) < o.Radius-clearanceTol {
				return false
			}
		}
	}
	return true
}

// splitAtHits splits each edge at the hit points on it, in cycle order. An
// atomic edge whose END is a hit point is a chain boundary.
func splitAtHits(edges []kernel.Edge2, hits []hit) []atomicEdge {
	byEdge := map[int][]kernel.Point2{}
	for _, h := range hits {
		byEdge[h.i] = append(byEdge[h.i], h.p)
	}
	var out []atomicEdge
	for i, e := range edges {
		ps := byEdge[i]
		if len(ps) == 0 {
			out = append(out, atomicEdge{edge: e})
			continue
		}
		var ts []float64
		for _, p := range ps {
			t := paramAlong(e, p)
			if t >= -fracTol && t <= 1+fracTol {
				ts = append(ts, math.Min(1, math.Max(0, t)))
			}
		}
		sort.Float64s(ts)
		var dedup []float64
		for _, t := range ts {
			if len(dedup) == 0 || t-dedup[len(dedup)-1] > fracTol {
				dedup = append(dedup, t)
			}
		}
		start := 0.0
		for _, t := range dedup {
			if t > start+fracTol {
				out = append(out, atomicEdge{edge: subEdge(e, start, t)})
				start = t
			}
		}
		if start < 1-fracTol {
			out = append(out, atomicEdge{edge: subEdge(e, start, 1)})
		}
	}
	for i := range out {
		for _, h := range hits {
			if out[i].edge.End().Eq(h.p, 1e-6) {
				out[i].isHitEnd = true
				break
			}
		}
	}
	return out
}

// paramAlong returns the clamped fractional position of p along e (0..1).
func paramAlong(e kernel.Edge2, p kernel.Point2) float64 {
	if e.IsLine() {
		return math.Min(1, math.Max(0, fracAlongLine(e, p)))
	}
	a, _ := e.AsArc()
	s := a.Sweep()
	if s < kernel.AbsTol {
		return 0
	}
	return math.Min(1, math.Max(0, arcDelta(e, p)/s))
}

// subEdge returns the sub-edge of e from fraction t0 to t1 (0..1).
func subEdge(e kernel.Edge2, t0, t1 float64) kernel.Edge2 {
	p0 := e.PointAt(t0)
	p1 := e.PointAt(t1)
	if e.IsLine() {
		ne, err := kernel.NewLineEdge(p0, p1)
		if err != nil {
			// A zero-length sub-edge: return a degenerate edge; the caller's
			// NewChain2 will reject it with a precise error.
			return kernel.Edge2{}
		}
		return ne
	}
	a, _ := e.AsArc()
	ne, err := kernel.NewArcEdge(a.Center, p0, p1, a.CW)
	if err != nil {
		return kernel.Edge2{}
	}
	return ne
}
