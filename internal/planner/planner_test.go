package planner

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/comp"
	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/machine"
	"github.com/kutis96/cleancuts/internal/mesh"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/part"
	"github.com/kutis96/cleancuts/internal/planning"
	"github.com/kutis96/cleancuts/internal/stock"
	"github.com/kutis96/cleancuts/internal/tool"
	"github.com/kutis96/cleancuts/internal/toolpath"
)

// --- fixtures -----------------------------------------------------------

func P(x, y float64) kernel.Point2 { return kernel.P2(x, y) }

// tri is a keyed triangle (go vet rejects unkeyed literals of foreign
// structs).
func tri(a, b, c int) mesh.Tri { return mesh.Tri{A: a, B: b, C: c} }

// squareCCW returns the four corners of a centred square, CCW from above.
func squareCCW(half float64) []kernel.Point2 {
	return []kernel.Point2{P(half, half), P(-half, half), P(-half, -half), P(half, -half)}
}

// ringMesh builds a closed rectangular ring (outer minus inner) over [z0, z1]
// with consistent outward winding — the island-case fixture (copied from
// the part package tests; test self-containment over DRY).
func ringMesh(outer, inner []kernel.Point2, z0, z1 float64) mesh.Mesh {
	n := len(outer)
	v := make([]kernel.Point3, 0, 2*n)
	add := func(pts []kernel.Point2, z float64) []int {
		idx := make([]int, len(pts))
		for i, p := range pts {
			idx[i] = len(v)
			v = append(v, kernel.P3(p.X, p.Y, z))
		}
		return idx
	}
	Ob, Ot, Ib, It := add(outer, z0), add(outer, z1), add(inner, z0), add(inner, z1)
	t := make([]mesh.Tri, 0, 6*n)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		t = append(t, tri(Ob[i], Ob[j], Ot[j]), tri(Ob[i], Ot[j], Ot[i])) // outer walls
		t = append(t, tri(It[i], It[j], Ib[j]), tri(It[i], Ib[j], Ib[i])) // inner walls (into the hole)
		t = append(t, tri(Ot[i], Ot[j], It[j]), tri(Ot[i], It[j], It[i])) // top cap (+Z)
		t = append(t, tri(Ob[i], Ib[j], Ob[j]), tri(Ob[i], Ib[i], Ib[j])) // bottom cap (−Z)
	}
	return mesh.Mesh{Vertices: v, Triangles: t}
}

// boxMesh builds a closed solid box over the (CCW) footprint, z0..z1.
func boxMesh(foot []kernel.Point2, z0, z1 float64) mesh.Mesh {
	n := len(foot)
	v := make([]kernel.Point3, 0, 2*n)
	add := func(z float64) []int {
		idx := make([]int, n)
		for i, p := range foot {
			idx[i] = len(v)
			v = append(v, kernel.P3(p.X, p.Y, z))
		}
		return idx
	}
	b, t := add(z0), add(z1)
	tris := make([]mesh.Tri, 0, 2*n+2*(n-2))
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		tris = append(tris, tri(b[i], b[j], t[j]), tri(b[i], t[j], t[i]))
	}
	for i := 1; i < n-1; i++ {
		tris = append(tris, tri(t[0], t[i], t[i+1]), tri(b[0], b[i+1], b[i]))
	}
	return mesh.Mesh{Vertices: v, Triangles: tris}
}

// joinMesh concatenates two closed meshes into one (multi-body solid).
func joinMesh(a, b mesh.Mesh) mesh.Mesh {
	v := make([]kernel.Point3, 0, len(a.Vertices)+len(b.Vertices))
	v = append(v, a.Vertices...)
	v = append(v, b.Vertices...)
	t := make([]mesh.Tri, 0, len(a.Triangles)+len(b.Triangles))
	t = append(t, a.Triangles...)
	off := len(a.Vertices)
	for _, x := range b.Triangles {
		t = append(t, tri(x.A+off, x.B+off, x.C+off))
	}
	return mesh.Mesh{Vertices: v, Triangles: t}
}

// washer is an 80×80×10 plate with a 50×50 through-hole and a detached
// 20×20 centred island (three nested section loops).
func washer() mesh.Mesh {
	return joinMesh(
		ringMesh(squareCCW(40), squareCCW(25), 0, 10),
		boxMesh(squareCCW(10), 0, 10),
	)
}

// plateMesh loads the pinned example part: 60×40×10 plate, 20×20 hole.
func plateMesh(t *testing.T) mesh.Mesh {
	t.Helper()
	data, err := os.ReadFile("../../examples/parts/plate_60x40x10_hole20.stl")
	if err != nil {
		t.Fatalf("load example part: %v", err)
	}
	m, err := mesh.ParseSTL(data)
	if err != nil {
		t.Fatalf("parse example part: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate example part: %v", err)
	}
	return m
}

// --- the standard setup ---------------------------------------------------

func blockStock() stock.Stock {
	return stock.Stock{
		ID:    "block-120x80x20",
		Shape: stock.ShapeBox,
		Box:   &stock.Box{Min: kernel.P3(-10, -10, 0), Max: kernel.P3(110, 70, 20)},
	}
}

func endmill10() tool.Tool {
	return tool.Tool{
		ID:          "endmill-10mm-4f",
		Kind:        tool.KindFlatEndMill,
		Diameter:    10,
		Flutes:      4,
		FluteLength: 50,
		MaxRPM:      12000,
	}
}

func mill() machine.Machine {
	return machine.Machine{
		ID:      "mill-1",
		Dialect: machine.DialectGRBL,
		Units:   "mm",
		Axes: []machine.Axis{
			{Letter: "X", Min: -200, Max: 300, MaxFeed: 5000, MaxRapid: 10000},
			{Letter: "Y", Min: -200, Max: 200, MaxFeed: 5000, MaxRapid: 10000},
			{Letter: "Z", Min: -80, Max: 100, MaxFeed: 2000, MaxRapid: 5000},
		},
		SpindleMaxRPM: 15000,
		MinFeed:       50,
		MaxFeed:       5000,
		PostProcessor: machine.PostProcessor{
			SpindleStart: 3, SpindleStop: 5, ProgramEnd: 2,
			CoolantOn: 8, CoolantOff: 9, ArcFormat: "ij",
		},
	}
}

