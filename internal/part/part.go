// Package part is CleanCuts's part model and feature detection (ADR-018):
// a Part is a portable JSON *recipe over a mesh* (id, name, mesh file,
// optional slice_z/cut_z), and Detect classifies the mesh's section loops
// at the slice height into 2.5D features by nesting-depth parity — even
// depth is a material contour (planning.Profile, tool outside), odd depth a
// void contour (planning.Pocket, tool inside). One rule covers profiles,
// holes, and islands; intersecting loops are refused (Law 7).
//
// part is pure: it consumes mesh.Mesh values (the CLI reads files, Law 4)
// and depends on kernel, mesh, and planning (for the shared FeatureKind
// and KeepOut vocabulary). It emits no toolpaths — the planner composes.
package part

import (
	"fmt"
	"math"
	"sort"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/mesh"
	"github.com/kutis96/cleancuts/internal/planning"
)

// Part is the portable part recipe (ADR-018 §1, Law 6). Mesh names the
// geometry file (STL or 3MF, canonical WCS); the CLI resolves it and passes
// the parsed mesh to the core — part itself never touches I/O.
type Part struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Mesh is the mesh file the recipe is over (relative or absolute; the
	// CLI owns resolution).
	Mesh string `json:"mesh"`
	// SliceZ is the height at which the 2.5D section is taken. Nil (or an
	// omitted JSON key) selects the default: the midpoint of the two
	// topmost distinct vertex heights (the 2.5D rule, ADR-017).
	SliceZ *float64 `json:"slice_z,omitempty"`
	// CutZ is the tool-tip height of the cut. Nil selects the default:
	// the part's minimum Z (a cut through the part).
	CutZ *float64 `json:"cut_z,omitempty"`
}

// Validate enforces the recipe invariants (Law 5).
func (p Part) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("part: id is required")
	}
	if p.Mesh == "" {
		return fmt.Errorf("part %q: mesh is required (the STL/3MF geometry file)", p.ID)
	}
	if p.SliceZ != nil && !kernel.IsFinite(*p.SliceZ) {
		return fmt.Errorf("part %q: slice_z must be finite (got %g)", p.ID, *p.SliceZ)
	}
	if p.CutZ != nil && !kernel.IsFinite(*p.CutZ) {
		return fmt.Errorf("part %q: cut_z must be finite (got %g)", p.ID, *p.CutZ)
	}
	return nil
}

// EffectiveSliceZ returns the height to slice the part at: the explicit
// slice_z (re-validated against the 2.5D rule) or the default — the
// midpoint of the two topmost distinct vertex heights. A part with fewer
// than two vertex heights is flat and has no 2.5D section (precise refusal).
func (p Part) EffectiveSliceZ(m mesh.Mesh) (float64, error) {
	zs := m.VertexZs()
	if len(zs) < 2 {
		if len(zs) == 1 {
			return 0, fmt.Errorf("part %q: the mesh has a single vertex height (z=%g); a 2.5D part needs thickness, so no slice height exists",
				p.ID, zs[0])
		}
		return 0, fmt.Errorf("part %q: the mesh has no vertices", p.ID)
	}
	if p.SliceZ == nil {
		top, below := zs[len(zs)-1], zs[len(zs)-2]
		return (top + below) / 2, nil
	}
	z := *p.SliceZ
	if z <= zs[0]+kernel.AbsTol || z >= zs[len(zs)-1]-kernel.AbsTol {
		return 0, fmt.Errorf("part %q: slice_z=%g does not lie strictly inside the part (vertex heights %v)",
			p.ID, z, zs)
	}
	for i, lv := range zs {
		if math.Abs(z-lv) <= kernel.AbsTol {
			if i > 0 && i < len(zs)-1 {
				sug := (zs[i-1] + zs[i]) / 2
				return 0, fmt.Errorf("part %q: slice_z=%g lies on a part face (vertex height); slice strictly between adjacent levels, e.g. z=%g (midpoint of %g and %g)",
					p.ID, z, sug, zs[i-1], zs[i])
			}
			return 0, fmt.Errorf("part %q: slice_z=%g lies on a part face (vertex height); slice strictly between adjacent levels",
				p.ID, z)
		}
	}
	return z, nil
}

// EffectiveCutZ returns the recipe's cut height: the explicit cut_z or the
// default — the part's minimum Z (a cut through the part). The mesh's
// bounding box supplies the default.
func (p Part) EffectiveCutZ(m mesh.Mesh) (float64, error) {
	if err := m.Validate(); err != nil {
		return 0, err
	}
	if p.CutZ != nil {
		return *p.CutZ, nil
	}
	return m.Bounds().Min.Z, nil
}

