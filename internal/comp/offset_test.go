package comp

import (
	"errors"
	"math"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

func TestOffsetEdgeLine(t *testing.T) {
	// Bottom edge of a CCW square: (0,0) -> (10,0). Interior is +Y.
	e, _ := kernel.NewLineEdge(kernel.P2(0, 0), kernel.P2(10, 0))

	t.Run("outside offset goes away from interior", func(t *testing.T) {
		ne, err := offsetEdge(e, 2, Outside, true)
		if err != nil {
			t.Fatal(err)
		}
		l, _ := ne.AsLine()
		if !l.Start.Eq(kernel.P2(0, -2), 1e-9) || !l.End.Eq(kernel.P2(10, -2), 1e-9) {
			t.Fatalf("offset = %v -> %v, want (0,-2) -> (10,-2)", l.Start, l.End)
		}
	})
	t.Run("inside offset goes into interior", func(t *testing.T) {
		ne, err := offsetEdge(e, 2, Inside, true)
		if err != nil {
			t.Fatal(err)
		}
		l, _ := ne.AsLine()
		if !l.Start.Eq(kernel.P2(0, 2), 1e-9) || !l.End.Eq(kernel.P2(10, 2), 1e-9) {
			t.Fatalf("offset = %v -> %v, want (0,2) -> (10,2)", l.Start, l.End)
		}
	})

	// The same physical edge traversed the other way (CW contour): outside
	// must land on the SAME side — orientation must not flip the cut.
	eRev, _ := kernel.NewLineEdge(kernel.P2(10, 0), kernel.P2(0, 0))
	ne, err := offsetEdge(eRev, 2, Outside, false)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := ne.AsLine()
	if !l.Start.Eq(kernel.P2(10, -2), 1e-9) || !l.End.Eq(kernel.P2(0, -2), 1e-9) {
		t.Fatalf("reversed outside offset = %v -> %v, want (10,-2) -> (0,-2)", l.Start, l.End)
	}
}

func TestOffsetEdgeArc(t *testing.T) {
	// A CCW contour whose boundary includes a CCW quarter arc (a convex
	// bump): centre (0,0), from (10,0) to (0,10), R=10.
	bump, err := kernel.NewArcEdge(kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(0, 10), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("outside offset of convex arc is R+r", func(t *testing.T) {
		ne, err := offsetEdge(bump, 2, Outside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 12) {
			t.Fatalf("radius = %g, want 12", a.Radius)
		}
		if !a.Center.Eq(kernel.P2(0, 0), 1e-9) {
			t.Fatalf("centre = %v, want (0,0)", a.Center)
		}
		if a.CW {
			t.Fatalf("direction must be preserved (CCW)")
		}
	})
	t.Run("inside offset of convex arc is R-r", func(t *testing.T) {
		ne, err := offsetEdge(bump, 2, Inside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 8) {
			t.Fatalf("radius = %g, want 8", a.Radius)
		}
	})

	// A concave notch in a CCW contour: a semicircular bite from the top
	// edge, centre (5,10), R=2, running (7,10) -> (5,8) -> (3,10), which
	// sweeps CW — the opposite sense to the contour. Outside is the side
	// away from the material, i.e. INTO the notch, so it shrinks the radius.
	notch, err := kernel.NewArcEdge(kernel.P2(5, 10), kernel.P2(7, 10), kernel.P2(3, 10), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("outside offset of concave arc is R-r", func(t *testing.T) {
		ne, err := offsetEdge(notch, 1, Outside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 1) {
			t.Fatalf("radius = %g, want 1", a.Radius)
		}
		if !a.Center.Eq(kernel.P2(5, 10), 1e-9) {
			t.Fatalf("centre = %v, want (5,10)", a.Center)
		}
	})
	t.Run("inside offset of concave arc is R+r", func(t *testing.T) {
		ne, err := offsetEdge(notch, 1, Inside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 3) {
			t.Fatalf("radius = %g, want 3", a.Radius)
		}
	})
}

func TestOffsetEdgeArcTooLarge(t *testing.T) {
	// A convex arc R=5 with an inside offset of r=5 degenerates (R-r = 0).
	bump, err := kernel.NewArcEdge(kernel.P2(0, 0), kernel.P2(5, 0), kernel.P2(0, 5), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = offsetEdge(bump, 5, Inside, true)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	_, err = offsetEdge(bump, 6, Inside, true)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	// A concave notch R=5 with an outside offset of r=6 also degenerates:
	// the tool simply does not fit in the notch.
	notch, _ := kernel.NewArcEdge(kernel.P2(5, 10), kernel.P2(10, 10), kernel.P2(0, 10), true)
	if _, err := offsetEdge(notch, 6, Outside, true); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

// polyChain builds a closed CCW chain from polygon vertices.
func polyChain(t *testing.T, pts ...kernel.Point2) kernel.Chain2 {
	t.Helper()
	var edges []kernel.Edge2
	for i := range pts {
		e, err := kernel.NewLineEdge(pts[i], pts[(i+1)%len(pts)])
		if err != nil {
			t.Fatal(err)
		}
		edges = append(edges, e)
	}
	c, err := kernel.NewChain2(edges)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustArc(t *testing.T, c, a, b kernel.Point2) kernel.Edge2 {
	t.Helper()
	e, err := kernel.NewArcEdge(c, a, b, false)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// minDistTo returns the smallest distance from a sampled point of c to ref.
func minDistTo(c, ref kernel.Chain2) float64 {
	var best = math.Inf(1)
	for _, e := range c.Edges {
		for k := 0; k <= 10; k++ {
			d := kernel.PointToChainDist(e.PointAt(float64(k)/10), ref)
			if d < best {
				best = d
			}
		}
	}
	return best
}

func TestOffsetSquareProfile(t *testing.T) {
	// 10×10 CCW square, outside offset r=2 → 14×14 mitred square.
	sq := polyChain(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(10, 10), kernel.P2(0, 10))
	res, err := Offset(sq, Options{Radius: 2, Direction: Outside})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Chains) != 1 {
		t.Fatalf("chains = %d, want 1 (diagnostics: %v)", len(res.Chains), res.Diagnostics)
	}
	ch := res.Chains[0]
	if !ch.IsClosed() {
		t.Fatalf("chain must be closed")
	}
	if got := math.Abs(ch.SignedArea()); !kernel.EqualTol(got, 196) {
		t.Fatalf("area = %g, want 196 (14×14)", got)
	}
	if d := minDistTo(ch, sq); d < 2-1e-6 {
		t.Fatalf("offset comes within %g of the kept boundary (want ≥ 2)", d)
	}
	// Reversed input order must give the same (up-to-rotation) result.
	resRev, err := Offset(sq.Reversed(), Options{Radius: 2, Direction: Outside})
	if err != nil {
		t.Fatal(err)
	}
	if len(resRev.Chains) != 1 {
		t.Fatalf("reversed input: chains = %d, want 1 (diagnostics: %v)", len(resRev.Chains), resRev.Diagnostics)
	}
	if !kernel.EqualTol(math.Abs(resRev.Chains[0].SignedArea()), 196) {
		t.Fatalf("reversed input: area = %g, want 196", resRev.Chains[0].SignedArea())
	}
}

func TestOffsetSquarePocket(t *testing.T) {
	// 10×10 square pocket, inside offset r=2 → 6×6 loop.
	sq := polyChain(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(10, 10), kernel.P2(0, 10))
	res, err := Offset(sq, Options{Radius: 2, Direction: Inside})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Chains) != 1 {
		t.Fatalf("chains = %d, want 1 (diagnostics: %v)", len(res.Chains), res.Diagnostics)
	}
	if got := math.Abs(res.Chains[0].SignedArea()); !kernel.EqualTol(got, 36) {
		t.Fatalf("area = %g, want 36 (6×6)", got)
	}
}

func TestOffsetCirclePocket(t *testing.T) {
	// A radius-10 circle built from four quarter arcs, inside offset r=2:
	// the arcs meet tangentially, so the result is a clean R=8 circle.
	c, err := kernel.NewChain2([]kernel.Edge2{
		mustArc(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(0, 10)),
		mustArc(t, kernel.P2(0, 0), kernel.P2(0, 10), kernel.P2(-10, 0)),
		mustArc(t, kernel.P2(0, 0), kernel.P2(-10, 0), kernel.P2(0, -10)),
		mustArc(t, kernel.P2(0, 0), kernel.P2(0, -10), kernel.P2(10, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Offset(c, Options{Radius: 2, Direction: Inside})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Chains) != 1 {
		t.Fatalf("chains = %d, want 1 (diagnostics: %v)", len(res.Chains), res.Diagnostics)
	}
	ch := res.Chains[0]
	if !ch.IsClosed() {
		t.Fatal("chain must be closed")
	}
	arcs := 0
	for _, e := range ch.Edges {
		a, ok := e.AsArc()
		if !ok {
			t.Fatalf("expected arcs only, got %v", e)
		}
		arcs++
		if !kernel.EqualTol(a.Radius, 8) {
			t.Fatalf("arc radius = %g, want 8", a.Radius)
		}
	}
	if arcs != 4 {
		t.Fatalf("arcs = %d, want 4 (tangent joins must not insert edges)", arcs)
	}
	if got := math.Abs(ch.SignedArea()); !kernel.EqualTol(got, 64*math.Pi) {
		t.Fatalf("area = %g, want %g (π·8²)", got, 64*math.Pi)
	}
}

func TestOffsetNarrowSlotUnreachable(t *testing.T) {
	// A 3×10 slot with r=2: no tool-centre position clears both walls
	// (width 3 < 2·r). The safe outcome is no cut, material left in place,
	// and a diagnostic the planner can act on (smaller tool) — never a cut
	// that shreds the slot walls (cutter-compensation.md §3).
	slot := polyChain(t, kernel.P2(0, 0), kernel.P2(3, 0), kernel.P2(3, 10), kernel.P2(0, 10))
	res, err := Offset(slot, Options{Radius: 2, Direction: Inside})
	if err != nil {
		t.Fatalf("a too-narrow feature must resolve safely, not error: %v", err)
	}
	if len(res.Chains) != 0 {
		t.Fatalf("chains = %d, want 0 (the tool does not fit; over-cutting is forbidden)", len(res.Chains))
	}
	if !res.Unreachable() {
		t.Fatal("must report the feature as unreachable")
	}
}

func TestOffsetUProfile(t *testing.T) {
	// U-shape: 10×10 with a 4×6 notch out of the top edge (area 76).
	// Outside offset r=1 → one closed chain (area 12×12 − 2×6 = 132),
	// with every point at least r from the kept boundary.
	u := polyChain(t,
		kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(10, 10),
		kernel.P2(7, 10), kernel.P2(7, 4), kernel.P2(3, 4), kernel.P2(3, 10),
		kernel.P2(0, 10))
	if !kernel.EqualTol(math.Abs(u.SignedArea()), 76) {
		t.Fatalf("test fixture: area = %g, want 76", u.SignedArea())
	}
	res, err := Offset(u, Options{Radius: 1, Direction: Outside})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Chains) != 1 {
		t.Fatalf("chains = %d, want 1 (diagnostics: %v)", len(res.Chains), res.Diagnostics)
	}
	ch := res.Chains[0]
	if !ch.IsClosed() {
		t.Fatal("chain must be closed")
	}
	if got := math.Abs(ch.SignedArea()); !kernel.EqualTol(got, 132) {
		t.Fatalf("area = %g, want 132", got)
	}
	if d := minDistTo(ch, u); d < 1-1e-6 {
		t.Fatalf("offset comes within %g of the kept boundary (want ≥ 1)", d)
	}
}

func TestOffsetNarrowNotchTooLarge(t *testing.T) {
	// The same U with a 2-wide notch and r=2: the tool does not fit in the
	// notch (width 2 < 2·r) — the notch corner is degenerate (the offset
	// edges cross). Safe outcome: refuse the contour (ErrTooLarge); the
	// planner retries with a smaller tool. Whatever is produced must never
	// over-cut.
	u := polyChain(t,
		kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(10, 10),
		kernel.P2(6, 10), kernel.P2(6, 4), kernel.P2(4, 4), kernel.P2(4, 10),
		kernel.P2(0, 10))
	res, err := Offset(u, Options{Radius: 2, Direction: Outside})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, result = %+v; want ErrTooLarge (safe refusal, never an over-cut)", err, res)
	}
}

func TestOffsetSelfIntersectingInput(t *testing.T) {
	// A bowtie is not a well-formed contour: the kept region is ambiguous,
	// so Offset must refuse rather than guess.
	bow := polyChain(t, kernel.P2(0, 0), kernel.P2(10, 10), kernel.P2(10, 0), kernel.P2(0, 10))
	_, err := Offset(bow, Options{Radius: 1, Direction: Outside})
	if err == nil {
		t.Fatal("expected an error for a self-intersecting contour")
	}
}

func TestOffsetOpenChainRejected(t *testing.T) {
	e1, _ := kernel.NewLineEdge(kernel.P2(0, 0), kernel.P2(10, 0))
	e2, _ := kernel.NewLineEdge(kernel.P2(10, 0), kernel.P2(10, 10))
	open, _ := kernel.NewChain2([]kernel.Edge2{e1, e2})
	if open.IsClosed() {
		t.Fatal("test fixture must be open")
	}
	if _, err := Offset(open, Options{Radius: 1, Direction: Outside}); err == nil {
		t.Fatal("expected an error for an open chain")
	}
}

func TestReachableRejectsWrongSideAndTooClose(t *testing.T) {
	// isReachable is the safety gate behind safeResolve: a chain on the wrong
	// side of the boundary, or closer than r to it, must be rejected.
	// Fixture: a 10×10 square (the kept region) and a 6×6 chain centred in it.
	orig := polyChain(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(10, 10), kernel.P2(0, 10))
	inner := polyChain(t, kernel.P2(2, 2), kernel.P2(8, 2), kernel.P2(8, 8), kernel.P2(2, 8)) // 2 from the boundary

	if isReachable(inner, orig, Options{Radius: 3, Direction: Inside}) {
		t.Fatal("a chain 2 from the boundary must be rejected at r=3 (too close)")
	}
	if !isReachable(inner, orig, Options{Radius: 2, Direction: Inside}) {
		t.Fatal("a chain 2 from the boundary must be accepted at r=2 (tangent is safe)")
	}
	outer := polyChain(t, kernel.P2(-2, -2), kernel.P2(12, -2), kernel.P2(12, 12), kernel.P2(-2, 12))
	if isReachable(outer, orig, Options{Radius: 2, Direction: Inside}) {
		t.Fatal("an outside chain must be rejected for an inside offset")
	}
	if !isReachable(outer, orig, Options{Radius: 2, Direction: Outside}) {
		t.Fatal("an outside chain 2 from the boundary must be accepted at r=2")
	}
}

func TestOffsetOptionsValidate(t *testing.T) {
	cases := []struct {
		name    string
		o       Options
		wantErr bool
	}{
		{"valid", Options{Radius: 5, Direction: Outside, Join: JoinMitre}, false},
		{"zero radius", Options{Radius: 0}, true},
		{"negative radius", Options{Radius: -1}, true},
		{"nan radius", Options{Radius: math.NaN()}, true},
		{"bad direction", Options{Radius: 1, Direction: 7}, true},
		{"bad join", Options{Radius: 1, Join: 99}, true},
		{"negative mitre limit", Options{Radius: 1, MitreLimit: -1}, true},
		{"negative fillet radius", Options{Radius: 1, FilletRadius: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.o.validate(); (err != nil) != tc.wantErr {
				t.Fatalf("validate err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestOffsetDefaults(t *testing.T) {
	o := Options{Radius: 4}
	if got := o.mitreLimitMM(); got != DefaultMitreLimit*4 {
		t.Fatalf("default mitre limit = %g, want %g", got, DefaultMitreLimit*4)
	}
	if got := o.filletRadius(); got != 4 {
		t.Fatalf("default fillet radius = %g, want 4", got)
	}
	o.MitreLimit = 3
	o.FilletRadius = 7
	if got := o.mitreLimitMM(); got != 12 {
		t.Fatalf("mitre limit = %g, want 12", got)
	}
	if got := o.filletRadius(); got != 7 {
		t.Fatalf("fillet radius = %g, want 7", got)
	}
}
