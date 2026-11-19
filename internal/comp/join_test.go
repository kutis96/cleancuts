package comp

import (
	"math"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

func line(a, b kernel.Point2) kernel.Edge2 {
	e, err := kernel.NewLineEdge(a, b)
	if err != nil {
		panic(err)
	}
	return e
}

func arc(center, start, end kernel.Point2, cw bool) kernel.Edge2 {
	e, err := kernel.NewArcEdge(center, start, end, cw)
	if err != nil {
		panic(err)
	}
	return e
}

// squareCorner returns the offset edges of the bottom-right corner of a CCW
// square at v=(10,0), offset outside by r=2: the bottom edge goes to y=-2,
// the right edge goes to x=12.
func squareCorner() (prev, next kernel.Edge2, v kernel.Point2) {
	return line(kernel.P2(0, -2), kernel.P2(10, -2)),
		line(kernel.P2(12, 0), kernel.P2(12, 10)),
		kernel.P2(10, 0)
}

func TestJoinMitreLineLine(t *testing.T) {
	prev, next, v := squareCorner()
	r, err := resolveJoin(prev, next, v, Options{Radius: 2, Direction: Outside})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.mid) != 0 {
		t.Fatalf("mitre join must insert no edges, got %v", r.mid)
	}
	// The mitre point is where the two offset lines cross: (12,-2). It must
	// be exactly r from both original edges (y=0 and x=10) — tangent to the
	// cut, the never-over-cut invariant at the corner.
	m := kernel.P2(12, -2)
	if math.Abs(m.Y-0)-2 != 0 || math.Abs(m.X-10)-2 != 0 {
		t.Fatalf("sanity: mitre %v is not r from both original edges", m)
	}
	if !r.tail.End().Eq(m, 1e-9) {
		t.Fatalf("tail end = %v, want %v", r.tail.End(), m)
	}
	if !r.head.Start().Eq(m, 1e-9) {
		t.Fatalf("head start = %v, want %v", r.head.Start(), m)
	}
	if !r.tail.Start().Eq(kernel.P2(0, -2), 1e-9) {
		t.Fatalf("tail start = %v, want (0,-2)", r.tail.Start())
	}
	if !r.head.End().Eq(kernel.P2(12, 10), 1e-9) {
		t.Fatalf("head end = %v, want (12,10)", r.head.End())
	}
}