// Feature is one 2.5D feature at the slice height (ADR-018 §2). Kind is the
// shared planning vocabulary: Profile (kept material inside, tool outside)
// or Pocket (kept material outside, tool inside).
type Feature struct {
	Index   int
	Kind    planning.FeatureKind
	Contour kernel.Chain2
	// Depth is the nesting depth in the section: 0 = outermost, 1 = a void
	// inside a profile, 2 = an island inside that void, and so on.
	Depth int
	// Area is the |signed area| of the contour (mm²).
	Area float64
}

// String renders the feature for errors and diagnostics.
func (f Feature) String() string {
	return fmt.Sprintf("feature %d (%s, %g mm², depth %d)", f.Index, f.Kind, f.Area, f.Depth)
}

// Detect slices the mesh at height z and classifies the closed loops into
// features (ADR-018 §2):
//
//   - parent(i) = the innermost (smallest-area) loop that strictly contains
//     a vertex of loop i (kernel.PointInContour; boundary counts as outside
//     — the safe side); outermost loops have no parent;
//   - depth(i) = 0 for outermost, else depth(parent)+1;
//   - kind: even depth → planning.Profile (material contour, comp.Outside),
//     odd depth → planning.Pocket (void contour, comp.Inside).
//
// Loops are returned ordered by depth ascending, then area descending
// (profiles first, largest first, then the voids they enclose). Any two
// non-nested loops whose edges intersect are refused: the kept material
// would be ambiguous, and milling both would over-cut (Law 7).
func Detect(m mesh.Mesh, z float64) ([]Feature, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if !kernel.IsFinite(z) {
		return nil, fmt.Errorf("part: slice height z=%g is not finite", z)
	}
	loopList, err := m.Slice(z)
	if err != nil {
		return nil, err
	}
	n := len(loopList)
	if n == 0 {
		return nil, fmt.Errorf("part: the section at z=%g has no loops (no material at this height)", z)
	}

	areas := make([]float64, n)
	for i, c := range loopList {
		a := c.SignedArea()
		if math.Abs(a) < 1e-6 {
			return nil, fmt.Errorf("part: section loop %d at z=%g is degenerate (area %g)", i, z, a)
		}
		areas[i] = math.Abs(a)
	}

	// Intersecting loops are refused first (Law 7): a valid solid's section
	// is a disjoint, nested set, and overlapping loops make the kept
	// material ambiguous — milling both would over-cut. This check also
	// makes the containment test below sound: with no crossings, one vertex
	// inside a loop implies the whole loop is inside it.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if hit, ok := loopsCross(loopList[i], loopList[j]); ok {
				return nil, fmt.Errorf("part: section loops %d and %d intersect at (%g, %g) — the mesh is not a valid solid at z=%g (the kept material would be ambiguous); refuse to plan",
					i, j, hit.X, hit.Y, z)
			}
		}
	}

	// Parent = innermost (smallest-area) loop that strictly contains a
	// vertex of the loop (kernel.PointInContour; its boundary reading —
	// boundary counts as outside — is the safe side).
	parent := make([]int, n)
	for i := range loopList {
		v := loopList[i].Start()
		best := -1
		for j := range loopList {
			if j == i {
				continue
			}
			if kernel.PointInContour(v, loopList[j]) {
				if best == -1 || areas[j] < areas[best] {
					best = j
				}
			}
		}
		parent[i] = best
	}

	// Depth along the parent chain. A parent is always strictly smaller in
	// area, so the chain cannot cycle — plain memoised recursion is exact.
	depth := make([]int, n)
	done := make([]bool, n)
	var depthOf func(i int) int
	depthOf = func(i int) int {
		if done[i] {
			return depth[i]
		}
		d := 0
		if parent[i] != -1 {
			d = depthOf(parent[i]) + 1
		}
		depth[i], done[i] = d, true
		return d
	}

	out := make([]Feature, 0, n)
	for i := range loopList {
		d := depthOf(i)
		kind := planning.Profile
		if d%2 == 1 {
			kind = planning.Pocket
		}
		out = append(out, Feature{
			Index:   i,
			Kind:    kind,
			Contour: loopList[i],
			Depth:   d,
			Area:    areas[i],
		})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Depth != out[b].Depth {
			return out[a].Depth < out[b].Depth
		}
		return out[a].Area > out[b].Area
	})
	return out, nil
}

// loopsCross returns the first point where an edge of a crosses an edge of
// b, ok=false if the two simple loops are disjoint (they may be nested —
// nested loops never cross).
func loopsCross(a, b kernel.Chain2) (kernel.Point2, bool) {
	for _, e1 := range a.Edges {
		for _, e2 := range b.Edges {
			if hit := kernel.CurveIntersect(e1, e2); len(hit) > 0 {
				return hit[0], true
			}
		}
	}
	return kernel.Point2{}, false
}
