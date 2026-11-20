package toolpath

import (
	"math"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

func baseLoopParams() LoopParams {
	return LoopParams{
		Milling:   moves.MillingClimb,
		Feed:      600,
		Spindle:   8000,
		SafeZ:     5,
		CutZ:      -10,
		Coolant:   moves.CoolantFlood,
		ToolID:    "endmill-10",
		Operation: "test",
	}
}

func chain(t *testing.T, edges ...kernel.Edge2) kernel.Chain2 {
	t.Helper()
	c, err := kernel.NewChain2(edges)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func lineEdge(t *testing.T, a, b kernel.Point2) kernel.Edge2 {
	t.Helper()
	e, err := kernel.NewLineEdge(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func arcEdge(t *testing.T, c, a, b kernel.Point2, cw bool) kernel.Edge2 {
	t.Helper()
	e, err := kernel.NewArcEdge(c, a, b, cw)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestLoopSquare(t *testing.T) {
	c := chain(t,
		lineEdge(t, kernel.P2(0, 0), kernel.P2(100, 0)),
		lineEdge(t, kernel.P2(100, 0), kernel.P2(100, 60)),
		lineEdge(t, kernel.P2(100, 60), kernel.P2(0, 60)),
		lineEdge(t, kernel.P2(0, 60), kernel.P2(0, 0)),
	)
	p, err := Loop(c, baseLoopParams())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("path should validate: %v", err)
	}
	if len(p.Moves) != 6 {
		t.Fatalf("moves = %d, want 6 (plunge + 4 cuts + retract)", len(p.Moves))
	}
	if p.Moves[0].Kind != moves.KindRapid || p.Moves[5].Kind != moves.KindRapid {
		t.Fatalf("first/last moves should be rapid, got %v / %v", p.Moves[0].Kind, p.Moves[5].Kind)
	}
	for i := 1; i <= 4; i++ {
		if p.Moves[i].Kind != moves.KindLinear {
			t.Fatalf("cutting move %d should be linear, got %v", i, p.Moves[i].Kind)
		}
		if p.Moves[i].From.Z != -10 || p.Moves[i].To.Z != -10 {
			t.Fatalf("cutting move %d should be at CutZ, got %v -> %v", i, p.Moves[i].From, p.Moves[i].To)
		}
	}
	if got := p.Length(); !kernel.EqualTol(got, 320) {
		t.Fatalf("length = %g, want 320", got)
	}
}

func TestLoopCircle(t *testing.T) {
	// A radius-10 circle as two half-arcs, CCW about (0,0).
	c := chain(t,
		arcEdge(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(-10, 0), false),
		arcEdge(t, kernel.P2(0, 0), kernel.P2(-10, 0), kernel.P2(10, 0), false),
	)
	p, err := Loop(c, baseLoopParams())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Moves) != 4 {
		t.Fatalf("moves = %d, want 4 (plunge + 2 arcs + retract)", len(p.Moves))
	}
	for i := 1; i <= 2; i++ {
		m := p.Moves[i]
		if m.Kind != moves.KindArcCCW {
			t.Fatalf("arc move %d kind = %v, want arc_ccw", i, m.Kind)
		}
		if !m.Center.Eq(kernel.P3(0, 0, -10), 1e-9) {
			t.Fatalf("arc move %d centre = %v, want (0,0,-10)", i, m.Center)
		}
	}
	if got := p.Length(); !kernel.EqualTol(got, 20*math.Pi) {
		t.Fatalf("length = %g, want %g", got, 20*math.Pi)
	}
}

func TestLoopReversedCircleIsCW(t *testing.T) {
	c := chain(t,
		arcEdge(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(-10, 0), false),
		arcEdge(t, kernel.P2(0, 0), kernel.P2(-10, 0), kernel.P2(10, 0), false),
	).Reversed()
	p, err := Loop(c, baseLoopParams())
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if p.Moves[i].Kind != moves.KindArcCW {
			t.Fatalf("reversed arc move %d kind = %v, want arc_cw", i, p.Moves[i].Kind)
		}
	}
}

func TestLoopMixedLineArc(t *testing.T) {
	// A D-shape: line bottom, semicircular right end (CCW contour).
	c := chain(t,
		lineEdge(t, kernel.P2(0, 0), kernel.P2(10, 0)),
		arcEdge(t, kernel.P2(10, 5), kernel.P2(10, 0), kernel.P2(10, 10), false),
		lineEdge(t, kernel.P2(10, 10), kernel.P2(0, 10)),
		lineEdge(t, kernel.P2(0, 10), kernel.P2(0, 0)),
	)
	p, err := Loop(c, baseLoopParams())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("path should validate: %v", err)
	}
	// 3 lines + 1 arc, in order.
	kinds := []moves.Kind{p.Moves[1].Kind, p.Moves[2].Kind, p.Moves[3].Kind, p.Moves[4].Kind}
	want := []moves.Kind{moves.KindLinear, moves.KindArcCCW, moves.KindLinear, moves.KindLinear}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("move kinds = %v, want %v", kinds, want)
		}
	}
	// 30 (three 10 mm lines) + 5π (semicircle of radius 5).
	if got := p.Length(); !kernel.EqualTol(got, 30+5*math.Pi) {
		t.Fatalf("length = %g, want %g", got, 30+5*math.Pi)
	}
}

func TestLoopOpenChain(t *testing.T) {
	// An open chain (in-and-out style path) must work: plunge at the start,
	// cuts, retract at the (different) end.
	c := chain(t,
		lineEdge(t, kernel.P2(0, 0), kernel.P2(10, 0)),
		lineEdge(t, kernel.P2(10, 0), kernel.P2(10, 5)),
	)
	p, err := Loop(c, baseLoopParams())
	if err != nil {
		t.Fatal(err)
	}
	if p.Moves[0].From.X != 0 || p.Moves[0].From.Y != 0 {
		t.Fatalf("plunge should start at the chain start, got %v", p.Moves[0].From)
	}
	last := p.Moves[len(p.Moves)-1]
	if !last.To.Eq(kernel.P3(10, 5, 5), 1e-9) {
		t.Fatalf("retract should end at the chain end at SafeZ, got %v", last.To)
	}
}

func TestLoopErrors(t *testing.T) {
	c := chain(t,
		lineEdge(t, kernel.P2(0, 0), kernel.P2(10, 0)),
		lineEdge(t, kernel.P2(10, 0), kernel.P2(10, 5)),
	)
	base := baseLoopParams()

	if _, err := Loop(c, LoopParams{Feed: 0, SafeZ: 5, CutZ: -10}); err == nil {
		t.Error("expected error for zero feed")
	}
	if _, err := Loop(c, LoopParams{Feed: 100, SafeZ: -5, CutZ: 10}); err == nil {
		t.Error("expected error for CutZ >= SafeZ")
	}
	// A chain with a gap must be rejected by the contour validation.
	gap, _ := kernel.NewChain2([]kernel.Edge2{
		lineEdge(t, kernel.P2(0, 0), kernel.P2(1, 0)),
		lineEdge(t, kernel.P2(5, 0), kernel.P2(6, 0)),
	})
	if _, err := Loop(gap, base); err == nil {
		t.Error("expected error for a chain with a gap")
	}
}
