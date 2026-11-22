package mesh

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// --- fixtures -----------------------------------------------------------

// cube is the unit cube [0,1]³ with consistent outward winding (12 tris).
func cube() Mesh {
	v := []kernel.Point3{
		kernel.P3(0, 0, 0), kernel.P3(1, 0, 0), kernel.P3(1, 1, 0), kernel.P3(0, 1, 0),
		kernel.P3(0, 0, 1), kernel.P3(1, 0, 1), kernel.P3(1, 1, 1), kernel.P3(0, 1, 1),
	}
	t := []Tri{
		{0, 2, 1}, {0, 3, 2}, // bottom (-z)
		{4, 5, 6}, {4, 6, 7}, // top (+z)
		{0, 1, 5}, {0, 5, 4}, // y=0 (-y)
		{2, 3, 7}, {2, 7, 6}, // y=1 (+y)
		{0, 7, 3}, {0, 4, 7}, // x=0 (-x)
		{1, 2, 6}, {1, 6, 5}, // x=1 (+x)
	}
	return Mesh{Vertices: v, Triangles: t}
}

// tetra is the tetrahedron on (0,0,0),(1,0,0),(0,1,0),(0,0,1), outward.
func tetra() Mesh {
	return Mesh{
		Vertices: []kernel.Point3{
			kernel.P3(0, 0, 0), kernel.P3(1, 0, 0), kernel.P3(0, 1, 0), kernel.P3(0, 0, 1),
		},
		Triangles: []Tri{{0, 2, 1}, {0, 3, 2}, {0, 1, 3}, {1, 2, 3}},
	}
}

// tube is a 2×2×1 plate with a 1×1 through-hole: outer box [0,2]²,
// inner box [0.5,1.5]², z in [0,1] — 32 triangles, consistent winding.
func tube() Mesh {
	v := []kernel.Point3{
		kernel.P3(0, 0, 0), kernel.P3(2, 0, 0), kernel.P3(2, 2, 0), kernel.P3(0, 2, 0),
		kernel.P3(0, 0, 1), kernel.P3(2, 0, 1), kernel.P3(2, 2, 1), kernel.P3(0, 2, 1),
		kernel.P3(0.5, 0.5, 0), kernel.P3(1.5, 0.5, 0), kernel.P3(1.5, 1.5, 0), kernel.P3(0.5, 1.5, 0),
		kernel.P3(0.5, 0.5, 1), kernel.P3(1.5, 0.5, 1), kernel.P3(1.5, 1.5, 1), kernel.P3(0.5, 1.5, 1),
	}
	// Winding verified programmatically: closed 2-manifold, every normal
	// points material→void (a void's walls point INTO the void), and the
	// z=0.5 slice yields opposite-oriented loops (outer −4, hole +1).
	t := []Tri{
		// outer walls
		{0, 1, 5}, {0, 5, 4}, // y=0 → −y
		{6, 2, 3}, {7, 6, 3}, // y=2 → +y
		{7, 3, 0}, {4, 7, 0}, // x=0 → −x
		{1, 2, 6}, {1, 6, 5}, // x=2 → +x
		// inner walls (toward the hole)
		{8, 11, 15}, {8, 15, 12}, // x=0.5 → +x
		{14, 10, 9}, {13, 14, 9}, // x=1.5 → −x
		{13, 9, 8}, {12, 13, 8}, // y=0.5 → +y
		{11, 10, 14}, {11, 14, 15}, // y=1.5 → −y
		// top annulus (+z)
		{4, 5, 13}, {4, 13, 12}, {5, 6, 14}, {5, 14, 13},
		{6, 7, 15}, {6, 15, 14}, {7, 4, 12}, {7, 12, 15},
		// bottom annulus (-z)
		{9, 1, 0}, {8, 9, 0}, {10, 2, 1}, {9, 10, 1},
		{11, 3, 2}, {10, 11, 2}, {8, 0, 3}, {11, 8, 3},
	}
	return Mesh{Vertices: v, Triangles: t}
}

