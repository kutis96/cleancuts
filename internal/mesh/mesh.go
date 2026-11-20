// Package mesh is CleanCuts's boundary/import module for part geometry
// (ADR-017): pure parsers (STL ASCII/binary, 3MF) over in-memory bytes, a
// validated triangle-soup model in the canonical WCS (mm, ADR-001), and
// Slice(z) — the section primitive that yields the 2D contours the planner
// needs. File I/O stays in the CLI (Law 4, core purity); see
// docs/mesh.md for the contract and its precise error cases.
package mesh

import (
	"fmt"
	"math"
	"sort"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// Tri is one triangle as vertex indices (into Mesh.Vertices). The winding
// (A→B→C, right-hand rule) defines the outward normal.
type Tri struct {
	A, B, C int
}

// Mesh is a triangle soup: vertex positions in the canonical WCS (mm) and
// index triples. It carries no frame, no units, no global state — it is a
// value (Law 1, core purity).
type Mesh struct {
	Vertices  []kernel.Point3
	Triangles []Tri
}

// Validate enforces the mesh invariants (Law 5): finite vertices, indices
// in range, no repeated index within a triangle, and no degenerate
// (near-collinear) triangle.
func (m Mesh) Validate() error {
	if len(m.Vertices) == 0 {
		return fmt.Errorf("mesh: needs at least one vertex")
	}
	if len(m.Triangles) == 0 {
		return fmt.Errorf("mesh: needs at least one triangle")
	}
	for i, v := range m.Vertices {
		if !v.Finite() {
			return fmt.Errorf("mesh: vertex %d is not finite (%v)", i, v)
		}
	}
	for i, t := range m.Triangles {
		for _, idx := range [3]int{t.A, t.B, t.C} {
			if idx < 0 || idx >= len(m.Vertices) {
				return fmt.Errorf("mesh: triangle %d references vertex %d out of range (0..%d)",
					i, idx, len(m.Vertices)-1)
			}
		}
		if t.A == t.B || t.B == t.C || t.A == t.C {
			return fmt.Errorf("mesh: triangle %d repeats a vertex index (a=%d b=%d c=%d)",
				i, t.A, t.B, t.C)
		}
		a, b, c := m.Vertices[t.A], m.Vertices[t.B], m.Vertices[t.C]
		ab, ac := b.Sub(a), c.Sub(a)
		// Scale-invariant degeneracy test: a triangle is near-collinear iff
		// |ab × ac| is tiny relative to |ab|·|ac| (1e-9 relative ≈ an angle
		// under ~1e-9 rad) — independent of the part's mm scale.
		cx := ab.Y*ac.Z - ab.Z*ac.Y
		cy := ab.Z*ac.X - ab.X*ac.Z
		cz := ab.X*ac.Y - ab.Y*ac.X
		mag2 := cx*cx + cy*cy + cz*cz
		len2 := (ab.X*ab.X + ab.Y*ab.Y + ab.Z*ab.Z) * (ac.X*ac.X + ac.Y*ac.Y + ac.Z*ac.Z)
		if len2 > 0 && mag2 <= 1e-18*len2 {
			return fmt.Errorf("mesh: triangle %d is degenerate (near-collinear: %v, %v, %v)",
				i, a, b, c)
		}
	}
	return nil
}

// VertexZs returns the sorted distinct vertex heights (deduplicated within
// kernel.AbsTol) — the map of where the part's faces live. The caller
// slices strictly between adjacent levels (docs/mesh.md, the 2.5D rule).
func (m Mesh) VertexZs() []float64 {
	zs := make([]float64, 0, len(m.Vertices))
	for _, v := range m.Vertices {
		zs = append(zs, v.Z)
	}
	sort.Float64s(zs)
	out := []float64{}
	for _, z := range zs {
		if len(out) == 0 || z-out[len(out)-1] > kernel.AbsTol {
			out = append(out, z)
		}
	}
	return out
}

// Bounds is the axis-aligned bounding box of the mesh.
type Bounds struct {
	Min, Max kernel.Point3
}

// Bounds returns the mesh's axis-aligned bounding box.
func (m Mesh) Bounds() Bounds {
	if len(m.Vertices) == 0 {
		return Bounds{}
	}
	b := Bounds{Min: m.Vertices[0], Max: m.Vertices[0]}
	for _, v := range m.Vertices[1:] {
		b.Min.X = min3(b.Min.X, v.X)
		b.Min.Y = min3(b.Min.Y, v.Y)
		b.Min.Z = min3(b.Min.Z, v.Z)
		b.Max.X = max3(b.Max.X, v.X)
		b.Max.Y = max3(b.Max.Y, v.Y)
		b.Max.Z = max3(b.Max.Z, v.Z)
	}
	return b
}

func min3(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func max3(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

// IsClosedMesh reports whether the mesh is a closed 2-manifold surface:
// every undirected edge belongs to exactly two triangles. Edges are keyed
// by their endpoint *positions*, not indices — STL is a vertex soup that
// repeats shared vertices per triangle, so index-based counting would
// report every valid STL as open. An open mesh (missing faces) reports
// false; the CLI surfaces this as a warning.
func (m Mesh) IsClosedMesh() bool {
	count := map[geoEdge]int{}
	for _, t := range m.Triangles {
		a, b, c := m.Vertices[t.A], m.Vertices[t.B], m.Vertices[t.C]
		for _, e := range [3][2]kernel.Point3{{a, b}, {b, c}, {c, a}} {
			lo, hi := e[0], e[1]
			if lessPoint3(hi, lo) {
				lo, hi = hi, lo
			}
			count[geoEdge{lo: lo, hi: hi}]++
		}
	}
	for _, n := range count {
		if n != 2 {
			return false
		}
	}
	return true
}

type geoEdge struct{ lo, hi kernel.Point3 }

// lessPoint3 orders points for canonical edge keys (any total order works).
func lessPoint3(a, b kernel.Point3) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}

// Slice returns the closed intersection loops of the solid with the
// horizontal plane at height z (canonical WCS), each as a kernel.Chain2 of
// line segments, in the mesh's own winding (consumers measure SignedArea;
// for a consistent solid, outer loops and hole loops come out oppositely
// oriented — ADR-001, ADR-017).
//
// Slice is defined for the clean case (ADR-017): the plane does not pass
// through any vertex or along any edge, so every triangle is fully above,
// fully below, or straddles with exactly one crossing segment, and each
// crossing point joins exactly two segments. Anything else is refused with
// a precise, actionable error (Law 7 in import form: the safe failure is
// refusing to extract, not extracting wrong). For 2.5D parts, slice at the
// midpoint of the two VertexZs() levels bracketing the feature height.
func (m Mesh) Slice(z float64) ([]kernel.Chain2, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	// Pass 1 — the clean-case gate (ADR-017): no vertex may lie on the
	// plane (within AbsTol); that is the coplanar case v1 refuses.
	for i, t := range m.Triangles {
		for _, idx := range [3]int{t.A, t.B, t.C} {
			p := m.Vertices[idx]
			if p.Z >= z-kernel.AbsTol && p.Z <= z+kernel.AbsTol {
				return nil, m.planeThroughVertexError(z, p, i)
			}
		}
	}

	// Pass 2 — collect the crossing segments: a straddling triangle
	// (2 vertices one side, 1 the other) contributes exactly one.
	type seg struct{ a, b kernel.Point2 }
	segs := make([]seg, 0, len(m.Triangles))
	for _, t := range m.Triangles {
		pts := [3]kernel.Point3{m.Vertices[t.A], m.Vertices[t.B], m.Vertices[t.C]}
		above := 0
		for _, p := range pts {
			if p.Z > z {
				above++
			}
		}
		if above == 0 || above == 3 {
			continue // fully one side: no crossing
		}
		// Exactly two edges connect opposite sides.
		var cross [2]kernel.Point2
		n := 0
		for e := 0; e < 3; e++ {
			f := (e + 1) % 3
			ze, zf := pts[e].Z, pts[f].Z
			if (ze-z)*(zf-z) < 0 {
				t1 := (z - ze) / (zf - ze)
				cross[n] = kernel.P2(
					pts[e].X+t1*(pts[f].X-pts[e].X),
					pts[e].Y+t1*(pts[f].Y-pts[e].Y),
				)
				n++
			}
		}
		segs = append(segs, seg{cross[0], cross[1]})
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("mesh: plane z=%g does not intersect the solid (vertex z levels: %v — slice between adjacent levels)",
			z, m.VertexZs())
	}

	// Stitch: in the clean case every crossing point joins exactly two
	// segments. Crossing points computed from shared vertices are
	// bit-identical, so exact float keys stitch correctly.
	type pkey struct{ x, y float64 }
	key := func(p kernel.Point2) pkey { return pkey{p.X, p.Y} }
	adj := map[pkey][]int{}
	for i, s := range segs {
		adj[key(s.a)] = append(adj[key(s.a)], i)
		adj[key(s.b)] = append(adj[key(s.b)], i)
	}
	for p, idxs := range adj {
		if len(idxs) != 2 {
			return nil, fmt.Errorf("mesh: slice at z=%g: intersection point (%g, %g) joins %d segments (want 2) — the mesh is not a clean 2-manifold at this height",
				z, p.x, p.y, len(idxs))
		}
	}

	// Walk each cycle.
	used := make([]bool, len(segs))
	loops := []kernel.Chain2{}
	for i := range segs {
		if used[i] {
			continue
		}
		start := key(segs[i].a)
		pts := []kernel.Point2{segs[i].a}
		cur := i
		dirFromA := true // traverse cur from a to b
		steps := 0
		for {
			steps++
			if steps > len(segs)+1 {
				return nil, fmt.Errorf("mesh: slice at z=%g: loop did not close after %d segments (inconsistent intersection graph)",
					z, steps)
			}
			s := segs[cur]
			exit := s.b
			if !dirFromA {
				exit = s.a
			}
			pts = append(pts, exit)
			used[cur] = true
			if key(exit) == start {
				break
			}
			// Next segment: the other one at exit.
			var next int
			found := false
			for _, j := range adj[key(exit)] {
				if j == cur {
					continue
				}
				next = j
				found = true
				break
			}
			if !found {
				return nil, fmt.Errorf("mesh: slice at z=%g: no continuation at (%g, %g) — the mesh is not a clean 2-manifold at this height",
					z, exit.X, exit.Y)
			}
			dirFromA = key(segs[next].a) == key(exit)
			cur = next
		}
		if len(pts) < 3 {
			return nil, fmt.Errorf("mesh: slice at z=%g: degenerate %d-point loop (want ≥ 3)", z, len(pts))
		}
		edges := make([]kernel.Edge2, len(pts)-1)
		for j := range edges {
			e, err := kernel.NewLineEdge(pts[j], pts[j+1])
			if err != nil {
				return nil, fmt.Errorf("mesh: slice at z=%g: %v", z, err)
			}
			edges[j] = e
		}
		chain, err := kernel.NewChain2(edges)
		if err != nil {
			return nil, fmt.Errorf("mesh: slice at z=%g: %v", z, err)
		}
		if !chain.IsClosed() {
			return nil, fmt.Errorf("mesh: slice at z=%g: loop did not close (start %v, end %v)",
				z, chain.Start(), chain.End())
		}
		loops = append(loops, chain)
	}
	return loops, nil
}

// planeThroughVertexError builds the precise, actionable refusal for a
// plane that hits a vertex (the coplanar case, ADR-017): it names the
// vertex and the triangle, and — when the levels allow — suggests the
// midpoint of the adjacent levels, the documented 2.5D rule.
func (m Mesh) planeThroughVertexError(z float64, v kernel.Point3, tri int) error {
	zs := m.VertexZs()
	// Suggest the midpoint of the adjacent-level interval closest to z —
	// the documented 2.5D rule (docs/mesh.md). Works at the extreme levels
	// too: for z at the lowest level, the interval below it does not exist,
	// so the one above it wins.
	if len(zs) >= 2 {
		var bestMid, bestLo, bestHi float64
		bestDist := math.Inf(1)
		for i := 0; i+1 < len(zs); i++ {
			lo, hi := zs[i], zs[i+1]
			var d float64
			switch {
			case z < lo:
				d = lo - z
			case z > hi:
				d = z - hi
			default:
				d = 0 // z lies on this interval's span (a vertex level)
			}
			if d < bestDist {
				bestDist, bestMid, bestLo, bestHi = d, (lo+hi)/2, lo, hi
			}
		}
		if bestMid > bestLo+kernel.AbsTol && bestMid < bestHi-kernel.AbsTol {
			return fmt.Errorf("mesh: slice plane z=%g passes through vertex (%g, %g, %g) in triangle %d — the coplanar case is not a v1 section (ADR-017). Slice between adjacent levels: z=%g (midpoint of %g and %g) works for 2.5D parts",
				z, v.X, v.Y, v.Z, tri, bestMid, bestLo, bestHi)
		}
	}
	return fmt.Errorf("mesh: slice plane z=%g passes through vertex (%g, %g, %g) in triangle %d — choose a z strictly between adjacent vertex levels %v",
		z, v.X, v.Y, v.Z, tri, zs)
}