func baseParams(t *testing.T) Params {
	t.Helper()
	return Params{
		Part:    part.Part{ID: "plate", Name: "60x40 plate", Mesh: "plate.stl"},
		Mesh:    plateMesh(t),
		Stock:   blockStock(),
		Tool:    endmill10(),
		Machine: mill(),
		Feed:    500,
		Spindle: 8000,
		Coolant: moves.CoolantFlood,
	}
}

// ptrF is a float64 pointer helper.
func ptrF(v float64) *float64 { return &v }

// cuttingArea computes the signed shoelace area of a path's closed cutting
// loop (valid for the all-linear fixtures here; arcs would need the arc
// term). The modulo closes the loop — the last vertex links back to the
// first, which a plain consecutive-vertex sum misses.
func cuttingArea(t *testing.T, p moves.Path) float64 {
	t.Helper()
	pts := make([]kernel.Point2, 0, len(p.Moves))
	for _, m := range p.Moves {
		switch m.Kind {
		case moves.KindLinear, moves.KindArcCCW, moves.KindArcCW:
			if len(pts) == 0 {
				pts = append(pts, P(m.From.X, m.From.Y))
			}
			pts = append(pts, P(m.To.X, m.To.Y))
		}
	}
	if len(pts) < 3 {
		t.Fatalf("need at least 3 cutting vertices, got %d", len(pts))
	}
	var area float64
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		area += a.X*b.Y - a.Y*b.X
	}
	return area / 2
}

// --- the composition -------------------------------------------------------

func TestPlanPlate(t *testing.T) {
	out, err := Plan(baseParams(t))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// The Z model: defaults (slice 5 = midpoint of 0..10; cut 0 = part
	// bottom; safe = stock top 20 + 5).
	if out.SliceZ != 5 || out.CutZ != 0 || out.SafeZ != 25 {
		t.Fatalf("Z model = slice %g / cut %g / safe %g, want 5 / 0 / 25", out.SliceZ, out.CutZ, out.SafeZ)
	}

	// Two features, ordered: profile (depth 0) then pocket (depth 1).
	if len(out.Features) != 2 {
		t.Fatalf("features = %d, want 2", len(out.Features))
	}
	prof, pok := out.Features[0], out.Features[1]
	if prof.Feature.Kind != planning.Profile || prof.Feature.Area != 2400 {
		t.Fatalf("feature 0 = %+v, want the 2400 mm² profile", prof.Feature)
	}
	if pok.Feature.Kind != planning.Pocket || pok.Feature.Area != 400 {
		t.Fatalf("feature 1 = %+v, want the 400 mm² pocket", pok.Feature)
	}

	for name, fp := range map[string]FeaturePath{"profile": prof, "pocket": pok} {
		p := fp.Path
		if err := p.Validate(); err != nil {
			t.Fatalf("%s: invalid path: %v", name, err)
		}
		if p.ToolID != "endmill-10mm-4f" {
			t.Fatalf("%s: tool_id = %q", name, p.ToolID)
		}
		// Self-contained: plunge (rapid, safe→cut) … retract (rapid, cut→safe).
		first, last := p.Moves[0], p.Moves[len(p.Moves)-1]
		if first.Kind != moves.KindRapid || first.From.Z != out.SafeZ || first.To.Z != out.CutZ {
			t.Fatalf("%s: first move = %+v, want a plunge from safe %g to cut %g", name, first, out.SafeZ, out.CutZ)
		}
		if last.Kind != moves.KindRapid || last.From.Z != out.CutZ || last.To.Z != out.SafeZ {
			t.Fatalf("%s: last move = %+v, want a retract to safe %g", name, last, out.SafeZ)
		}
		// The entry is the plunge column, and the path is closed (retract
		// point == entry: the loop returns to where it started).
		if !last.From.Eq(kernel.P3(first.From.X, first.From.Y, out.CutZ), 1e-9) {
			t.Fatalf("%s: the loop must close (retract at the entry)", name)
		}
		if !fp.Entry.Eq(P(first.From.X, first.From.Y), 1e-9) {
			t.Fatalf("%s: entry %v does not match the plunge column %v", name, fp.Entry, P(first.From.X, first.From.Y))
		}
		// Process values on every cutting move.
		for i, m := range p.Moves {
			if m.Kind == moves.KindRapid {
				continue
			}
			if m.From.Z != out.CutZ || m.To.Z != out.CutZ {
				t.Fatalf("%s: cutting move %d at Z %g..%g, want the cut Z %g", name, i, m.From.Z, m.To.Z, out.CutZ)
			}
			if m.Feed != 500 {
				t.Fatalf("%s: cutting move %d feed %g, want 500", name, i, m.Feed)
			}
			if m.Spindle != 8000 {
				t.Fatalf("%s: cutting move %d spindle %g, want 8000", name, i, m.Spindle)
			}
			if m.Milling != moves.MillingClimb {
				t.Fatalf("%s: cutting move %d milling %q, want climb", name, i, m.Milling)
			}
			if m.Coolant != moves.CoolantFlood {
				t.Fatalf("%s: cutting move %d coolant %q, want flood", name, i, m.Coolant)
			}
		}
	}

	// The climb predicate (ADR-009) with a CCW spindle: the profile mills
	// CW (negative signed area) and the pocket CCW (positive) — both climb.
	// Offsets: 60×40 + 2·5 → 70×50 = 3500; 20×20 − 2·5 → 10×10 = 100.
	pa := cuttingArea(t, prof.Path)
	if pa > 0 || math.Abs(math.Abs(pa)-3500) > 1e-9 {
		t.Fatalf("profile cutting area = %g, want −3500 (CW climb, 70×50 offset)", pa)
	}
	pka := cuttingArea(t, pok.Path)
	if pka < 0 || math.Abs(pka-100) > 1e-9 {
		t.Fatalf("pocket cutting area = %g, want +100 (CCW climb, 10×10 offset)", pka)
	}

	if len(out.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", out.Diagnostics)
	}
}