func TestJoinMitreLimitFallsBackToChamfer(t *testing.T) {
	prev, next, v := squareCorner()
	// Mitre limit 0.5 (i.e. 1mm): the mitre point (12,-2) is sqrt(8) ≈ 2.83
	// from the vertex, beyond the limit, so the join must chamfer.
	r, err := resolveJoin(prev, next, v, Options{Radius: 2, Direction: Outside, MitreLimit: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.mid) != 1 || !r.mid[0].IsLine() {
		t.Fatalf("chamfer join must insert one line edge, got %v", r.mid)
	}
	// Chamfer points are 1mm back along each offset edge from the vertex.
	if !r.tail.End().Eq(kernel.P2(9, -2), 1e-9) {
		t.Fatalf("chamfer tail end = %v, want (9,-2)", r.tail.End())
	}
	if !r.mid[0].Start().Eq(kernel.P2(9, -2), 1e-9) || !r.mid[0].End().Eq(kernel.P2(12, 1), 1e-9) {
		t.Fatalf("chamfer segment = %v -> %v, want (9,-2) -> (12,1)", r.mid[0].Start(), r.mid[0].End())
	}
	if !r.head.Start().Eq(kernel.P2(12, 1), 1e-9) {
		t.Fatalf("chamfer head start = %v, want (12,1)", r.head.Start())
	}
}

func TestJoinChamferExplicit(t *testing.T) {
	prev, next, v := squareCorner()
	// Explicit chamfer, limit 2 (i.e. 4mm).
	r, err := resolveJoin(prev, next, v, Options{Radius: 2, Direction: Outside, Join: JoinChamfer, MitreLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.mid) != 1 || !r.mid[0].IsLine() {
		t.Fatalf("chamfer join must insert one line edge, got %v", r.mid)
	}
	if !r.tail.End().Eq(kernel.P2(6, -2), 1e-9) {
		t.Fatalf("chamfer tail end = %v, want (6,-2)", r.tail.End())
	}
	if !r.head.Start().Eq(kernel.P2(12, 4), 1e-9) {
		t.Fatalf("chamfer head start = %v, want (12,4)", r.head.Start())
	}
}

func TestJoinMitreLineArc(t *testing.T) {
	// A D-corner: the bottom edge (0,0)->(8,0) meets a concave arc (centre
	// (10,0), R=2, (8,0)->(10,2), CW) of a CCW contour whose material is
	// above the line and away from the arc centre. Outside offset by r=1:
	// the line goes to y=-1; the arc (opposite sense) goes to R=1.
	prev := line(kernel.P2(0, -1), kernel.P2(8, -1))
	next := arc(kernel.P2(10, 0), kernel.P2(9, 0), kernel.P2(10, 1), true)
	v := kernel.P2(8, 0)

	r, err := resolveJoin(prev, next, v, Options{Radius: 1, Direction: Outside})
	if err != nil {
		t.Fatal(err)
	}
	// The offset line y=-1 is tangent to the offset circle (centre (10,0),
	// R=1): the mitre point is the tangency (10,-1), exactly r from both
	// original edges.
	m := kernel.P2(10, -1)
	if math.Abs(m.Dist(kernel.P2(10, 0))-(2-1)) > 1e-9 {
		t.Fatalf("sanity: mitre %v is not at distance r from the original arc", m)
	}
	if len(r.mid) != 0 {
		t.Fatalf("mitre join must insert no edges, got %v", r.mid)
	}
	if !r.tail.End().Eq(m, 1e-9) {
		t.Fatalf("tail end = %v, want %v", r.tail.End(), m)
	}
	if !r.head.Start().Eq(m, 1e-9) {
		t.Fatalf("head start = %v, want %v", r.head.Start(), m)
	}
	ha, ok := r.head.AsArc()
	if !ok || !ha.Center.Eq(kernel.P2(10, 0), 1e-9) || !kernel.EqualTol(ha.Radius, 1) {
		t.Fatalf("head arc = %v, want centre (10,0) R=1", r.head)
	}
}

func TestJoinMitreArcArc(t *testing.T) {
	// Two convex arcs meeting at v=(0,5): prev about (0,0) R=5 (material on
	// the far side, so outside = R-r = 4), next about (5,5) R=5 (material on
	// the near side, so outside = R+r = 6). The mitre is the supporting-
	// circle intersection on the far side of the material — the one closest
	// to the vertex, not the one that would fold back into it.
	prev := arc(kernel.P2(0, 0), kernel.P2(4, 0), kernel.P2(0, 4), false)
	next := arc(kernel.P2(5, 5), kernel.P2(-1, 5), kernel.P2(5, 11), true)
	v := kernel.P2(0, 5)

	r, err := resolveJoin(prev, next, v, Options{Radius: 1, Direction: Outside})
	if err != nil {
		t.Fatal(err)
	}
	// Exact mitre: circles (0,0) R=4 and (5,5) R=6 meet at (1.5,1.5) ±
	// sqrt(11.5)·(-√2/2, √2/2). The far-side (correct) point:
	s := math.Sqrt(23) / 2
	m := kernel.P2(1.5-s, 1.5+s)
	if !r.tail.End().Eq(m, 1e-9) {
		t.Fatalf("tail end = %v, want %v (the far-side intersection)", r.tail.End(), m)
	}
	if !r.head.Start().Eq(m, 1e-9) {
		t.Fatalf("head start = %v, want %v", r.head.Start(), m)
	}
	// The mitre must be exactly r from both original arcs (tangent to the
	// cut — the never-over-cut invariant at the corner).
	if math.Abs(m.Dist(kernel.P2(0, 0))-4) > 1e-9 {
		t.Fatalf("mitre %v is not at the offset radius from arc 1's centre", m)
	}
	if math.Abs(m.Dist(kernel.P2(5, 5))-6) > 1e-9 {
		t.Fatalf("mitre %v is not at the offset radius from arc 2's centre", m)
	}
}

func TestJoinTangentLineArc(t *testing.T) {
	// A horizontal line meeting a circle at its bottom point: the offset
	// edges meet tangentially, so the join is a bare point (no inserted edge,
	// no trimming).
	prev := line(kernel.P2(0, 0), kernel.P2(10, 0))
	next := arc(kernel.P2(10, 5), kernel.P2(10, 0), kernel.P2(15, 5), false)
	r, err := resolveJoin(prev, next, kernel.P2(10, 0), Options{Radius: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.mid) != 0 {
		t.Fatalf("tangent join must insert no edges, got %v", r.mid)
	}
	if !r.tail.End().Eq(kernel.P2(10, 0), 1e-9) || !r.head.Start().Eq(kernel.P2(10, 0), 1e-9) {
		t.Fatalf("tangent join must keep the shared point: %v / %v", r.tail.End(), r.head.Start())
	}
}

func TestJoinTangentArcArc(t *testing.T) {
	// Two continuous arcs (same centre and radius) meeting at (0,5).
	prev := arc(kernel.P2(0, 0), kernel.P2(5, 0), kernel.P2(0, 5), false)
	next := arc(kernel.P2(0, 0), kernel.P2(0, 5), kernel.P2(-5, 0), false)
	r, err := resolveJoin(prev, next, kernel.P2(0, 5), Options{Radius: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.mid) != 0 {
		t.Fatalf("tangent join must insert no edges, got %v", r.mid)
	}
}

func TestJoinFilletLineLine(t *testing.T) {
	// The square corner, filleted with rf = r = 2. The fillet centre is the
	// vertex itself (each offset line shifted by rf toward v returns to the
	// original edge), the tangent points are the offset endpoints, and the
	// fillet is a quarter arc of radius 2 about v.
	prev, next, v := squareCorner()
	r, err := resolveJoin(prev, next, v, Options{Radius: 2, Direction: Outside, Join: JoinFillet})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.mid) != 1 || !r.mid[0].IsArc() {
		t.Fatalf("fillet join must insert one arc, got %v", r.mid)
	}
	fe, _ := r.mid[0].AsArc()
	if !fe.Center.Eq(v, 1e-9) {
		t.Fatalf("fillet centre = %v, want the vertex %v", fe.Center, v)
	}
	if !kernel.EqualTol(fe.Radius, 2) {
		t.Fatalf("fillet radius = %g, want 2", fe.Radius)
	}
	if !r.mid[0].Start().Eq(kernel.P2(10, -2), 1e-9) || !r.mid[0].End().Eq(kernel.P2(12, 0), 1e-9) {
		t.Fatalf("fillet arc = %v -> %v, want (10,-2) -> (12,0)", r.mid[0].Start(), r.mid[0].End())
	}
	if !r.tail.End().Eq(kernel.P2(10, -2), 1e-9) || !r.head.Start().Eq(kernel.P2(12, 0), 1e-9) {
		t.Fatalf("fillet trims: tail end %v, head start %v", r.tail.End(), r.head.Start())
	}
}

func TestJoinFilletRefusesOversizedRadius(t *testing.T) {
	// A fillet radius larger than the offset edges can support must refuse
	// (the tangent points would fall off the edges) rather than produce a
	// bogus corner.
	prev := line(kernel.P2(0, -2), kernel.P2(10, -2))
	next := line(kernel.P2(12, 0), kernel.P2(12, 10))
	v := kernel.P2(10, 0)
	if _, err := resolveJoin(prev, next, v, Options{Radius: 2, Direction: Outside, Join: JoinFillet, FilletRadius: 50}); err == nil {
		t.Fatalf("oversized fillet must be an error")
	}
}
