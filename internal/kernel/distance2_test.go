package kernel

import (
	"math"
	"testing"
)

func TestPointToSegmentDist(t *testing.T) {
	l, _ := NewLineEdge(P2(0, 0), P2(1, 0))
	seg, _ := l.AsLine()
	cases := []struct {
		name string
		p    Point2
		want float64
	}{
		{"projection interior", P2(0.5, 1), 1},
		{"past the end", P2(2, 2), math.Sqrt(5)}, // nearest point is (1,0)
		{"before the start", P2(-1, 0), 1},
		{"on the segment", P2(0.25, 0), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointToSegmentDist(tc.p, seg); !EqualTol(got, tc.want) {
				t.Fatalf("dist = %g, want %g", got, tc.want)
			}
		})
	}
}

func TestPointToArcDist(t *testing.T) {
	a, err := NewArcEdge(P2(0, 0), P2(1, 0), P2(0, 1), false)
	if err != nil {
		t.Fatal(err)
	}
	arc, _ := a.AsArc()
	cases := []struct {
		name string
		p    Point2
		want float64
	}{
		{"radial gap on span", P2(0.5, 0.5), 1 - math.Sqrt(0.5)},
		{"on the arc", P2(math.Sqrt2/2, math.Sqrt2/2), 0},
		{"off span: nearer start", P2(0.5, -0.5), math.Sqrt(0.5)},
		{"off span: nearer end", P2(-0.5, 0.5), math.Sqrt(0.5)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointToArcDist(tc.p, arc); !EqualTol(got, tc.want) {
				t.Fatalf("dist = %g, want %g", got, tc.want)
			}
		})
	}
}

func TestPointToChainDist(t *testing.T) {
	// 10x10 rectangle; the closest boundary point to (12, 12) is the corner.
	var edges []Edge2
	for _, seg := range [][2]Point2{
		{P2(0, 0), P2(10, 0)}, {P2(10, 0), P2(10, 10)},
		{P2(10, 10), P2(0, 10)}, {P2(0, 10), P2(0, 0)},
	} {
		e, _ := NewLineEdge(seg[0], seg[1])
		edges = append(edges, e)
	}
	c, _ := NewChain2(edges)
	if got, want := PointToChainDist(P2(12, 12), c), 2*math.Sqrt2; !EqualTol(got, want) { // nearest is corner (10,10)
		t.Fatalf("dist = %g, want %g", got, want)
	}
	if got := PointToChainDist(P2(12, 12), Chain2{}); got != math.Inf(1) {
		t.Fatalf("empty chain dist = %v, want +Inf", got)
	}
}