func TestPlanExplicitCutZAndClearance(t *testing.T) {
	p := baseParams(t)
	partCut := 2.0
	p.Part.CutZ = &partCut
	p.SafeClearance = 3.0
	out, err := Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if out.CutZ != 2 || out.SafeZ != 23 {
		t.Fatalf("Z model = cut %g / safe %g, want 2 / 23", out.CutZ, out.SafeZ)
	}

	// A planner-level cut_z override beats the recipe's.
	over := 3.5
	p.CutZ = &over
	out, err = Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if out.CutZ != 3.5 {
		t.Fatalf("override cut_z = %g, want 3.5", out.CutZ)
	}
}

// --- keep-outs --------------------------------------------------------------

// TestPlanFloatingClampEntry pins ADR-018 §4 end to end: a clamp floating
// above the cutting plane but inside the plunge column blocks the entry
// underneath it (the path at the cut height is unaffected — the zone does
// not apply there).
func TestPlanFloatingClampEntry(t *testing.T) {
	base := baseParams(t)
	feats, err := part.Detect(base.Mesh, 5)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	v0 := feats[0].Contour.Start() // the profile's first vertex
	clamp := planning.KeepOut{
		Name: "tab-clamp",
		Box: &planning.Box{
			Min: kernel.P3(v0.X-8, v0.Y-8, 12), // above the cut (0)…
			Max: kernel.P3(v0.X+8, v0.Y+8, 18), // …below the safe height (25)
		},
	}

	// Without the clamp, the entry is the first clear vertex in chain order
	// — the offset contour's start, under the clamp.
	clean, err := Plan(base)
	if err != nil {
		t.Fatalf("Plan (no clamp): %v", err)
	}
	blocked := clean.Features[0].Entry

	// With the clamp, the same entry must be refused: the plunge column
	// through it would pass through the clamp.
	base.KeepOuts = []planning.KeepOut{clamp}
	withClamp, err := Plan(base)
	if err != nil {
		t.Fatalf("Plan (clamp): %v", err)
	}
	entry := withClamp.Features[0].Entry
	if entry.Eq(blocked, 1e-9) {
		t.Fatalf("entry %v sits under the floating clamp — the plunge would hit it", entry)
	}
	// The entry must clear the clamp's footprint entirely.
	if entry.X >= v0.X-8 && entry.X <= v0.X+8 && entry.Y >= v0.Y-8 && entry.Y <= v0.Y+8 {
		t.Fatalf("entry %v is inside the clamp footprint", entry)
	}
	// The rest of the plan is unchanged: the path at the cut height does
	// not touch the zone (it applies only at z 12..18).
	if err := withClamp.Validate(); err != nil {
		t.Fatalf("result: %v", err)
	}
}

// TestPlanPathThroughKeepOutRefused pins the hard constraint (ADR-012,
// Law 7): a keep-out at the cut height that the tool envelope must pass
// through refuses the plan, naming the zone.
func TestPlanPathThroughKeepOutRefused(t *testing.T) {
	base := baseParams(t)
	// Compute the profile's offset contour and put a jaw over its first
	// vertex: the path itself passes through the jaw's footprint.
	feats, err := part.Detect(base.Mesh, 5)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	res, err := comp.Offset(feats[0].Contour, comp.Options{Radius: base.Tool.Radius(), Direction: comp.Outside})
	if err != nil {
		t.Fatalf("Offset: %v", err)
	}
	if len(res.Chains) == 0 {
		t.Fatal("fixture: expected an offset chain")
	}
	c := res.Chains[0].Start()
	base.KeepOuts = []planning.KeepOut{{
		Name: "jaw",
		Box: &planning.Box{
			Min: kernel.P3(c.X-4, c.Y-4, 0),
			Max: kernel.P3(c.X+4, c.Y+4, 20),
		},
	}}
	_, err = Plan(base)
	if err == nil {
		t.Fatal("expected a refusal: the profile path passes through the jaw at the cut height")
	}
	for _, want := range []string{"jaw", "keep-out"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should mention %q", err, want)
		}
	}
}

// --- refusals ----------------------------------------------------------------

func TestPlanToolTooBigForPocket(t *testing.T) {
	p := baseParams(t)
	p.Tool = tool.Tool{
		ID:          "endmill-30mm",
		Kind:        tool.KindFlatEndMill,
		Diameter:    30, // radius 15 > half the 20 mm hole
		Flutes:      4,
		FluteLength: 50,
		MaxRPM:      8000,
	}
	p.Spindle = 5000
	_, err := Plan(p)
	if err == nil {
		t.Fatal("expected a refusal: a 30 mm tool cannot mill a 20 mm hole")
	}
	for _, want := range []string{"endmill-30mm", "smaller tool"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should mention %q", err, want)
		}
	}
}

func TestPlanProcessRefusals(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Params)
		wantSub string
	}{
		{"feed below machine minimum", func(p *Params) { p.Feed = 40 }, "minimum 50"},
		{"feed above machine maximum", func(p *Params) { p.Feed = 6000 }, "maximum 5000"},
		{"spindle above tool max", func(p *Params) { p.Spindle = 13000 }, "endmill-10mm-4f"},
		{"spindle above machine max", func(p *Params) { p.Spindle = 16000 }, "mill-1"},
		{"unknown coolant", func(p *Params) { p.Coolant = moves.Coolant("turbo") }, "unknown coolant"},
		{"inch machine (v1 is mm-only)", func(p *Params) { p.Machine.Units = "inch" }, "millimetres"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseParams(t)
			c.mutate(&p)
			_, err := Plan(p)
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("want refusal mentioning %q, got %v", c.wantSub, err)
			}
		})
	}
}

func TestPlanFluteTooShort(t *testing.T) {
	p := baseParams(t)
	p.Tool.FluteLength = 5 // the part is 10 mm deep
	_, err := Plan(p)
	if err == nil || !strings.Contains(err.Error(), "flute") {
		t.Fatalf("want a flute-length refusal, got %v", err)
	}
}

