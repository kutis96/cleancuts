package kernel

import (
	"math"
	"testing"
)

func rectChain(w, h float64) Chain2 {
	var edges []Edge2
	for _, seg := range [][2]Point2{
		{P2(0, 0), P2(w, 0)}, {P2(w, 0), P2(w, h)},
		{P2(w, h), P2(0, h)}, {P2(0, h), P2(0, 0)},
	} {
		e, err := NewLineEdge(seg[0], seg[1])
		if err != nil {
			panic(err)
		}
		edges = append(edges, e)
	}
	c, err := NewChain2(edges)
	if err != nil {
		panic(err)
	}
	return c
}

func TestPointInContourRectangle(t *testing.T) {
	c := rectChain(10, 10)
	cases := []struct {
		name string
		p    Point2
		want bool
	}{
		{"centre", P2(5, 5), true},
		{"left of part", P2(-1, 5), false},
		{"right of part", P2(11, 5), false},
		{"above part", P2(5, 11), false},
		{"on an edge", P2(5, 0), false},
		{"on the boundary", P2(0, 5), false},
		{"at a vertex", P2(10, 10), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointInContour(tc.p, c); got != tc.want {
				t.Fatalf("PointInContour(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestPointInContourCircle(t *testing.T) {
	c, err := circleAsQuarters(P2(0, 0), 10)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		p    Point2
		want bool
	}{
		{"centre", P2(0, 0), true},
		{"inside", P2(7, 7), true},
		{"inside at 45 degrees", P2(5, 5), true},
		{"outside", P2(11, 0), false},
		{"far outside", P2(-50, -50), false},
		{"on the boundary", P2(10, 0), false},
		{"on the boundary at 45 degrees", P2(10*math.Sqrt2/2, 10*math.Sqrt2/2), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointInContour(tc.p, c); got != tc.want {
				t.Fatalf("PointInContour(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestPointInContourUShapedNotch(t *testing.T) {
	// A 10x10 rectangle with a 4x6 notch cut from the top (x in [3,7],
	// y in [4,10]). CCW.
	var edges []Edge2
	for _, seg := range [][2]Point2{
		{P2(0, 0), P2(10, 0)}, {P2(10, 0), P2(10, 10)}, {P2(10, 10), P2(7, 10)},
		{P2(7, 10), P2(7, 4)}, {P2(7, 4), P2(3, 4)}, {P2(3, 4), P2(3, 10)},
		{P2(3, 10), P2(0, 10)}, {P2(0, 10), P2(0, 0)},
	} {
		e, err := NewLineEdge(seg[0], seg[1])
		if err != nil {
			t.Fatal(err)
		}
		edges = append(edges, e)
	}
	c, err := NewChain2(edges)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.SignedArea(), 76.0; !EqualTol(got, want) {
		t.Fatalf("sanity: area = %g, want %g", got, want)
	}
	cases := []struct {
		name string
		p    Point2
		want bool
	}{
		{"in the notch (cut away)", P2(5, 5), false},
		{"below the notch", P2(5, 2), true},
		{"left wing", P2(1, 8), true},
		{"right wing", P2(9, 8), true},
		{"above the notch (outside)", P2(5, 12), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointInContour(tc.p, c); got != tc.want {
				t.Fatalf("PointInContour(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestPointInContourOpenChain(t *testing.T) {
	e1, _ := NewLineEdge(P2(0, 0), P2(1, 0))
	c, _ := NewChain2([]Edge2{e1})
	if PointInContour(P2(0.5, 0), c) {
		t.Fatalf("open chain must report outside")
	}
}
