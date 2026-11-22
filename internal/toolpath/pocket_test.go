package toolpath

import (
	"math"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

// --- fixtures -----------------------------------------------------------

func P(x, y float64) kernel.Point2 { return kernel.P2(x, y) }

// rectCCW is an axis-aligned rectangle (x0,y0)–(x1,y1) as a closed CCW chain.
func rectCCW(t *testing.T, x0, y0, x1, y1 float64) kernel.Chain2 {
	t.Helper()
	pts := []kernel.Point2{P(x0, y0), P(x1, y0), P(x1, y1), P(x0, y1)}
	edges := make([]kernel.Edge2, 0, 4)
	for i := 0; i < 4; i++ {
		e, err := kernel.NewLineEdge(pts[i], pts[(i+1)%4])
		if err != nil {
			t.Fatalf("rect edge %d: %v", i, err)
		}
		edges = append(edges, e)
	}
	c, err := kernel.NewChain2(edges)
	if err != nil {
		t.Fatalf("rect chain: %v", err)
	}
	return c
}

// circleCCW is a full circle (centre (cx, cy), radius r) as four quarter
// arcs, counter-clockwise (angles 0° → 90° → 180° → 270° → 360°).
func circleCCW(t *testing.T, cx, cy, r float64) kernel.Chain2 {
	t.Helper()
	c := P(cx, cy)
	right, top := P(cx+r, cy), P(cx, cy+r)
	left, bot := P(cx-r, cy), P(cx, cy-r)
	edges := make([]kernel.Edge2, 0, 4)
	for _, pair := range [][2]kernel.Point2{
		{right, top}, {top, left}, {left, bot}, {bot, right},
	} {
		e, err := kernel.NewArcEdge(c, pair[0], pair[1], false)
		if err != nil {
			t.Fatalf("arc %v→%v: %v", pair[0], pair[1], err)
		}
		edges = append(edges, e)
	}
	ch, err := kernel.NewChain2(edges)
	if err != nil {
		t.Fatalf("circle chain: %v", err)
	}
	return ch
}

// --- pure model functions -----------------------------------------------

func TestEngagement(t *testing.T) {
	cases := []struct {
		name    string
		R, s    float64
		want    float64
		wantErr bool
	}{
		{"full radius at 2R", 5, 10, 5, false},
		{"small stepover approx s^2/2R", 5, 1, 5 - math.Sqrt(25-0.25), false},
		{"half radius stepover", 1, 1, 1 - math.Sqrt(3)/2, false},
		{"zero radius refused", 0, 1, 0, true},
		{"zero stepover refused", 5, 0, 0, true},
		{"negative stepover refused", 5, -1, 0, true},
		{"stepover above 2R refused", 5, 10.1, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Engagement(c.R, c.s)
			if c.wantErr {
				if err == nil {
					t.Fatalf("Engagement(%g, %g) = %g, want error", c.R, c.s, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Engagement(%g, %g): %v", c.R, c.s, err)
			}
			if math.Abs(got-c.want) > 1e-9 {
				t.Fatalf("Engagement(%g, %g) = %g, want %g", c.R, c.s, got, c.want)
			}
		})
	}
}

func TestChipLoadFeed(t *testing.T) {
	cases := []struct {
		name     string
		F, aE, D float64
		want     float64
		wantErr  bool
	}{
		{"full engagement gives full feed", 800, 10, 10, 800, false},
		{"quarter engagement gives half feed", 800, 2.5, 10, 400, false},
		{"zero engagement gives zero feed", 800, 0, 10, 0, false},
		{"half model", 1000, 2.5, 10, 500, false},
		{"zero feed refused", 0, 1, 10, 0, true},
		{"zero diameter refused", 800, 1, 0, 0, true},
		{"negative engagement refused", 800, -0.1, 10, 0, true},
		{"engagement above diameter refused", 800, 11, 10, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ChipLoadFeed(c.F, c.aE, c.D)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ChipLoadFeed(%g, %g, %g) = %g, want error", c.F, c.aE, c.D, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChipLoadFeed(%g, %g, %g): %v", c.F, c.aE, c.D, err)
			}
			if math.Abs(got-c.want) > 1e-9 {
				t.Fatalf("ChipLoadFeed(%g, %g, %g) = %g, want %g", c.F, c.aE, c.D, got, c.want)
			}
		})
	}
}