func TestPlanCutZBounds(t *testing.T) {
	// (A cut_z above the stock top is unreachable with a sane part — the
	// cut must be below slice_z, which lies inside the part, which lies
	// inside the stock — so Plan's "cut air" guard is defense-in-depth.)
	cases := []struct {
		name    string
		cutZ    float64
		wantSub string
	}{
		{"below the stock bottom", -5, "stock bottom"},
		{"above the section", 6, "strictly below slice_z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseParams(t)
			p.Part.CutZ = ptrF(c.cutZ)
			_, err := Plan(p)
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("want refusal mentioning %q, got %v", c.wantSub, err)
			}
		})
	}
}

func TestPlanPartOutsideStock(t *testing.T) {
	p := baseParams(t)
	p.Stock = stock.Stock{
		ID:    "too-small",
		Shape: stock.ShapeBox,
		Box:   &stock.Box{Min: kernel.P3(0, 0, 0), Max: kernel.P3(30, 30, 20)},
	}
	_, err := Plan(p)
	if err == nil || !strings.Contains(err.Error(), "stock") {
		t.Fatalf("want a stock-fit refusal, got %v", err)
	}
}

// --- the island case (kept material is never milled, ADR-021) --------------

func TestPlanKeepsSolidIsland(t *testing.T) {
	// The island is solid material rising above the section: it classifies
	// as a Boss and is never milled (Law 7 — milling its outline would cut
	// through the solid island). The plan covers profile + hole only.
	p := Params{
		Part: part.Part{ID: "washer", Name: "washer with island", Mesh: "washer.stl"},
		Mesh: washer(),
		Stock: stock.Stock{
			ID:    "block-100",
			Shape: stock.ShapeBox,
			Box:   &stock.Box{Min: kernel.P3(-50, -50, 0), Max: kernel.P3(50, 50, 15)},
		},
		Tool:    endmill10(),
		Machine: mill(),
		Feed:    500,
		Spindle: 8000,
	}
	out, err := Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(out.Features) != 2 {
		t.Fatalf("features = %d, want 2 (profile, hole — the solid island is kept)", len(out.Features))
	}
	want := []struct {
		kind  planning.FeatureKind
		depth int
	}{
		{planning.Profile, 0},
		{planning.Pocket, 1},
	}
	for i, w := range want {
		f := out.Features[i]
		if f.Feature.Kind != w.kind || f.Feature.Depth != w.depth {
			t.Fatalf("feature %d = kind %s depth %d, want %s depth %d", i, f.Feature.Kind, f.Feature.Depth, w.kind, w.depth)
		}
		if f.Path.Moves[0].Kind != moves.KindRapid {
			t.Fatalf("feature %d: first move must be the plunge rapid", i)
		}
		for _, m := range f.Path.Moves {
			if m.Kind != moves.KindRapid && m.Milling != moves.MillingClimb {
				t.Fatalf("feature %d: cutting move milling %q, want climb", i, m.Milling)
			}
		}
	}
	// Offsets (r=5, CCW spindle, climb): profile 90×90 CW (−8100),
	// pocket 40×40 CCW (+1600).
	wantAreas := []float64{-8100, +1600}
	for i, wa := range wantAreas {
		a := cuttingArea(t, out.Features[i].Path)
		if math.Abs(a-wa) > 1e-9 {
			t.Fatalf("feature %d cutting area = %g, want %g", i, a, wa)
		}
	}
}

// --- the Testmesh case (face section + boss, ADR-021/025) -----------------

func testmesh(t *testing.T) mesh.Mesh {
	t.Helper()
	data, err := os.ReadFile("../../examples/parts/testmesh_cyl_on_base.stl")
	if err != nil {
		t.Fatalf("load testmesh: %v", err)
	}
	m, err := mesh.ParseSTL(data)
	if err != nil {
		t.Fatalf("parse testmesh: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate testmesh: %v", err)
	}
	return m
}

func TestPlanTestmeshProfileSkipsBoss(t *testing.T) {
	// The friend's part: profile mode at the base-top face (z=-10) mills
	// the 50×50 base outline and leaves the cylinder (a Boss) untouched —
	// never milled (ADR-021, Law 7). The annulus clear is the pocket
	// operation (ADR-022/026, a later slice).
	slice, cut := -10.0, -15.0
	p := Params{
		Part: part.Part{ID: "testmesh", Name: "cylinder on base", Mesh: "testmesh.stl", SliceZ: &slice, CutZ: &cut},
		Mesh: testmesh(t),
		Stock: stock.Stock{
			ID:    "block-70x70x30",
			Shape: stock.ShapeBox,
			Box:   &stock.Box{Min: kernel.P3(-35, -35, -25), Max: kernel.P3(35, 35, 5)},
		},
		Tool:    endmill10(),
		Machine: mill(),
		Feed:    500,
		Spindle: 8000,
	}
	out, err := Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(out.Features) != 1 {
		t.Fatalf("features = %d, want 1 (the base outline; the boss is kept)", len(out.Features))
	}
	f := out.Features[0]
	if f.Feature.Kind != planning.Profile || f.Feature.Area != 2500 {
		t.Fatalf("feature 0 = %+v, want the 2500 mm² base profile", f.Feature)
	}
	// Offset outward by r=5: a 60×60 contour (area 3600).
	a := cuttingArea(t, f.Path)
	if math.Abs(math.Abs(a)-3600) > 1e-9 {
		t.Fatalf("cutting |area| = %g, want 3600 (60×60 offset outline)", a)
	}
	// No move may enter the cylinder footprint (radius 12.5 + tool 5 =
	// 17.5 from the origin for a tool centre).
	for i, mv := range f.Path.Moves {
		if mv.Kind == moves.KindRapid {
			continue
		}
		for _, q := range []kernel.Point2{P(mv.From.X, mv.From.Y), P(mv.To.X, mv.To.Y)} {
			if math.Hypot(q.X, q.Y) < 17.5-1e-9 {
				t.Fatalf("move %d point (%g,%g) is inside the kept cylinder + tool radius", i, q.X, q.Y)
			}
		}
	}
}

// --- ToProgram (ADR-019 §2) -------------------------------------------------

