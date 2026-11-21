package kernel

import (
	"fmt"
	"math"
)

// edgeKind discriminates the curve stored in an Edge2.
type edgeKind int

const (
	edgeLine edgeKind = iota
	edgeArc
)

// Edge2 is a single 2D curve: a finite line segment or a circular arc.
//
// Exactly one of Line or Arc is meaningful. Use NewLineEdge / NewArcEdge to
// construct and IsLine / IsArc to discriminate. Edge2 is a plain value (no
// interface) so contours are trivially comparable, copyable, and serialisable
// — this is the shared contour primitive for import, cutter compensation
// (ADR-013), and generation (ADR-014).
//
// An arc is never a full circle (sweep in (0, 2π)); represent a full circle
// as two or more arcs (ADR-014).
type Edge2 struct {
	kind edgeKind
	Line Line2
	Arc  Arc2
}

// NewLineEdge wraps the finite segment a→b. Endpoints must be finite and
// distinct.
func NewLineEdge(a, b Point2) (Edge2, error) {
	if !a.Finite() || !b.Finite() {
		return Edge2{}, fmt.Errorf("kernel: line edge has a non-finite endpoint (%v -> %v)", a, b)
	}
	if a.Eq(b, AbsTol) {
		return Edge2{}, fmt.Errorf("kernel: line edge has zero length at %v", a)
	}
	return Edge2{kind: edgeLine, Line: Line2{Start: a, End: b}}, nil
}

// NewArcEdge wraps an arc from start to end about center, sweeping cw or
// counter-clockwise. The radius is derived from center and start; end must be
// equidistant from center, and the sweep must be in (0, 2π) — a full circle
// must be split into arcs (ADR-014).
func NewArcEdge(center, start, end Point2, cw bool) (Edge2, error) {
	if !center.Finite() || !start.Finite() || !end.Finite() {
		return Edge2{}, fmt.Errorf("kernel: arc edge has a non-finite point (centre %v, start %v, end %v)",
			center, start, end)
	}
	r := center.Dist(start)
	if r <= AbsTol {
		return Edge2{}, fmt.Errorf("kernel: arc edge has zero radius (centre %v, start %v)", center, start)
	}
	if !EqualTol(r, center.Dist(end)) {
		return Edge2{}, fmt.Errorf("kernel: arc centre %v is not equidistant from endpoints (r=%g, end r=%g)",
			center, r, center.Dist(end))
	}
	a := Arc2{
		Center: center,
		Radius: r,
		A0:     NormalizeAngle(math.Atan2(start.Y-center.Y, start.X-center.X)),
		A1:     NormalizeAngle(math.Atan2(end.Y-center.Y, end.X-center.X)),
		CW:     cw,
	}
	if s := a.Sweep(); s <= AbsTol || s >= 2*math.Pi-AbsTol {
		return Edge2{}, fmt.Errorf("kernel: arc sweep must be in (0, 2π); split a full circle into arcs (got %g rad)", s)
	}
	return Edge2{kind: edgeArc, Arc: a}, nil
}

// IsLine reports whether e is a line segment.
func (e Edge2) IsLine() bool { return e.kind == edgeLine }

// IsArc reports whether e is a circular arc.
func (e Edge2) IsArc() bool { return e.kind == edgeArc }

// AsLine returns the line segment if e is a line.
func (e Edge2) AsLine() (Line2, bool) {
	if e.kind != edgeLine {
		return Line2{}, false
	}
	return e.Line, true
}

// AsArc returns the arc if e is an arc.
func (e Edge2) AsArc() (Arc2, bool) {
	if e.kind != edgeArc {
		return Arc2{}, false
	}
	return e.Arc, true
}

// Start returns the edge's start point (first point along travel).
func (e Edge2) Start() Point2 {
	if e.kind == edgeLine {
		return e.Line.Start
	}
	return e.Arc.Start()
}

// End returns the edge's end point (last point along travel).
func (e Edge2) End() Point2 {
	if e.kind == edgeLine {
		return e.Line.End
	}
	return e.Arc.End()
}

// Len returns the edge length in mm (segment length or arc length).
func (e Edge2) Len() float64 {
	if e.kind == edgeLine {
		return e.Line.Len()
	}
	return e.Arc.Len()
}

// PointAt returns the point at fractional position t (clamped to [0, 1])
// along the edge, in the direction of travel.
func (e Edge2) PointAt(t float64) Point2 {
	if e.kind == edgeLine {
		return e.Line.PointAt(t)
	}
	return e.Arc.PointAt(t)
}

// Validate enforces the invariants of an Edge2.
func (e Edge2) Validate() error {
	if !e.Start().Finite() || !e.End().Finite() {
		return fmt.Errorf("kernel: edge has non-finite endpoints: %v", e)
	}
	if e.kind == edgeLine {
		if e.Line.Len() <= AbsTol {
			return fmt.Errorf("kernel: line edge has zero length: %v", e)
		}
		return nil
	}
	if e.Arc.Radius <= AbsTol {
		return fmt.Errorf("kernel: arc edge has zero radius: %v", e)
	}
	if s := e.Arc.Sweep(); s <= AbsTol || s >= 2*math.Pi-AbsTol {
		return fmt.Errorf("kernel: arc edge sweep must be in (0, 2π) (got %g rad)", s)
	}
	return nil
}

