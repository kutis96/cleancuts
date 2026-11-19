package kernel

import (
	"math"
	"testing"
)

func TestNewLineEdge(t *testing.T) {
	cases := []struct {
		name    string
		a, b    Point2
		wantErr bool
	}{
		{"valid", P2(0, 0), P2(10, 5), false},
		{"zero length", P2(1, 2), P2(1, 2), true},
		{"non-finite start", P2(math.NaN(), 0), P2(1, 0), true},
		{"non-finite end", P2(0, 0), P2(1, math.Inf(1)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewLineEdge(tc.a, tc.b)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !e.IsLine() || e.IsArc() {
				t.Fatalf("edge kind wrong: line=%v arc=%v", e.IsLine(), e.IsArc())
			}
			if e.Start() != tc.a || e.End() != tc.b {
				t.Fatalf("endpoints = %v -> %v, want %v -> %v", e.Start(), e.End(), tc.a, tc.b)
			}
			if got, want := e.Len(), tc.a.Dist(tc.b); !EqualTol(got, want) {
				t.Fatalf("Len = %g, want %g", got, want)
			}
		})
	}
}

func TestNewArcEdge(t *testing.T) {
	quad, err := NewArcEdge(P2(0, 0), P2(1, 0), P2(0, 1), false)
	if err != nil {
		t.Fatalf("valid quadrant: %v", err)
	}
	if !quad.IsArc() {
		t.Fatalf("expected arc")
	}
	if got, want := quad.Len(), math.Pi/2; !EqualTol(got, want) {
		t.Fatalf("quarter circle Len = %g, want %g", got, want)
	}
	if got, want := quad.Start(), P2(1, 0); !got.Eq(want, AbsTol) {
		t.Fatalf("Start = %v, want %v", got, want)
	}
	if got, want := quad.End(), P2(0, 1); !got.Eq(want, AbsTol) {
		t.Fatalf("End = %v, want %v", got, want)
	}

	cases := []struct {
		name         string
		center, a, b Point2
		cw           bool
	}{
		{"zero radius", P2(0, 0), P2(0, 0), P2(1, 0), false},
		{"end not on circle", P2(0, 0), P2(1, 0), P2(2, 1), false},
		{"full circle (start == end)", P2(0, 0), P2(1, 0), P2(1, 0), false},
		{"degenerate CW same points", P2(0, 0), P2(1, 0), P2(1, 0), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewArcEdge(tc.center, tc.a, tc.b, tc.cw); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}

	// A CW arc is accepted and sweeps the other way.
	cw, err := NewArcEdge(P2(0, 0), P2(0, 1), P2(1, 0), true)
	if err != nil {
		t.Fatalf("CW quadrant: %v", err)
	}
	if got, want := cw.Len(), math.Pi/2; !EqualTol(got, want) {
		t.Fatalf("CW Len = %g, want %g", got, want)
	}
}

func TestChain2Basics(t *testing.T) {
	// 10x6 rectangle, counter-clockwise.
	rect := []Edge2{}
	for _, seg := range [][2]Point2{
		{P2(0, 0), P2(10, 0)}, {P2(10, 0), P2(10, 6)},
		{P2(10, 6), P2(0, 6)}, {P2(0, 6), P2(0, 0)},
	} {
		e, err := NewLineEdge(seg[0], seg[1])
		if err != nil {
			t.Fatal(err)
		}
		rect = append(rect, e)
	}
	c, err := NewChain2(rect)
	if err != nil {
		t.Fatalf("rectangle chain: %v", err)
	}
	if !c.IsClosed() {
		t.Fatalf("rectangle should be closed")
	}
	if got, want := c.Len(), 32.0; !EqualTol(got, want) {
		t.Fatalf("Len = %g, want %g", got, want)
	}
	if got, want := c.SignedArea(), 60.0; !EqualTol(got, want) {
		t.Fatalf("SignedArea = %g, want %g", got, want)
	}
	if got := c.Start(); !got.Eq(P2(0, 0), AbsTol) {
		t.Fatalf("Start = %v", got)
	}

	// Reversed: same shape, other direction.
	r := c.Reversed()
	if got, want := r.SignedArea(), -60.0; !EqualTol(got, want) {
		t.Fatalf("Reversed SignedArea = %g, want %g", got, want)
	}
	if !r.Start().Eq(P2(0, 0), AbsTol) {
		t.Fatalf("Reversed Start = %v (closed chain keeps its start point)", r.Start())
	}
}

func TestChain2RejectsBrokenChain(t *testing.T) {
	e1, _ := NewLineEdge(P2(0, 0), P2(1, 0))
	e2, _ := NewLineEdge(P2(5, 0), P2(5, 1)) // gap between (1,0) and (5,0)
	e3, _ := NewLineEdge(P2(5, 1), P2(0, 1))
	if _, err := NewChain2([]Edge2{e1, e2, e3}); err == nil {
		t.Fatalf("expected error for open chain with a gap")
	}
}

func TestChain2ClosedCircleOfTwoHalfArcs(t *testing.T) {
	// ADR-014: a full circle is two or more arcs. Two half-arcs must form a
	// valid closed chain with area π.
	top, err := NewArcEdge(P2(0, 0), P2(1, 0), P2(-1, 0), false) // 0°..180° CCW
	if err != nil {
		t.Fatal(err)
	}
	bot, err := NewArcEdge(P2(0, 0), P2(-1, 0), P2(1, 0), false) // 180°..360° CCW
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewChain2([]Edge2{top, bot})
	if err != nil {
		t.Fatalf("two half-arcs must form a valid closed chain: %v", err)
	}
	if !c.IsClosed() {
		t.Fatalf("two half-arcs must be closed")
	}
	if got, want := c.SignedArea(), math.Pi; math.Abs(got-want) > 1e-9 {
		t.Fatalf("circle area = %g, want %g", got, want)
	}
}

// circleAsQuarters builds the unit circle as four CCW quarter arcs — the
// ADR-014 representation of a full circle.
func circleAsQuarters(center Point2, r float64) (Chain2, error) {
	mk := func(a0, a1 float64) (Edge2, error) {
		return NewArcEdge(center,
			P2(center.X+r*math.Cos(a0), center.Y+r*math.Sin(a0)),
			P2(center.X+r*math.Cos(a1), center.Y+r*math.Sin(a1)),
			false)
	}
	var edges []Edge2
	for _, q := range []struct{ a0, a1 float64 }{
		{0, math.Pi / 2}, {math.Pi / 2, math.Pi},
		{math.Pi, 3 * math.Pi / 2}, {3 * math.Pi / 2, 2 * math.Pi},
	} {
		e, err := mk(q.a0, q.a1)
		if err != nil {
			return Chain2{}, err
		}
		edges = append(edges, e)
	}
	return NewChain2(edges)
}

func TestChain2CircleAreaAndReversal(t *testing.T) {
	c, err := circleAsQuarters(P2(0, 0), 1)
	if err != nil {
		t.Fatalf("circle: %v", err)
	}
	if got, want := c.SignedArea(), math.Pi; math.Abs(got-want) > 1e-9 {
		t.Fatalf("circle SignedArea = %g, want %g", got, want)
	}
	if got, want := c.Reversed().SignedArea(), -math.Pi; math.Abs(got-want) > 1e-9 {
		t.Fatalf("reversed circle SignedArea = %g, want %g", got, want)
	}
}

func TestChain2OpenChain(t *testing.T) {
	e1, _ := NewLineEdge(P2(0, 0), P2(1, 0))
	e2, _ := NewLineEdge(P2(1, 0), P2(1, 1))
	c, err := NewChain2([]Edge2{e1, e2})
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	if c.IsClosed() {
		t.Fatalf("open chain reports closed")
	}
	if got, want := c.SignedArea(), 0.0; got != want {
		t.Fatalf("open SignedArea = %g, want 0", got)
	}
	if got, want := c.Len(), 2.0; !EqualTol(got, want) {
		t.Fatalf("Len = %g, want %g", got, want)
	}
	r := c.Reversed()
	if !r.Start().Eq(P2(1, 1), AbsTol) || !r.End().Eq(P2(0, 0), AbsTol) {
		t.Fatalf("Reversed endpoints = %v -> %v, want (1,1) -> (0,0)", r.Start(), r.End())
	}
}