func TestToProgramInsertsVerifiedLinks(t *testing.T) {
	base := baseParams(t)
	res, err := Plan(base)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	prog, err := res.ToProgram("plate-first-cut", base.KeepOuts, base.Tool.Radius())
	if err != nil {
		t.Fatalf("ToProgram: %v", err)
	}
	if err := prog.Validate(); err != nil {
		t.Fatalf("program: %v", err)
	}
	// Two features + one link between them.
	if len(prog.Paths) != 3 {
		t.Fatalf("paths = %d, want 3 (feature, link, feature)", len(prog.Paths))
	}
	ids := []string{prog.Paths[0].ID, prog.Paths[1].ID, prog.Paths[2].ID}
	if ids[0] != "profile-0" || ids[1] != "link-1" || ids[2] != "pocket-1" {
		t.Fatalf("path IDs = %v, want [profile-0 link-1 pocket-1]", ids)
	}
	// The link is one rapid from the profile's retract top to the pocket's
	// plunge top, both at the safe height.
	link := prog.Paths[1]
	if len(link.Moves) != 1 || link.Moves[0].Kind != moves.KindRapid {
		t.Fatalf("link = %+v, want one rapid move", link.Moves)
	}
	mv := link.Moves[0]
	if mv.From.Z != res.SafeZ || mv.To.Z != res.SafeZ {
		t.Fatalf("link at Z %g..%g, want the safe height %g on both ends", mv.From.Z, mv.To.Z, res.SafeZ)
	}
	// The Z model rides along in metadata.
	if prog.Metadata["cut_z"] != "0" || prog.Metadata["safe_z"] != "25" || prog.Metadata["slice_z"] != "5" {
		t.Fatalf("metadata = %v", prog.Metadata)
	}
}

func TestToProgramRefusesLinkThroughKeepOut(t *testing.T) {
	base := baseParams(t)
	res, err := Plan(base)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// A tall wall between the two entries, covering the safe height: the
	// link rapid would sweep straight through it.
	e1 := res.Features[0].Entry
	e2 := res.Features[1].Entry
	mid := kernel.P2((e1.X+e2.X)/2, (e1.Y+e2.Y)/2)
	base.KeepOuts = []planning.KeepOut{{
		Name: "tall-clamp",
		Box: &planning.Box{
			Min: kernel.P3(mid.X-2, mid.Y-2, 20), // covers safeZ = 25
			Max: kernel.P3(mid.X+2, mid.Y+2, 30),
		},
	}}
	_, err = res.ToProgram("prog", base.KeepOuts, base.Tool.Radius())
	if err == nil || !strings.Contains(err.Error(), "tall-clamp") {
		t.Fatalf("want a refusal naming the clamp, got %v", err)
	}
}

func TestToProgramOmitsZeroLengthLink(t *testing.T) {
	// Two features sharing the same entry point: the travel is zero-length
	// and must be omitted, not emitted (ADR-019 §2).
	loop := moves.Path{
		ID:   "a",
		Name: "a",
		Moves: []moves.Move{
			{Kind: moves.KindRapid, From: kernel.P3(0, 0, 25), To: kernel.P3(0, 0, 0)},
			{Kind: moves.KindLinear, From: kernel.P3(0, 0, 0), To: kernel.P3(10, 0, 0), Feed: 500, Milling: moves.MillingClimb},
			{Kind: moves.KindLinear, From: kernel.P3(10, 0, 0), To: kernel.P3(10, 10, 0), Feed: 500, Milling: moves.MillingClimb},
			{Kind: moves.KindLinear, From: kernel.P3(10, 10, 0), To: kernel.P3(0, 10, 0), Feed: 500, Milling: moves.MillingClimb},
			{Kind: moves.KindLinear, From: kernel.P3(0, 10, 0), To: kernel.P3(0, 0, 0), Feed: 500, Milling: moves.MillingClimb},
			{Kind: moves.KindRapid, From: kernel.P3(0, 0, 0), To: kernel.P3(0, 0, 25)},
		},
	}
	loopB := loop
	loopB.ID = "b"
	res := Result{
		SliceZ: 5,
		CutZ:   0,
		SafeZ:  25,
		Features: []FeaturePath{
			{Feature: part.Feature{Index: 0, Kind: planning.Profile, Area: 100}, Path: loop, Entry: kernel.P2(0, 0)},
			{Feature: part.Feature{Index: 1, Kind: planning.Profile, Area: 100}, Path: loopB, Entry: kernel.P2(0, 0)},
		},
	}
	prog, err := res.ToProgram("prog", nil, 5)
	if err != nil {
		t.Fatalf("ToProgram: %v", err)
	}
	if len(prog.Paths) != 2 {
		t.Fatalf("paths = %d, want 2 (no zero-length link)", len(prog.Paths))
	}
}

func TestToProgramRefusals(t *testing.T) {
	res := Result{SliceZ: 5, CutZ: 0, SafeZ: 25, Features: []FeaturePath{{
		Feature: part.Feature{Index: 0, Kind: planning.Profile, Area: 100},
		Path: moves.Path{
			ID: "a",
			Moves: []moves.Move{
				{Kind: moves.KindRapid, From: kernel.P3(0, 0, 25), To: kernel.P3(0, 0, 0)},
				{Kind: moves.KindRapid, From: kernel.P3(0, 0, 0), To: kernel.P3(0, 0, 25)},
			},
		},
		Entry: kernel.P2(0, 0),
	}}}
	if _, err := res.ToProgram("", nil, 5); err == nil {
		t.Fatal("want a refusal for an empty program ID")
	}
	if _, err := res.ToProgram("p", nil, -1); err == nil {
		t.Fatal("want a refusal for a negative tool radius")
	}
}

// --- result invariants -------------------------------------------------------

func TestResultValidate(t *testing.T) {
	cases := []struct {
		name    string
		r       Result
		wantSub string
	}{
		{"no features", Result{CutZ: 0, SafeZ: 25}, "no features"},
		{"cut not below safe", Result{CutZ: 10, SafeZ: 10, Features: []FeaturePath{{Path: moves.Path{ID: "p", Moves: []moves.Move{{Kind: moves.KindRapid, From: kernel.P3(0, 0, 10), To: kernel.P3(0, 0, 5)}}}}}}, "SafeZ"},
		{"slice not above cut", Result{SliceZ: 0, CutZ: 0, SafeZ: 25, Features: []FeaturePath{{Path: moves.Path{ID: "p", Moves: []moves.Move{{Kind: moves.KindRapid, From: kernel.P3(0, 0, 10), To: kernel.P3(0, 0, 5)}}}}}}, "SliceZ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.r.Validate()
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("want error mentioning %q, got %v", c.wantSub, err)
			}
		})
	}
}

