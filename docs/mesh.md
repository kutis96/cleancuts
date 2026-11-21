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

**The 2.5D rule (how callers get feature contours):** a section at height
`h` is the boundary of (plane h ∩ solid). Two cases (ADR-025):

- `h` **strictly between adjacent `VertexZs()` levels** → a *band section*:
  `Slice(h)` (below). Conventionally the **midpoint** of the two levels
  bracketing the feature height. For prismatic parts this is exact and is
  what real slicers do.
- `h` **on a vertex height** → a *face section*: `FaceLoops(h)` (below) —
  the boundary of the upward-facing face(s) at that height. The prismatic
  coplanar case, exact by edge pairing (no polygon union).

### Errors (precise, actionable — Law 7 in import form)

| Situation | Error says |
|---|---|
| plane through a vertex / along an edge (within `AbsTol`) | the offending vertex (triangle + coords), and a suggested z — the midpoint of the adjacent `VertexZs()` levels |
| crossing point joins ≠ 2 segments | the point and the count: "the mesh is not a clean 2-manifold at this height" |
| no intersection | the z levels available, so the caller can pick one |

### v1 limitations (documented, refused — not silent)

- *General* coplanar (face-height) sectioning — sloped faces, arbitrary
  polygon unions — remains the deferred case (a later refinement or B-rep);
  the *prismatic* face case is now handled by `FaceLoops` (ADR-025).
- T-junctions within a coplanar band; non-manifold stitching.
- A mesh that is not a closed 2-manifold *at all* (open STL) may still slice
  cleanly between faces; the `IsClosedMesh()` warning tells the user.

## FaceLoops(h) — the face section (ADR-025)

`FaceLoops(h)` returns the **boundary loops of the union of the solid's
upward-facing horizontal faces at height h**, as `[]kernel.Chain2` of line
segments, in the mesh's own winding (consumers measure `SignedArea`).

- **Face triangles:** triangles whose three vertices all lie at z=h (within
  `AbsTol`) and whose normal points +Z (a horizontal triangle's normal is
  exactly ±Z, so the test is exact; a downward face is not a top face and
  is excluded).
- **Boundary extraction (edge pairing):** an undirected edge (exact float
  key, as in `Slice`) that belongs to exactly one face triangle is a
  boundary edge. Each boundary vertex joins exactly two boundary edges and
  the edges stitch into closed loops (the same 2-regular stitching as
  `Slice`).
- **Refusals (Law 7 in import form):** a vertex joining ≠ 2 boundary edges,
  a dangling boundary edge, or no face at all at the height (e.g. a
  bottom-only face) → a precise, actionable error naming the offending
  vertex/edge or the height. Never a silent wrong loop.

This is the section a pocket on a face needs: the Testmesh base top at
z=−10 yields the 50×50 square and the Ø25 circle (the cylinder's footprint,
a **Boss** per ADR-021); the annulus between them is the clearable pocket
region (ADR-022). `Slice` itself is unchanged — the two primitives are
siblings (`band` vs. `face`), dispatched by `part`.

## RegionTopZ — the material-above test (ADR-021)

`RegionTopZ(outer, inners) (topZ, has, err)` returns the **highest material
Z within the region** bounded by `outer`, excluding the interiors of
`inners` (each of which must lie inside `outer`). It is computed over the
mesh triangles whose *centroid* is inside `outer` and outside every `inner`
(`kernel.PointInContour`; the boundary counts as outside — the safe side).

- `has` is `true` when the region contains material (at least one such
  triangle), `false` when it is a void.
- `topZ` is the max vertex Z over those triangles (meaningful only when
  `has`).
- It is a **pure, testable** primitive (Law 5): no I/O, no global state.

This is the signal `part.Detect` uses to split a nested loop into a **Boss**
(`has && topZ > sliceZ`, material rises above the section — kept) and a
**Pocket** (no material above — a void). See ADR-021.

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

**v0.1:** `Mesh` + `Validate` + `VertexZs` + `Bounds` + `IsClosedMesh` +
`Slice(z)`; STL ASCII/binary and 3MF parsers; table-driven tests pinning
exact sections (cube, tetrahedron, tube-with-hole) and every error case above.
**v0.2:** `RegionTopZ` — the material-above test for boss/pocket
classification (ADR-021).
**Later:** coplanar sectioning (polygon union) or B-rep import; multi-object
3MF selection; mesh stock behind the `stock` surface (ADR-015).