func TestRowYs(t *testing.T) {
	cases := []struct {
		name       string
		minY, maxY float64
		s, R       float64
		want       []float64
	}{
		{"top coverage row added (the ADR-027 case)", 0, 12, 10, 5, []float64{5, 7}},
		{"region shorter than stepover gets one middle row", 0, 4, 10, 5, []float64{2}},
		{"even progression needs no extra row", 0, 20, 5, 2.5, []float64{2.5, 7.5, 12.5, 17.5}},
		{"s = 2R covered at the top", 0, 20, 8, 4, []float64{4, 12, 16}},
		{"single stepover span", -10, 10, 20, 10, []float64{0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rowYs(c.minY, c.maxY, c.s, c.R)
			if len(got) != len(c.want) {
				t.Fatalf("rowYs = %v, want %v", got, c.want)
			}
			for i := range got {
				if math.Abs(got[i]-c.want[i]) > 1e-9 {
					t.Fatalf("rowYs = %v, want %v", got, c.want)
				}
			}
			// Coverage property (ADR-027): every end is within R of a row,
			// and consecutive rows are at most s apart.
			if math.Abs(got[0]-c.minY) > c.R+1e-9 {
				t.Fatalf("first row %g is more than R=%g from the bottom edge", got[0], c.R)
			}
			if c.maxY-got[len(got)-1] > c.R+1e-9 {
				t.Fatalf("last row %g leaves the top edge %g more than R=%g uncovered", got[len(got)-1], c.maxY, c.R)
			}
			for i := 1; i < len(got); i++ {
				if got[i]-got[i-1] > c.s+1e-9 {
					t.Fatalf("rows %g and %g are more than s=%g apart", got[i-1], got[i], c.s)
				}
			}
		})
	}
}

func TestRowIntervals(t *testing.T) {
	t.Run("rectangle interior row", func(t *testing.T) {
		c := rectCCW(t, -20, -15, 20, 15)
		iv, err := rowIntervals(c, 0)
		if err != nil {
			t.Fatalf("rowIntervals: %v", err)
		}
		if len(iv) != 1 || iv[0].lo != -20 || iv[0].hi != 20 {
			t.Fatalf("row 0 of rectangle: got %v, want one interval [-20, 20]", iv)
		}
	})
	t.Run("row on the top edge is the safe tangent reading", func(t *testing.T) {
		c := rectCCW(t, -20, -15, 20, 15)
		iv, err := rowIntervals(c, 15)
		if err != nil {
			t.Fatalf("rowIntervals: %v", err)
		}
		if len(iv) != 1 || iv[0].lo != -20 || iv[0].hi != 20 {
			t.Fatalf("row 15 of rectangle: got %v, want one interval [-20, 20] (on the boundary — the tool disc is tangent, the safe side)", iv)
		}
	})
	t.Run("local maximum on the row is a touch, not a crossing", func(t *testing.T) {
		// Pentagon with its top vertex exactly on the row y=0; the interior
		// is entirely below the row, so there is no interval.
		pts := []kernel.Point2{P(-10, -10), P(10, -10), P(10, -2), P(0, 0), P(-10, -2)}
		var edges []kernel.Edge2
		for i := 0; i < len(pts); i++ {
			e, _ := kernel.NewLineEdge(pts[i], pts[(i+1)%len(pts)])
			edges = append(edges, e)
		}
		c, _ := kernel.NewChain2(edges)
		iv, err := rowIntervals(c, 0)
		if err != nil {
			t.Fatalf("rowIntervals: %v", err)
		}
		if len(iv) != 0 {
			t.Fatalf("row at the local maximum: got %v, want no interval (the row only touches the contour)", iv)
		}
	})
	t.Run("local minimum on the row is a touch, not a crossing", func(t *testing.T) {
		// Pentagon with its bottom vertex exactly on the row y=0; the
		// interior is entirely above the row, so there is no interval.
		pts := []kernel.Point2{P(-10, 10), P(10, 10), P(10, 2), P(0, 0), P(-10, 2)}
		var edges []kernel.Edge2
		for i := 0; i < len(pts); i++ {
			e, _ := kernel.NewLineEdge(pts[i], pts[(i+1)%len(pts)])
			edges = append(edges, e)
		}
		c, _ := kernel.NewChain2(edges)
		iv, err := rowIntervals(c, 0)
		if err != nil {
			t.Fatalf("rowIntervals: %v", err)
		}
		if len(iv) != 0 {
			t.Fatalf("row at the local minimum: got %v, want no interval (the row only touches the contour)", iv)
		}
	})
	t.Run("circle interior row", func(t *testing.T) {
		c := circleCCW(t, 0, 0, 5)
		iv, err := rowIntervals(c, 3)
		if err != nil {
			t.Fatalf("rowIntervals: %v", err)
		}
		if len(iv) != 1 || math.Abs(iv[0].lo+4) > 1e-9 || math.Abs(iv[0].hi-4) > 1e-9 {
			t.Fatalf("row 3 of circle r=5: got %v, want one interval [-4, 4]", iv)
		}
	})
	t.Run("circle tangency is a touch, not a crossing", func(t *testing.T) {
		c := circleCCW(t, 0, 0, 5)
		iv, err := rowIntervals(c, 5)
		if err != nil {
			t.Fatalf("rowIntervals: %v", err)
		}
		if len(iv) != 0 {
			t.Fatalf("row at the circle top: got %v, want no interval (tangency)", iv)
		}
	})
}