// String returns a human-readable form for logs and error messages.
func (e Edge2) String() string {
	if e.kind == edgeLine {
		return fmt.Sprintf("Edge2(%v)", e.Line)
	}
	return fmt.Sprintf("Edge2(%v)", e.Arc)
}

// Chain2 is an ordered chain of 2D edges. A chain is either open (Start ≠
// End, e.g. an in-and-out slot path) or closed (Start = End, a contour).
//
// A closed chain with positive SignedArea runs counter-clockwise in the
// canonical XY plane; negative SignedArea is clockwise. Orientation is a
// *measured* property (ADR-001), never an assumption: code that needs a
// particular side (inside/outside offset, climb direction) reads
// SignedArea rather than trusting input order.
type Chain2 struct {
	Edges []Edge2
}

// NewChain2 validates and builds a chain: every edge valid, consecutive
// edges share endpoints, and a closed chain has at least 2 edges (two
// half-arcs make a full circle, ADR-014 — see IsClosed).
func NewChain2(edges []Edge2) (Chain2, error) {
	if len(edges) == 0 {
		return Chain2{}, fmt.Errorf("kernel: chain needs at least 1 edge (got 0)")
	}
	for i, e := range edges {
		if err := e.Validate(); err != nil {
			return Chain2{}, fmt.Errorf("kernel: chain edge %d: %w", i, err)
		}
		if i > 0 && !edges[i].Start().Eq(edges[i-1].End(), AbsTol) {
			return Chain2{}, fmt.Errorf("kernel: chain edge %d starts at %v but edge %d ends at %v",
				i, edges[i].Start(), i-1, edges[i-1].End())
		}
	}
	return Chain2{Edges: edges}, nil
}

// IsClosed reports whether the chain forms a closed loop. A closed chain
// needs at least 2 edges — this is what lets a full circle be two half-arcs
// (ADR-014) — and start/end must coincide.
func (c Chain2) IsClosed() bool {
	return len(c.Edges) >= 2 && c.Edges[0].Start().Eq(c.Edges[len(c.Edges)-1].End(), AbsTol)
}

// Start returns the first point of the chain.
func (c Chain2) Start() Point2 {
	if len(c.Edges) == 0 {
		return Point2{}
	}
	return c.Edges[0].Start()
}

// End returns the last point of the chain.
func (c Chain2) End() Point2 {
	if len(c.Edges) == 0 {
		return Point2{}
	}
	return c.Edges[len(c.Edges)-1].End()
}

// Len returns the total length of the chain in mm.
func (c Chain2) Len() float64 {
	var total float64
	for _, e := range c.Edges {
		total += e.Len()
	}
	return total
}

// Reversed returns a copy of the chain running the other way: the edge order
// is reversed and each edge is flipped (a line swaps endpoints, an arc swaps
// angles and direction). The reversed chain is closed iff the original was.
func (c Chain2) Reversed() Chain2 {
	out := make([]Edge2, len(c.Edges))
	for i, e := range c.Edges {
		j := len(c.Edges) - 1 - i
		switch {
		case e.IsLine():
			out[j] = Edge2{kind: edgeLine, Line: Line2{Start: e.Line.End, End: e.Line.Start}}
		case e.IsArc():
			out[j] = Edge2{kind: edgeArc, Arc: Arc2{
				Center: e.Arc.Center, Radius: e.Arc.Radius,
				A0: e.Arc.A1, A1: e.Arc.A0, CW: !e.Arc.CW,
			}}
		}
	}
	return Chain2{Edges: out}
}

// SignedArea returns the signed area enclosed by a closed chain (mm²),
// positive for counter-clockwise and negative for clockwise travel. An open
// chain has no enclosed area and returns 0.
func (c Chain2) SignedArea() float64 {
	if !c.IsClosed() {
		return 0
	}
	var area float64
	for _, e := range c.Edges {
		if e.IsLine() {
			l := e.Line
			area += 0.5 * (l.Start.X*l.End.Y - l.End.X*l.Start.Y)
			continue
		}
		a := e.Arc
		// ∮ (x dy − y dx)/2 over the arc, with the arc's signed sweep.
		s := a.signedSweep()
		term := a.Radius*a.Radius*s +
			a.Radius*(a.Center.X*(math.Sin(a.A1)-math.Sin(a.A0))-
				a.Center.Y*(math.Cos(a.A1)-math.Cos(a.A0)))
		area += 0.5 * term
	}
	return area
}

// Validate enforces the invariants of a Chain2.
func (c Chain2) Validate() error {
	_, err := NewChain2(c.Edges)
	return err
}
