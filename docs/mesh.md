# Mesh import and slicing (the part's geometry)

The part's shape comes in as a **triangle mesh** — STL or 3MF (ADR-017).
`internal/mesh` is the boundary module (Law 1, Law 4): it parses file bytes
into a `Mesh` value in the canonical WCS (mm) and provides the section
primitive the planner needs. File I/O stays in the CLI; parsing and slicing
are pure.

## The model

```go
type Mesh struct {
    Vertices  []kernel.Point3   // positions in canonical WCS (mm)
    Triangles []Tri             // index triples (A, B, C), winding = normal
}
```

- **`Validate()`** — finite vertices, indices in range, no duplicate index
  per triangle, no degenerate (near-collinear) triangle.
- **`VertexZs()`** — the sorted distinct vertex heights (deduplicated within
  `AbsTol`). The map of where the part's faces live.
- **`Bounds()`** — the axis-aligned min/max box (for display and sanity).
- **`IsClosedMesh()`** — watertightness: every undirected edge belongs to
  exactly two triangles. The CLI reports non-closed meshes as a warning.
- **`Slice(z)`** — the primitive (below).

## Slice(z) — the section primitive

`Slice(z)` returns the closed intersection loops of the solid with the
horizontal plane at height z, as `[]kernel.Chain2` of line segments, in the
mesh's own winding (consumers measure `SignedArea` — for a consistent solid,
outer loops and hole loops come out oppositely oriented).

It is defined for the **clean case** (ADR-017): every triangle is fully
above, fully below, or straddles the plane with exactly one crossing
segment. Then each crossing point joins exactly two segments and the loops
stitch deterministically.

**The 2.5D rule (how callers get feature contours):** slice at a z
*strictly between adjacent `VertexZs()` levels* — conventionally the
**midpoint** of the two levels bracketing the feature height. For prismatic
parts this is exact and is what real slicers do. Slicing exactly at a face
height is the coplanar case, which v1 refuses with a precise error (below)
rather than doing a polygon-boolean union it hasn't earned the right to get
wrong.

### Errors (precise, actionable — Law 7 in import form)

| Situation | Error says |
|---|---|
| plane through a vertex / along an edge (within `AbsTol`) | the offending vertex (triangle + coords), and a suggested z — the midpoint of the adjacent `VertexZs()` levels |
| crossing point joins ≠ 2 segments | the point and the count: "the mesh is not a clean 2-manifold at this height" |
| no intersection | the z levels available, so the caller can pick one |

### v1 limitations (documented, refused — not silent)

- Coplanar (face-height) sectioning — needs polygon union; later refinement.
- T-junctions within a coplanar band; non-manifold stitching.
- A mesh that is not a closed 2-manifold *at all* (open STL) may still slice
  cleanly between faces; the `IsClosedMesh()` warning tells the user.

## Formats

### STL

- **ASCII** and **binary** (little-endian) are both accepted; detection is
  "try the ASCII grammar, fall back to binary" — so a binary file with a
  `solid` header still works.
- Facet normals in the file are *parsed but not trusted*: the winding
  defines orientation (STL normals are unreliable in the wild).
- Errors name the line number (ASCII) or the triangle index (binary).

### 3MF

- A 3MF file is a ZIP containing an XML model; parsing uses the standard
  library only (`archive/zip`, `encoding/xml`) — no new dependencies.
- The **first `<object>` with a `<mesh>`** is used (documented; multi-object
  models take the first, the part recipe will name objects later).
- **Units:** the model's `unit` attribute is honoured exactly once, at
  import (ADR-001): `millimeter` (the default) passes through, `inch`
  ×25.4, `centimeter` ×10; any other unit is rejected with a precise error
  instead of guessed.

## Status

**v0.1 (this slice):** `Mesh` + `Validate` + `VertexZs` + `Bounds` +
`IsClosedMesh` + `Slice(z)`; STL ASCII/binary and 3MF parsers; table-driven
tests pinning exact sections (cube, tetrahedron, tube-with-hole) and every
error case above.
**Later:** coplanar sectioning (polygon union) or B-rep import; multi-object
3MF selection; mesh stock behind the `stock` surface (ADR-015).