func TestSubtractIntervals(t *testing.T) {
	a := []interval{{lo: -20, hi: 20}}
	cases := []struct {
		name string
		b    []interval
		want []interval
	}{
		{"no bosses", nil, []interval{{lo: -20, hi: 20}}},
		{"one boss in the middle",
			[]interval{{lo: -5, hi: 5}},
			[]interval{{lo: -20, hi: -5}, {lo: 5, hi: 20}}},
		{"boss touching the left edge",
			[]interval{{lo: -20, hi: -10}},
			[]interval{{lo: -10, hi: 20}}},
		{"two overlapping bosses merge",
			[]interval{{lo: -5, hi: 0}, {lo: -2, hi: 4}},
			[]interval{{lo: -20, hi: -5}, {lo: 4, hi: 20}}},
		{"boss covering the whole row",
			[]interval{{lo: -25, hi: 25}},
			nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := subtractIntervals(a, c.b)
			if len(got) != len(c.want) {
				t.Fatalf("subtract = %v, want %v", got, c.want)
			}
			for i := range got {
				if math.Abs(got[i].lo-c.want[i].lo) > 1e-9 || math.Abs(got[i].hi-c.want[i].hi) > 1e-9 {
					t.Fatalf("subtract = %v, want %v", got, c.want)
				}
			}
		})
	}
}

// --- join arc -----------------------------------------------------------

func TestJoinArcPrefersBulgeInsideRegion(t *testing.T) {
	region := rectCCW(t, -10, -10, 10, 10)
	p1, p2 := P(9, -2), P(9, 2) // chord near the right wall
	e, ok := joinArc(p1, p2, region, nil)
	if !ok {
		t.Fatalf("joinArc: no candidate passed (want the arc bulging into the region)")
	}
	a, _ := e.AsArc()
	if !a.Center.Eq(P(9, 0), 1e-9) {
		t.Fatalf("join centre = %v, want (9, 0) (the chord midpoint)", a.Center)
	}
	if math.Abs(a.Radius-2) > 1e-9 {
		t.Fatalf("join radius = %g, want 2 (half the chord)", a.Radius)
	}
	bulge := a.PointAt(0.5)
	if !bulge.Eq(P(7, 0), 1e-9) {
		t.Fatalf("bulge = %v, want (7, 0) (inside the region, not (11, 0) outside)", bulge)
	}
}

func TestJoinArcRefusesBoss(t *testing.T) {
	region := rectCCW(t, -10, -10, 10, 10)
	// A boss disc centred on the chord, larger than the join radius: both
	// semicircles run inside kept material, so no candidate can pass the
	// gate — the safe outcome is a lift, not a join through the boss.
	boss := circleCCW(t, 0, 0, 6)
	e, ok := joinArc(P(-5, 0), P(5, 0), region, []kernel.Chain2{boss})
	if ok {
		a, _ := e.AsArc()
		t.Fatalf("joinArc passed with bulge %v, but the boss (r=6 at the origin) swallows both semicircles", a.PointAt(0.5))
	}
}

func TestJoinArcRoutesAroundBoss(t *testing.T) {
	// The boss (r=3 at (0, 8)) swallows the upper bulge (0, 5) and touches
	// the upper arc: that candidate fails the gate (the safe side — even a
	// tangent touch degrades), but the lower semicircle clears it with room
	// to spare, so the join still succeeds around the boss.
	region := rectCCW(t, -10, -10, 10, 10)
	boss := circleCCW(t, 0, 8, 3)
	e, ok := joinArc(P(-5, 0), P(5, 0), region, []kernel.Chain2{boss})
	if !ok {
		t.Fatalf("joinArc: the lower semicircle clears the boss — the join should succeed")
	}
	a, _ := e.AsArc()
	if !a.PointAt(0.5).Eq(P(0, -5), 1e-9) {
		t.Fatalf("bulge = %v, want (0, -5) (the lower arc, around the boss)", a.PointAt(0.5))
	}
}

