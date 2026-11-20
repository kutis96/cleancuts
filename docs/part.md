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
| `slice_z`  | no       | Height at which the section is taken. Default: midpoint of the two topmost distinct vertex heights. |
| `cut_z`    | no       | Tool-tip height of the cut. Default: the part's minimum Z (a cut through the part). |

`Part.Validate()` (Law 5): `id` and `mesh` non-empty, `slice_z`/`cut_z` finite.

## The slice-z rule (the 2.5D rule, ADR-017)

`slice_z` must lie **strictly between two adjacent distinct `VertexZs`
levels** — the plane must not pass through a vertex (the clean case of
`mesh.Slice`, whose safe failure is a precise refusal).

- **Default:** with levels `z₁ < z₂ < … < zₙ`, the default is
  `(zₙ₋₁ + zₙ) / 2` — the slice that sees the topmost cross-section.
- **Explicit `slice_z`:** `part.EffectiveSliceZ` re-checks the rule and, on
  violation, names the offending height and suggests a valid one (e.g. the
  midpoint of the adjacent levels). A part with a single vertex height is
  refused: it is flat, not a 2.5D solid.

## Feature detection — nesting parity (`part.Detect`)

`Detect(mesh, z)` slices at `z` and classifies the loops:

1. `loops = mesh.Slice(z)` (closed `kernel.Chain2`, mesh's own winding).
2. **Parent** of a loop = the innermost (smallest-area) loop that strictly
   contains one of its vertices (`kernel.PointInContour`; boundary counts as
   outside — the safe side). Outermost loops have no parent.
3. **Depth** = 0 for outermost, else `parent.depth + 1`.
4. **Kind by parity (ADR-018 §2):**

| Depth | Kind              | Kept material | Tool side  | comp direction |
|-------|-------------------|---------------|------------|----------------|
| even  | `planning.Profile`| inside        | outside    | `comp.Outside` |
| odd   | `planning.Pocket` | outside       | inside     | `comp.Inside`  |

One rule covers profiles (depth 0), holes (depth 1), islands (depth 2), and
arbitrary nesting. It agrees with the ADR-009 climb predicate: with a CCW
spindle, profiles/islands mill CCW and pockets CW — both in climb.

5. **Disjointness (Law 7):** any edge of one loop intersecting any edge of
   another is refused with the two loops named — the kept material would be
   ambiguous and milling both would over-cut. (A valid solid's section is a
   disjoint, nested set; this catches broken meshes.)

`Feature` is a value: `Index`, `Kind`, `Contour`, `Depth`, `Area`
(|signed area|, mm²). Ordering: depth ascending, then area descending —
profiles first, largest first, then the voids they enclose.

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
