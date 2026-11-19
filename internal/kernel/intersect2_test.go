package kernel

import (
	"math"
	"testing"
)

func countNear(t *testing.T, name string, got []Point2, want []Point2) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d points %v, want %d (%v)", name, len(got), got, len(want), want)
	}
	for i, p := range got {
		if !p.Eq(want[i], 1e-9) {
			t.Errorf("%s: point %d = %v, want %v", name, i, p, want[i])
		}
	}
}

func TestLineArcIntersect(t *testing.T) {
	quad, err := NewArcEdge(P2(0, 0), P2(1, 0), P2(0, 1), false) // unit quarter, 0°..90° CCW
	if err != nil {
		t.Fatal(err)
	}
	arc, _ := quad.AsArc()

	line := func(a, b Point2) Line2 { e, _ := NewLineEdge(a, b); l, _ := e.AsLine(); return l }

	t.Run("one hit on span", func(t *testing.T) {
		countNear(t, "hit", LineArcIntersect(line(P2(0.5, -0.5), P2(0.5, 1.5)), arc), []Point2{P2(0.5, math.Sqrt(3)/2)})
	})
	t.Run("tangent", func(t *testing.T) {
		countNear(t, "tangent", LineArcIntersect(line(P2(1, -0.5), P2(1, 0.5)), arc), []Point2{P2(1, 0)})
	})
	t.Run("miss (outside circle)", func(t *testing.T) {
		countNear(t, "miss", LineArcIntersect(line(P2(2, 0), P2(2, 1)), arc), nil)
	})
	t.Run("crosses circle but off span", func(t *testing.T) {
		countNear(t, "off span", LineArcIntersect(line(P2(-0.5, -1.5), P2(-0.5, 1.5)), arc), nil)
	})
	t.Run("segment stops short of circle", func(t *testing.T) {
		countNear(t, "short segment", LineArcIntersect(line(P2(0.5, -0.5), P2(0.5, 0.2)), arc), nil)
	})
}

func TestArcArcIntersect(t *testing.T) {
	// Unit circle centred at the origin: the 0°..90° quadrant.
	a1, err := NewArcEdge(P2(0, 0), P2(1, 0), P2(0, 1), false)
	if err != nil {
		t.Fatal(err)
	}
	// Unit circle centred at (1,0): the 90°..180° quadrant (from (1,1) to (0,0)).
	a2, err := NewArcEdge(P2(1, 0), P2(1, 1), P2(0, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	ar1, _ := a1.AsArc()
	ar2, _ := a2.AsArc()

	t.Run("one shared point", func(t *testing.T) {
		countNear(t, "shared", ArcArcIntersect(ar1, ar2), []Point2{P2(0.5, math.Sqrt(3)/2)})
	})

	t.Run("external tangency", func(t *testing.T) {
		// Circle (2,0) R=1 touches the origin circle at (1,0).
		b, err := NewArcEdge(P2(2, 0), P2(1, 0), P2(2, 1), true) // 180°..90° CW
		if err != nil {
			t.Fatal(err)
		}
		arb, _ := b.AsArc()
		countNear(t, "tangent", ArcArcIntersect(ar1, arb), []Point2{P2(1, 0)})
	})

	t.Run("too far apart", func(t *testing.T) {
		b, _ := NewArcEdge(P2(5, 5), P2(6, 5), P2(5, 6), false)
		arb, _ := b.AsArc()
		countNear(t, "far", ArcArcIntersect(ar1, arb), nil)
	})

	t.Run("nested circle", func(t *testing.T) {
		b, _ := NewArcEdge(P2(0, 0), P2(0.5, 0), P2(0, 0.5), false)
		arb, _ := b.AsArc()
		countNear(t, "nested", ArcArcIntersect(ar1, arb), nil)
	})

	t.Run("concentric", func(t *testing.T) {
		b, _ := NewArcEdge(P2(0, 0), P2(2, 0), P2(0, 2), false)
		arb, _ := b.AsArc()
		countNear(t, "concentric", ArcArcIntersect(ar1, arb), nil)
	})
}

func TestCurveIntersectDispatch(t *testing.T) {
	t.Run("line-line crossing", func(t *testing.T) {
		e1, _ := NewLineEdge(P2(0, 0), P2(2, 2))
		e2, _ := NewLineEdge(P2(0, 2), P2(2, 0))
		countNear(t, "line-line", CurveIntersect(e1, e2), []Point2{P2(1, 1)})
	})
	t.Run("line-line supporting lines meet outside segments", func(t *testing.T) {
		e1, _ := NewLineEdge(P2(0, 0), P2(1, 0))
		e2, _ := NewLineEdge(P2(2, -1), P2(2, 1))
		countNear(t, "out of range", CurveIntersect(e1, e2), nil)
	})
	t.Run("parallel lines", func(t *testing.T) {
		e1, _ := NewLineEdge(P2(0, 0), P2(1, 0))
		e2, _ := NewLineEdge(P2(2, 0), P2(3, 0))
		countNear(t, "parallel", CurveIntersect(e1, e2), nil)
	})
	t.Run("line-arc via dispatch", func(t *testing.T) {
		e1, _ := NewLineEdge(P2(0.5, -0.5), P2(0.5, 1.5))
		e2, _ := NewArcEdge(P2(0, 0), P2(1, 0), P2(0, 1), false)
		countNear(t, "line-arc", CurveIntersect(e1, e2), []Point2{P2(0.5, math.Sqrt(3)/2)})
	})
	t.Run("arc-line via dispatch", func(t *testing.T) {
		e1, _ := NewArcEdge(P2(0, 0), P2(1, 0), P2(0, 1), false)
		e2, _ := NewLineEdge(P2(0.5, -0.5), P2(0.5, 1.5))
		countNear(t, "arc-line", CurveIntersect(e1, e2), []Point2{P2(0.5, math.Sqrt(3)/2)})
	})
}