// book is two open plates sharing one vertical edge — not a solid: a slice
// through it leaves dangling crossing points (the non-manifold case).
func book() Mesh {
	return Mesh{
		Vertices: []kernel.Point3{
			kernel.P3(0, 0, 0), kernel.P3(0, 1, 0), kernel.P3(0, 1, 1), kernel.P3(0, 0, 1),
			kernel.P3(1, 0, 0), kernel.P3(1, 0, 1),
		},
		Triangles: []Tri{
			{0, 1, 3}, {1, 2, 3}, // plate A in the x=0 plane
			{0, 4, 5}, {0, 5, 3}, // plate B in the y=0 plane
		},
	}
}

// --- Validate / diagnostics --------------------------------------------

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		m       Mesh
		wantErr bool
		msgPart string
	}{
		{"cube ok", cube(), false, ""},
		{"tetra ok", tetra(), false, ""},
		{"tube ok", tube(), false, ""},
		{"no vertices", Mesh{Triangles: []Tri{{0, 1, 2}}}, true, "at least one vertex"},
		{"no triangles", Mesh{Vertices: []kernel.Point3{kernel.P3(0, 0, 0)}}, true, "at least one triangle"},
		{"index out of range", Mesh{
			Vertices:  []kernel.Point3{kernel.P3(0, 0, 0), kernel.P3(1, 0, 0)},
			Triangles: []Tri{{0, 1, 5}},
		}, true, "out of range"},
		{"repeated index", Mesh{
			Vertices:  []kernel.Point3{kernel.P3(0, 0, 0), kernel.P3(1, 0, 0), kernel.P3(0, 1, 0)},
			Triangles: []Tri{{0, 0, 1}},
		}, true, "repeats a vertex"},
		{"degenerate triangle", Mesh{
			Vertices: []kernel.Point3{
				kernel.P3(0, 0, 0), kernel.P3(1, 0, 0), kernel.P3(2, 0, 0), kernel.P3(0, 1, 0),
			},
			Triangles: []Tri{{0, 1, 2}},
		}, true, "degenerate"},
		{"nan vertex", Mesh{
			Vertices:  []kernel.Point3{kernel.P3(math.NaN(), 0, 0), kernel.P3(1, 0, 0), kernel.P3(0, 1, 0)},
			Triangles: []Tri{{0, 1, 2}},
		}, true, "not finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), tc.msgPart) {
				t.Fatalf("error %q does not mention %q", err, tc.msgPart)
			}
		})
	}
}

func TestVertexZs(t *testing.T) {
	zs := cube().VertexZs()
	if len(zs) != 2 || zs[0] != 0 || zs[1] != 1 {
		t.Fatalf("VertexZs = %v, want [0 1]", zs)
	}
}

func TestBounds(t *testing.T) {
	b := tube().Bounds()
	if b.Min != (kernel.Point3{}) {
		t.Errorf("Min = %v, want (0,0,0)", b.Min)
	}
	if b.Max != kernel.P3(2, 2, 1) {
		t.Errorf("Max = %v, want (2,2,1)", b.Max)
	}
}

func TestIsClosedMesh(t *testing.T) {
	if !cube().IsClosedMesh() {
		t.Errorf("a cube is a closed 2-manifold")
	}
	if !tube().IsClosedMesh() {
		t.Errorf("a tube is a closed 2-manifold")
	}
	open := cube()
	open.Triangles = open.Triangles[:len(open.Triangles)-1] // remove one face
	if open.IsClosedMesh() {
		t.Errorf("a cube missing a face must not be closed")
	}
	if book().IsClosedMesh() {
		t.Errorf("two plates sharing an edge must not be closed")
	}
}

// --- Slice --------------------------------------------------------------

func TestSliceCube(t *testing.T) {
	loops, err := cube().Slice(0.5)
	if err != nil {
		t.Fatalf("Slice(0.5): %v", err)
	}
	if len(loops) != 1 {
		t.Fatalf("got %d loops, want 1", len(loops))
	}
	loop := loops[0]
	if !loop.IsClosed() {
		t.Fatalf("loop is not closed")
	}
	// The section is a 1×1 square; the two triangles per wall split each
	// side at its midpoint, so the loop has 8 (collinear) points.
	if len(loop.Edges) != 8 {
		t.Errorf("loop has %d edges, want 8 (4 corners + 4 edge midpoints)", len(loop.Edges))
	}
	if got := math.Abs(loop.SignedArea()); math.Abs(got-1.0) > 1e-12 {
		t.Errorf("section area = %g, want 1", got)
	}
}

