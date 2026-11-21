# Part: recipe, features, first-cut plan

> Phase 2, slice K (ADR-018). The part model (`internal/part`) and the
> first-cut planner (`internal/planner`) that maps
> `part + mesh + stock + fixture + tool + machine + process` to an ordered,
> verified list of `moves.Path`.

## What a part is

A **part** is a portable JSON *recipe over a mesh* (Law 6): it names the
geometry file and the two Z values that turn the mesh's section into 2.5D
features to mill. The core never reads files — the CLI resolves `mesh` and
passes the parsed `mesh.Mesh` in (core purity, ADR-004/Law 4).

```json
{
  "id": "plate-60x40-hole20",
  "name": "60 x 40 x 10 plate with a 20 x 20 centred through-hole",
  "mesh": "plate_60x40x10_hole20.stl"
}
```

| Field      | Required | Meaning |
|------------|----------|---------|
| `id`       | yes      | Stable part identifier (errors name it). |
| `name`     | no       | Human-readable name (paths and messages use it). |
| `mesh`     | yes      | Mesh file (STL ASCII/binary or 3MF) in the canonical WCS (mm). Resolved by the CLI. |
| `slice_z`  | no       | Height at which the section is taken (band **or** face — see below). Default: midpoint of the two topmost distinct vertex heights. |
| `cut_z`    | no       | Tool-tip height of the cut. Default: the part's minimum Z (a cut through the part). |
| `operation`| no       | `profile` (default: mill each contour — profiles outside, voids inside) or `pocket` (ADR-022/026: clear the region `outer − bosses` down to `cut_z` with the trochoidal raster). |
| `material` | no       | Workpiece material name, matched against the tool's `feeds[].material` when feed/spindle are not given explicitly (ADR-024). |
| `stepover` | no       | Pocket stepover `s` in mm (ADR-022/026). Default: the tool radius. Must be > 0 and ≤ 2R (coverage), else refused. |
| `lead_in`  | no       | Entry lead-in (ADR-023) — see below. Default: tangent, `min(tool radius, 3 mm)`. |

`Part.Validate()` (Law 5): `id` and `mesh` non-empty, `slice_z`/`cut_z`/`stepover`
finite, `lead_in`/`operation` well-formed.

## The slice-z rule (band and face sections, ADR-017 + ADR-025)

The section at height `h` is the boundary of (plane h ∩ solid). Two cases:

- **Band section** — `h` strictly between two adjacent distinct `VertexZs`
  levels: the cross-section boundary, computed by `mesh.Slice` (the clean
  case, whose safe failure is a precise refusal).
- **Face section** (ADR-025) — `h` on a vertex height: the boundary of the
  upward-facing face(s) at `h`, computed by `mesh.FaceLoops` (edge pairing
  over the coplanar up-facing triangles; precise refusal on a non-manifold
  face complex). This is the section a pocket on a face needs — the
  Testmesh base top at z=−10 (square + cylinder circle) is a face section.

- **Default:** with levels `z₁ < z₂ < … < zₙ`, the default is
  `(zₙ₋₁ + zₙ) / 2` — the slice that sees the topmost cross-section (a band
  section).
- **Explicit `slice_z`:** `part.EffectiveSliceZ` requires the height to lie
  in the part's Z range (a vertex height is now legal — a face section) and
  refuses anything outside, naming the vertex levels. A part with a single
  vertex height is refused: it is flat, not a 2.5D solid.

## Feature detection — nesting + material Z (`part.Detect`)

`Detect(mesh, z)` slices at `z` and classifies the loops:

1. `loops = mesh.Slice(z)` (closed `kernel.Chain2`, mesh's own winding).
2. **Parent** of a loop = the innermost (smallest-area) loop that strictly
   contains one of its vertices (`kernel.PointInContour`; boundary counts as
   outside — the safe side). Outermost loops have no parent.
3. **Depth** = 0 for outermost, else `parent.depth + 1`.
4. **Kind by depth + material Z (ADR-021, refining ADR-018 §2):**

| Depth | Kind                | Rule | Kept material | Tool side |
|-------|---------------------|------|---------------|-----------|
| 0     | `planning.Profile`  | —    | inside        | outside   |
| >0    | `planning.Boss`     | region has material **above** `z` (region top Z > `z`) | inside (kept) | **never milled** — a hard obstacle |
| >0    | `planning.Pocket`   | no material above `z` (a void) | outside | inside    |

The "material above the slice plane" test is `mesh.RegionTopZ(outer, inners)`
(ADR-021). This generalises the old parity rule: a through-hole has no
material above `z`, so it is still a Pocket (the plate example is unchanged);
a raised boss (a cylinder on a base, sectioned at the base top) has material
above `z`, so it is a Boss — **kept, never milled** — and its parent pocket
clears *around* it (ADR-022).

The rule agrees with the ADR-009 climb predicate: with a CCW (right-handed)
spindle, profiles mill CW and pockets CCW — both in climb (the standard CAM
convention). A Boss has no milling mode: it is a keep-out.

5. **Disjointness (Law 7):** any edge of one loop intersecting any edge of
   another is refused with the two loops named — the kept material would be
   ambiguous and milling both would over-cut. (A valid solid's section is a
   disjoint, nested set; this catches broken meshes.)

`Feature` is a value: `Index`, `Kind`, `Contour`, `Depth`, `Area`
(|signed area|, mm²). Ordering: depth ascending, then area descending —
profiles first, largest first, then the voids/bosses they enclose.

## Lead-in / lead-out (ADR-023)

A part may declare an entry lead-in so the cutter starts **outside the cut
area** and the cutter can start at a specified XY before the first edge
(the friend's request). It is portable JSON on the recipe, with a CLI
override (`--lead-in`, `--lead-in-length`):

| Field             | Required | Meaning |
|-------------------|----------|---------|
| `lead_in.kind`    | no       | `straight` or `tangent`. Default: `tangent`. |
| `lead_in.length`  | no       | Lead-in length in mm. Default: `min(tool radius, 3 mm)`. |

Precise geometry (the entry point is the path's first contour point `S`,
`û` the unit direction of the first edge):

- **`straight`:** the entry segment is the first edge extended backward by
  `length`: a cutting move from `S − length·û` to `S`, then the normal
  path. The plunge happens at the extension's start (over air in the
  removal side), not at `S`.
- **`tangent`:** a quarter-circle arc of radius `length`, tangent to the
  first edge at `S`, bulging onto the removal side (the side away from kept
  material); the arc starts at `S + length·n̂` (`n̂` the removal-side normal
  of the first edge) and ends at `S`, then the normal path. The plunge
  happens at the arc's start.

**Safety (Law 7):** the lead-in segment (line or arc) is checked against
*every* section loop: it must not intersect any loop edge, and the plunge
point must be outside every boss at ≥ tool radius from its boundary
(the tool disc, not just the tip, clears kept material). If the requested
kind/length fails, it degrades `tangent → straight → none`, recording a
diagnostic at each step. It never over-cuts; the safe outcome is a shorter
or absent lead-in, said so (ADR-013 philosophy). The lead-in is a cutting
move (milling mode + feed), not a rapid. See ADR-023 and operations.md.

## Operation (profile vs. pocket)

- **`profile` (default):** mill each cuttable contour — profiles with the
  tool outside, voids with the tool inside. **Bosses are never milled**
  (ADR-021); a void loop nested inside a boss is skipped too (it is kept
  material's interior — Law 7).
- **`pocket` (ADR-022/026):** for each depth-0 loop, clear the region
  (the loop minus its Boss loops) down to `cut_z` with the trochoidal
  serpentine. Void loops are air (part of the region); Boss loops are hard
  keep-outs. Facing a face = a pocket of a face region with no bosses — no
  separate operation needed (operations.md).

## The Z model (`planner`, v1 single-depth)

| Quantity | Value | Rule |
|----------|-------|------|
| `cut_z`  | `Part.cut_z`, default part min-Z | Strictly below `slice_z`; at or above the stock bottom (the table); the flute must reach the part top (below). |
| `safe_z` | stock top + `safe_clearance` (default **5 mm**) | Rapids clear the whole stock; the part is inside it. |
| plunge   | entry XY, `safe_z → cut_z` | The column must clear every keep-out active anywhere in `[cut_z, safe_z]` (`planning.PickStartBetween`). |
| retract  | same entry, `cut_z → safe_z` | Symmetric; the loop is closed, so retract point == entry. |

Checks (precise refusals, all in `planner.Plan`):

- `part + mesh` fit the stock: `stock.Fits` per feature contour at
  `slice_z` with the part top (ADR-015).
- `cut_z < slice_z`, `cut_z ≥ stock` bottom.
- `flute_length` (when > 0) ≥ part top − `cut_z` — otherwise the top of the
  wall is left uncut.
- Feed ∈ machine `[min_feed, max_feed]` (non-zero bounds); spindle ≤
  machine `spindle_max_rpm` and tool `max_rpm` (non-zero). **Refused, not
  clamped** — clamping/scheduling is phase 4 (ADR-010).

## The planner contract (`planner.Plan`)

```
Params{Part, Mesh, Stock, KeepOuts, Tool, Machine,
       Feed, Spindle, Milling (default climb), Coolant,
       CutZ (0 = default), SafeClearance (0 = 5 mm)}
    → Result{SliceZ, CutZ, SafeZ, Features []FeaturePath, Diagnostics}
```

Per feature (ADR-018 §3), exactly:

```
comp.Offset(contour, dir by kind, tool radius)
  → per resulting chain:
      planning.PlanFeature(chain, kind, spindle, mode, cutZ, safeZ, keepOuts)
      → toolpath.Loop(plan.Chain, feed, spindle, safeZ, cutZ, coolant, tool)
      → safety.CheckPath(path, tool radius, keepOuts)
```

- **Fully unreachable feature** (comp drops it all): hard refusal naming the
  feature, tool, and radius — the part cannot be made with this tool.
- **Partial drop** (narrow corner): remaining chains are milled, the drop is
  a `Diagnostic` (residual material = the safe side, ADR-013).
- Each `moves.Path` is self-contained (plunge → cutting loop → retract) and
  verified. **Linking rapids between features are the program layer's job**
  (slice L: `moves.Program` emits a rapid from the previous entry to the
  next at `safe_z` and verifies it with `safety`); `FeaturePath.Entry` is
  the data L needs.

## v1 limitations (documented, safe)

- **Single-depth passes**: one cut per feature at `cut_z`; roughing/depth
  scheduling is phase 3 (the Z model is a planner-owned parameter, so a pass
  structure slots in without changing the part/feature contract).
- **Per-feature milling-mode override**: not exposed in v1 (one requested
  mode for the program; the default is climb). The seam already exists —
  `planning.Params.Milling` per feature; ADR-005 covers the override model.
- **Coplanar/T-junction sections**: refused by `mesh.Slice` (ADR-017) —
  slice between face heights.

## Status

- **Implemented (slice K):** `internal/part` (recipe, `EffectiveSliceZ`,
  `Detect` with parity + disjointness), `planning.PickStartBetween`,
  `stock.ZRange`, `internal/planner` (`Plan`, Z model, checks, composition),
  `schemas/part.schema.json`, example part recipe, table-driven tests
  (incl. the plate: 60×40 profile + 20×20 pocket, opposite winding, climb
  achieved, keep-out entry respected).