// --- the full path ------------------------------------------------------

func baseParams(region kernel.Chain2) PocketParams {
	return PocketParams{
		Operation: "pocket",
		ToolID:    "test-tool",
		Milling:   moves.MillingClimb,
		FeedFull:  800,
		Spindle:   6000,
		SafeZ:     10,
		CutZ:      -5,
		Radius:    5,
		Stepover:  5,
		Region:    region,
	}
}

func TestPocketRectangle(t *testing.T) {
	region := rectCCW(t, -20, -15, 20, 15)
	p := baseParams(region)
	path, diags, err := Pocket(p)
	if err != nil {
		t.Fatalf("Pocket: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// ADR-020: the first move is a plunge whose From is at safe Z.
	m0 := path.Moves[0]
	if m0.Kind != moves.KindRapid || m0.From.Z != 10 || m0.To.Z != -5 {
		t.Fatalf("first move = %+v, want a plunge from SafeZ=10 to CutZ=-5", m0)
	}
	// The last move is a retract to safe Z.
	ml := path.Moves[len(path.Moves)-1]
	if ml.Kind != moves.KindRapid || ml.To.Z != 10 {
		t.Fatalf("last move = %+v, want a retract to SafeZ=10", ml)
	}

	// Structure: 6 rows × 1 line + 5 join arcs + plunge + retract = 13.
	if len(path.Moves) != 13 {
		t.Fatalf("path has %d moves, want 13 (plunge + 6 lines + 5 join arcs + retract)", len(path.Moves))
	}

	// Serpentine: row 0 left→right, row 1 right→left, …
	lines := []moves.Move{}
	arcs := []moves.Move{}
	for _, m := range path.Moves {
		switch m.Kind {
		case moves.KindLinear:
			lines = append(lines, m)
		case moves.KindArcCCW, moves.KindArcCW:
			arcs = append(arcs, m)
		}
	}
	if len(lines) != 6 || len(arcs) != 5 {
		t.Fatalf("got %d lines and %d arcs, want 6 and 5", len(lines), len(arcs))
	}
	wantY := []float64{-12.5, -7.5, -2.5, 2.5, 7.5, 12.5}
	for i, l := range lines {
		if l.From.Z != -5 || l.To.Z != -5 {
			t.Fatalf("line %d is not at CutZ: %+v", i, l)
		}
		if math.Abs(l.From.Y-wantY[i]) > 1e-9 {
			t.Fatalf("line %d at y=%g, want %g", i, l.From.Y, wantY[i])
		}
		if i%2 == 0 {
			if l.From.X != -20 || l.To.X != 20 {
				t.Fatalf("even line %d runs (%g → %g), want (-20 → 20)", i, l.From.X, l.To.X)
			}
		} else {
			if l.From.X != 20 || l.To.X != -20 {
				t.Fatalf("odd line %d runs (%g → %g), want (20 → -20)", i, l.From.X, l.To.X)
			}
		}
	}

	// Constant chip-load feed on every cutting move (ADR-022).
	aE, _ := Engagement(5, 5)
	feed, _ := ChipLoadFeed(800, aE, 10)
	for i, m := range path.Moves {
		if m.Kind == moves.KindRapid {
			continue
		}
		if math.Abs(m.Feed-feed) > 1e-9 {
			t.Fatalf("cutting move %d feed = %g, want %g (F_full*sqrt(a_e/D))", i, m.Feed, feed)
		}
		if m.Milling != moves.MillingClimb {
			t.Fatalf("cutting move %d milling = %q, want climb", i, m.Milling)
		}
	}

	// Join arcs: radius s/2, centre the chord midpoint, bulge inside.
	for i, a := range arcs {
		r := a.Center.Dist(a.From)
		if math.Abs(r-2.5) > 1e-9 {
			t.Fatalf("arc %d radius = %g, want 2.5 (s/2)", i, r)
		}
		mid := kernel.P2((a.From.X+a.To.X)/2, (a.From.Y+a.To.Y)/2)
		if math.Abs(a.Center.X-mid.X) > 1e-9 || math.Abs(a.Center.Y-mid.Y) > 1e-9 {
			t.Fatalf("arc %d centre = %v, want the chord midpoint %v", i, a.Center, mid)
		}
		// The centre is inside the region, or on the boundary when both
		// chord ends sit on the wall (the arc is tangent there — the safe
		// limit, e.g. the joins at x = ±20).
		cc := kernel.P2(a.Center.X, a.Center.Y)
		if !kernel.PointInContour(cc, region) && kernel.PointToChainDist(cc, region) > 1e-9 {
			t.Fatalf("arc %d centre %v is outside the region", i, a.Center)
		}
	}

	// Metadata records the engagement model (inspectability, ADR-003).
	if path.Metadata["a_e"] == "" || path.Metadata["stepover"] == "" {
		t.Fatalf("metadata missing engagement model: %v", path.Metadata)
	}
	if err := path.Validate(); err != nil {
		t.Fatalf("path is invalid: %v", err)
	}
}

func TestPocketWithBoss(t *testing.T) {
	// The Testmesh shape, at tool-centre scale: the 50×50 base inset by
	// R=5 gives the 40×40 region; the Ø25 cylinder offset outside by R=5
	// gives the r=17.5 boss.
	region := rectCCW(t, -20, -20, 20, 20)
	boss := circleCCW(t, 0, 0, 17.5)
	p := baseParams(region)
	p.Bosses = []kernel.Chain2{boss}
	path, diags, err := Pocket(p)
	if err != nil {
		t.Fatalf("Pocket: %v", err)
	}

	// The boss must be cleared, never cut: no cutting move may cross the
	// boss disc, and no plunge may land inside it.
	bossR := 17.5
	for i, m := range path.Moves {
		switch m.Kind {
		case moves.KindLinear:
			hw := math.Sqrt(bossR*bossR - m.From.Y*m.From.Y) // |y| < bossR
			if math.Abs(m.From.Y) >= bossR {
				continue // the row misses the boss
			}
			// The interval may touch the boss boundary at its endpoints (the
			// clipping point — the tool disc is tangent there, the safe limit)
			// but must not cross it: the open span (−hw, hw) must be free.
			lo, hi := math.Min(m.From.X, m.To.X), math.Max(m.From.X, m.To.X)
			if hi > -hw+1e-9 && lo < hw-1e-9 {
				t.Fatalf("linear move %d at y=%g crosses the boss disc (x %g→%g inside −%g..%g)", i, m.From.Y, m.From.X, m.To.X, hw, hw)
			}
		case moves.KindRapid:
			if m.From.Z == m.To.Z {
				continue // a horizontal rapid at safe Z is fine anywhere
			}
			// A plunge: the column must stay outside the boss.
			d := math.Hypot(m.From.X, m.From.Y)
			if d < bossR-1e-9 {
				t.Fatalf("plunge %d at (%g, %g) is inside the boss disc (r=%g)", i, m.From.X, m.From.Y, bossR)
			}
		case moves.KindArcCCW, moves.KindArcCW:
			// Sample the arc (its analytic clearance was gate-checked):
			// the bulge and a quarter point must clear the boss.
			for _, tt := range []float64{0.25, 0.5, 0.75} {
				cx := aPoint(m, tt).X
				cy := aPoint(m, tt).Y
				if math.Hypot(cx, cy) < bossR-1e-9 {
					t.Fatalf("arc move %d passes inside the boss disc at (%g, %g)", i, cx, cy)
				}
			}
		}
	}

	// Lifts exist: rows that the boss split have a retract/rapid/plunge
	// triple between their intervals.
	lifts := 0
	for i := 1; i+1 < len(path.Moves); i++ {
		if path.Moves[i-1].Kind == moves.KindRapid &&
			path.Moves[i].Kind == moves.KindRapid &&
			path.Moves[i+1].Kind == moves.KindRapid &&
			path.Moves[i-1].To.Z == 10 && path.Moves[i+1].To.Z == -5 {
			lifts++
		}
	}
	if lifts == 0 {
		t.Fatalf("expected lifts across the boss, but found none (diagnostics: %v)", diags)
	}

	// Coverage: the outer rows (y = ±17.5, tangent to the boss) are cut
	// full-width, so the annulus is cleared end to end.
	full := 0
	for _, m := range path.Moves {
		if m.Kind == moves.KindLinear && math.Abs(m.From.Y) > 17 {
			if m.From.X != -20 && m.To.X != -20 {
				continue
			}
			if math.Abs(m.From.X-m.To.X) > 39.9 {
				full++
			}
		}
	}
	if full < 2 {
		t.Fatalf("expected the two outer rows (y = ±17.5) cut full-width, found %d", full)
	}
	if err := path.Validate(); err != nil {
		t.Fatalf("path is invalid: %v", err)
	}
}

// aPoint evaluates an arc move at fraction t of its sweep.
func aPoint(m moves.Move, t float64) kernel.Point2 {
	c := kernel.P2(m.Center.X, m.Center.Y)
	a0 := math.Atan2(m.From.Y-m.Center.Y, m.From.X-m.Center.X)
	a1 := math.Atan2(m.To.Y-m.Center.Y, m.To.X-m.Center.X)
	var sweep float64
	if m.CW {
		sweep = -kernel.NormalizeAngle(a0 - a1)
	} else {
		sweep = kernel.NormalizeAngle(a1 - a0)
	}
	theta := a0 + t*sweep
	r := c.Dist(kernel.P2(m.From.X, m.From.Y))
	return kernel.P2(c.X+math.Cos(theta)*r, c.Y+math.Sin(theta)*r)
}

func TestPocketDropsSlivers(t *testing.T) {
	// A boss strip that leaves only 0.05 mm slivers on the rows it
	// crosses: those are dropped as residual (the safe side, ADR-026) with
	// a diagnostic; the other rows are cut full-width.
	region := rectCCW(t, -20, -20, 20, 20)
	boss := rectCCW(t, -19.95, -5, 19.95, 5)
	p := baseParams(region)
	p.Bosses = []kernel.Chain2{boss}
	path, diags, err := Pocket(p)
	if err != nil {
		t.Fatalf("Pocket: %v", err)
	}
	slivers := 0
	for _, d := range diags {
		if d.Level == "warning" {
			slivers++
		}
	}
	// The boss strip leaves a 0.05 mm sliver on each side, on both the
	// y=−2.5 and y=+2.5 rows: four slivers, all dropped as residual.
	if slivers != 4 {
		t.Fatalf("want 4 sliver warnings (rows y=±2.5, both sides), got %d: %v", slivers, diags)
	}
	// No cutting move at y = ±2.5 (the sliver rows are gone).
	for i, m := range path.Moves {
		if m.Kind == moves.KindLinear && (math.Abs(math.Abs(m.From.Y)-2.5) < 1e-9) {
			t.Fatalf("linear move %d at sliver row y=%g — it should have been dropped", i, m.From.Y)
		}
	}
	if err := path.Validate(); err != nil {
		t.Fatalf("path is invalid: %v", err)
	}
}

func TestPocketRefusals(t *testing.T) {
	region := rectCCW(t, -20, -20, 20, 20)
	cases := []struct {
		name   string
		mutate func(*PocketParams)
	}{
		{"stepover above 2R", func(p *PocketParams) { p.Stepover = 11 }},
		{"zero stepover", func(p *PocketParams) { p.Stepover = 0 }},
		{"zero radius", func(p *PocketParams) { p.Radius = 0 }},
		{"zero feed", func(p *PocketParams) { p.FeedFull = 0 }},
		{"cut z above safe z", func(p *PocketParams) { p.CutZ = 11 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseParams(region)
			c.mutate(&p)
			if _, _, err := Pocket(p); err == nil {
				t.Fatalf("Pocket with %s: want a precise refusal, got success", c.name)
			}
		})
	}
	t.Run("open region refused", func(t *testing.T) {
		p := baseParams(kernel.Chain2{Edges: []kernel.Edge2{
			mustEdge(t, P(-20, -20), P(20, -20)),
		}})
		if _, _, err := Pocket(p); err == nil {
			t.Fatalf("Pocket with an open region: want a refusal, got success")
		}
	})
	t.Run("degenerate region refused", func(t *testing.T) {
		e1, _ := kernel.NewLineEdge(P(0, 0), P(10, 0))
		e2, _ := kernel.NewLineEdge(P(10, 0), P(0, 0))
		p := baseParams(kernel.Chain2{Edges: []kernel.Edge2{e1, e2}})
		if _, _, err := Pocket(p); err == nil {
			t.Fatalf("Pocket with a zero-area region: want a refusal, got success")
		}
	})
}

func mustEdge(t *testing.T, a, b kernel.Point2) kernel.Edge2 {
	t.Helper()
	e, err := kernel.NewLineEdge(a, b)
	if err != nil {
		t.Fatalf("edge: %v", err)
	}
	return e
}