func TestSliceTetrahedron(t *testing.T) {
	// Cross-section at z=h of the unit tetrahedron is a right triangle of
	// legs (1−h): area (1−h)²/2.
	for h, want := range map[float64]float64{0.25: 0.5625 / 2, 0.5: 0.25 / 2, 0.75: 0.0625 / 2} {
		loops, err := tetra().Slice(h)
		if err != nil {
			t.Fatalf("Slice(%g): %v", h, err)
		}
		if len(loops) != 1 {
			t.Fatalf("Slice(%g): got %d loops, want 1", h, len(loops))
		}
		if got := math.Abs(loops[0].SignedArea()); math.Abs(got-want) > 1e-12 {
			t.Errorf("Slice(%g) area = %g, want %g", h, got, want)
		}
	}
}

func TestSliceTubeTwoLoopsOppositeOrientation(t *testing.T) {
	loops, err := tube().Slice(0.5)
	if err != nil {
		t.Fatalf("Slice(0.5): %v", err)
	}
	if len(loops) != 2 {
		t.Fatalf("got %d loops, want 2 (outer + hole)", len(loops))
	}
	var outer, hole float64
	for _, l := range loops {
		a := l.SignedArea()
		if a > 0 {
			outer += a
		} else {
			hole += a
		}
	}
	// Outer 2×2 = 4, hole 1×1 = 1 (in magnitude). Which one is positive
	// depends on the solid's winding; the invariant for a consistent solid
	// is that the two loops run opposite ways — the even-odd convention
	// downstream (PointInContour, fill rules).
	mags := []float64{math.Abs(outer), math.Abs(hole)}
	if math.Abs(mags[0]-1) > 1e-12 && math.Abs(mags[1]-1) > 1e-12 {
		// one must be the 1×1 hole, the other the 2×2 outer
		t.Errorf("loop area magnitudes = %v, want one of 1 and one of 4", mags)
	}
	if math.Abs(mags[0]-4) > 1e-12 && math.Abs(mags[1]-4) > 1e-12 {
		t.Errorf("loop area magnitudes = %v, want one of 1 and one of 4", mags)
	}
	if (outer > 0) == (hole > 0) {
		t.Errorf("outer and hole loops must have opposite orientation (got %g, %g)", outer, hole)
	}
}

func TestSliceAtVertexLevelRefused(t *testing.T) {
	_, err := cube().Slice(0)
	if err == nil {
		t.Fatalf("slicing exactly at a vertex level must be refused")
	}
	msg := err.Error()
	for _, part := range []string{"passes through vertex", "z=0.5", "0 and 1"} {
		if !strings.Contains(msg, part) {
			t.Errorf("error should mention %q: %s", part, msg)
		}
	}
}

func TestSliceAboveSolidRefused(t *testing.T) {
	_, err := cube().Slice(5)
	if err == nil {
		t.Fatalf("slicing above the solid must be refused")
	}
	if !strings.Contains(err.Error(), "does not intersect") || !strings.Contains(err.Error(), "[0 1]") {
		t.Errorf("error should name the miss and the available levels: %v", err)
	}
}

