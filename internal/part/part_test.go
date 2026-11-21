package part

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/mesh"
	"github.com/kutis96/cleancuts/internal/planning"
)

// --- fixtures -----------------------------------------------------------

// plateMesh loads the pinned example part: a 60×40×10 plate with a 20×20
// centred through-hole (the mesh package pins this file's section).
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

// P is shorthand for kernel.P2 in fixtures.
func P(x, y float64) kernel.Point2 { return kernel.P2(x, y) }

// tri is a keyed triangle (go vet rejects unkeyed literals of foreign
// structs, and the keyed form is more readable).
func tri(a, b, c int) mesh.Tri { return mesh.Tri{A: a, B: b, C: c} }

// squareCCW returns the four corners of a centred square, CCW from above.
func squareCCW(half float64) []kernel.Point2 {
	return []kernel.Point2{P(half, half), P(-half, half), P(-half, -half), P(half, -half)}
}

// ringMesh builds a closed "washer" solid: the rectangular ring (outer minus
// inner) extruded over [z0, z1], consistently wound (outer walls face
// material→void outwards, inner walls face into the hole, caps outwards).
func ringMesh(outer, inner []kernel.Point2, z0, z1 float64) mesh.Mesh {
	n := len(outer)
	if len(inner) != n {
		panic("ringMesh: outer and inner need the same corner count")
	}
	// 2n vertices: outer bottom, outer top, inner bottom, inner top.
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
		// Outer walls (normal away from the material, i.e. out of the part).
		t = append(t, tri(Ob[i], Ob[j], Ot[j]), tri(Ob[i], Ot[j], Ot[i]))
		// Inner walls (the hole's walls point INTO the hole — ADR-017 note).
		t = append(t, tri(It[i], It[j], Ib[j]), tri(It[i], Ib[j], Ib[i]))
		// Top cap (frame quads split; normal +Z).
		t = append(t, tri(Ot[i], Ot[j], It[j]), tri(Ot[i], It[j], It[i]))
		// Bottom cap (frame quads split; normal −Z).
		t = append(t, tri(Ob[i], Ib[j], Ob[j]), tri(Ob[i], Ib[i], Ib[j]))
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
		// Vertical wall quads, split; normal out of the material.
		tris = append(tris, tri(b[i], b[j], t[j]), tri(b[i], t[j], t[i]))
	}
	for i := 1; i < n-1; i++ {
		// Caps fanned from corner 0; foot is CCW, so +Z / −Z normals.
		tris = append(tris, tri(t[0], t[i], t[i+1]), tri(b[0], b[i+1], b[i]))
	}
	return mesh.Mesh{Vertices: v, Triangles: tris}
}

// joinMesh concatenates two closed meshes (multi-body solid, e.g. a part
// plus a detached island) into one.
func joinMesh(a, b mesh.Mesh) mesh.Mesh {
	v := make([]kernel.Point3, 0, len(a.Vertices)+len(b.Vertices))
	v = append(v, a.Vertices...)
	v = append(v, b.Vertices...)
	t := make([]mesh.Tri, 0, len(a.Triangles)+len(b.Triangles))
	for _, x := range a.Triangles {
		t = append(t, x)
	}
	off := len(a.Vertices)
	for _, x := range b.Triangles {
		t = append(t, tri(x.A+off, x.B+off, x.C+off))
	}
	return mesh.Mesh{Vertices: v, Triangles: t}
}

// washer is an 80×80×10 plate with a 50×50 through-hole and a detached
// 20×20 centred island: three nested section loops (profile, pocket,
// island) — the ADR-018 §2 parity case.
func washer() mesh.Mesh {
	return joinMesh(
		ringMesh(squareCCW(40), squareCCW(25), 0, 10),
		boxMesh(squareCCW(10), 0, 10),
	)
}

// stepped is a 40×40×5 base with a centred 20×20×5 step on top:
// vertex heights [0, 5, 10] — the multi-level case for the slice-z rule.
func stepped() mesh.Mesh {
	return joinMesh(boxMesh(squareCCW(20), 0, 5), boxMesh(squareCCW(10), 5, 10))
}

// overlappingBoxes is two 20×20×10 boxes overlapping on a 10×10 region —
// a broken "solid" whose section at z=5 has two intersecting loops.
func overlappingBoxes() mesh.Mesh {
	a := []kernel.Point2{P(0, 0), P(20, 0), P(20, 20), P(0, 20)}
	b := []kernel.Point2{P(10, 10), P(30, 10), P(30, 30), P(10, 30)}
	return joinMesh(boxMesh(a, 0, 10), boxMesh(b, 0, 10))
}

