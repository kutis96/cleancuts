package stock

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// --- fixtures -----------------------------------------------------------

func boxStock() Stock {
	return Stock{
		ID:    "block",
		Shape: ShapeBox,
		Box: &Box{
			Min: kernel.P3(-10, -10, 0),
			Max: kernel.P3(110, 70, 20),
		},
	}
}

func cylStock() Stock {
	return Stock{
		ID:    "cyl",
		Shape: ShapeCylinder,
		Cylinder: &Cylinder{
			Center: kernel.P2(0, 0),
			Radius: 30,
			MinZ:   0,
			MaxZ:   15,
		},
	}
}

func prismStock() Stock {
	return Stock{
		ID:    "prism",
		Shape: ShapePrism,
		Prism: &Prism{
			Edges: []Edge{
				{Line: &LineEdge{Start: kernel.P2(10, 0), End: kernel.P2(70, 0)}},
				{Arc: &ArcEdge{Center: kernel.P2(70, 10), Start: kernel.P2(70, 0), End: kernel.P2(80, 10), CW: false}},
				{Line: &LineEdge{Start: kernel.P2(80, 10), End: kernel.P2(80, 40)}},
				{Arc: &ArcEdge{Center: kernel.P2(70, 40), Start: kernel.P2(80, 40), End: kernel.P2(70, 50), CW: false}},
				{Line: &LineEdge{Start: kernel.P2(70, 50), End: kernel.P2(10, 50)}},
				{Arc: &ArcEdge{Center: kernel.P2(10, 40), Start: kernel.P2(10, 50), End: kernel.P2(0, 40), CW: false}},
				{Line: &LineEdge{Start: kernel.P2(0, 40), End: kernel.P2(0, 10)}},
				{Arc: &ArcEdge{Center: kernel.P2(10, 10), Start: kernel.P2(0, 10), End: kernel.P2(10, 0), CW: false}},
			},
			MinZ: 0,
			MaxZ: 12,
		},
	}
}