// TestExamplePartParsesAndSlices pins the shipped example part to the
// parser and slicer: it is a 60×40×10 plate with a 20×20 centred
// through-hole, so a slice at z=5 must give a 2400 loop and a 400 loop in
// opposite orientations.
func TestExamplePartParsesAndSlices(t *testing.T) {
	data, err := os.ReadFile("../../examples/parts/plate_60x40x10_hole20.stl")
	if err != nil {
		t.Fatalf("reading example part: %v", err)
	}
	m, err := ParseSTL(data)
	if err != nil {
		t.Fatalf("ParseSTL(example): %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("example part fails validation: %v", err)
	}
	if !m.IsClosedMesh() {
		t.Fatalf("example part must be a closed 2-manifold")
	}
	loops, err := m.Slice(5)
	if err != nil {
		t.Fatalf("Slice(5): %v", err)
	}
	if len(loops) != 2 {
		t.Fatalf("got %d loops, want 2 (outer + hole)", len(loops))
	}
	var mags []float64
	for _, l := range loops {
		mags = append(mags, math.Abs(l.SignedArea()))
	}
	if !(math.Abs(mags[0]-2400) < 1e-9 && math.Abs(mags[1]-400) < 1e-9) &&
		!(math.Abs(mags[0]-400) < 1e-9 && math.Abs(mags[1]-2400) < 1e-9) {
		t.Errorf("loop areas = %v, want 2400 and 400", mags)
	}
}

func TestSliceNonManifoldRefused(t *testing.T) {
	_, err := book().Slice(0.5)
	if err == nil {
		t.Fatalf("slicing a non-manifold intersection must be refused")
	}
	// The book has a point joining 1 segment and one joining 4; the map
	// order decides which is reported first — both are the right refusal.
	if !strings.Contains(err.Error(), "segments (want 2)") {
		t.Errorf("error should name the dangling point and count: %v", err)
	}
}

// --- FaceLoops (the face section, ADR-025) ------------------------------

func TestFaceLoopsCubeTop(t *testing.T) {
	loops, err := cube().FaceLoops(1)
	if err != nil {
		t.Fatalf("FaceLoops(1): %v", err)
	}
	if len(loops) != 1 {
		t.Fatalf("got %d loops, want 1 (the top face)", len(loops))
	}
	if !loops[0].IsClosed() {
		t.Fatalf("loop is not closed")
	}
	// |area| = 1 (1×1 face). Orientation is deterministic (the total-order
	// walk) but a consumer detail: pin the magnitude, read the sign via
	// SignedArea like everything else downstream does.
	if got := math.Abs(loops[0].SignedArea()); math.Abs(got-1.0) > 1e-12 {
		t.Errorf("top face |area| = %g, want 1", got)
	}
}

func TestFaceLoopsCubeBottomRefused(t *testing.T) {
	_, err := cube().FaceLoops(0)
	if err == nil {
		t.Fatalf("a downward-only face must be refused")
	}
	if !strings.Contains(err.Error(), "no upward-facing face") {
		t.Errorf("error should name the missing upward face: %v", err)
	}
}

func TestFaceLoopsTubeTopTwoLoops(t *testing.T) {
	// The tube's top face is an annulus: the outer boundary (2×2 = 4) and
	// the hole boundary (1×1 = 1), as two separate loops (ADR-025).
	loops, err := tube().FaceLoops(1)
	if err != nil {
		t.Fatalf("FaceLoops(1): %v", err)
	}
	if len(loops) != 2 {
		t.Fatalf("got %d loops, want 2 (outer + hole)", len(loops))
	}
	mags := []float64{math.Abs(loops[0].SignedArea()), math.Abs(loops[1].SignedArea())}
	if !(math.Abs(mags[0]-4) < 1e-12 && math.Abs(mags[1]-1) < 1e-12) &&
		!(math.Abs(mags[0]-1) < 1e-12 && math.Abs(mags[1]-4) < 1e-12) {
		t.Errorf("loop |areas| = %v, want 4 (2×2 outer) and 1 (1×1 hole)", mags)
	}
	// Deterministic orientation: run twice, the signed areas must agree
	// (the total-order walk — no map-order leakage).
	again, err := tube().FaceLoops(1)
	if err != nil {
		t.Fatalf("FaceLoops(1) again: %v", err)
	}
	sig := func(loops []kernel.Chain2) string {
		s := ""
		for _, l := range loops {
			if l.SignedArea() > 0 {
				s += "+"
			} else {
				s += "-"
			}
		}
		return s
	}
	if sig(loops) != sig(again) {
		t.Errorf("orientation not deterministic: %s vs %s", sig(loops), sig(again))
	}
}

func TestFaceLoopsTestmesh(t *testing.T) {
	data, err := os.ReadFile("../../examples/parts/testmesh_cyl_on_base.stl")
	if err != nil {
		t.Fatalf("reading testmesh: %v", err)
	}
	m, err := ParseSTL(data)
	if err != nil {
		t.Fatalf("ParseSTL(testmesh): %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("testmesh fails validation: %v", err)
	}

	// Base top (z=-10): the 50×50 square + the Ø25 cylinder circle
	// (a tessellated 289-gon), opposite orientation.
	loops, err := m.FaceLoops(-10)
	if err != nil {
		t.Fatalf("FaceLoops(-10): %v", err)
	}
	if len(loops) != 2 {
		t.Fatalf("got %d loops, want 2 (square + circle)", len(loops))
	}
	areas := []float64{math.Abs(loops[0].SignedArea()), math.Abs(loops[1].SignedArea())}
	if !(math.Abs(areas[0]-2500) < 1e-9 && math.Abs(areas[1]-490.87) < 1.0) &&
		!(math.Abs(areas[0]-490.87) < 1.0 && math.Abs(areas[1]-2500) < 1e-9) {
		t.Errorf("loop areas = %v, want 2500 (50×50) and ≈ 490.87 (Ø25 circle)", areas)
	}
	// The circle loop is a polygon: many edges, every vertex at radius
	// ≈ 12.5 from the origin (the cylinder is centred).
	var circle kernel.Chain2
	for _, l := range loops {
		if len(l.Edges) > 50 {
			circle = l
		}
	}
	if circle.Edges == nil {
		t.Fatalf("no circle-like loop found (edges: %d, %d)", len(loops[0].Edges), len(loops[1].Edges))
	}
	for _, e := range circle.Edges {
		l, _ := e.AsLine()
		r := math.Hypot(l.Start.X, l.Start.Y)
		if math.Abs(r-12.5) > 0.01 {
			t.Errorf("circle vertex (%g,%g) at radius %g, want ≈ 12.5", l.Start.X, l.Start.Y, r)
			break
		}
	}

	// Cylinder top (z=0): a single loop, the Ø25 circle.
	top, err := m.FaceLoops(0)
	if err != nil {
		t.Fatalf("FaceLoops(0): %v", err)
	}
	if len(top) != 1 || math.Abs(math.Abs(top[0].SignedArea())-490.87) > 1.0 {
		t.Fatalf("cylinder top: want one loop of area ≈ 490.87, got %d loops", len(top))
	}

	// Base bottom (z=-20): downward-only — refused.
	if _, err := m.FaceLoops(-20); err == nil || !strings.Contains(err.Error(), "no upward-facing face") {
		t.Errorf("base bottom must be refused with the no-upward-face error, got %v", err)
	}
}

func TestFaceLoopsPinchPointRefused(t *testing.T) {
	// A figure-eight face: two +Z triangles sharing only the vertex A.
	// A joins four boundary edges — a pinch point, refused rather than
	// guessed (a figure-eight "loop" is not a valid contour).
	m := Mesh{
		Vertices: []kernel.Point3{
			kernel.P3(0, 0, 0), kernel.P3(1, 0, 0), kernel.P3(0.5, 0.8, 0),
			kernel.P3(-1, 0, 0), kernel.P3(-0.5, 0.8, 0),
		},
		Triangles: []Tri{
			{A: 0, B: 1, C: 2}, {A: 0, B: 4, C: 3},
		},
	}
	_, err := m.FaceLoops(0)
	if err == nil {
		t.Fatalf("a non-manifold face boundary must be refused")
	}
	if !strings.Contains(err.Error(), "boundary edges (want 2)") {
		t.Errorf("error should name the offending boundary vertex degree: %v", err)
	}
}

func TestFaceLoopsEdgeOwnedByThreeRefused(t *testing.T) {
	// Three +Z triangles sharing the edge (A,B): the edge is owned by
	// three face triangles — non-manifold, refused.
	m := Mesh{
		Vertices: []kernel.Point3{
			kernel.P3(0, 0, 0), kernel.P3(1, 0, 0), kernel.P3(0, 1, 0),
			kernel.P3(2, 1, 0), kernel.P3(1, 2, 0),
		},
		Triangles: []Tri{
			{A: 0, B: 1, C: 2}, {A: 0, B: 1, C: 3}, {A: 0, B: 1, C: 4},
		},
	}
	_, err := m.FaceLoops(0)
	if err == nil {
		t.Fatalf("an edge owned by three face triangles must be refused")
	}
	if !strings.Contains(err.Error(), "belongs to 3 face triangles") {
		t.Errorf("error should name the triple-owned edge: %v", err)
	}
}