// --- pocket (ADR-022, ADR-026, ADR-027, ADR-028) ---------------------------

// smallEndmill is a Ø0.5 2-flute endmill (R = 0.25) for the small fixtures.
func smallEndmill() tool.Tool {
	return tool.Tool{
		ID:          "endmill-0.5mm-2f",
		Kind:        tool.KindFlatEndMill,
		Diameter:    0.5,
		Flutes:      2,
		FluteLength: 10,
		MaxRPM:      20000,
	}
}

// tube2 is a 2×2×1 plate with a 1×1 centred through-hole (z 0..1).
func tube2() mesh.Mesh {
	v := []kernel.Point3{
		kernel.P3(0, 0, 0), kernel.P3(2, 0, 0), kernel.P3(2, 2, 0), kernel.P3(0, 2, 0),
		kernel.P3(0, 0, 1), kernel.P3(2, 0, 1), kernel.P3(2, 2, 1), kernel.P3(0, 2, 1),
		kernel.P3(0.5, 0.5, 0), kernel.P3(1.5, 0.5, 0), kernel.P3(1.5, 1.5, 0), kernel.P3(0.5, 1.5, 0),
		kernel.P3(0.5, 0.5, 1), kernel.P3(1.5, 0.5, 1), kernel.P3(1.5, 1.5, 1), kernel.P3(0.5, 1.5, 1),
	}
	t := []mesh.Tri{
		tri(0, 1, 5), tri(0, 5, 4), tri(6, 2, 3), tri(7, 6, 3), tri(7, 3, 0), tri(4, 7, 0), tri(1, 2, 6), tri(1, 6, 5),
		tri(8, 11, 15), tri(8, 15, 12), tri(14, 10, 9), tri(13, 14, 9), tri(13, 9, 8), tri(12, 13, 8),
		tri(11, 10, 14), tri(11, 14, 15),
		tri(4, 5, 13), tri(4, 13, 12), tri(5, 6, 14), tri(5, 14, 13), tri(6, 7, 15), tri(6, 15, 14), tri(7, 4, 12), tri(7, 12, 15),
		tri(9, 1, 0), tri(8, 9, 0), tri(10, 2, 1), tri(9, 10, 1), tri(11, 3, 2), tri(10, 11, 2), tri(8, 0, 3), tri(11, 8, 3),
	}
	return mesh.Mesh{Vertices: v, Triangles: t}
}

// recessPlate is a 4×4×1 plate (z 0..1) with a 1×1 recess [1.5,2.5]² cut
// into the top, floor at z=0.5 — a blind void at the top face (ADR-028).
func recessPlate() mesh.Mesh {
	v := []kernel.Point3{
		kernel.P3(0, 0, 0), kernel.P3(4, 0, 0), kernel.P3(4, 4, 0), kernel.P3(0, 4, 0),
		kernel.P3(0, 0, 0.5), kernel.P3(4, 0, 0.5), kernel.P3(4, 4, 0.5), kernel.P3(0, 4, 0.5),
		kernel.P3(1.5, 0.5, 0.5), kernel.P3(2.5, 0.5, 0.5), kernel.P3(2.5, 2.5, 0.5), kernel.P3(1.5, 2.5, 0.5),
		kernel.P3(0, 0, 1), kernel.P3(4, 0, 1), kernel.P3(4, 4, 1), kernel.P3(0, 4, 1),
		kernel.P3(1.5, 0.5, 1), kernel.P3(2.5, 0.5, 1), kernel.P3(2.5, 2.5, 1), kernel.P3(1.5, 2.5, 1),
	}
	t := []mesh.Tri{
		// bottom (−z)
		tri(0, 2, 1), tri(0, 3, 2),
		// outer walls (full height)
		tri(0, 1, 13), tri(0, 13, 12),
		tri(14, 2, 3), tri(15, 14, 3),
		tri(0, 15, 3), tri(0, 12, 15),
		tri(1, 2, 14), tri(1, 14, 13),
		// recess floor (+z) at z=0.5 — the kept material a blind void has
		tri(8, 9, 10), tri(8, 10, 11),
		// top annulus (+z) at z=1
		tri(12, 13, 17), tri(12, 17, 16),
		tri(13, 14, 18), tri(13, 18, 17),
		tri(14, 15, 19), tri(14, 19, 18),
		tri(15, 12, 16), tri(15, 16, 19),
		// recess walls (normals into the recess, toward (2,2))
		tri(8, 11, 19), tri(8, 19, 16),
		tri(10, 9, 17), tri(10, 17, 18),
		tri(9, 8, 16), tri(9, 16, 17),
		tri(11, 10, 18), tri(11, 18, 19),
	}
	return mesh.Mesh{Vertices: v, Triangles: t}
}

func pocketStock(mn, mx kernel.Point3) stock.Stock {
	return stock.Stock{ID: "pocket-stock", Shape: stock.ShapeBox, Box: &stock.Box{Min: mn, Max: mx}}
}