// --- Validate -----------------------------------------------------------

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		stock   Stock
		wantErr bool
		msgPart string // expected substring (only when wantErr)
	}{
		{"box ok", boxStock(), false, ""},
		{"cylinder ok", cylStock(), false, ""},
		{"prism ok", prismStock(), false, ""},
		{"missing id", Stock{Shape: ShapeBox, Box: &Box{Min: kernel.P3(0, 0, 0), Max: kernel.P3(1, 1, 1)}}, true, "id is required"},
		{"no geometry", Stock{ID: "s", Shape: ShapeBox}, true, "exactly one"},
		{"two geometries", Stock{ID: "s", Shape: ShapeBox, Box: &Box{Min: kernel.P3(0, 0, 0), Max: kernel.P3(1, 1, 1)},
			Cylinder: &Cylinder{Radius: 5, MinZ: 0, MaxZ: 1}}, true, "exactly one"},
		{"shape mismatch", Stock{ID: "s", Shape: ShapeCylinder, Box: &Box{Min: kernel.P3(0, 0, 0), Max: kernel.P3(1, 1, 1)}}, true, "shape is"},
		{"box min==max z", Stock{ID: "s", Shape: ShapeBox, Box: &Box{Min: kernel.P3(0, 0, 5), Max: kernel.P3(1, 1, 5)}}, true, "z min"},
		{"box min>max x", Stock{ID: "s", Shape: ShapeBox, Box: &Box{Min: kernel.P3(5, 0, 0), Max: kernel.P3(1, 1, 1)}}, true, "x min"},
		{"box nan corner", Stock{ID: "s", Shape: ShapeBox, Box: &Box{Min: kernel.P3(math.NaN(), 0, 0), Max: kernel.P3(1, 1, 1)}}, true, "finite"},
		{"cylinder zero radius", Stock{ID: "s", Shape: ShapeCylinder, Cylinder: &Cylinder{Radius: 0, MinZ: 0, MaxZ: 1}}, true, "radius"},
		{"cylinder inverted z", Stock{ID: "s", Shape: ShapeCylinder, Cylinder: &Cylinder{Radius: 5, MinZ: 2, MaxZ: 1}}, true, "min_z"},
		{"prism too few edges", Stock{ID: "s", Shape: ShapePrism, Prism: &Prism{
			Edges: []Edge{{Line: &LineEdge{Start: kernel.P2(0, 0), End: kernel.P2(1, 0)}}}, MinZ: 0, MaxZ: 1}}, true, "at least 3"},
		{"prism two edges", Stock{ID: "s", Shape: ShapePrism, Prism: &Prism{
			Edges: []Edge{
				{Line: &LineEdge{Start: kernel.P2(0, 0), End: kernel.P2(1, 0)}},
				{Line: &LineEdge{Start: kernel.P2(1, 0), End: kernel.P2(1, 1)}},
			}, MinZ: 0, MaxZ: 1}}, true, "at least 3"},
		{"prism edge with both line and arc", Stock{ID: "s", Shape: ShapePrism, Prism: &Prism{
			Edges: []Edge{
				{Line: &LineEdge{Start: kernel.P2(0, 0), End: kernel.P2(1, 0)},
					Arc: &ArcEdge{Center: kernel.P2(0, 0), Start: kernel.P2(1, 0), End: kernel.P2(0, 1)}},
				{Line: &LineEdge{Start: kernel.P2(1, 1), End: kernel.P2(0, 1)}},
				{Line: &LineEdge{Start: kernel.P2(0, 1), End: kernel.P2(0, 0)}},
			}, MinZ: 0, MaxZ: 1}}, true, "exactly one"},
		{"prism arc off circle", Stock{ID: "s", Shape: ShapePrism, Prism: &Prism{
			Edges: []Edge{
				{Arc: &ArcEdge{Center: kernel.P2(0, 0), Start: kernel.P2(1, 0), End: kernel.P2(2, 0)}}, // end not on r=1
				{Line: &LineEdge{Start: kernel.P2(2, 0), End: kernel.P2(0, 0)}},
				{Line: &LineEdge{Start: kernel.P2(0, 0), End: kernel.P2(2, 0)}},
			}, MinZ: 0, MaxZ: 1}}, true, "equidistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.stock.Validate()
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

// --- SectionAt ----------------------------------------------------------

func TestSectionAtBox(t *testing.T) {
	s := boxStock()
	c, err := s.SectionAt(10)
	if err != nil {
		t.Fatalf("SectionAt(10): %v", err)
	}
	if !c.IsClosed() {
		t.Fatalf("box section is not closed")
	}
	if len(c.Edges) != 4 {
		t.Fatalf("box section has %d edges, want 4", len(c.Edges))
	}
	want := 120.0 * 80.0
	if got := c.SignedArea(); got != want {
		t.Errorf("box section area = %g, want %g", got, want)
	}
	if c.SignedArea() <= 0 {
		t.Errorf("box section should be CCW (positive area), got %g", c.SignedArea())
	}
}

func TestSectionAtCylinder(t *testing.T) {
	s := cylStock()
	c, err := s.SectionAt(7.5)
	if err != nil {
		t.Fatalf("SectionAt(7.5): %v", err)
	}
	if len(c.Edges) != 2 {
		t.Fatalf("cylinder section has %d edges, want 2 half-arcs", len(c.Edges))
	}
	for _, e := range c.Edges {
		if !e.IsArc() {
			t.Fatalf("cylinder section edge is not an arc: %v", e)
		}
	}
	if got, want := c.SignedArea(), math.Pi*30*30; math.Abs(got-want) > 1e-6 {
		t.Errorf("circle area = %g, want %g (π·30²)", got, want)
	}
}

func TestSectionAtPrism(t *testing.T) {
	s := prismStock()
	c, err := s.SectionAt(6)
	if err != nil {
		t.Fatalf("SectionAt(6): %v", err)
	}
	if !c.IsClosed() {
		t.Fatalf("prism section is not closed")
	}
	// Rounded rectangle area = 80·50 − 4·(10² − π·10²/4).
	want := 80*50 - 4*(100-math.Pi*100/4)
	if got := c.SignedArea(); math.Abs(got-want) > 1e-9 {
		t.Errorf("prism section area = %g, want %g", got, want)
	}
	if c.SignedArea() <= 0 {
		t.Errorf("prism section should be CCW, got area %g", c.SignedArea())
	}
}

func TestSectionAtPrismNormalisesClockwiseInput(t *testing.T) {
	s := prismStock()
	// Feed the same footprint clockwise.
	s.Prism.Edges = reversedEdges(s.Prism.Edges)
	if err := s.Validate(); err != nil {
		t.Fatalf("reversed prism should still validate: %v", err)
	}
	c, err := s.SectionAt(6)
	if err != nil {
		t.Fatalf("SectionAt: %v", err)
	}
	if c.SignedArea() <= 0 {
		t.Errorf("SectionAt must normalise to CCW, got area %g", c.SignedArea())
	}
}

func reversedEdges(in []Edge) []Edge {
	out := make([]Edge, len(in))
	for i, e := range in {
		j := len(in) - 1 - i
		if e.Line != nil {
			out[j] = Edge{Line: &LineEdge{Start: e.Line.End, End: e.Line.Start}}
		} else {
			out[j] = Edge{Arc: &ArcEdge{Center: e.Arc.Center, Start: e.Arc.End, End: e.Arc.Start, CW: !e.Arc.CW}}
		}
	}
	return out
}

func TestSectionAtOutsideRange(t *testing.T) {
	s := boxStock()
	for _, z := range []float64{-1e-6, -1, 20 + 1e-6, 21} {
		if _, err := s.SectionAt(z); err == nil {
			t.Errorf("SectionAt(%g) should fail (stock spans 0..20)", z)
		}
	}
}

func TestSectionAtInclusiveAtBoundaries(t *testing.T) {
	s := boxStock()
	for _, z := range []float64{0, 20} {
		if _, err := s.SectionAt(z); err != nil {
			t.Errorf("SectionAt(%g) at the boundary should succeed, got %v", z, err)
		}
	}
}

// --- TopAt --------------------------------------------------------------

func TestTopAt(t *testing.T) {
	cases := []struct {
		name    string
		stock   Stock
		x, y    float64
		want    float64
		wantErr bool
	}{
		{"box inside", boxStock(), 50, 30, 20, false},
		{"box on boundary", boxStock(), 110, 0, 20, false},
		{"box outside", boxStock(), 200, 0, 0, true},
		{"cylinder inside", cylStock(), 10, 0, 15, false},
		{"cylinder on rim", cylStock(), 30, 0, 15, false},
		{"cylinder outside", cylStock(), 31, 0, 0, true},
		{"prism inside", prismStock(), 40, 25, 12, false},
		{"prism outside", prismStock(), 85, 25, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.stock.TopAt(tc.x, tc.y)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got top=%g", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("TopAt(%g,%g): %v", tc.x, tc.y, err)
			}
			if got != tc.want {
				t.Errorf("TopAt = %g, want %g", got, tc.want)
			}
		})
	}
}