// flatSquare is a degenerate single-level "part" (a flat square, one
// vertex height) — no 2.5D section exists.
func flatSquare() mesh.Mesh {
	return mesh.Mesh{
		Vertices: []kernel.Point3{
			kernel.P3(0, 0, 0), kernel.P3(10, 0, 0), kernel.P3(10, 10, 0), kernel.P3(0, 10, 0),
		},
		Triangles: []mesh.Tri{tri(0, 1, 2), tri(0, 2, 3)},
	}
}

func mustValidate(t *testing.T, m mesh.Mesh) {
	t.Helper()
	if err := m.Validate(); err != nil {
		t.Fatalf("fixture mesh failed Validate: %v", err)
	}
}

// --- Part model ---------------------------------------------------------

func TestPartValidate(t *testing.T) {
	f := 2.5
	cases := []struct {
		name    string
		p       Part
		wantErr bool
		msgPart string
	}{
		{"minimal ok", Part{ID: "p1", Mesh: "a.stl"}, false, ""},
		{"full ok", Part{ID: "p1", Name: "n", Mesh: "a.stl", SliceZ: &f, CutZ: &f}, false, ""},
		{"missing id", Part{Mesh: "a.stl"}, true, "id is required"},
		{"missing mesh", Part{ID: "p1"}, true, "mesh is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.p.Validate()
			if c.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.msgPart) {
				t.Fatalf("error %q does not mention %q", err, c.msgPart)
			}
		})
	}
}