func TestPlanTestmeshPocket(t *testing.T) {
	m := testmesh(t)
	pr := part.Part{ID: "testmesh", Name: "cylinder on base", Mesh: "testmesh.stl"}
	pr.Operation = part.OpPocket
	sz, cz := -10.0, -15.0
	pr.SliceZ = &sz
	pr.CutZ = &cz
	p := Params{
		Part:    pr,
		Mesh:    m,
		Stock:   pocketStock(kernel.P3(-50, -50, -20), kernel.P3(50, 50, 5)),
		Tool:    endmill10(),
		Machine: mill(),
		Feed:    800,
		Spindle: 6000,
		Coolant: moves.CoolantFlood,
	}
	out, err := Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// The Z model: slice -10 (face), cut -15, safe = stock top 5 + 5.
	if out.SliceZ != -10 || out.CutZ != -15 || out.SafeZ != 10 {
		t.Fatalf("Z model = slice %g / cut %g / safe %g, want -10 / -15 / 10", out.SliceZ, out.CutZ, out.SafeZ)
	}
	// Exactly one feature: the annulus pocket. The Boss is kept, never milled.
	if len(out.Features) != 1 {
		t.Fatalf("features = %d, want exactly 1 (the annulus pocket; the boss is kept)", len(out.Features))
	}
	fp := out.Features[0]
	if fp.Path.ID != "pocket-0" {
		t.Fatalf("path ID = %q, want pocket-0", fp.Path.ID)
	}
	first := fp.Path.Moves[0]
	if first.Kind != moves.KindRapid || first.From.Z != out.SafeZ || first.To.Z != out.CutZ {
		t.Fatalf("first move = %+v, want a plunge from safe %g to cut %g (ADR-020)", first, out.SafeZ, out.CutZ)
	}
	if !fp.Entry.Eq(P(first.From.X, first.From.Y), 1e-9) {
		t.Fatalf("entry %v does not match the plunge column %v", fp.Entry, P(first.From.X, first.From.Y))
	}

	// Constant chip-load feed on every cutting move (ADR-022): the default
	// stepover is the tool radius (5 mm).
	aE, err := toolpath.Engagement(5, 5)
	if err != nil {
		t.Fatal(err)
	}
	wantFeed, err := toolpath.ChipLoadFeed(800, aE, 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, mv := range fp.Path.Moves {
		if mv.Kind == moves.KindRapid {
			continue
		}
		if math.Abs(mv.Feed-wantFeed) > 1e-9 {
			t.Fatalf("cutting move %d feed = %g, want %g (F_full*sqrt(a_e/D))", i, mv.Feed, wantFeed)
		}
	}

	// The boss is cleared around, never cut (Law 7): the tool centre must
	// stay outside the boss keep-out on every cutting move and at every
	// plunge column. The model is the tessellated boss (the STL section —
	// a ~289-gon, not the ideal circle), and the planner's keep-out is its
	// mitred offset, whose boundary lies within the corner sagitta
	// 5·(1−cos(π/289)) ≈ 0.3 µm of the true Minkowski sum. We therefore
	// check the distance to the boss contour against the tool radius with
	// a 5 µm representation allowance (CNC tolerances are orders of
	// magnitude larger; an ideal-circle check at 1e-9 would reject a path
	// that is exactly safe for the model).
	feats, err := part.Detect(m, -10)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	var bossContour kernel.Chain2
	foundBoss := false
	for _, f := range feats {
		if f.Kind == planning.Boss {
			bossContour = f.Contour
			foundBoss = true
		}
	}
	if !foundBoss {
		t.Fatalf("no Boss feature in the testmesh section — the fixture changed?")
	}
	const toolR, bossTol = 5.0, 0.005
	clear := func(what string, q kernel.Point2) {
		if d := kernel.PointToChainDist(q, bossContour); d < toolR-bossTol {
			t.Fatalf("%s at %v is inside the boss keep-out (dist %g < %g)", what, q, d, toolR)
		}
	}
	for i, mv := range fp.Path.Moves {
		switch mv.Kind {
		case moves.KindLinear:
			clear(fmt.Sprintf("linear move %d", i), P(mv.From.X, mv.From.Y))
			clear(fmt.Sprintf("linear move %d", i), P(mv.To.X, mv.To.Y))
		case moves.KindArcCCW, moves.KindArcCW:
			for _, tt := range []float64{0.25, 0.5, 0.75} {
				clear(fmt.Sprintf("arc move %d", i), arcPointAt(mv, tt))
			}
		case moves.KindRapid:
			if mv.From.Z != mv.To.Z {
				clear(fmt.Sprintf("plunge %d", i), P(mv.From.X, mv.From.Y))
			}
		}
	}

	// Coverage (ADR-027): the two rows tangent to the boss keep-out
	// (y = ±17.5) are cut across the full width — the annulus is cleared
	// end to end. Each such row may be split by the boss contact into two
	// intervals (a lift between); their combined length still spans the
	// full 40 mm.
	span := map[float64]float64{}
	for _, mv := range fp.Path.Moves {
		if mv.Kind == moves.KindLinear && math.Abs(mv.From.Y) > 17 {
			span[mv.From.Y] += math.Abs(mv.From.X - mv.To.X)
		}
	}
	if len(span) != 2 {
		t.Fatalf("want the two tangent rows (y=±17.5), got %v", span)
	}
	for y, w := range span {
		if w < 39.8 {
			t.Fatalf("row y=%g spans %g mm, want the full 40 mm width", y, w)
		}
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("result is invalid: %v", err)
	}
}

// arcPointAt evaluates an arc move at fraction t of its sweep.
func arcPointAt(mv moves.Move, t float64) kernel.Point2 {
	c := P(mv.Center.X, mv.Center.Y)
	r := c.Dist(P(mv.From.X, mv.From.Y))
	a0 := math.Atan2(mv.From.Y-mv.Center.Y, mv.From.X-mv.Center.X)
	a1 := math.Atan2(mv.To.Y-mv.Center.Y, mv.To.X-mv.Center.X)
	var sweep float64
	if mv.CW {
		sweep = -kernel.NormalizeAngle(a0 - a1)
	} else {
		sweep = kernel.NormalizeAngle(a1 - a0)
	}
	theta := a0 + t*sweep
	return P(c.X+math.Cos(theta)*r, c.Y+math.Sin(theta)*r)
}

func TestPlanPocketBlindFloorKept(t *testing.T) {
	m := recessPlate()
	if err := m.Validate(); err != nil {
		t.Fatalf("validate recess plate: %v", err)
	}
	pr := part.Part{ID: "recess", Name: "plate with a blind recess", Mesh: "recess.stl"}
	pr.Operation = part.OpPocket
	sz, cz := 1.0, 0.25
	pr.SliceZ = &sz
	pr.CutZ = &cz
	p := Params{
		Part:    pr,
		Mesh:    m,
		Stock:   pocketStock(kernel.P3(-1, -1, 0), kernel.P3(5, 5, 1.5)),
		Tool:    smallEndmill(),
		Machine: mill(),
		Feed:    800,
		Spindle: 8000,
	}
	out, err := Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(out.Features) != 1 {
		t.Fatalf("features = %d, want 1 (the face pocket)", len(out.Features))
	}
	fp := out.Features[0]

	// The blind floor [1.5,2.5]², offset outside by R=0.25, is a keep-out
	// [1.25,2.75]² (ADR-028): no cutting move may cross its interior on a
	// row that overlaps it, and no plunge may land inside it.
	lo, hi := 1.25, 2.75
	for i, mv := range fp.Path.Moves {
		switch mv.Kind {
		case moves.KindLinear, moves.KindArcCCW, moves.KindArcCW:
			y := mv.From.Y
			if y > lo && y < hi {
				xlo, xhi := math.Min(mv.From.X, mv.To.X), math.Max(mv.From.X, mv.To.X)
				if xhi > lo+1e-9 && xlo < hi-1e-9 {
					t.Fatalf("cutting move %d at y=%g crosses the blind floor keep-out [%g, %g]", i, y, lo, hi)
				}
			}
		case moves.KindRapid:
			if mv.From.Z != mv.To.Z {
				if mv.From.X > lo+1e-9 && mv.From.X < hi-1e-9 && mv.From.Y > lo+1e-9 && mv.From.Y < hi-1e-9 {
					t.Fatalf("plunge %d at (%g, %g) is inside the blind floor keep-out", i, mv.From.X, mv.From.Y)
				}
			}
		}
	}
	// The face around the floor is still cleared: full-width rows exist
	// above and below the keep-out.
	full := 0
	for _, mv := range fp.Path.Moves {
		if mv.Kind == moves.KindLinear && math.Abs(mv.From.X-mv.To.X) > 3.49 {
			full++
		}
	}
	if full < 4 {
		t.Fatalf("want at least 4 full-width rows (around the keep-out), got %d", full)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("result is invalid: %v", err)
	}
}

func TestPlanPocketThroughHoleIsAir(t *testing.T) {
	m := tube2()
	if err := m.Validate(); err != nil {
		t.Fatalf("validate tube: %v", err)
	}
	pr := part.Part{ID: "tube", Name: "plate with a through-hole", Mesh: "tube.stl"}
	pr.Operation = part.OpPocket
	sz, cz := 1.0, 0.25
	pr.SliceZ = &sz
	pr.CutZ = &cz
	p := Params{
		Part:    pr,
		Mesh:    m,
		Stock:   pocketStock(kernel.P3(-1, -1, 0), kernel.P3(3, 3, 1.5)),
		Tool:    smallEndmill(),
		Machine: mill(),
		Feed:    800,
		Spindle: 8000,
	}
	out, err := Plan(p)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(out.Features) != 1 {
		t.Fatalf("features = %d, want 1 (the face pocket)", len(out.Features))
	}
	fp := out.Features[0]
	// The through-hole [0.5,1.5]² is air (an empty column, ADR-028): the
	// raster cuts across it — rows overlap the hole's x-span and are NOT
	// split by it (no lifts, no keep-out).
	across := 0
	for _, mv := range fp.Path.Moves {
		if mv.Kind == moves.KindLinear &&
			mv.From.Y > 0.5 && mv.From.Y < 1.5 &&
			math.Min(mv.From.X, mv.To.X) < 0.5 && math.Max(mv.From.X, mv.To.X) > 1.5 {
			across++
		}
	}
	if across < 3 {
		t.Fatalf("want at least 3 rows cut across the through-hole (it is air), got %d", across)
	}
	for _, d := range out.Diagnostics {
		if strings.Contains(d.Message, "lifted between intervals") {
			t.Fatalf("unexpected interval split on the through-hole: %v", d)
		}
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("result is invalid: %v", err)
	}
}

func TestPlanPocketRefusals(t *testing.T) {
	t.Run("band section refused", func(t *testing.T) {
		pr := part.Part{ID: "plate", Mesh: "plate.stl"}
		pr.Operation = part.OpPocket
		sz := 5.0 // strictly between the plate's vertex heights 0 and 10
		pr.SliceZ = &sz
		p := baseParams(t)
		p.Part = pr
		_, err := Plan(p)
		if err == nil || !strings.Contains(err.Error(), "face section") {
			t.Fatalf("want a refusal naming the face-section requirement, got %v", err)
		}
	})
	t.Run("chip-load feed below machine minimum refused", func(t *testing.T) {
		m := testmesh(t)
		pr := part.Part{ID: "testmesh", Mesh: "testmesh.stl"}
		pr.Operation = part.OpPocket
		sz, cz := -10.0, -15.0
		pr.SliceZ = &sz
		pr.CutZ = &cz
		p := Params{
			Part: pr, Mesh: m,
			Stock:   pocketStock(kernel.P3(-50, -50, -20), kernel.P3(50, 50, 5)),
			Tool:    endmill10(),
			Machine: mill(), // MinFeed 50
			Feed:    100,    // → chip feed ≈ 25.9 < 50
			Spindle: 6000,
		}
		_, err := Plan(p)
		if err == nil || !strings.Contains(err.Error(), "chip-load feed") {
			t.Fatalf("want a refusal naming the chip-load feed minimum, got %v", err)
		}
	})
	t.Run("stepover above 2R refused", func(t *testing.T) {
		m := testmesh(t)
		pr := part.Part{ID: "testmesh", Mesh: "testmesh.stl"}
		pr.Operation = part.OpPocket
		sz, cz := -10.0, -15.0
		pr.SliceZ = &sz
		pr.CutZ = &cz
		pr.Stepover = ptrF(11) // > 2 × R = 10
		p := Params{
			Part: pr, Mesh: m,
			Stock:   pocketStock(kernel.P3(-50, -50, -20), kernel.P3(50, 50, 5)),
			Tool:    endmill10(),
			Machine: mill(),
			Feed:    800,
			Spindle: 6000,
		}
		_, err := Plan(p)
		if err == nil || !strings.Contains(err.Error(), "stepover") {
			t.Fatalf("want a refusal naming the stepover, got %v", err)
		}
	})
}