// --- Contains -----------------------------------------------------------

func TestContains(t *testing.T) {
	s := boxStock()
	cases := []struct {
		name string
		p    kernel.Point3
		want bool
	}{
		{"inside", kernel.P3(50, 30, 10), true},
		{"on boundary", kernel.P3(110, 0, 0), true},
		{"outside xy", kernel.P3(120, 0, 10), false},
		{"below z", kernel.P3(50, 30, -1), false},
		{"above z", kernel.P3(50, 30, 21), false},
		{"nan point", kernel.P3(math.NaN(), 0, 0), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Contains(tc.p); got != tc.want {
				t.Errorf("Contains(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
	// An invalid stock contains nothing, not even a point that would fit.
	bad := Stock{ID: "bad", Shape: ShapeBox}
	if bad.Contains(kernel.P3(0, 0, 0)) {
		t.Errorf("an invalid stock must contain nothing")
	}
}

// --- Fits ---------------------------------------------------------------

func rectContour(x0, y0, x1, y1 float64) kernel.Chain2 {
	pts := []kernel.Point2{
		kernel.P2(x0, y0), kernel.P2(x1, y0), kernel.P2(x1, y1), kernel.P2(x0, y1),
	}
	edges := make([]kernel.Edge2, len(pts))
	for i, p := range pts {
		e, err := kernel.NewLineEdge(p, pts[(i+1)%len(pts)])
		if err != nil {
			panic(err)
		}
		edges[i] = e
	}
	c, err := kernel.NewChain2(edges)
	if err != nil {
		panic(err)
	}
	return c
}

func TestFits(t *testing.T) {
	s := boxStock() // x: -10..110, y: -10..70, z: 0..20
	cases := []struct {
		name    string
		part    kernel.Chain2
		partZ   float64
		partTop float64
		wantErr bool
		msgPart string
	}{
		{"inside", rectContour(0, 0, 100, 60), 5, 15, false, ""},
		{"flush with stock", rectContour(-10, -10, 110, 70), 0, 20, false, ""},
		{"vertex outside", rectContour(0, 0, 120, 60), 5, 15, true, "outside the stock section"},
		{"below stock", rectContour(0, 0, 100, 60), -1, 15, true, "below the stock"},
		{"above stock", rectContour(0, 0, 100, 60), 25, 30, true, "above the stock"},
		{"part top above stock top", rectContour(0, 0, 100, 60), 5, 25, true, "above the stock top"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.Fits(tc.part, tc.partZ, tc.partTop)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.msgPart) {
					t.Fatalf("error %q does not mention %q", err, tc.msgPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFitsArcBulgeCaughtByMidpoint(t *testing.T) {
	// A part contour whose vertices all lie inside the stock box (x ∈ [-10, 110])
	// but whose arc bulges past the box's right wall (x = 110): the midpoint
	// check must catch what a vertex-only check would miss.
	//
	// Geometry: endpoints (100, -5) and (100, 25), both inside the box, with a
	// CCW arc about centre (95, 10). Both endpoints are exactly √(5² + 15²)
	// from the centre; the arc's rightmost point is (95 + √250, 10) ≈
	// (110.81, 10) — outside the box, and the arc's midpoint is that point.
	edges := []kernel.Edge2{}
	line := func(a, b kernel.Point2) {
		e, err := kernel.NewLineEdge(a, b)
		if err != nil {
			panic(err)
		}
		edges = append(edges, e)
	}
	line(kernel.P2(0, 10), kernel.P2(100, -5))
	arc, err := kernel.NewArcEdge(kernel.P2(95, 10), kernel.P2(100, -5), kernel.P2(100, 25), false)
	if err != nil {
		panic(err)
	}
	edges = append(edges, arc)
	line(kernel.P2(100, 25), kernel.P2(0, 30))
	line(kernel.P2(0, 30), kernel.P2(0, 10))
	part, err := kernel.NewChain2(edges)
	if err != nil {
		panic(err)
	}
	// Vertex-only check would pass: all four vertices are inside the box.
	s := boxStock()
	if err := s.Fits(part, 10, 20); err == nil {
		t.Fatalf("Fits should reject an arc bulging outside the stock (rightmost point x=%g > 110)", 95+math.Sqrt(250))
	}
}

func TestFitsOpenPartRejected(t *testing.T) {
	e1, _ := kernel.NewLineEdge(kernel.P2(0, 0), kernel.P2(10, 0))
	e2, _ := kernel.NewLineEdge(kernel.P2(10, 0), kernel.P2(10, 10))
	open, err := kernel.NewChain2([]kernel.Edge2{e1, e2})
	if err != nil {
		panic(err)
	}
	if err := boxStock().Fits(open, 5, 15); err == nil {
		t.Fatalf("Fits must reject an open part contour")
	}
}

// --- JSON round-trip ----------------------------------------------------

func TestJSONRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stock Stock
	}{
		{"box", boxStock()},
		{"cylinder", cylStock()},
		{"prism", prismStock()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.stock)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back Stock
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := back.Validate(); err != nil {
				t.Fatalf("round-tripped stock fails validation: %v", err)
			}
			if !reflect.DeepEqual(tc.stock, back) {
				t.Errorf("round trip differs:\n in: %+v\nout: %+v", tc.stock, back)
			}
		})
	}
}

func TestJSONPointKeysAreLowercase(t *testing.T) {
	// ADR-016: the Go JSON must match the schema spelling {x, y, z}.
	data, err := json.Marshal(boxStock())
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"x":-10`) {
		t.Errorf("expected lowercase x key, got: %s", s)
	}
	if strings.Contains(s, `"X":`) {
		t.Errorf("uppercase X key leaked into JSON: %s", s)
	}
}