func TestPartJSONRoundTrip(t *testing.T) {
	orig := Part{ID: "plate", Name: "a plate", Mesh: "parts/plate.stl"}
	slice := 5.0
	cut := -2.5
	orig.SliceZ = &slice
	orig.CutZ = &cut

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"slice_z":5`) {
		t.Fatalf("slice_z missing from JSON: %s", data)
	}
	var got Part
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != orig.ID || got.Mesh != orig.Mesh {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.SliceZ == nil || *got.SliceZ != 5 {
		t.Fatalf("slice_z lost in round-trip: %v", got.SliceZ)
	}
	if got.CutZ == nil || *got.CutZ != -2.5 {
		t.Fatalf("cut_z lost in round-trip: %v", got.CutZ)
	}

	// Omitted keys stay nil (the defaults apply), not zero values.
	var bare Part
	if err := json.Unmarshal([]byte(`{"id":"p","mesh":"m.stl"}`), &bare); err != nil {
		t.Fatalf("unmarshal bare: %v", err)
	}
	if bare.SliceZ != nil || bare.CutZ != nil {
		t.Fatalf("bare recipe must have nil slice_z/cut_z, got %v / %v", bare.SliceZ, bare.CutZ)
	}
}

// --- slice-z rule -------------------------------------------------------

func TestEffectiveSliceZ(t *testing.T) {
	m := plateMesh(t)

	cases := []struct {
		name    string
		sliceZ  *float64
		want    float64
		wantErr string
	}{
		{"default is midpoint of top two levels", nil, 5, ""},
		{"explicit between levels", ptr(2.5), 2.5, ""},
		{"on the top face is refused", ptr(10), 0, "does not lie strictly inside"},
		{"on the bottom face is refused", ptr(0), 0, "does not lie strictly inside"},
		{"below the part is refused", ptr(-5), 0, "does not lie strictly inside"},
		{"above the part is refused", ptr(15), 0, "does not lie strictly inside"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			part := Part{ID: "plate", Mesh: "plate.stl", SliceZ: c.sliceZ}
			got, err := part.EffectiveSliceZ(m)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error mentioning %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("slice z = %g, want %g", got, c.want)
			}
		})
	}

	t.Run("on an interior face is refused with a valid suggestion", func(t *testing.T) {
		m := stepped()
		mustValidate(t, m)
		part := Part{ID: "step", Mesh: "step.stl", SliceZ: ptr(5)}
		_, err := part.EffectiveSliceZ(m)
		if err == nil {
			t.Fatal("expected a refusal for slice_z on an interior face")
		}
		for _, want := range []string{"lies on a part face", "z=2.5"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q should mention %q", err, want)
			}
		}
	})

	t.Run("flat part has no slice height", func(t *testing.T) {
		mustValidate(t, flatSquare())
		_, err := Part{ID: "flat", Mesh: "flat.stl"}.EffectiveSliceZ(flatSquare())
		if err == nil || !strings.Contains(err.Error(), "single vertex height") {
			t.Fatalf("want 'single vertex height' refusal, got %v", err)
		}
	})
}

func TestEffectiveCutZ(t *testing.T) {
	m := plateMesh(t)
	part := Part{ID: "plate", Mesh: "plate.stl"}

	got, err := part.EffectiveCutZ(m)
	if err != nil {
		t.Fatalf("default cut_z: %v", err)
	}
	if got != 0 {
		t.Fatalf("default cut_z = %g, want 0 (part min Z)", got)
	}

	cut := -2.5
	part.CutZ = &cut
	got, err = part.EffectiveCutZ(m)
	if err != nil {
		t.Fatalf("explicit cut_z: %v", err)
	}
	if got != -2.5 {
		t.Fatalf("explicit cut_z = %g, want -2.5", got)
	}
}

// --- feature detection ---------------------------------------------------

func TestDetectPlateProfileAndPocket(t *testing.T) {
	m := plateMesh(t)
	feats, err := Detect(m, 5)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(feats) != 2 {
		t.Fatalf("want 2 features (profile + hole), got %d", len(feats))
	}
	// Ordered: profile (depth 0) first, pocket (depth 1) second.
	if feats[0].Kind != planning.Profile || feats[0].Depth != 0 {
		t.Fatalf("feature 0: want profile at depth 0, got %+v", feats[0])
	}
	if feats[1].Kind != planning.Pocket || feats[1].Depth != 1 {
		t.Fatalf("feature 1: want pocket at depth 1, got %+v", feats[1])
	}
	// 60×40 = 2400 mm²; 20×20 = 400 mm².
	if feats[0].Area != 2400 {
		t.Fatalf("profile area = %g, want 2400", feats[0].Area)
	}
	if feats[1].Area != 400 {
		t.Fatalf("pocket area = %g, want 400", feats[1].Area)
	}
	// A consistent solid's section loops are oppositely oriented (ADR-017):
	// the planner/planning derive traversal from SignedArea, but pin the
	// invariant here too.
	if (feats[0].Contour.SignedArea() > 0) == (feats[1].Contour.SignedArea() > 0) {
		t.Fatalf("profile and pocket loops must have opposite orientation, got %g / %g",
			feats[0].Contour.SignedArea(), feats[1].Contour.SignedArea())
	}
}

func TestDetectWasherIslandsByParity(t *testing.T) {
	m := washer()
	mustValidate(t, m)
	if !m.IsClosedMesh() {
		t.Fatal("fixture: washer must be a closed 2-manifold")
	}
	feats, err := Detect(m, 5)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(feats) != 3 {
		t.Fatalf("want 3 features (profile, hole, island), got %d", len(feats))
	}
	want := []struct {
		kind  planning.FeatureKind
		depth int
		area  float64
	}{
		{planning.Profile, 0, 6400}, // 80×80
		{planning.Pocket, 1, 2500},  // 50×50 void
		{planning.Profile, 2, 400},  // 20×20 island (even depth = material)
	}
	for i, w := range want {
		if feats[i].Kind != w.kind || feats[i].Depth != w.depth || feats[i].Area != w.area {
			t.Fatalf("feature %d: got kind=%s depth=%d area=%g; want kind=%s depth=%d area=%g",
				i, feats[i].Kind, feats[i].Depth, feats[i].Area, w.kind, w.depth, w.area)
		}
	}
}

func TestDetectRefusesIntersectingLoops(t *testing.T) {
	m := overlappingBoxes()
	mustValidate(t, m)
	_, err := Detect(m, 5)
	if err == nil || !strings.Contains(err.Error(), "intersect") {
		t.Fatalf("want an 'intersect' refusal for overlapping section loops, got %v", err)
	}
}

func TestDetectPropagatesSliceErrors(t *testing.T) {
	// A plane through a vertex is the clean-case refusal of mesh.Slice
	// (ADR-017); Detect must surface it, not paper over it.
	m := plateMesh(t)
	_, err := Detect(m, 10)
	if err == nil || !strings.Contains(err.Error(), "vertex") {
		t.Fatalf("want the slice refusal at a face height, got %v", err)
	}
}

// ptr is a float64 pointer helper for the table-driven cases.
func ptr(v float64) *float64 { return &v }
