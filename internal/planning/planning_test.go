package planning

import (
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/toolpath"
)

// squareCCW is a 10×10 CCW square (signed area +100).
func squareCCW(t *testing.T) kernel.Chain2 {
	t.Helper()
	c, err := kernel.NewChain2([]kernel.Edge2{
		mustEdge(t, kernel.P2(0, 0), kernel.P2(10, 0)),
		mustEdge(t, kernel.P2(10, 0), kernel.P2(10, 10)),
		mustEdge(t, kernel.P2(10, 10), kernel.P2(0, 10)),
		mustEdge(t, kernel.P2(0, 10), kernel.P2(0, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.SignedArea() <= 0 {
		t.Fatal("fixture must be CCW")
	}
	return c
}

func mustEdge(t *testing.T, a, b kernel.Point2) kernel.Edge2 {
	t.Helper()
	e, err := kernel.NewLineEdge(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// Fixture orientations (10×10 square);
// a planned chain's orientation is read from its first edge's end:
// CCW ends at (10,0), CW ends at (0,10).
//
// ADR-009 semantics: the planner first SELECTS the mode (default climb, or
// the explicit request), then ORIENTS the contour to achieve it — regardless
// of the input orientation.
func TestModeDerivation(t *testing.T) {
	ccw := kernel.P2(10, 0)
	cw := kernel.P2(0, 10)
	cases := []struct {
		name         string
		kind         FeatureKind
		spindle      SpindleRotation
		request      moves.Milling // "" = default (climb)
		inputCW      bool
		wantMode     moves.Milling
		wantFirstEnd kernel.Point2
	}{
		// Default request (climb): the plan orients for climb.
		{"profile ccw-spindle climbs CW", Profile, RotationCCW, "", false, moves.MillingClimb, cw},
		{"profile ccw-spindle climbs CW (from CW input)", Profile, RotationCCW, "", true, moves.MillingClimb, cw},
		{"profile cw-spindle climbs CCW", Profile, RotationCW, "", false, moves.MillingClimb, ccw},
		{"profile cw-spindle climbs CCW (from CW input)", Profile, RotationCW, "", true, moves.MillingClimb, ccw},
		{"pocket ccw-spindle climbs CCW", Pocket, RotationCCW, "", false, moves.MillingClimb, ccw},
		{"pocket ccw-spindle climbs CCW (from CW input)", Pocket, RotationCCW, "", true, moves.MillingClimb, ccw},
		{"pocket cw-spindle climbs CW", Pocket, RotationCW, "", false, moves.MillingClimb, cw},
		{"pocket cw-spindle climbs CW (from CW input)", Pocket, RotationCW, "", true, moves.MillingClimb, cw},
		// Explicit conventional request: the plan orients for conventional.
		{"profile ccw-spindle conventional is CCW", Profile, RotationCCW, moves.MillingConventional, false, moves.MillingConventional, ccw},
		{"profile cw-spindle conventional is CW", Profile, RotationCW, moves.MillingConventional, false, moves.MillingConventional, cw},
		{"pocket ccw-spindle conventional is CW", Pocket, RotationCCW, moves.MillingConventional, false, moves.MillingConventional, cw},
		{"pocket cw-spindle conventional is CCW", Pocket, RotationCW, moves.MillingConventional, false, moves.MillingConventional, ccw},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := squareCCW(t)
			if tc.inputCW {
				c = c.Reversed()
			}
			plan, err := PlanFeature(c, Params{Kind: tc.kind, Spindle: tc.spindle, Milling: tc.request, CutZ: -10})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Milling != tc.wantMode {
				t.Fatalf("mode = %v, want %v", plan.Milling, tc.wantMode)
			}
			if !plan.Chain.Edges[0].End().Eq(tc.wantFirstEnd, 1e-9) {
				t.Fatalf("planned first edge ends at %v, want %v (wrong orientation)",
					plan.Chain.Edges[0].End(), tc.wantFirstEnd)
			}
			// Cross-check: the documented predicate must agree with the plan.
			plannedCCW := plan.Chain.SignedArea() > 0
			climb := ((tc.kind == Pocket) == tc.spindle.SpindleCCW()) == plannedCCW
			if (plan.Milling == moves.MillingClimb) != climb {
				t.Fatalf("mode %v inconsistent with the oriented chain (ccw=%v)", plan.Milling, plannedCCW)
			}
		})
	}
}

func TestExplicitMillingOverride(t *testing.T) {
	// A CCW square for a profile with a CCW spindle naturally gives
	// conventional; requesting climb must re-orient instead.
	c := squareCCW(t)
	p := Params{Kind: Profile, Spindle: RotationCCW, Milling: moves.MillingConventional, CutZ: -10}
	plan, err := PlanFeature(c, p)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Milling != moves.MillingConventional {
		t.Fatalf("mode = %v, want conventional (the override)", plan.Milling)
	}
	if plan.Chain.Edges[0].Start().X != 0 || plan.Chain.Edges[0].Start().Y != 0 {
		t.Fatalf("chain should keep its CCW orientation, got first edge %v", plan.Chain.Edges[0])
	}
}

func TestOpenChainRejected(t *testing.T) {
	open, _ := kernel.NewChain2([]kernel.Edge2{
		mustEdge(t, kernel.P2(0, 0), kernel.P2(10, 0)),
		mustEdge(t, kernel.P2(10, 0), kernel.P2(10, 5)),
	})
	if _, err := PlanFeature(open, Params{Kind: Profile, CutZ: -10}); err == nil {
		t.Fatal("expected an error for an open chain")
	}
}

func TestPickStartAvoidsKeepOut(t *testing.T) {
	c := squareCCW(t) // vertices (0,0), (10,0), (10,10), (0,10)
	// A jaw over the first vertex at the cutting depth.
	jaw := KeepOut{
		Name: "vise-jaw",
		Box: &Box{
			Min: kernel.P3(-1, -1, -20),
			Max: kernel.P3(1, 1, 20),
		},
	}
	plan, err := PlanFeature(c, Params{Kind: Pocket, Spindle: RotationCCW, CutZ: -10, KeepOuts: []KeepOut{jaw}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Start.Eq(kernel.P2(10, 0), 1e-9) {
		t.Fatalf("start = %v, want (10,0) — the first vertex clear of the jaw", plan.Start)
	}
	if !plan.Chain.Edges[0].Start().Eq(kernel.P2(10, 0), 1e-9) {
		t.Fatalf("chain must start at the chosen vertex, got %v", plan.Chain.Edges[0])
	}
}

func TestPickStartCylinder(t *testing.T) {
	c := squareCCW(t)
	clamp := KeepOut{
		Name: "edge-clamp",
		Cyl:  &Cylinder{Center: kernel.P2(0, 0), Radius: 2, ZMin: -20, ZMax: 20},
	}
	plan, err := PlanFeature(c, Params{Kind: Pocket, Spindle: RotationCCW, CutZ: -10, KeepOuts: []KeepOut{clamp}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Start.Eq(kernel.P2(10, 0), 1e-9) {
		t.Fatalf("start = %v, want (10,0)", plan.Start)
	}
}

func TestKeepOutAboveCutIsIgnored(t *testing.T) {
	c := squareCCW(t)
	// A clamp that floats above the cutting depth does not apply.
	float := KeepOut{
		Name: "floating-clamp",
		Box:  &Box{Min: kernel.P3(-1, -1, 0), Max: kernel.P3(1, 1, 20)},
	}
	plan, err := PlanFeature(c, Params{Kind: Pocket, Spindle: RotationCCW, CutZ: -10, KeepOuts: []KeepOut{float}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Start.Eq(kernel.P2(0, 0), 1e-9) {
		t.Fatalf("start = %v, want (0,0) — the clamp is above the cut", plan.Start)
	}
}

func TestAllVerticesInKeepOutRefuses(t *testing.T) {
	c := squareCCW(t)
	room := KeepOut{
		Name: "vise",
		Box:  &Box{Min: kernel.P3(-5, -5, -20), Max: kernel.P3(15, 15, 20)},
	}
	_, err := PlanFeature(c, Params{Kind: Pocket, Spindle: RotationCCW, CutZ: -10, KeepOuts: []KeepOut{room}})
	if err == nil {
		t.Fatal("expected a refusal when every vertex is in a keep-out")
	}
	if !strings.Contains(err.Error(), "vise") {
		t.Fatalf("error should name the zone, got: %v", err)
	}
}

func TestRotate(t *testing.T) {
	c := squareCCW(t)
	r, err := Rotate(c, kernel.P2(10, 10))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Edges[0].Start().Eq(kernel.P2(10, 10), 1e-9) {
		t.Fatalf("rotated chain must start at (10,10), got %v", r.Edges[0].Start())
	}
	if r.Edges[0].End().Eq(kernel.P2(10, 10), 1e-9) {
		t.Fatal("first edge must not be zero-length")
	}
	if !kernel.EqualTol(r.SignedArea(), c.SignedArea()) {
		t.Fatalf("rotation must preserve orientation and area: %g vs %g", r.SignedArea(), c.SignedArea())
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("rotated chain must validate: %v", err)
	}
	if _, err := Rotate(c, kernel.P2(5, 5)); err == nil {
		t.Fatal("expected an error for a non-vertex start")
	}
}

func TestKeepOutValidate(t *testing.T) {
	if err := (KeepOut{Box: &Box{}, Cyl: &Cylinder{}}).Validate(); err == nil {
		t.Error("a zone with both box and cylinder must be rejected")
	}
	if err := (KeepOut{}).Validate(); err == nil {
		t.Error("a zone with neither box nor cylinder must be rejected")
	}
	bad := KeepOut{Box: &Box{Min: kernel.P3(1, 1, 1), Max: kernel.P3(1, 2, 2)}}
	if err := bad.Validate(); err == nil {
		t.Error("a box with equal min/max on an axis must be rejected")
	}
	zero := KeepOut{Cyl: &Cylinder{Radius: 0, ZMin: 0, ZMax: 1}}
	if err := zero.Validate(); err == nil {
		t.Error("a zero-radius cylinder must be rejected")
	}
}

func TestSpindleCCW(t *testing.T) {
	if !(SpindleRotation("")).SpindleCCW() {
		t.Error("empty rotation should default to CCW (right-handed convention)")
	}
	if !RotationCCW.SpindleCCW() || RotationCW.SpindleCCW() {
		t.Error("explicit rotations must report correctly")
	}
}

// TestPlanFeedsGeneration pins the ADR-009 seam: the planner orients and
// starts the chain; the generator emits exactly that traversal with the mode
// recorded on every cutting move.
func TestPlanFeedsGeneration(t *testing.T) {
	c := squareCCW(t)
	jaw := KeepOut{Box: &Box{Min: kernel.P3(-1, -1, -20), Max: kernel.P3(1, 1, 20)}}
	plan, err := PlanFeature(c, Params{Kind: Pocket, Spindle: RotationCCW, CutZ: -10, KeepOuts: []KeepOut{jaw}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := toolpath.Loop(plan.Chain, toolpath.LoopParams{
		Operation: "pocket",
		Milling:   plan.Milling,
		Feed:      500,
		SafeZ:     5,
		CutZ:      -10,
		ToolID:    "t1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Moves[1].From.X != 10 || p.Moves[1].From.Y != 0 {
		t.Fatalf("first cutting move must leave the chosen entry (10,0), got %v", p.Moves[1].From)
	}
	for i := 1; i <= 4; i++ {
		if p.Moves[i].Milling != plan.Milling {
			t.Fatalf("cutting move %d mode = %v, want %v (the plan's mode)", i, p.Moves[i].Milling, plan.Milling)
		}
	}
}
